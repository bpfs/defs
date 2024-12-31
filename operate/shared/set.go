package shared

import (
	"bytes"
	"context"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
	"github.com/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// RequestSetFileSegmentPubSub 请求设置文件片段的共享状态
// 参数:
//   - ctx: 上下文对象,用于控制请求的生命周期
//   - host: libp2p网络主机实例
//   - nps: 节点发布订阅系统实例
//   - fileID: 要设置共享状态的文件ID
//   - pubkeyHash: 文件所有者的公钥哈希
//   - enableSharing: 是否启用共享
//
// 返回值:
//   - error: 如果请求发送成功返回nil,否则返回错误信息
func RequestSetFileSegmentPubSub(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	fileID string,
	pubkeyHash []byte,
	enableSharing bool,
) error {
	// 获取本地节点的地址信息
	addrInfo := peer.AddrInfo{
		ID:    host.ID(),    // 设置节点ID
		Addrs: host.Addrs(), // 设置节点地址列表
	}

	// 序列化地址信息为JSON格式
	addrInfoBytes, err := addrInfo.MarshalJSON()
	if err != nil {
		logger.Errorf("序列化 AddrInfo 失败: %v", err)
		return err
	}

	// 构造请求数据
	requestData := &pb.RequestSetFileSegmentPubSub{
		FileId:        fileID,        // 设置文件ID
		PubkeyHash:    pubkeyHash,    // 设置公钥哈希
		AddrInfo:      addrInfoBytes, // 设置地址信息
		EnableSharing: enableSharing, // 设置是否开启共享
	}

	// 序列化请求数据为二进制格式
	data, err := requestData.Marshal()
	if err != nil {
		logger.Errorf("序列化请求数据失败: %v", err)
		return err
	}

	// 获取设置共享文件请求的发布主题
	topic, err := nps.GetTopic(PubSubSetFileSegmentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return err
	}

	// 发布消息到网络
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发送消息失败: %v", err)
		return err
	}

	return nil
}

// HandleSetFileSegmentRequestPubSub 处理设置文件片段共享状态的请求
// 参数:
//   - ctx: 上下文对象,用于控制请求处理的生命周期
//   - opt: 文件系统配置选项
//   - db: 数据库实例
//   - fs: 文件系统接口
//   - nps: 节点发布订阅系统实例
//   - res: 接收到的消息
func HandleSetFileSegmentRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	res *pubsub.Message,
) {
	// 解析请求数据
	request := new(pb.RequestSetFileSegmentPubSub)
	if err := request.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析设置共享文件请求数据失败: %v", err)
		return
	}

	// 解析请求者的地址信息
	var addrInfo peer.AddrInfo
	if err := addrInfo.UnmarshalJSON(request.AddrInfo); err != nil {
		logger.Errorf("解析请求者地址信息失败: %v", err)
		return
	}

	// 验证请求参数的有效性
	if request.FileId == "" || len(request.PubkeyHash) == 0 {
		logger.Error("无效的文件ID或公钥哈希")
		return
	}

	// 获取文件片段存储实例
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 检查文件是否存在
	segments, err := store.GetFileSegmentStoragesByFileID(request.FileId, request.PubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段失败: %v", err)
		return
	}
	if len(segments) == 0 {
		logger.Error("文件不存在")
		return
	}

	// 验证文件所有权
	if !bytes.Equal(segments[0].P2PkhScript, request.PubkeyHash) {
		logger.Error("无权限设置该文件的共享状态")
		return
	}

	// 更新文件的共享状态
	if err := store.UpdateFileSegmentShared(request.FileId, request.PubkeyHash, request.EnableSharing); err != nil {
		logger.Errorf("更新文件共享状态失败: %v", err)
		return
	}

	logger.Infof("成功设置文件共享状态: fileID=%s", request.FileId)
}
