package downloads

import (
	"fmt"
	"time"

	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/pb"
)

// Start 开始下载任务
// 返回值:
//   - error: 错误信息
//
// 功能:
//   - 启动多个协程处理下载任务
//   - 初始化任务通道和定时器
//   - 强制更新索引清单
//   - 记录任务开始日志
func (t *DownloadTask) Start() error {
	// 启动goroutine处理各种事件
	go func() {
		// 创建定时器，每30秒触发一次
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// 无限循环处理事件,直到任务完成或取消
		for {
			select {
			case <-t.ctx.Done():
				// 上下文被取消,退出处理循环
				logger.Info("任务上下文已取消，退出处理循环")
				return

			case <-ticker.C:
				// 每30秒触发一次片段索引请求
				logger.Info("定时触发片段索引请求")
				if err := t.ForceSegmentIndex(); err != nil {
					logger.Errorf("定时触发片段索引请求失败: %v", err)
					t.NotifyTaskError(err)
				}

			case <-t.chSegmentIndex:
				// 处理片段索引请求
				logger.Info("收到片段索引请求")
				if err := t.handleSegmentIndex(); err != nil {
					logger.Errorf("处理片段索引请求失败: %v", err)
					t.NotifyTaskError(err)
				}

			case <-t.chSegmentProcess:
				// 处理文件片段：将文件片段整合并写入队列
				if err := t.handleSegmentProcess(); err != nil {
					logger.Errorf("处理文件片段失败: %v", err)
					t.NotifyTaskError(err)
				}

			case <-t.chNodeDispatch:
				// 节点分发：以节点为单位从队列中读取文件片段
				if err := t.handleNodeDispatch(); err != nil {
					logger.Errorf("处理节点分发请求失败: %v", err)
					t.NotifyTaskError(err)
				}

			case peerSegments := <-t.chNetworkTransfer:
				// 网络传输：向目标节点传输文件片段
				logger.Infof("收到网络传输请求: segments=%d", len(peerSegments))
				if err := t.handleNetworkTransfer(peerSegments); err != nil {
					logger.Errorf("处理网络传输请求失败: %v", err)
					t.NotifyTaskError(err)
				}

			case <-t.chSegmentVerify:
				// 片段验证：验证已传输片段的完整性
				logger.Info("收到片段验证请求")
				if err := t.handleSegmentVerify(); err != nil {
					logger.Errorf("处理片段验证失败: %v", err)
					t.NotifyTaskError(err)
				}

			case <-t.chSegmentMerge:
				// 处理片段合并
				logger.Info("收到片段合并请求")
				if err := t.handleSegmentMerge(); err != nil {
					logger.Errorf("处理片段合并失败: %v", err)
					t.NotifyTaskError(err)
				}

			case <-t.chFileFinalize:
				// 文件完成：处理文件上传完成后的操作
				logger.Info("收到文件完成请求")
				if err := t.handleFileFinalize(); err != nil {
					logger.Errorf("处理文件完成请求失败: %v", err)
					t.NotifyTaskError(err)
				}
				return // 文件处理完成，退出循环

			case <-t.chPause:
				// 暂停：暂停当前上传任务
				logger.Info("收到暂停信号")
				// 先取消上下文
				t.cancel()
				if err := t.handlePause(); err != nil {
					logger.Errorf("处理暂停请求失败: %v", err)
					t.NotifyTaskError(err)
				}
				return

			case <-t.chCancel:
				// 取消：取消当前上传任务
				logger.Info("收到取消信号")
				// 先取消上下文
				t.cancel()
				if err := t.handleCancel(); err != nil {
					logger.Errorf("处理取消请求失败: %v", err)
					t.NotifyTaskError(err)
				}
				return

			case <-t.chDelete:
				// 删除：删除当前上传任务及相关资源
				logger.Info("收到删除信号")
				// 先取消上下文
				t.cancel()
				if err := t.handleDelete(); err != nil {
					logger.Errorf("处理删除请求失败: %v", err)
					t.NotifyTaskError(err)
				}
				return

			}
		}
	}()

	// 触发初始的文件片段索引
	return t.ForceSegmentIndex()
}

