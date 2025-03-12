// package downloads_ 实现文件下载相关功能
package downloads

import (
	"context"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/pb"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/pubsub"
)

// RequestFileInfoPubSub 发送文件信息请求并等待响应
// 参数:
//   - ctx: 上下文,用于控制请求的生命周期
//   - host: libp2p网络主机实例
//   - nps: 发布订阅系统
//   - taskID: 任务唯一标识
//   - fileID: 文件唯一标识
//   - pubkeyHash: 所有者的公钥哈希
//
// 返回值:
//   - *pb.DownloadPubSubFileInfoResponse: 文件信息响应对象,包含文件元数据
//   - error: 错误信息,如果请求失败则返回错误原因
//
// 功能:
//   - 构造并发送文件信息请求
//   - 等待并解析响应数据
//   - 返回文件信息响应对象
func RequestFileInfoPubSub(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	taskID string,
	fileID string,
	pubkeyHash []byte,
) (*pb.DownloadPubSubFileInfoResponse, error) {
	// 获取本地节点的地址信息
	addrInfo := peer.AddrInfo{
		ID:    host.ID(),    // 设置节点ID
		Addrs: host.Addrs(), // 设置节点地址列表
	}

	// 序列化地址信息为JSON格式
	addrInfoBytes, err := addrInfo.MarshalJSON()
	if err != nil {
		logger.Errorf("序列化 AddrInfo 失败: %v", err)
		return nil, err
	}

	// 构造文件信息请求数据
	requestData := &pb.DownloadPubSubFileInfoRequest{
		TaskId:     taskID,        // 设置任务ID
		FileId:     fileID,        // 设置文件ID
		PubkeyHash: pubkeyHash,    // 设置公钥哈希
		AddrInfo:   addrInfoBytes, // 设置地址信息
	}

	// 序列化请求数据为二进制格式
	data, err := requestData.Marshal()
	if err != nil {
		logger.Errorf("序列化请求数据失败: %v", err)
		return nil, err
	}

	// 获取文件信息请求的发布主题
	topic, err := nps.GetTopic(PubSubFileInfoRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return nil, err
	}

	// 发送消息并等待响应
	reply, err := topic.PublishWithReply(ctx, data)
	if err != nil {
		logger.Errorf("发送消息失败: %v", err)
		return nil, err
	}

	// 解析响应数据为文件信息响应对象
	responseData := &pb.DownloadPubSubFileInfoResponse{}
	if err := responseData.Unmarshal(reply); err != nil {
		logger.Errorf("解析响应数据失败: %v", err)
		return nil, err
	}

	// 记录响应的文件名称
	// logger.Infof("响应文件名称: %v", responseData.FileMeta.Name)

	return responseData, nil
}

// RequestManifestPubSub 发送索引清单请求
// 参数:
//   - ctx: 上下文,用于控制请求的生命周期
//   - host: libp2p网络主机实例
//   - nps: 发布订阅系统
//   - taskID: 任务唯一标识
//   - fileID: 文件唯一标识
//   - pubkeyHash: 所有者的公钥哈希
//   - requestedSegmentIds: 请求下载的片段ID列表
//
// 返回值:
//   - error: 错误信息,如果请求失败则返回错误原因
//
// 功能:
//   - 构造并发送索引清单请求
//   - 不等待响应直接返回
//   - 记录请求日志信息
func RequestManifestPubSub(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	taskID string,
	fileID string,
	pubkeyHash []byte,
	requestedSegmentIds []string,
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

	// 打印参数信息
	// logger.Infof("构造索引清单请求数据, TaskID=%s, FileID=%s, PubkeyHash=%x, RequestedSegmentIds=%d - %v",
	// 	taskID, fileID, pubkeyHash, len(requestedSegmentIds), requestedSegmentIds)

	// 构造索引清单请求数据
	requestData := &pb.DownloadPubSubManifestRequest{
		TaskId:              taskID,              // 设置任务ID
		FileId:              fileID,              // 设置文件ID
		PubkeyHash:          pubkeyHash,          // 设置公钥哈希
		AddrInfo:            addrInfoBytes,       // 设置地址信息
		RequestedSegmentIds: requestedSegmentIds, // 设置请求的片段ID列表
	}

	// 序列化请求数据为二进制格式
	data, err := requestData.Marshal()
	if err != nil {
		logger.Errorf("序列化请求数据失败: %v", err)
		return err
	}

	// 获取索引清单请求的发布主题
	topic, err := nps.GetTopic(PubSubDownloadManifestRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return err
	}

	// 发送消息
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发送消息失败: %v", err)
		return err
	}

	return nil
}

