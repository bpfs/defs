// Package database 提供数据库操作相关功能
package database

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/pb"

	"github.com/dgraph-io/badger/v4"
)

// InsertTx 在事务中插入一条新的下载片段记录
// 参数:
//   - txn: *badger.Txn 数据库事务对象
//   - record: *pb.DownloadSegmentRecord 要插入的下载片段记录
//
// 返回值:
//   - error: 如果插入过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) InsertTx(txn *badger.Txn, record *pb.DownloadSegmentRecord) error {
	// 尝试插入记录
	err := s.store.TxInsert(txn, record.SegmentId, record)
	if err != nil {
		// 如果错误是因为记录已存在
		if err == badgerhold.ErrKeyExists {
			// 尝试更新已存在的记录
			updateErr := s.store.TxUpdate(txn, record.SegmentId, record)
			if updateErr != nil {
				// 如果更新也失败,记录错误日志并返回错误信息
				logger.Errorf("更新下载片段记录 %v 时出错: %v", record.SegmentId, updateErr)
				return updateErr
			}
			// 更新成功,记录日志
			logger.Infof("下载片段记录 %s 已存在，已更新", record.SegmentId)
			return nil
		}
		// 如果是其他错误,记录错误日志并返回
		logger.Errorf("保存下载片段记录时出错: %v", err)
		return err
	}

	return nil
}

// UpdateTx 在事务中更新下载片段记录,只更新与现有记录不同的字段
// 参数:
//   - txn: *badger.Txn 数据库事务对象
//   - record: *pb.DownloadSegmentRecord 要更新的记录
//   - validateContent: ...bool 是否校验内容哈希（可选参数）
//
// 返回值:
//   - error: 如果更新过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) UpdateTx(txn *badger.Txn, record *pb.DownloadSegmentRecord, validateContent ...bool) error {
	// 参数有效性检查
	if record == nil || record.SegmentId == "" {
		return fmt.Errorf("无效的更新记录")
	}

	// 获取现有记录
	var existingRecord pb.DownloadSegmentRecord
	err := s.store.TxGet(txn, record.SegmentId, &existingRecord)
	if err != nil {
		logger.Errorf("获取现有记录失败: %v", err)
		return err
	}

	// 标记是否需要更新
	needsUpdate := false

	// TaskId变更检查（通常不应该变）
	if record.TaskId != "" && record.TaskId != existingRecord.TaskId {
		existingRecord.TaskId = record.TaskId
		needsUpdate = true
		logger.Warnf("片段任务ID发生变更: %s -> %s", existingRecord.TaskId, record.TaskId)
	}

	// SegmentIndex变更检查
	if record.SegmentIndex != existingRecord.SegmentIndex {
		existingRecord.SegmentIndex = record.SegmentIndex
		needsUpdate = true
	}

	// Status变更检查
	if record.Status != pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_UNSPECIFIED &&
		record.Status != existingRecord.Status {
		existingRecord.Status = record.Status
		needsUpdate = true
	}

	// IsRsCodes变更检查
	if record.IsRsCodes != existingRecord.IsRsCodes {
		existingRecord.IsRsCodes = record.IsRsCodes
		needsUpdate = true
	}

	// Size_变更检查
	if record.Size_ > 0 && record.Size_ != existingRecord.Size_ {
		existingRecord.Size_ = record.Size_
		needsUpdate = true
	}

	// Crc32Checksum变更检查
	if record.Crc32Checksum > 0 && record.Crc32Checksum != existingRecord.Crc32Checksum {
		existingRecord.Crc32Checksum = record.Crc32Checksum
		needsUpdate = true
	}

	// 内容变更检查
	if record.SegmentContent != nil {
		shouldUpdate := true
		// 如果需要校验内容
		if len(validateContent) > 0 && validateContent[0] {
			// 如果有校验和，优先使用校验和比较
			if record.Crc32Checksum > 0 && existingRecord.Crc32Checksum > 0 {
				shouldUpdate = record.Crc32Checksum != existingRecord.Crc32Checksum
			} else {
				// 否则使用SHA256比较
				oldHash := sha256.Sum256(existingRecord.SegmentContent)
				newHash := sha256.Sum256(record.SegmentContent)
				shouldUpdate = !bytes.Equal(oldHash[:], newHash[:])
			}
		}
		if shouldUpdate {
			existingRecord.SegmentContent = record.SegmentContent
			needsUpdate = true
		}
	}

	// 只有在字段发生变化时才执行更新
	if needsUpdate {
		if err := s.store.TxUpdate(txn, record.SegmentId, &existingRecord); err != nil {
			logger.Errorf("更新记录失败: %v", err)
			return err
		}
		logger.Debugf("更新分片记录: ID=%s, Status=%s, Index=%d",
			record.SegmentId, existingRecord.Status, existingRecord.SegmentIndex)
	}

	return nil
}

