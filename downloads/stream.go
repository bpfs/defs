package downloads

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/network"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/bpfs/dep2p/utils"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

var (
	// 文件下载请求清单（回应）
	StreamDownloadChecklistResponseProtocol = fmt.Sprintf("defs@stream/download/checklist/response/%s", version)

	// 文件下载本地
	StreamDownloadLocalProtocol = fmt.Sprintf("defs@stream/download/local/%s", version)

	// 异步发送文件
	StreamAsyncDownloadProtocol = fmt.Sprintf("defs@stream/async/download/%s", version)
)

// 流协议
type StreamProtocol struct {
	Ctx      context.Context     // 全局上下文
	Opt      *opts.Options       // 文件存储选项配置
	Afe      afero.Afero         // 文件系统接口
	P2P      *dep2p.DeP2P        // 网络主机
	PubSub   *pubsub.DeP2PPubSub // 网络订阅
	Download *DownloadManager    // 管理所有下载任务
}

type RegisterStreamProtocolInput struct {
	fx.In
	LC       fx.Lifecycle
	Ctx      context.Context     // 全局上下文
	Opt      *opts.Options       // 文件存储选项配置
	Afe      afero.Afero         // 文件系统接口
	P2P      *dep2p.DeP2P        // 网络主机
	PubSub   *pubsub.DeP2PPubSub // 网络订阅
	Download *DownloadManager    // 管理所有下载任务
}

// RegisterDownloadStreamProtocol 注册下载流
func RegisterDownloadStreamProtocol(input RegisterStreamProtocolInput) {
	// 流协议
	usp := &StreamProtocol{
		Ctx:      input.Ctx,
		P2P:      input.P2P,
		Opt:      input.Opt,
		Afe:      input.Afe,
		PubSub:   input.PubSub,
		Download: input.Download,
	}

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 注册文件下载请求清单（回应）
			streams.RegisterStreamHandler(input.P2P.Host(), protocol.ID(StreamDownloadChecklistResponseProtocol), streams.HandlerWithRW(usp.handleDownloadChecklistResponse))

			// 注册文件下载本地
			streams.RegisterStreamHandler(input.P2P.Host(), protocol.ID(StreamDownloadLocalProtocol), streams.HandlerWithRW(usp.handleStreamGetSliceToLocal))

			// 注册文件下载本地
			streams.RegisterStreamHandler(input.P2P.Host(), protocol.ID(StreamAsyncDownloadProtocol), streams.HandlerWithRW(usp.handleStreamAsyncDownload))

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 清理资源等停止逻辑
			return nil
		},
	})
}

// 文件下载响应(清单)
type FileDownloadResponseChecklistPayload struct {
	TaskID          string             // 任务唯一标识
	FileID          string             // 文件唯一标识
	Name            string             // 文件名，包括扩展名，描述文件的名称
	Size            int64              // 文件大小，单位为字节，描述文件的总大小
	ContentType     string             // MIME类型，表示文件的内容类型，如"text/plain"
	Checksum        []byte             // 文件的校验和
	SliceTable      map[int]*HashTable // 文件片段的哈希表，记录每个片段的哈希值，支持纠错和数据完整性验证
	AvailableSlices []int              // 本地存储的文件片段信息
}

// handleDownloadChecklistResponse 处理下载请求清单（响应）
func (sp *StreamProtocol) handleDownloadChecklistResponse(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	payload := new(FileDownloadResponseChecklistPayload)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 6603, "解码错误"
	}

	logrus.Infof("[ %s ]使用流消息回复了[ %s ]的索引清单", req.Message.Sender, payload.FileID)

	// logrus.Printf("=== 索引清单响应 ===")
	// logrus.Printf("TaskID: %s", payload.TaskID)
	// logrus.Printf("FileID: %s", payload.FileID)
	// logrus.Printf("Name: %s", payload.Name)
	// logrus.Printf("Size: %d", payload.Size)
	// logrus.Printf("ContentType: %s", payload.ContentType)
	// logrus.Printf("SliceTable: %v", payload.SliceTable)
	// logrus.Printf("AvailableSlices: %v", payload.AvailableSlices)
	// logrus.Printf("==================")

	task, ok := sp.Download.Tasks[payload.TaskID]
	if !ok {
		return 6604, "下载任务不存在"
	}

	// 解析发送者点节点id
	receiver, err := peer.Decode(req.Message.Sender)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 6603, "解码错误"
	}

	// 更新下载任务中特定片段的节点信息
	go task.UpdateDownloadPieceInfo(payload, receiver)

	return 200, "成功"
}

// StreamGetSliceToLocalRequest 发送下载文件片段的任务到网络的请求消息
type StreamGetSliceToLocalRequest struct {
	DownloadMaximumSize int64          // 下载最大回复大小
	UserPubHash         []byte         // 用户的公钥哈希
	TaskID              string         // 任务唯一标识
	FileID              string         // 文件唯一标识，用于在系统内部唯一区分文件
	PrioritySegment     int            // 优先下载的文件片段索引
	SegmentInfo         map[int]string // 文件片段的索引和唯一标识的映射
}

// StreamGetSliceToLocalResponse 发送下载文件片段的任务到网络的响应消息
type StreamGetSliceToLocalResponse struct {
	SegmentInfo map[int][]byte // 文件片段的索引和内容的映射
}

