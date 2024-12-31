package database

import (
	"fmt"

	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
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

	// 打印更新前的记录状态
	// logger.Infof("更新前的记录状态: ID=%s", record.SegmentId)
	// logger.Infof("- Status: %s", existingRecord.Status)
	// logger.Infof("- SegmentIndex: %d", existingRecord.SegmentIndex)
	// logger.Infof("- IsRsCodes: %v", existingRecord.IsRsCodes)
	// logger.Infof("- ContentLength: %d", len(existingRecord.SegmentContent))
	// logger.Infof("- TaskId: %s", existingRecord.TaskId)

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

	// 内容更新
	if record.SegmentContent != nil {
		// 打印更新前的内容长度
		// logger.Infof("内容更新检查: ID=%s", record.SegmentId)
		// logger.Infof("- 现有内容长度: %d", len(existingRecord.SegmentContent))
		// logger.Infof("- 新内容长度: %d", len(record.SegmentContent))

		// 如果传入有内容，直接更新
		existingRecord.SegmentContent = record.SegmentContent
		needsUpdate = true

		// 更新校验和
		if record.Crc32Checksum > 0 {
			existingRecord.Crc32Checksum = record.Crc32Checksum
		}

		// logger.Infof("内容已更新: ID=%s, 新内容长度=%d, 校验和=%d",
		// 	record.SegmentId,
		// 	len(existingRecord.SegmentContent),
		// 	existingRecord.Crc32Checksum)
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
//   - includeContent: 可选参数，是否包含片段内容，默认为 false
//
// 返回值:
//   - []*pb.DownloadSegmentRecord: 符合条件的下载片段记录列表
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadSegmentStore) FindByTaskID(taskID string, includeContent ...bool) ([]*pb.DownloadSegmentRecord, error) {
	// 先获取预计的记录数量
	count, err := s.store.Count(&pb.DownloadSegmentRecord{}, badgerhold.Where("TaskId").Eq(taskID))
	if err != nil {
		logger.Errorf("计算记录数量失败: %v", err)
		return nil, err
	}

	// logger.Printf("找到任务 %s 的记录数量: %d", taskID, count)

	// 预分配切片容量
	records := make([]*pb.DownloadSegmentRecord, 0, count)

	// 检查是否需要包含内容
	needContent := len(includeContent) > 0 && includeContent[0]

	err = s.store.ForEach(badgerhold.Where("TaskId").Eq(taskID), func(record *pb.DownloadSegmentRecord) error {
		// logger.Printf("处理记录: TaskId=%s, SegmentIndex=%d, SegmentId=%s",
		// 	record.TaskId, record.SegmentIndex, record.SegmentId)

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
func (s *DownloadSegmentStore) FindByTaskIDAndStatus(taskID string, status pb.SegmentDownloadStatus, includeContent ...bool) ([]*pb.DownloadSegmentRecord, error) {
	// 先获取预计的记录数量
	count, err := s.store.Count(&pb.DownloadSegmentRecord{},
		badgerhold.Where("TaskId").Eq(taskID).And("Status").Eq(status))
	if err != nil {
		logger.Errorf("计算记录数量失败: %v", err)
		return nil, err
	}

	// 预分配切片容量
	records := make([]*pb.DownloadSegmentRecord, 0, count)

	// 检查是否需要包含内容
	needContent := len(includeContent) > 0 && includeContent[0]

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

		// 检查片段是否已完成下载（状态为完成且内容不为空）
		if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED &&
			len(segment.SegmentContent) > 0 {
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
	return record.SegmentContent, nil
}
