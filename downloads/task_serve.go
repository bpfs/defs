package downloads

import (
	"fmt"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
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

			}
		}
	}()

	// 触发初始的文件片段索引
	return t.ForceSegmentIndex()
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
