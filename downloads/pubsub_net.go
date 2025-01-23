package downloads

import (
	"context"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/pb"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	ps "github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/pubsub"
)

// HandleFileInfoRequestPubSub 处理文件信息请求
// 参数:
//   - ctx: 上下文对象
//   - opt: 配置选项
//   - db: 数据库实例
//   - fs: 文件系统实例
//   - host: libp2p网络主机
//   - nps: 发布订阅系统
//   - download: 下载管理器
//   - res: 接收到的消息
//
// 返回值: void
//
// 功能:
//   - 解析并验证文件信息请求
//   - 建立与请求节点的连接
//   - 获取文件元数据和分片信息
//   - 发送响应消息给请求节点
func HandleFileInfoRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	host host.Host,
	nps *pubsub.NodePubSub,
	download *DownloadManager,
	res *pubsub.Message,
) {
	// 检查请求数据是否为空
	if len(res.Data) == 0 {
		logger.Warn("请求数据为空")
		return
	}

	// 解析请求数据为文件信息请求对象
	payload := new(pb.DownloadPubSubFileInfoRequest)
	if err := payload.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析请求数据失败: %v", err)
		return
	}

	// 解析请求节点的地址信息
	addrInfo := peer.AddrInfo{}
	if err := addrInfo.UnmarshalJSON(payload.AddrInfo); err != nil {
		logger.Errorf("解析 AddrInfo 失败: %v", err)
		return
	}

	// 将请求节点的地址信息添加到本地节点的 Peerstore
	host.Peerstore().AddAddrs(addrInfo.ID, addrInfo.Addrs, ps.RecentlyConnectedAddrTTL)

	// 尝试与请求节点建立连接
	if err := host.Connect(ctx, addrInfo); err != nil {
		logger.Warnf("无法连接到地址 %s: %v", addrInfo.ID.String(), err)
	}

	// 获取请求的文件信息
	fileInfo, err := GetFileInfoResponse(db, payload.TaskId, payload.FileId, payload.PubkeyHash)
	if err != nil {
		logger.Errorf("获取文件信息失败: %v", err)
		return
	}

	// 验证文件名是否有效
	if fileInfo.FileMeta.Name == "" {
		logger.Error("文件名为空")
		return
	}

	// 序列化响应数据
	fileInfoData, err := fileInfo.Marshal()
	if err != nil {
		logger.Errorf("序列化 BlockSyncInfo 失败: %v", err)
		return
	}

	// 获取响应主题
	topic, err := nps.GetTopic(PubSubFileInfoRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return
	}

	// 发送响应消息给请求节点
	if err := ReplyToMessage(ctx, topic, res.GetMetadata().GetMessageID(), fileInfoData); err != nil {
		logger.Errorf("发送回复消息失败: %v", err)
	}
}

// HandleDownloadManifestRequestPubSub 处理索引清单请求
// 参数:
//   - ctx: 上下文对象
//   - opt: 配置选项
//   - db: 数据库实例
//   - fs: 文件系统实例
//   - nps: 发布订阅系统
//   - download: 下载管理器
//   - res: 接收到的消息
//
// 返回值: void
//
// 功能:
//   - 解析索引清单请求数据
//   - 获取请求的文件片段清单
//   - 发送清单响应给请求节点
func HandleDownloadManifestRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	download *DownloadManager,
	res *pubsub.Message,
) {
	// 检查请求数据是否为空
	if len(res.Data) == 0 {
		logger.Warn("请求数据为空")
		return
	}

	// 解析请求数据为清单请求对象
	payload := new(pb.DownloadPubSubManifestRequest)
	if err := payload.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析索引清单请求数据失败: %v", err)
		return
	}

	// 获取文件片段清单
	manifest, err := GetManifestResponse(
		db,
		payload.TaskId,
		payload.FileId,
		payload.PubkeyHash,
		payload.RequestedSegmentIds,
	)
	if err != nil {
		logger.Errorf("获取文件片段清单失败: %v", err)
		return
	}

	// 序列化响应数据
	data, err := manifest.Marshal()
	if err != nil {
		logger.Errorf("序列化响应数据失败: %v", err)
		return
	}

	// 获取响应主题
	topic, err := nps.GetTopic(PubSubDownloadManifestResponseTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return
	}

	// 发布响应消息
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发布消息失败: %v", err)
		return
	}
}

// HandleDownloadContentRequestPubSub 处理片段内容请求
// 参数:
//   - ctx: 上下文对象
//   - opt: 配置选项
//   - db: 数据库实例
//   - fs: 文件系统实例
//   - nps: 发布订阅系统
//   - download: 下载管理器
//   - res: 接收到的消息
//
// 返回值: void
//
// 功能:
//   - 解析片段内容请求数据
//   - 获取请求的片段内容
//   - 发送片段内容响应给请求节点
func HandleDownloadContentRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	download *DownloadManager,
	res *pubsub.Message,
) {
	// 检查请求数据是否为空
	if len(res.Data) == 0 {
		logger.Warn("请求数据为空")
		return
	}

	// 解析请求数据为片段内容请求对象
	payload := new(pb.SegmentContentRequest)
	if err := payload.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析片段内容请求数据失败: %v", err)
		return
	}

	// 获取请求的片段内容
	content, err := GetSegmentContent(db, payload.TaskId, payload.FileId, payload.SegmentId, payload.SegmentIndex, payload.PubkeyHash, payload.RequestedSegmentIds)
	if err != nil {
		logger.Errorf("获取片段内容失败: %v", err)
		return
	}
	content.TaskId = payload.TaskId

	// 序列化响应数据
	responseData, err := content.Marshal()
	if err != nil {
		logger.Errorf("序列化响应数据失败: %v", err)
		return
	}

	// 获取响应主题
	topic, err := nps.GetTopic(PubSubDownloadContentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return
	}

	// 发送响应消息给请求节点
	if err := ReplyToMessage(ctx, topic, res.GetMetadata().GetMessageID(), responseData); err != nil {
		logger.Errorf("发送响应失败: %v", err)
	}
}
