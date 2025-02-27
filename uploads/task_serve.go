package uploads

import (
	"fmt"

	"github.com/bpfs/defs/v2/database"
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
				logger.Info("任务上下文已取消，退出处理循环")
				return

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

	// 触发初始的文件片段处理
	return t.ForceSegmentProcess()
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
