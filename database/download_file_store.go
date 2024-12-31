package database

import (
	"bytes"
	"fmt"

	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
)

// DownloadFileStore 管理下载文件记录的存储
type DownloadFileStore struct {
	store *badgerhold.Store
}

// NewDownloadFileStore 创建一个新的下载文件存储管理器
func NewDownloadFileStore(store *badgerhold.Store) *DownloadFileStore {
	return &DownloadFileStore{
		store: store,
	}
}

// Insert 插入一条新的下载文件记录
// 参数:
//   - record: 要插入的下载文件记录
//
// 返回值:
//   - error: 如果插入过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) Insert(record *pb.DownloadFileRecord) error {
	// 首先尝试插入记录到数据库
	err := s.store.Insert(record.TaskId, record)
	if err != nil {
		// 如果错误是因为记录已存在
		if err == badgerhold.ErrKeyExists {
			// 尝试更新已存在的记录
			updateErr := s.store.Update(record.TaskId, record)
			if updateErr != nil {
				// 如果更新也失败,记录错误日志并返回错误信息
				logger.Errorf("更新下载文件记录 %v 时出错: %v", record.TaskId, updateErr)
				return updateErr
			}
			// 更新成功,记录日志
			logger.Infof("下载文件记录 %s 已存在，已更新", record.TaskId)
			return nil
		}
		// 如果是其他错误,记录错误日志并返回
		logger.Errorf("保存下载文件记录时出错: %v", err)
		return err
	}

	return nil
}

// Update 更新现有的下载文件记录
// 参数:
//   - record: 要更新的下载文件记录
//
// 返回值:
//   - error: 如果更新过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) Update(record *pb.DownloadFileRecord) error {
	if record == nil || record.TaskId == "" {
		return fmt.Errorf("无效的更新记录")
	}

	// 获取现有记录
	var existingRecord pb.DownloadFileRecord
	err := s.store.Get(record.TaskId, &existingRecord)
	if err != nil {
		return fmt.Errorf("获取现有记录失败: %v", err)
	}

	// 检查字段是否需要更新
	needsUpdate := false

	// FileId 变更（通常不应该变）
	if record.FileId != "" && record.FileId != existingRecord.FileId {
		existingRecord.FileId = record.FileId
		needsUpdate = true
		logger.Warnf("文件ID发生变更: %s -> %s", existingRecord.FileId, record.FileId)
	}

	// Status 变更
	if record.Status != pb.DownloadStatus_DOWNLOAD_STATUS_UNSPECIFIED &&
		record.Status != existingRecord.Status {
		existingRecord.Status = record.Status
		needsUpdate = true
	}

	// FileMeta 变更
	if record.FileMeta != nil {
		existingRecord.FileMeta = record.FileMeta
		needsUpdate = true
	}

	// SliceTable 变更
	if record.SliceTable != nil && len(record.SliceTable) > 0 {
		existingRecord.SliceTable = record.SliceTable
		needsUpdate = true
	}

	// FirstKeyShare 变更
	if len(record.FirstKeyShare) > 0 {
		existingRecord.FirstKeyShare = record.FirstKeyShare
		needsUpdate = true
	}

	// PubkeyHash 变更
	if len(record.PubkeyHash) > 0 && !bytes.Equal(record.PubkeyHash, existingRecord.PubkeyHash) {
		existingRecord.PubkeyHash = record.PubkeyHash
		needsUpdate = true
	}

	// StartedAt 变更
	if record.StartedAt > 0 && record.StartedAt != existingRecord.StartedAt {
		existingRecord.StartedAt = record.StartedAt
		needsUpdate = true
	}

	// 只有在字段发生变化时才执行更新
	if needsUpdate {
		if err := s.store.Update(record.TaskId, &existingRecord); err != nil {
			return fmt.Errorf("更新记录失败: %v", err)
		}
		logger.Debugf("更新下载文件记录: TaskID=%s, Status=%s", record.TaskId, existingRecord.Status)
	}

	return nil
}

// Get 根据任务ID获取下载文件记录
// 参数:
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - *pb.DownloadFileRecord: 找到的下载文件记录
//   - bool: 记录是否存在
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) Get(taskID string) (*pb.DownloadFileRecord, bool, error) {
	var record pb.DownloadFileRecord
	err := s.store.Get(taskID, &record)
	if err == badgerhold.ErrNotFound {
		return nil, false, nil // 记录不存在，返回 (nil, false, nil)
	}

	if err != nil {
		logger.Errorf("获取下载文件记录失败: %v", err)
		return nil, false, err // 系统错误，返回 (nil, false, err)
	}

	return &record, true, nil
}

// Delete 删除指定任务ID的下载文件记录
// 参数:
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - error: 如果删除过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) Delete(taskID string) error {
	return s.store.Delete(taskID, &pb.DownloadFileRecord{})
}

// FindByStatus 根据下载状态查找文件记录
// 参数:
//   - status: 下载状态
//
// 返回值:
//   - []*pb.DownloadFileRecord: 符合条件的下载文件记录列表
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) FindByStatus(status pb.DownloadStatus) ([]*pb.DownloadFileRecord, error) {
	var records []*pb.DownloadFileRecord
	err := s.store.Find(&records, badgerhold.Where("Status").Eq(status))
	if err != nil {
		logger.Errorf("查找下载文件记录失败: %v", err)
		return nil, err
	}
	return records, nil
}

// FindByFileID 根据文件ID查找下载记录
// 参数:
//   - fileID: 文件的唯一标识符
//
// 返回值:
//   - []*pb.DownloadFileRecord: 符合条件的下载文件记录列表
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) FindByFileID(fileID string) ([]*pb.DownloadFileRecord, error) {
	var records []*pb.DownloadFileRecord
	err := s.store.Find(&records, badgerhold.Where("FileId").Eq(fileID))
	if err != nil {
		logger.Errorf("查找下载文件记录失败: %v", err)
		return nil, err
	}
	return records, nil
}
