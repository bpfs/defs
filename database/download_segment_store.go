package database

import (
	"bytes"
	"fmt"
	"os"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/pb"
)

// DownloadSegmentStore 管理下载片段记录的存储
type DownloadSegmentStore struct {
	store *badgerhold.Store
}

// NewDownloadSegmentStore 创建一个新的下载片段存储管理器
func NewDownloadSegmentStore(store *badgerhold.Store) *DownloadSegmentStore {
	return &DownloadSegmentStore{
		store: store,
	}
}

// Insert 插入一条新的下载片段记录
// 参数:
//   - record: 要插入的下载片段记录
//
// 返回值:
//   - error: 如果插入过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) Insert(record *pb.DownloadSegmentRecord) error {
	err := s.store.Insert(record.SegmentId, record)
	if err != nil {
		// 如果错误是因为记录已存在
		if err == badgerhold.ErrKeyExists {
			// 尝试更新已存在的记录
			updateErr := s.store.Update(record.SegmentId, record)
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

// Update 更新下载片段记录
// 只更新与现有记录不同的字段
// 参数:
//   - record: *pb.DownloadSegmentRecord 要更新的记录
//   - validateContent: bool 是否校验内容哈希（可选）
//
// 返回值:
//   - error: 如果更新过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) Update(record *pb.DownloadSegmentRecord, validateContent ...bool) error {
	if record == nil || record.SegmentId == "" {
		return fmt.Errorf("无效的更新记录")
	}

	// 获取现有记录
	var existingRecord pb.DownloadSegmentRecord
	err := s.store.Get(record.SegmentId, &existingRecord)
	if err != nil {
		logger.Errorf("获取现有记录失败: %v", err)
		return err
	}

	// 检查字段是否需要更新
	needsUpdate := false

	// TaskId变更（通常不应该变）
	if record.TaskId != "" && record.TaskId != existingRecord.TaskId {
		existingRecord.TaskId = record.TaskId
		needsUpdate = true
		// logger.Warnf("片段任务ID发生变更: %s -> %s", existingRecord.TaskId, record.TaskId)
	}

	// SegmentIndex变更
	if record.SegmentIndex != existingRecord.SegmentIndex {
		existingRecord.SegmentIndex = record.SegmentIndex
		needsUpdate = true
	}

	// Status变更
	if record.Status != pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_UNSPECIFIED &&
		record.Status != existingRecord.Status {
		existingRecord.Status = record.Status
		needsUpdate = true
	}

	// IsRsCodes变更
	if record.IsRsCodes != existingRecord.IsRsCodes {
		existingRecord.IsRsCodes = record.IsRsCodes
		needsUpdate = true
	}

	// Size_变更
	if record.Size_ > 0 && record.Size_ != existingRecord.Size_ {
		existingRecord.Size_ = record.Size_
		needsUpdate = true
	}

	// Crc32Checksum变更
	if record.Crc32Checksum > 0 && record.Crc32Checksum != existingRecord.Crc32Checksum {
		existingRecord.Crc32Checksum = record.Crc32Checksum
		needsUpdate = true
	}

	// StoragePath变更
	if record.StoragePath != "" && record.StoragePath != existingRecord.StoragePath {
		existingRecord.StoragePath = record.StoragePath
		needsUpdate = true
	}

	// EncryptionKey变更
	if record.EncryptionKey != nil && !bytes.Equal(record.EncryptionKey, existingRecord.EncryptionKey) {
		existingRecord.EncryptionKey = record.EncryptionKey
		needsUpdate = true
	}

	// 只有在字段发生变化时才执行更新
	if needsUpdate {
		if err := s.store.Update(record.SegmentId, &existingRecord); err != nil {
			logger.Errorf("更新记录失败: %v", err)
			return err
		}
		// logger.Infof("记录已更新: ID=%s", record.SegmentId)
		// logger.Infof("更新后的最终状态:")
		// logger.Infof("- Status: %s", existingRecord.Status)
		// logger.Infof("- SegmentIndex: %d", existingRecord.SegmentIndex)
		// logger.Infof("- IsRsCodes: %v", existingRecord.IsRsCodes)
		// logger.Infof("- ContentLength: %d", len(existingRecord.SegmentContent))
		// logger.Infof("- TaskId: %s", existingRecord.TaskId)
	} else {
		logger.Infof("记录无需更新: ID=%s", record.SegmentId)
	}

	return nil
}

// Get 根据片段ID获取下载片段记录
// 参数:
//   - segmentID: 片段的唯一标识符
//
// 返回值:
//   - *pb.DownloadSegmentRecord: 找到的下载片段记录
//   - bool: 记录是否存在
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) Get(segmentID string) (*pb.DownloadSegmentRecord, bool, error) {
	var record pb.DownloadSegmentRecord
	err := s.store.Get(segmentID, &record)
	if err == badgerhold.ErrNotFound {
		return nil, false, nil // 记录不存在，返回 (nil, false, nil)
	}

	if err != nil {
		logger.Errorf("获取下载片段记录失败: %v", err)
		return nil, false, err // 系统错误，返回 (nil, false, err)
	}

	return &record, false, nil
}

