package uploads

import (
	"fmt"
	"time"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/bitset"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
)

// CreateUploadFileRecord 创建上传文件记录并保存到数据库
// 该方法用于初始化一个新的文件上传记录，并将其保存到持久化存储中
//
// 参数:
//   - db: 上传文件数据库存储接口
//   - taskID: 上传任务的唯一标识
//   - fileID: 文件的唯一标识
//   - name: 文件名称/路径
//   - fileMeta: 文件的元数据信息
//   - fileSecurity: 文件的安全相关信息
//   - status: 文件上传状态
//
// 返回值:
//   - error: 如果创建或存储过程中发生错误则返回相应错误，否则返回 nil
func CreateUploadFileRecord(
	db *badgerhold.Store,
	taskID string,
	fileID string,
	name string,
	fileMeta *pb.FileMeta,
	fileSecurity *pb.FileSecurity,
	status pb.UploadStatus,
) error {
	// 创建 FileSegmentStorageStore 实例
	store := database.NewUploadFileStore(db)

	// 构建完整的 UploadFileRecord 对象
	fileRecord := &pb.UploadFileRecord{
		TaskId:       taskID,                        // 任务唯一标识
		FileId:       fileID,                        // 文件唯一标识
		Path:         name,                          // 文件路径
		FileMeta:     fileMeta,                      // 文件元数据
		FileSecurity: fileSecurity,                  // 文件安全信息
		SliceTable:   make(map[int64]*pb.HashTable), // 初始化空的分片哈希表
		StartedAt:    time.Now().Unix(),             // 记录任务创建时间
		Status:       status,                        // 设置初始状态
	}

	// 将文件记录保存到数据库
	if err := store.CreateUploadFile(fileRecord); err != nil {
		logger.Errorf("创建上传文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}

	return nil
}

// UpdateUploadFileStatus 更新上传文件的状态
// 参数:
//   - db: *badgerhold.Store 数据库存储接口
//   - taskID: string 上传任务的唯一标识
//   - status: pb.UploadStatus 新的文件状态
//
// 返回值:
//   - error: 如果更新过程中发生错误则返回相应错误，否则返回 nil
func UpdateUploadFileStatus(db *badgerhold.Store, taskID string, status pb.UploadStatus) error {
	// 创建文件存储接口
	store := database.NewUploadFileStore(db)

	// 获取当前文件记录
	fileRecord, exists, err := store.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		logger.Errorf("文件记录不存在: taskID=%s", taskID)
		return err
	}

	// 更新状态
	fileRecord.Status = status
	logger.Infof("更新文件状态: taskID=%s, status=%s", taskID, status.String())

	// 将更新后的记录保存到数据库
	if err := store.UpdateUploadFile(fileRecord); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, status=%s, err=%v", taskID, status.String(), err)
		return err
	}

	return nil
}

// UpdateUploadFileHashTable 更新上传文件的哈希表
// 该方法仅在文件处于编码状态时更新哈希表，并将状态改为待上传
// 参数:
//   - db: *badgerhold.Store 数据库存储接口
//   - taskID: string 上传任务的唯一标识
//   - sliceTable: map[int64]*pb.HashTable 分片哈希表
//
// 返回值:
//   - error: 如果更新过程中发生错误则返回相应错误，否则返回 nil
func UpdateUploadFileHashTable(db *badgerhold.Store, taskID string, sliceTable map[int64]*pb.HashTable) error {
	// 检查参数
	if sliceTable == nil {
		logger.Errorf("哈希表不能为空: taskID=%s", taskID)
		return fmt.Errorf("哈希表不能为空: taskID=%s", taskID)
	}

	// 创建文件存储接口
	store := database.NewUploadFileStore(db)

	// 获取当前文件记录
	fileRecord, exists, err := store.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		logger.Errorf("文件记录不存在: taskID=%s", taskID)
		return err
	}

	// 检查文件状态是否为编码中
	if fileRecord.Status != pb.UploadStatus_UPLOAD_STATUS_ENCODING {
		logger.Errorf("文件状态不是编码中: taskID=%s, status=%s", taskID, fileRecord.Status.String())
		return err
	}

	// 更新哈希表和状态
	fileRecord.SliceTable = sliceTable
	fileRecord.Status = pb.UploadStatus_UPLOAD_STATUS_PENDING
	logger.Infof("更新文件哈希表并将状态更新为待上传: taskID=%s", taskID)

	// 将更新后的记录保存到数据库
	if err := store.UpdateUploadFile(fileRecord); err != nil {
		logger.Errorf("更新文件哈希表失败: taskID=%s, err=%v", taskID, err)
		return err
	}

	return nil
}

// ValidateAndUpdateUploadStatus 校验并更新上传状态
// 该方法用于开始上传前的状态校验，会根据实际进度强制更新任务状态：
// 1. 如果所有分片已完成，则更新为已完成状态并返回错误
// 2. 如果还有未完成分片，则更新为上传中状态
//
// 参数:
//   - db: *badgerhold.Store 数据库存储接口
//   - fileRecord: *pb.UploadFileRecord 文件记录
//   - progress: bitset.BitSet 上传进度位图
//
// 返回值:
//   - error: 如果所有分片已完成或发生其他错误则返回错误信息，否则返回 nil
func ValidateAndUpdateUploadStatus(db *badgerhold.Store, fileRecord *pb.UploadFileRecord, progress bitset.BitSet) error {
	// 检查参数
	if fileRecord == nil {
		logger.Error("文件记录为空")
		return fmt.Errorf("文件记录为空")
	}

	// 获取总分片数
	totalShards := progress.Len()
	if totalShards == 0 {
		logger.Error("进度位图为空")
		return fmt.Errorf("进度位图为空")
	}

	// 创建文件存储接口
	store := database.NewUploadFileStore(db)

	// 计算已完成的分片数
	completedShards := progress.Count()
	isCompleted := completedShards == uint(totalShards)

	if isCompleted {
		// 如果所有分片都已完成，更新为已完成状态并返回错误
		if fileRecord.Status != pb.UploadStatus_UPLOAD_STATUS_COMPLETED {
			logger.Infof("检测到任务所有分片已完成，更新状态为已完成: taskID=%s", fileRecord.TaskId)
			fileRecord.Status = pb.UploadStatus_UPLOAD_STATUS_COMPLETED
			fileRecord.FinishedAt = time.Now().Unix()
			if err := store.UpdateUploadFile(fileRecord); err != nil {
				logger.Errorf("更新文件状态为已完成失败: taskID=%s, err=%v", fileRecord.TaskId, err)
				return err
			}
		}
		logger.Errorf("所有分片已完成，无需继续上传: taskID=%s", fileRecord.TaskId)
		return fmt.Errorf("所有分片已完成，无需继续上传")
	}

	// 如果还有未完成分片，强制更新为上传中状态
	if fileRecord.Status != pb.UploadStatus_UPLOAD_STATUS_UPLOADING {
		if fileRecord.Status != pb.UploadStatus_UPLOAD_STATUS_PAUSED {
			logger.Infof("更新任务状态为上传中: taskID=%s, oldStatus=%s",
				fileRecord.TaskId, fileRecord.Status.String())
			fileRecord.Status = pb.UploadStatus_UPLOAD_STATUS_UPLOADING
			fileRecord.FinishedAt = 0 // 清除完成时间
			if err := store.UpdateUploadFile(fileRecord); err != nil {
				logger.Errorf("更新文件状态为上传中失败: taskID=%s, err=%v", fileRecord.TaskId, err)
				return err
			}
		}
	}

	return nil
}