// RequestStreamGetSliceToLocal 向指定的节点发送请求以下载文件片段
// 参数：
//   - p2p: *dep2p.DeP2P 表示 DeP2P 网络主机
//   - receiver: peer.ID 目标节点的 ID
//   - downloadMaximumSize: int64 下载最大回复大小
//   - userPubHash: []byte 用户的公钥哈希
//   - fileID: string 文件唯一标识
//   - prioritySegment: int 优先下载的文件片段索引
//   - segmentInfo: map[int]string 文件片段的索引和唯一标识的映射
//
// 返回值：
//   - *StreamGetSliceToLocalResponse: 下载文件片段的响应消息
//   - error: 如果发生错误，返回错误信息
func RequestStreamGetSliceToLocal(p2p *dep2p.DeP2P, receiver peer.ID, downloadMaximumSize int64, userPubHash []byte, taskID, fileID string, prioritySegment int, segmentInfo map[int]string) (*StreamGetSliceToLocalResponse, error) {
	ask := StreamGetSliceToLocalRequest{
		DownloadMaximumSize: downloadMaximumSize,
		UserPubHash:         userPubHash,
		TaskID:              taskID,
		FileID:              fileID,
		PrioritySegment:     prioritySegment,
		SegmentInfo:         segmentInfo,
	}

	network.StreamMutex.Lock()
	// 发送获取片段到目标节点
	res, err := network.SendStream(p2p, StreamDownloadLocalProtocol, "", receiver, ask)
	if err != nil {
		logrus.Errorf("[%s]发送请求时失败: %v", utils.WhereAmI(), err)
		return nil, err
	}

	if res != nil && res.Code == 200 && res.Data != nil {
		reply := new(StreamGetSliceToLocalResponse)
		if err := util.DecodeFromBytes(res.Data, reply); err != nil {
			logrus.Errorf("[%s]解码响应时失败: %v", utils.WhereAmI(), err)
			return nil, err
		}
		return reply, nil
	}

	logrus.Warnf("未获取数据片段")
	return nil, nil
}

// handleStreamGetSliceToLocal 处理下载到本地
// 参数：
//   - req: *streams.RequestMessage 请求消息
//   - res: *streams.ResponseMessage 响应消息
//
// 返回值：
//   - int32: 状态码
//   - string: 状态信息
func (sp *StreamProtocol) handleStreamGetSliceToLocal(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	payload := new(StreamGetSliceToLocalRequest)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 6603, "解码错误"
	}

	// 处理下载请求
	reply, err := ProcessDownloadRequest(sp.Opt, sp.Afe, sp.P2P, sp.PubSub, sp.Download, payload.DownloadMaximumSize, payload.TaskID, payload.FileID, payload.PrioritySegment, payload.SegmentInfo, req.Message.Sender)
	if err != nil {
		logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
		return 300, "下载文件片段时失败"
	}

	replyBytes, err := util.EncodeToBytes(reply)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 300, "交易信息编码时失败"
	}

	res.Data = replyBytes
	return 200, "成功"
}

// StreamAsyncDownloadRequest 向指定的节点发送文件片段的请求消息
type StreamAsyncDownloadRequest struct {
	TaskID      string         // 任务唯一标识
	FileID      string         // 文件唯一标识，用于在系统内部唯一区分文件
	SegmentInfo map[int][]byte // 文件片段的索引和内容的映射
}

type StreamAsyncDownloadResponse struct {
	Segments map[int]string // 文件片段的索引和内容的映射
}

// RequestStreamAsyncDownload 向指定的节点发送文件片段
func RequestStreamAsyncDownload(p2p *dep2p.DeP2P, receiver peer.ID, taskID, fileID string, segmentInfo map[int][]byte) (*StreamAsyncDownloadResponse, error) {
	ask := StreamAsyncDownloadRequest{
		TaskID:      taskID,
		FileID:      fileID,
		SegmentInfo: segmentInfo,
	}

	network.StreamMutex.Lock()
	// 发送获取片段到目标节点
	res, err := network.SendStream(p2p, StreamAsyncDownloadProtocol, "", receiver, ask)
	if err != nil {
		logrus.Errorf("[%s]发送请求时失败: %v", utils.WhereAmI(), err)
		return nil, err
	}

	if res == nil || res.Code != 200 {
		return nil, fmt.Errorf("发送失败")
	}

	// 请求方已触发合并操作
	if res.Data == nil {
		return nil, nil
	}

	reply := new(StreamAsyncDownloadResponse)
	// 解码从对方接收到的数据到区块哈希列表的流响应对象
	if err := util.DecodeFromBytes(res.Data, reply); err != nil {
		logrus.Errorf("[%s]解码响应时失败: %v", utils.WhereAmI(), err)
		return nil, err
	}

	return reply, nil
}

// handleStreamAsyncDownload 处理异步发送文件片段
func (sp *StreamProtocol) handleStreamAsyncDownload(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	payload := new(StreamAsyncDownloadRequest)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 6603, "解码错误"
	}

	// 本地处理异步下载文件片段
	reply, err := localHandleAsyncDownload(sp.Opt, sp.Afe, sp.P2P, sp.Download, req.Message.Sender, payload.TaskID, payload.SegmentInfo)
	if err != nil {
		logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
		return 300, "处理文件片段时失败"
	}

	// 已触发合并操作
	if reply == nil {
		return 200, "成功"
	}

	replyBytes, err := util.EncodeToBytes(reply)
	if err != nil {
		logrus.Errorf("[%s]交易信息编码失败: %v", debug.WhereAmI(), err)
		return 300, "交易信息编码时失败"
	}

	res.Data = replyBytes

	return 200, "成功"
}