// Delete 删除指定片段ID的下载片段记录
// 参数:
//   - segmentID: 片段的唯一标识符
//
// 返回值:
//   - error: 如果删除过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) Delete(segmentID string) error {
	return s.store.Delete(segmentID, &pb.DownloadSegmentRecord{})
}

// FindByTaskID 根据任务ID查找相关的片段记录
// 参数:
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - []*pb.DownloadSegmentRecord: 符合条件的下载片段记录列表
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) FindByTaskID(taskID string) ([]*pb.DownloadSegmentRecord, error) {
	// 先获取预计的记录数量
	count, err := s.store.Count(&pb.DownloadSegmentRecord{}, badgerhold.Where("TaskId").Eq(taskID))
	if err != nil {
		logger.Errorf("计算记录数量失败: %v", err)
		return nil, err
	}

	// logger.Printf("找到任务 %s 的记录数量: %d", taskID, count)

	// 预分配切片容量
	records := make([]*pb.DownloadSegmentRecord, 0, count)

	err = s.store.ForEach(badgerhold.Where("TaskId").Eq(taskID), func(record *pb.DownloadSegmentRecord) error {
		lightRecord := &pb.DownloadSegmentRecord{
			SegmentId:     record.SegmentId,
			SegmentIndex:  record.SegmentIndex,
			TaskId:        record.TaskId,
			Size_:         record.Size_,
			Crc32Checksum: record.Crc32Checksum,
			IsRsCodes:     record.IsRsCodes,
			Status:        record.Status,
			StoragePath:   record.StoragePath,
			EncryptionKey: record.EncryptionKey,
		}

		records = append(records, lightRecord)
		return nil
	})

	if err != nil {
		logger.Errorf("查找下载片段记录失败: %v", err)
		return nil, err
	}

	// logger.Printf("返回记录数量: %d", len(records))
	// for i, r := range records {
	// 	logger.Printf("记录[%d]: TaskId=%s, SegmentIndex=%d, SegmentId=%s",
	// 		i, r.TaskId, r.SegmentIndex, r.SegmentId)
	// }

	return records, nil
}

// FindByTaskIDAndStatus 根据任务ID和状态查找片段记录
// 参数:
//   - taskID: 任务的唯一标识符
//   - status: 片段下载状态
//
// 返回值:
//   - []*pb.DownloadSegmentRecord: 符合条件的下载片段记录列表
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) FindByTaskIDAndStatus(taskID string, status pb.SegmentDownloadStatus) ([]*pb.DownloadSegmentRecord, error) {
	// 先获取预计的记录数量
	count, err := s.store.Count(&pb.DownloadSegmentRecord{},
		badgerhold.Where("TaskId").Eq(taskID).And("Status").Eq(status))
	if err != nil {
		logger.Errorf("计算记录数量失败: %v", err)
		return nil, err
	}

	// 预分配切片容量
	records := make([]*pb.DownloadSegmentRecord, 0, count)

	err = s.store.ForEach(
		badgerhold.Where("TaskId").Eq(taskID).And("Status").Eq(status),
		func(record *pb.DownloadSegmentRecord) error {
			lightRecord := &pb.DownloadSegmentRecord{
				SegmentId:     record.SegmentId,
				SegmentIndex:  record.SegmentIndex,
				TaskId:        record.TaskId,
				Size_:         record.Size_,
				Crc32Checksum: record.Crc32Checksum,
				IsRsCodes:     record.IsRsCodes,
				Status:        record.Status,
				StoragePath:   record.StoragePath,
				EncryptionKey: record.EncryptionKey,
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

// TaskByFileID 根据文件ID查找相关的片段记录和下载状态
// 参数:
//   - taskId: 任务唯一标识
//
// 返回值:
//   - int: 总片段数
//   - []int64: 已下载完成的片段索引列表
//   - int: 数据片段大小（不含校验片段）
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) TaskByFileID(taskId string) (int, []int64, int, error) {
	var records []*pb.DownloadSegmentRecord
	err := s.store.Find(&records, badgerhold.Where("TaskId").Eq(taskId))
	if err != nil {
		logger.Errorf("查找下载片段记录失败: %v", err)
		return 0, nil, 0, err
	}

	if len(records) == 0 {
		return 0, nil, 0, fmt.Errorf("任务 %s 没有分片记录", taskId)
	}

	// 统计总片段数、已完成的片段和数据片段数
	totalSegments := len(records)
	completedIndices := make([]int64, 0)
	dataSegmentCount := int(0)

	for _, segment := range records {
		// 统计数据片段（非校验片段）
		if !segment.IsRsCodes {
			dataSegmentCount++
		}

		// 检查片段是否已完成下载
		if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED &&
			segment.StoragePath != "" {
			completedIndices = append(completedIndices, segment.SegmentIndex)
		}
	}

	return totalSegments, completedIndices, dataSegmentCount, nil
}

// FindSegmentContent 根据片段ID获取片段内容
// 参数:
//   - segmentID: 片段的唯一标识符
//
// 返回值:
//   - []byte: 片段内容
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) FindSegmentContent(segmentID string) ([]byte, error) {
	var record pb.DownloadSegmentRecord
	err := s.store.Get(segmentID, &record)
	if err != nil {
		logger.Errorf("获取片段内容失败: %v", err)
		return nil, err
	}
	// 从文件中读取内容
	if record.StoragePath == "" {
		return nil, fmt.Errorf("片段存储路径为空")
	}
	return os.ReadFile(record.StoragePath)
}

// GetDownloadSegmentBySegmentID 根据片段ID获取下载片段记录
// 参数:
//   - segmentID: string 片段的唯一标识符
//
// 返回值:
//   - *pb.UploadSegmentRecord: 获取到的下载片段记录
//   - bool: 记录是否存在
//   - error: 如果发生系统错误返回错误信息，记录不存在则返回nil
func (s *DownloadSegmentStore) GetDownloadSegmentBySegmentID(segmentID string) (*pb.DownloadSegmentRecord, bool, error) {
	var segment pb.DownloadSegmentRecord

	// 从数据库中获取指定segmentID的片段记录
	err := s.store.Get(segmentID, &segment)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, false, nil
		}
		logger.Errorf("根据片段ID获取下载片段记录失败: %v", err)
		return nil, false, err
	}

	return &segment, true, nil
}