// HandleDownloadManifestResponsePubSub 处理索引清单响应
// 参数:
//   - ctx: 上下文,用于控制处理过程的生命周期
//   - opt: 文件存储选项配置
//   - nps: 发布订阅系统
//   - db: 数据库存储实例
//   - fs: 文件系统接口
//   - download: 下载管理器实例
//   - res: 接收到的流请求消息
//
// 功能:
//   - 解析索引清单响应数据
//   - 更新下载任务的节点和可用片段信息
//   - 记录任务更新日志
//   - 验证响应数据的有效性
func HandleDownloadManifestResponsePubSub(
	ctx context.Context,
	opt *fscfg.Options,
	nps *pubsub.NodePubSub,
	db *database.DB,
	fs afero.Afero,
	download *DownloadManager,
	res *pubsub.Message,
) {
	if len(res.Data) == 0 {
		logger.Warn("请求数据为空")
		return
	}

	// 解析响应数据为索引清单响应对象
	payload := new(pb.DownloadPubSubManifestResponse)
	if err := payload.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析索引清单响应数据失败: %v", err)
		return
	}
	// fromPeerID, err := peer.IDFromBytes(res.From)
	// if err != nil {
	// 	logger.Errorf("无法解析From字段为PeerID: %v", err)
	// 	return
	// }
	// 从 res.From 解析 AddrInfo
	var fromPeerInfo peer.AddrInfo
	if err := fromPeerInfo.UnmarshalJSON(res.From); err != nil {
		logger.Errorf("无法解析From字段为AddrInfo: %v", err)
		return
	}

	// logger.Infof("解析的索引清单响应数据: From=%s, TaskId=%s, AvailableSlices=%d -%+v",
	// 	fromPeerInfo.ID.String(), payload.TaskId, len(payload.AvailableSlices), payload.AvailableSlices)

	// 获取对应的下载任务
	task, ok := download.getTask(payload.TaskId)
	if !ok {
		logger.Warnf("下载任务: taskID=%s 不存在", payload.TaskId)
		return
	}

	// logger.Infof("fromPeerID.String()=%s, res.ReceivedFrom=%s", fromPeerInfo.ID.String(), res.ReceivedFrom)

	// 更新片段的节点信息并返回未完成的片段索引
	pendingSlices, err := UpdateSegmentNodes(db.BadgerDB, task.TaskID(), fromPeerInfo.ID.String(), payload.AvailableSlices)
	if err != nil {
		logger.Errorf("更新片段节点信息失败: %v", err)
		return
	}

	// 将未完成的片段添加到分片分配管理器
	if len(pendingSlices) > 0 {
		distribution := make(map[peer.ID][]string)
		sliceIDs := make([]string, 0, len(pendingSlices))
		for _, segmentID := range pendingSlices {
			sliceIDs = append(sliceIDs, segmentID)
		}
		distribution[fromPeerInfo.ID] = sliceIDs
		task.distribution.AddDistribution(distribution)
	}

	// logger.Infof("\n已更新下载任务的索引清单信息: taskID=%s, 节点=%s, 未完成片段数=%d",
	// 	task.TaskID(),
	// 	fromPeerInfo.ID.String(),
	// 	len(pendingSlices),
	// )

	// 强制触发节点分发
	if err := task.ForceNodeDispatch(); err != nil {
		logger.Errorf("触发片段处理时失败: %v", err)
		return
	}
}

// RequestContentPubSub 发送文件片段内容请求并等待响应
// 参数:
//   - ctx: 上下文,用于控制请求的生命周期
//   - h: libp2p网络主机实例
//   - nps: 发布订阅系统
//   - taskID: 任务唯一标识
//   - fileID: 文件唯一标识
//   - pubkeyHash: 所有者的公钥哈希
//   - segmentId: 请求的片段ID
//   - segmentIndex: 片段索引
//   - requestedSegmentIds: 请求下载的片段ID列表
//
// 返回值:
//   - *pb.SegmentContentResponse: 片段内容响应对象,包含片段数据和元信息
//   - error: 错误信息,如果请求失败则返回错误原因
//
// 功能:
//   - 构造并发送片段内容请求
//   - 等待并解析响应数据
//   - 返回片段内容响应对象
//   - 记录请求和响应日志
func RequestContentPubSub(
	ctx context.Context,
	h host.Host,
	nps *pubsub.NodePubSub,
	taskID string,
	fileID string,
	pubkeyHash []byte,
	segmentId string,
	segmentIndex int64,
	requestedSegmentIds []string,
) (*pb.SegmentContentResponse, error) {
	// 获取本地节点的地址信息
	addrInfo := peer.AddrInfo{
		ID:    h.ID(),    // 设置节点ID
		Addrs: h.Addrs(), // 设置节点地址列表
	}

	// 序列化地址信息为JSON格式
	addrInfoBytes, err := addrInfo.MarshalJSON()
	if err != nil {
		logger.Errorf("序列化 AddrInfo 失败: %v", err)
		return nil, err
	}

	// 构造片段内容请求数据
	requestData := &pb.SegmentContentRequest{
		TaskId:              taskID,              // 设置任务ID
		FileId:              fileID,              // 设置文件ID
		PubkeyHash:          pubkeyHash,          // 设置公钥哈希
		AddrInfo:            addrInfoBytes,       // 设置地址信息
		SegmentId:           segmentId,           // 设置片段ID
		SegmentIndex:        segmentIndex,        // 设置片段索引
		RequestedSegmentIds: requestedSegmentIds, // 设置请求的片段ID列表
	}

	// 序列化请求数据为二进制格式
	data, err := requestData.Marshal()
	if err != nil {
		logger.Errorf("序列化请求数据失败: %v", err)
		return nil, err
	}

	// 获取片段内容请求的发布主题
	topic, err := nps.GetTopic(PubSubDownloadContentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return nil, err
	}

	// 发送消息并等待响应
	reply, err := topic.PublishWithReply(ctx, data)
	if err != nil {
		logger.Errorf("发送消息失败: %v", err)
		return nil, err
	}

	// 解析响应数据为片段内容响应对象
	responseData := &pb.SegmentContentResponse{}
	if err := responseData.Unmarshal(reply); err != nil {
		logger.Errorf("解析响应数据失败: %v", err)
		return nil, err
	}

	// 记录收到片段内容响应的日志
	// logger.Infof("收到片段内容响应: taskID=%s, fileID=%s, segmentIndex=%d",
	// 	taskID,
	// 	fileID,
	// 	responseData.SegmentIndex,
	// )

	return responseData, nil
}