// Cancel 取消下载任务
// 返回值:
//   - error: 错误信息
//
// 功能:
//   - 使用事务取消下载任务
//   - 清理任务相关资源
//   - 记录取消操作日志
func (t *DownloadTask) Cancel() error {
	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)

	// 获取下载文件记录
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取下载文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("任务不存在: taskID=%s", t.taskId)
	}

	// 检查任务状态是否可以取消
	switch fileRecord.Status {
	case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED:
		// 已完成的任务不能取消
		return fmt.Errorf("任务已完成，无法取消")

	case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED:
		// 已经是取消状态，直接返回
		return nil

	default:
		// 其他所有状态都可以取消
		if err := t.ForceCancel(); err != nil {
			logger.Errorf("触发取消失败: taskID=%s, err=%v", t.taskId, err)
			return err
		}
		return nil
	}
}

// Pause 暂停下载任务
// 返回值:
//   - error: 错误信息
//
// 功能:
//   - 使用事务暂停下载任务
//   - 清理任务相关资源
//   - 记录暂停操作日志
func (t *DownloadTask) Pause() error {
	// 创建下载任务存储对象
	downloadFileStore := database.NewDownloadFileStore(t.db)

	// 获取当前任务状态
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取任务状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("任务不存在: taskID=%s", t.taskId)
	}

	// 检查任务状态是否可以暂停
	switch fileRecord.Status {
	case pb.DownloadStatus_DOWNLOAD_STATUS_PENDING,
		pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING:
		// 可以暂停的状态
		if err := t.ForcePause(); err != nil {
			logger.Errorf("触发暂停失败: taskID=%s, err=%v", t.taskId, err)
			return err
		}
		return nil

	case pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED:
		// 已经是暂停状态，直接返回
		return nil

	case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法暂停")

	case pb.DownloadStatus_DOWNLOAD_STATUS_FAILED:
		return fmt.Errorf("任务已失败，无法暂停")

	case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED:
		return fmt.Errorf("任务已取消，无法暂停")

	default:
		return fmt.Errorf("未知的任务状态，无法暂停")
	}
}

// Delete 删除下载任务相关的数据
// 该方法用于删除下载任务的所有相关数据
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func (t *DownloadTask) Delete() error {
	// 触发强制删除
	if err := t.ForceDelete(); err != nil {
		logger.Errorf("删除任务数据失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	return nil
}

// GetProgress 获取下载进度的百分比
// 返回值:
//   - int64: 下载进度百分比(0-100)
//
// 功能:
//   - 使用BitSet跟踪片段完成状态
//   - 计算当前下载进度百分比
//   - 记录进度日志
//   - 返回进度值
func (t *DownloadTask) GetProgress() (int64, error) {
	// 创建下载片段存储实例
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)
	downloadFileStore := database.NewDownloadFileStore(t.db)

	// 获取文件记录以确定总片段数
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil || !exists {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return 0, fmt.Errorf("获取文件记录失败")
	}

	// 获取切片表长度作为总片段数
	totalSegments := len(fileRecord.SliceTable)
	if totalSegments == 0 {
		return 0, fmt.Errorf("获取文件记录失败")
	}

	// 获取已完成的片段
	segments, err := downloadSegmentStore.FindByTaskIDAndStatus(
		t.taskId,
		pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED,
	)
	if err != nil {
		logger.Errorf("获取已完成片段失败: taskID=%s, err=%v", t.taskId, err)
		return 0, err
	}

	// 计算进度百分比
	progress := int64((float64(len(segments)) / float64(totalSegments)) * 100)

	// 确保进度值在有效范围内
	if progress > 100 {
		progress = 100
	} else if progress < 0 {
		progress = 0
	}

	logger.Debugf("下载进度: taskID=%s, 已完成=%d, 总数=%d, 进度=%d%%",
		t.taskId, len(segments), totalSegments, progress)

	return progress, err
}