// GetDownloadSegment 根据片段ID获取下载片段记录
// 参数:
//   - segmentID: string 片段的唯一标识符
//
// 返回值:
//   - *pb.DownloadSegmentRecord: 获取到的下载片段记录
//   - error: 如果获取成功返回nil，否则返回错误信息
func (s *DownloadSegmentStore) GetDownloadSegment(segmentID string) (*pb.DownloadSegmentRecord, error) {
	var segment pb.DownloadSegmentRecord // 定义一个DownloadSegmentRecord变量用于存储查询结果

	// 从数据库中获取指定segmentID的片段记录
	err := s.store.Get(segmentID, &segment)
	if err != nil {
		logger.Errorf("获取上传片段记录失败: %v", err) // 记录获取失败的错误日志
		return nil, err
	}

	return &segment, nil
}

// UpdateSegmentStatus 更新片段状态
// 参数:
//   - segmentID: string 片段ID
//   - status: pb.SegmentDownloadStatus 新的状态
//   - peerID: string 可选的节点ID，用于标记节点状态
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *DownloadSegmentStore) UpdateSegmentStatus(segmentID string, status pb.SegmentDownloadStatus, peerID ...string) error {
	// 获取片段记录
	segment, err := s.GetDownloadSegment(segmentID)
	if err != nil {
		logger.Errorf("获取片段记录失败: segmentID=%s, err=%v", segmentID, err)
		return err
	}

	// 更新状态
	segment.Status = status

	// 如果提供了节点ID且状态是失败，标记该节点不可用
	if len(peerID) > 0 && status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_FAILED {
		if segment.SegmentNode == nil {
			segment.SegmentNode = make(map[string]bool)
		}
		segment.SegmentNode[peerID[0]] = false
	}

	// 保存更新
	if err := s.UpdateDownloadSegment(segment); err != nil {
		logger.Errorf("更新片段状态失败: segmentID=%s, status=%s, err=%v",
			segmentID, status.String(), err)
		return err
	}

	return nil
}

// UpdateDownloadSegment 更新下载片段记录
// 参数:
//   - segment: *pb.DownloadSegmentRecord 要更新的下载片段记录
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *DownloadSegmentStore) UpdateDownloadSegment(segment *pb.DownloadSegmentRecord) error {
	// 更新数据库中的片段记录
	err := s.store.Update(segment.SegmentId, segment)
	if err != nil {
		logger.Errorf("更新下载片段记录失败: %v", err) // 记录更新失败的错误日志
		return err
	}

	// logger.Infof("成功更新下载片段记录: %s", segment.SegmentId) // 记录更新成功的日志
	return nil
}