// Get 根据片段ID获取下载片段记录
// 参数:
//   - txn: *badger.Txn 数据库事务对象
//   - segmentID: 片段的唯一标识符
//
// 返回值:
//   - *pb.DownloadSegmentRecord: 找到的下载片段记录
//   - bool: 记录是否存在
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) GetTx(txn *badger.Txn, segmentID string) (*pb.DownloadSegmentRecord, bool, error) {
	var record pb.DownloadSegmentRecord
	err := s.store.TxGet(txn, segmentID, &record)
	if err == badgerhold.ErrNotFound {
		return nil, false, nil // 记录不存在，返回 (nil, false, nil)
	}

	if err != nil {
		logger.Errorf("获取下载片段记录失败: %v", err)
		return nil, false, err // 系统错误，返回 (nil, false, err)
	}

	return &record, false, nil
}

// DeleteTx 在事务中删除指定片段ID的下载片段记录
// 参数:
//   - txn: *badger.Txn 数据库事务对象
//   - segmentID: string 片段的唯一标识符
//
// 返回值:
//   - error: 如果删除过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) DeleteTx(txn *badger.Txn, segmentID string) error {
	return s.store.TxDelete(txn, segmentID, &pb.DownloadSegmentRecord{})
}

// FindByTaskIDTx 在事务中根据任务ID查找相关的片段记录（不包含片段内容）
// 参数:
//   - txn: *badger.Txn 数据库事务对象
//   - taskID: string 任务的唯一标识符
//
// 返回值:
//   - []*pb.DownloadSegmentRecord: 符合条件的下载片段记录列表（不含片段内容）
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) FindByTaskIDTx(txn *badger.Txn, taskID string, includeContent ...bool) ([]*pb.DownloadSegmentRecord, error) {
	// 先获取预计的记录数量
	count, err := s.store.TxCount(txn, &pb.DownloadSegmentRecord{}, badgerhold.Where("TaskId").Eq(taskID))
	if err != nil {
		logger.Errorf("计算记录数量失败: %v", err)
		return nil, err
	}

	// 预分配切片容量
	records := make([]*pb.DownloadSegmentRecord, 0, count)

	// 检查是否需要包含内容
	needContent := len(includeContent) > 0 && includeContent[0]

	err = s.store.TxForEach(txn, badgerhold.Where("TaskId").Eq(taskID), func(record *pb.DownloadSegmentRecord) error {
		lightRecord := &pb.DownloadSegmentRecord{
			SegmentId:     record.SegmentId,
			SegmentIndex:  record.SegmentIndex,
			TaskId:        record.TaskId,
			Size_:         record.Size_,
			Crc32Checksum: record.Crc32Checksum,
			IsRsCodes:     record.IsRsCodes,
			Status:        record.Status,
		}

		// 如果需要包含内容，则复制内容
		if needContent {
			lightRecord.SegmentContent = record.SegmentContent
		}

		records = append(records, lightRecord)
		return nil
	})

	if err != nil {
		logger.Errorf("查找下载片段记录失败: %v", err)
		return nil, err
	}

	return records, nil
}

// func (s *DownloadSegmentStore) FindByTaskIDTx(txn *badger.Txn, taskID string) ([]*pb.DownloadSegmentRecord, error) {
// 	// 查询原始记录
// 	var fullRecords []*pb.DownloadSegmentRecord
// 	err := s.store.TxFind(txn, &fullRecords, badgerhold.Where("TaskId").Eq(taskID))
// 	if err != nil {
// 		logger.Errorf("查找下载片段记录失败: %v", err)
// 		return nil, err
// 	}

// 	// 转换为不包含内容的记录
// 	records := make([]*pb.DownloadSegmentRecord, len(fullRecords))
// 	for i, fr := range fullRecords {
// 		records[i] = &pb.DownloadSegmentRecord{
// 			SegmentId:     fr.SegmentId,
// 			SegmentIndex:  fr.SegmentIndex,
// 			TaskId:        fr.TaskId,
// 			Size_:         fr.Size_,
// 			Crc32Checksum: fr.Crc32Checksum,
// 			IsRsCodes:     fr.IsRsCodes,
// 			Status:        fr.Status,
// 			// 不复制 SegmentContent 字段，以减少内存使用
// 		}
// 	}

// 	return records, nil
// }
