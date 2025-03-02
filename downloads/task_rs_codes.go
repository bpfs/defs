package downloads

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
)

// HandleRecoverySegments 处理恢复所需片段的下载
//
// 参数:
//   - ctx: context.Context 上下文，用于控制请求的生命周期
//   - task: *DownloadTask 下载任务实例
//
// 返回值:
//   - error: 如果处理过程中出现错误，返回相应的错误信息
func (t *DownloadTask) HandleRecoverySegments(ctx context.Context) error {
	// 获取需要恢复的片段信息
	segmentsToDownload, err := GetSegmentsForRecovery(t.db, t.taskId)
	if err != nil {
		logger.Errorf("获取恢复片段信息失败: taskID=%s, error=%v", t.taskId, err)
		return err
	}

	// 如果没有需要恢复的片段，直接返回
	if len(segmentsToDownload) == 0 {
		logger.Infof("没有需要恢复的片段: taskID=%s", t.taskId)
		return nil
	}

	// 获取文件记录
	downloadFileStore := database.NewDownloadFileStore(t.db)
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil || !exists {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return fmt.Errorf("获取文件记录失败")
	}

	// 获取所有片段ID，用于请求
	allSegmentIds := make([]string, 0, len(fileRecord.SliceTable))
	for _, slice := range fileRecord.SliceTable {
		allSegmentIds = append(allSegmentIds, slice.SegmentId)
	}

	// 遍历需要下载的片段
	for segmentIndex, segmentID := range segmentsToDownload {
		// 发送片段内容请求
		response, err := RequestContentPubSub(
			ctx,
			t.host,
			t.nps,
			t.taskId,
			fileRecord.FileId,
			fileRecord.PubkeyHash,
			segmentID,
			segmentIndex,
			allSegmentIds,
		)
		if err != nil {
			logger.Errorf("请求片段内容失败: taskID=%s, segmentID=%s, error=%v",
				t.taskId, segmentID, err)
			continue
		}

		// 验证响应数据的有效性
		if response == nil || len(response.SegmentContent) == 0 {
			logger.Errorf("收到无效的片段内容响应: taskID=%s, segmentID=%s",
				t.taskId, segmentID)
			continue
		}

		// 验证并存储下载的文件片段
		if err := ValidateAndStoreSegment(
			t.db,
			fileRecord.FirstKeyShare,
			response,
		); err != nil {
			logger.Errorf("验证并存储片段失败: taskID=%s, segmentID=%s, error=%v",
				t.taskId, segmentID, err)
			return err
		}

		// 强制触发检查完成状态
		t.ForceSegmentVerify()

		// 更新下载进度
		progress, err := t.GetProgress()
		if err != nil {
			logger.Errorf("获取进度失败: %v", err)
			continue
		}

		// 更新任务状态
		t.NotifySegmentStatus(&pb.DownloadChan{
			TaskId:           t.taskId,
			SegmentIndex:     response.SegmentIndex,
			DownloadProgress: progress,
			IsComplete:       progress == 100,
		})

		logger.Infof("成功下载并保存恢复片段: taskID=%s, segmentID=%s, index=%d",
			t.taskId, segmentID, segmentIndex)
	}

	logger.Infof("订阅恢复处理完成: taskID=%s", t.taskId)
	return nil
}
