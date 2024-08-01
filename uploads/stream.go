package uploads

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/util"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/kbucket"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

var (
	// 对等距离
	StreamPeerDistanceProtocol = fmt.Sprintf("defs@stream/peer/distance/%s", version)

	// 发送任务到网络
	StreamSendingToNetworkProtocol = fmt.Sprintf("defs@stream/sending/network/%s", version)
)

// 流协议
type StreamProtocol struct {
	Ctx    context.Context     // 全局上下文
	Opt    *opts.Options       // 文件存储选项配置
	Afe    afero.Afero         // 文件系统接口
	P2P    *dep2p.DeP2P        // 网络主机
	PubSub *pubsub.DeP2PPubSub // 网络订阅
	Upload *UploadManager      // 管理所有上传任务
}

type RegisterStreamProtocolInput struct {
	fx.In
	LC     fx.Lifecycle
	Ctx    context.Context     // 全局上下文
	Opt    *opts.Options       // 文件存储选项配置
	Afe    afero.Afero         // 文件系统接口
	P2P    *dep2p.DeP2P        // 网络主机
	PubSub *pubsub.DeP2PPubSub // 网络订阅
	Upload *UploadManager      // 管理所有上传任务
}

// RegisterUploadStreamProtocol 注册上传流
func RegisterUploadStreamProtocol(input RegisterStreamProtocolInput) {
	// 流协议

	usp := &StreamProtocol{
		Ctx:    input.Ctx,
		Opt:    input.Opt,
		Afe:    input.Afe,
		P2P:    input.P2P,
		PubSub: input.PubSub,
	}

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 注册对等距离
			streams.RegisterStreamHandler(input.P2P.Host(), protocol.ID(StreamPeerDistanceProtocol), streams.HandlerWithRW(usp.handlePeerDistance))

			// 注册发送任务到网络的请求
			streams.RegisterStreamHandler(input.P2P.Host(), protocol.ID(StreamSendingToNetworkProtocol), streams.HandlerWithRW(usp.handleSendingToNetwork))

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 清理资源等停止逻辑
			return nil
		},
	})
}

// 对等距离的请求消息
type PeerDistanceReq struct {
	SegmentID     string   // 文件片段的唯一标识
	Size          int      // 分片大小，单位为字节，描述该片段的数据量大小
	IsRsCodes     bool     // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
	ExcludedPeers []string // 需要过滤的节点ID列表
}

// handlePeerDistance 处理对等距离
func (sp *StreamProtocol) handlePeerDistance(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	payload := new(PeerDistanceReq)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[%s]解码错误: %v", debug.WhereAmI(), err)
		return 6603, "解码错误"
	}

	var allPeers []peer.ID
	if payload.IsRsCodes {
		// 返回路由表中的节点总数
		size := sp.P2P.RoutingTable(1).Size()
		// 返回与给定 ID 最接近的 "count" 对等点的列表
		allPeers = sp.P2P.RoutingTable(1).NearestPeers(kbucket.ConvertKey(payload.SegmentID), size)
	} else {
		// 返回路由表中的节点总数
		size := sp.P2P.RoutingTable(2).Size()
		// 返回与给定 ID 最接近的 "count" 对等点的列表
		allPeers = sp.P2P.RoutingTable(2).NearestPeers(kbucket.ConvertKey(payload.SegmentID), size)
	}

	// 过滤掉需要排除的节点
	var receiverPeers []peer.ID
	for _, peerID := range allPeers {
		if !contains(payload.ExcludedPeers, peerID.String()) {
			receiverPeers = append(receiverPeers, peerID)
		}
	}

	receiverPeers = append(receiverPeers, sp.P2P.Host().ID())

	targetID, err := peer.Decode(req.Message.Sender)
	if err != nil {
		logrus.Errorf("[%s]查找最近的对等点时失败: %v", debug.WhereAmI(), err)
		return 6604, "查找最近的对等点时失败"
	}

	// 根据给定的目标ID和一组节点ID，找到并返回按距离排序的节点信息数组
	//peerInfo, err := FindNearestPeer([]byte(payload.SegmentID), receiverPeers)
	peerInfo, err := FindNearestPeer(targetID, receiverPeers)
	if err != nil {
		logrus.Errorf("[%s]查找最近的对等点时失败: %v", debug.WhereAmI(), err)
		return 6604, "查找最近的对等点时失败"
	}

	// 编码文件片段的哈希表
	peerInfoBytes, err := util.EncodeToBytes(peerInfo)
	if err != nil {
		return 6605, fmt.Sprintf("%s", err)
	}

	res.Data = peerInfoBytes
	return 200, "成功"
}

// 发送任务到网络的请求消息
type SendingToNetworkReq struct {
	FileID        string // 文件唯一标识，用于在系统内部唯一区分文件
	SegmentID     string // 文件片段的唯一标识
	TotalSegments int    // 文件总分片数
	Index         int    // 分片索引，表示该片段在文件中的顺序
	IsRsCodes     bool   // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
	SliceByte     []byte // 切片内容
}

// 发送任务到网络的响应消息
type SendingToNetworkRes struct {
	FileID        string  // 文件唯一标识，用于在系统内部唯一区分文件
	SegmentID     string  // 文件片段的唯一标识
	TotalSegments int     // 文件总分片数
	Index         int     // 分片索引，表示该片段在文件中的顺序
	IsRsCodes     bool    // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
	ID            peer.ID // 节点的ID
	UploadAt      int64   // 文件片段上传的时间戳
}

// handleSendingToNetwork 处理发送任务到网络
func (sp *StreamProtocol) handleSendingToNetwork(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	payload := new(SendingToNetworkReq)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 6603, "解码错误"
	}

	// logrus.Printf("FileID: %s", payload.FileID)
	// logrus.Printf("SegmentID: %s", payload.SegmentID)
	// logrus.Printf("TotalSegments: %d", payload.TotalSegments)
	// logrus.Printf("Index: %d", payload.Index)
	// logrus.Printf("IsRsCodes: %v", payload.IsRsCodes)
	// logrus.Printf("SliceByte: %d", len(payload.SliceByte))

	// 设置文件存储的子目录
	subDir := filepath.Join(paths.GetSlicePath(), sp.P2P.Host().ID().String(), payload.FileID)

	// 将文件片段内容写入本地存储
	if err := util.Write(sp.Opt, sp.Afe, subDir, payload.SegmentID, payload.SliceByte); err != nil {
		logrus.Error("存储接收内容失败, error:", err)
		return 500, "存储接收内容失败"
	}

	sendingToNetwork := SendingToNetworkRes{
		FileID:        payload.FileID,        // 文件唯一标识，用于在系统内部唯一区分文件
		SegmentID:     payload.SegmentID,     // 文件片段的唯一标识
		TotalSegments: payload.TotalSegments, // 文件总分片数
		Index:         payload.Index,         // 分片索引，表示该片段在文件中的顺序
		IsRsCodes:     payload.IsRsCodes,     // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
		ID:            sp.P2P.Host().ID(),    // 节点的ID
		UploadAt:      time.Now().Unix(),     // 文件片段上传的时间戳
	}

	// 编码文件片段的哈希表
	sendingToNetworkBytes, err := util.EncodeToBytes(sendingToNetwork)
	if err != nil {
		return 6605, fmt.Sprintf("%s", err)
	}

	res.Data = sendingToNetworkBytes
	return 200, "成功"
}

// contains 检查字符串切片中是否包含指定的字符串
func contains(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}
