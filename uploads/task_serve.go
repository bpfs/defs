package uploads

import (
	"fmt"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
)

// Start 开始或恢复上传任务
// 该方法会启动一个goroutine来处理上传任务的各种事件
//
// 返回值:
//   - error: 如果开始上传过程中发生错误，返回相应的错误信息
func (t *UploadTask) Start() error {
	// 启动goroutine处理各种事件
	go func() {
		// 无限循环处理事件,直到任务完成或取消
		for {
			select {
			case <-t.ctx.Done():
				// 上下文被取消,退出处理循环
				fmt.Println("==取消")
				logger.Info("任务上下文已取消，退出处理循环")
				return

			case <-t.chSegmentProcess:
				// 处理文件片段：将文件片段整合并写入队列

				if err := t.handleSegmentProcess(); err != nil {
					logger.Errorf("处理文件片段失败: %v", err)
					t.NotifyTaskError(err)
				}

			case segmentID := <-t.chSendClosest:
				// 发送最近的节点: 分片ID
				logger.Infof("收到发送最近的节点请求: segmentID=%s", segmentID)
				if err := t.handleSendClosest(segmentID); err != nil {
					logger.Errorf("发送最近的节点失败: segmentID=%s, err=%v", segmentID, err)
					t.NotifyTaskError(err)
				}
			case <-t.chSegmentVerify:
				// 片段验证：验证已传输片段的完整性
				logger.Info("收到片段验证请求")
				if err := t.handleSegmentVerify(); err != nil {
					logger.Errorf("处理片段验证失败: %v", err)
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
				fmt.Println("==收到暂停")
				logger.Info("收到暂停信号")
				// 先取消上下文
				t.cancel()
				if err := t.handlePause(); err != nil {
					logger.Errorf("处理暂停请求失败: %v", err)
					t.NotifyTaskError(err)
				}
				return
				// TODO:暂停的时候上下文已经取消，就算发送取消或者删除也接收不到消息，所以直接处理就行
				// case <-t.chCancel:
				// 	// 取消：取消当前上传任务
				// 	fmt.Println("==收到取消信号")
				// 	logger.Info("收到取消信号")
				// 	// 先取消上下文
				// 	t.cancel()
				// 	if err := t.handleCancel(); err != nil {
				// 		logger.Errorf("处理取消请求失败: %v", err)
				// 		t.NotifyTaskError(err)
				// 	}
				// 	return

				// case <-t.chDelete:
				// 	// 删除：删除当前上传任务及相关资源
				// 	logger.Info("收到删除信号")
				// 	// 先取消上下文
				// 	t.cancel()
				// 	if err := t.handleDelete(); err != nil {
				// 		logger.Errorf("处理删除请求失败: %v", err)
				// 		t.NotifyTaskError(err)
				// 	}
				// 	return

			}
		}
	}()

	// 触发初始的文件片段处理
	return t.ForceSegmentProcess()
}

// Pause 暂停上传任务
// 该方法用于暂停正在进行的上传任务
//
// 返回值:
//   - error: 如果暂停过程中发生错误，返回相应的错误信息
func (t *UploadTask) Pause() error {
	// 创建上传任务存储对象
	uploadTaskStore := database.NewUploadFileStore(t.db)

	// 获取当前任务状态
	fileRecord, exists, err := uploadTaskStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取任务状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("任务不存在: taskID=%s", t.taskId)
	}

	// 检查任务状态是否可以暂停
	switch fileRecord.Status {
	case pb.UploadStatus_UPLOAD_STATUS_PENDING,
		pb.UploadStatus_UPLOAD_STATUS_UPLOADING:
		// 可以暂停的状态
		if err := t.ForcePause(); err != nil {
			logger.Errorf("触发暂停失败: taskID=%s, err=%v", t.taskId, err)
			return err
		}

		return nil

	case pb.UploadStatus_UPLOAD_STATUS_PAUSED:
		// 已经是暂停状态，直接返回
		return nil

	case pb.UploadStatus_UPLOAD_STATUS_ENCODING:
		return fmt.Errorf("文件正在编码处理中，无法暂停")

	case pb.UploadStatus_UPLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法暂停")

	case pb.UploadStatus_UPLOAD_STATUS_FAILED:
		return fmt.Errorf("任务已失败，无法暂停")

	case pb.UploadStatus_UPLOAD_STATUS_CANCELED:
		return fmt.Errorf("任务已取消，无法暂停")

	case pb.UploadStatus_UPLOAD_STATUS_FILE_EXCEPTION:
		return fmt.Errorf("文件存在异常，无法暂停")

	default:
		return fmt.Errorf("未知的任务状态，无法暂停")
	}
}

// Cancel 取消上传任务
// 返回值:
//   - error: 如果取消过程中发生错误，返回相应的错误信息
func (t *UploadTask) Cancel() error {
	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("任务不存在: taskID=%s", t.taskId)
	}

	// 检查任务状态是否可以取消
	switch fileRecord.Status {
	case pb.UploadStatus_UPLOAD_STATUS_COMPLETED:
		// 已完成的任务不能取消
		return fmt.Errorf("任务已完成，无法取消")

	case pb.UploadStatus_UPLOAD_STATUS_CANCELED:
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

// Delete 删除上传任务相关的数据
// 该方法用于删除上传任务的所有相关数据
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func (t *UploadTask) Delete() error {
	// 触发强制删除
	if err := t.ForceDelete(); err != nil {
		logger.Errorf("删除任务数据失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	return nil
}

// GetTotalSize 返回文件大小加上奇偶校验片段的总大小
// 该方法计算文件的总大小,包括原始文件和奇偶校验片段
//
// 返回值:
//   - int64: 文件总大小（包括原始文件大小和奇偶校验片段大小）
//   - error: 如果计算过程中发生错误，返回相应的错误信息
func (t *UploadTask) GetTotalSize() (int64, error) {
	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return 0, err
	}
	if !exists {
		logger.Errorf("上传文件记录不存在: taskID=%s", t.taskId)
		return 0, fmt.Errorf("上传文件记录不存在: taskID=%s", t.taskId)
	}

	// 获取原始文件大小
	fileSize := fileRecord.FileMeta.Size_

	// 获取奇偶校验分片数量
	_, parityShards, err := t.GetShardCounts()
	if err != nil {
		logger.Errorf("获取分片数失败: taskID=%s, err=%v", t.taskId, err)
		return 0, err
	}

	// 计算奇偶校验片段总大小
	paritySize := int64(parityShards) * t.opt.GetShardSize()

	// 返回文件总大小(原始大小 + 校验片段大小)
	return fileSize + paritySize, nil
}

// GetShardCounts 返回文件的总分片数和冗余分片数
// 该方法统计文件的分片信息,包括总分片数和冗余分片数
//
// 返回值:
//   - totalShards: int64 总分片数
//   - parityShards: int64 冗余分片数
//   - error: 如果获取分片数失败，返回相应的错误信息
func (t *UploadTask) GetShardCounts() (totalShards, parityShards int64, err error) {
	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return 0, 0, err
	}
	if !exists {
		logger.Errorf("上传文件记录不存在: taskID=%s", t.taskId)
		return 0, 0, fmt.Errorf("上传文件记录不存在: taskID=%s", t.taskId)
	}

	// 遍历分片表,统计总分片数和冗余分片数
	for _, hashTable := range fileRecord.SliceTable {
		totalShards++ // 增加总分片计数
		if hashTable.IsRsCodes {
			parityShards++ // 果是校验分片则增加校验分片计数
		}
	}

	// 返回统计结果
	return totalShards, parityShards, nil
}
