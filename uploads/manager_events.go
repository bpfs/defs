package uploads

import (
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/pointsub"
)

// ManagerChannelEvents 处理上传管理器的通道事件
// 此方法启动一个新的goroutine来持续监听和处理各种通道事件
func (m *UploadManager) ManagerChannelEvents() {
	// 启动一个新的goroutine来处理通道事件
	go func() {
		for {
			// 无限循环，等待处理各种通道事件
			select {
			case <-m.ctx.Done():
				// 如果上下文被取消，则退出goroutine
				logger.Info("上下文已取消,退出事件处理循环")
				return

			case taskID := <-m.uploadChan:
				// 接收到新的上传任务请求
				logger.Infof("收到新的上传任务请求,taskID:%s", taskID)
				m.handleNewUploadTask(taskID)

			case payload := <-m.forwardChan:
				// 处理转发请求
				logger.Infof("收到文件片段转发请求,segmentID:%s", payload.SegmentId)
				m.handleForwardRequest(payload)
			}
		}
	}()
}

// handleNewUploadTask 处理新的上传任务请求
// 参数:
//   - taskID: string 要处理的上传任务ID
func (m *UploadManager) handleNewUploadTask(taskID string) {
	// 获取任务
	task, exists := m.getTask(taskID)
	if !exists {
		// 如果任务不存在，记录错误并返回
		logger.Errorf("任务 %s 不存在", taskID)
		m.NotifyError("任务[%s]不存在", taskID)
		return
	}

	// 开始上传任务
	go func() {
		if err := task.Start(); err != nil {
			logger.Errorf("启动任务 %s 失败: %v", taskID, err)
			m.NotifyError("启动任务[%s]失败", taskID)
		}
	}()
}

// handleForwardRequest 处理转发请求
// 将文件片段转发到网络中的其他节点
// 参数:
//   - payload: *pb.FileSegmentStorage 包含需要转发的文件片段信息
func (m *UploadManager) handleForwardRequest(payload *pb.FileSegmentStorage) {
	logger.Infof("处理转发请求: segmentID=%s", payload.SegmentId)

	// 获取存储的文件片段内容
	store := database.NewFileSegmentStorageSqlStore(m.db.SqliteDB)
	segmentStorage, err := store.GetFileSegmentStorage(payload.SegmentId)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败,segmentID:%s,err:%v", payload.SegmentId, err)
		return
	}

	// 验证数据
	if len(segmentStorage.SegmentContent) == 0 {
		logger.Errorf("文件片段内容为空: segmentID=%s", payload.SegmentId)
		return
	}
	payload.SegmentContent = segmentStorage.SegmentContent

	// 根据存储策略确定目标节点数
	targetNodeCount := 3 // TODO: 根据存储策略确定
	maxRetries := 3      // 每个节点的最大重试次数

	// 获取候选节点
	closestPeers := m.routingTable.NearestPeers(kbucket.ConvertKey(payload.SegmentId), targetNodeCount, 2)
	if len(closestPeers) == 0 {
		logger.Warnf("未找到合适的节点处理转发: segmentID=%s", payload.SegmentId)
		return
	}

	// 尝试向每个节点转发,直到成功或所有节点都失败
	for _, peer := range closestPeers {
		retryCount := 0
		for retryCount < maxRetries {
			if err := m.forwardToNode(peer, payload); err != nil {
				logger.Warnf("转发到节点失败(重试 %d/%d): peer=%s, err=%v",
					retryCount+1, maxRetries, peer.String(), err)
				retryCount++
				continue
			}
			// 转发成功,退出所有循环
			logger.Infof("成功转发片段到节点: segmentID=%s, peer=%s",
				payload.SegmentId, peer.String())
			return
		}
		// 当前节点重试次数达到上限,尝试下一个节点
		logger.Warnf("节点转发失败达到最大重试次数: peer=%s", peer.String())
	}

	// 所有节点都失败
	logger.Errorf("转发失败: 所有候选节点都无法完成转发, segmentID=%s", payload.SegmentId)
}

// forwardToNode 转发到单个节点
// 参数:
//   - peer: peer.ID 目标节点ID
//   - payload: *pb.FileSegmentStorage 包含需要转发的文件片段信息
//
// 返回值:
//   - error: 如果转发失败，返回相应的错误信息
func (m *UploadManager) forwardToNode(peer peer.ID, payload *pb.FileSegmentStorage) error {
	// 建立连接
	conn, err := pointsub.Dial(m.ctx, m.host, peer, protocol.ID(StreamForwardToNetworkProtocol))
	if err != nil {
		logger.Errorf("连接节点失败: peer=%s, err=%v", peer.String(), err)
		return err
	}
	defer conn.Close()

	// 创建StreamUtils实例
	streamUtils := NewStreamUtils(conn)

	// 写入数据
	if err := streamUtils.WriteSegmentData(payload); err != nil {
		return err
	}

	// 读取响应
	return streamUtils.ReadResponse()
}
