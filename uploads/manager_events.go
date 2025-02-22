package uploads

import (
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
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
// 参数:
//   - payload: *pb.FileSegmentStorage 包含需要转发的文件段信息
func (m *UploadManager) handleForwardRequest(payload *pb.FileSegmentStorage) {
	// 创建 FileSegmentStorageStore 实例
	store := database.NewFileSegmentStorageSqlStore(m.db.SqliteDB)

	// 获取文件片段存储记录
	segmentStorage, err := store.GetFileSegmentStorage(payload.SegmentId)
	// 如果获取失败，记录错误日志并返回错误
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败,segmentID:%s,err:%v", payload.SegmentId, err)
		return
	}

	// 将读取的数据添加到 payload 中
	payload.SegmentContent = segmentStorage.SegmentContent

	// 使用新的方法发送转发请求
	// Todo: 需要实现
	// if err := sendForwardRequest(m.ctx, m.host, m.routingTable, payload); err != nil {
	// 	logger.Errorf("转发文件失败,segmentID:%s,err:%v", payload.SegmentId, err)
	// }
}
