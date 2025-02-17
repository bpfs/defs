package downloads

// ManagerChannelEvents 处理下载管理器的通道事件
// 功能:
//   - 启动一个goroutine监听下载管理器的通道事件
//   - 处理上下文取消和新的下载任务请求
//   - 通过select实现多路复用监听
func (m *DownloadManager) ManagerChannelEvents() {
	// 启动一个goroutine处理事件
	go func() {
		// 无限循环监听事件
		for {
			// 使用select监听多个通道
			select {
			case <-m.ctx.Done():
				// 如果上下文被取消，记录日志并退出函数
				logger.Info("下载管理器上下文已取消,退出事件监听")
				return

			case taskID := <-m.downloadChan:
				// 从下载通道接收到新的任务ID
				logger.Infof("收到新的下载任务请求: %s", taskID)
				// 处理新的下载任务请求
				m.handleNewDownloadTask(taskID)
			}
		}
	}()
}

// handleNewDownloadTask 处理新的下载任务请求
// 参数:
//   - taskID: 任务ID
//
// 功能:
//   - 根据任务ID获取下载任务
//   - 启动下载任务的执行
//   - 异步处理下载任务
//   - 记录任务执行状态和错误信息
func (m *DownloadManager) handleNewDownloadTask(taskID string) {
	// 获取任务
	task, exists := m.getTask(taskID)
	if !exists {
		// 如果任务不存在，记录错误并返回
		logger.Errorf("任务不存在,taskID:%s", taskID)
		m.NotifyError("任务[%s]不存在", taskID)
		return
	}

	// 启动下载任务
	if err := task.Start(); err != nil {
		logger.Errorf("启动下载任务 %s 失败: %v", taskID, err)
		m.NotifyError("启动任务[%s]失败", taskID)
		return
	}
}
