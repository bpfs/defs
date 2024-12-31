package uploads

import (
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/utils/logger"
)

// GetProgress 计算并返回当前上传进度
// 返回值:
//   - int64: 返回0-100之间的进度值
func (t *UploadTask) GetProgress() (int64, error) {
	// 创建存储实例
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 获取上传进度信息
	total, completed, err := uploadSegmentStore.GetUploadProgress(t.TaskID())
	if err != nil {
		logger.Errorf("获取上传进度失败: taskID=%s, err=%v", t.TaskID(), err)
		return 0, err
	}

	// 防止除零错误
	if total == 0 {
		return 0, nil
	}

	// 计算进度百分比
	progress := int64((float64(completed) / float64(total)) * 100)

	// 确保进度值在有效范围内
	if progress > 100 {
		progress = 100
	} else if progress < 0 {
		progress = 0
	}

	return progress, nil
}
