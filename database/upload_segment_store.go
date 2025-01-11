package database

import (
	"fmt"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/pb"
)

// UploadSegmentStore 处理上传片段记录的数据库操作
type UploadSegmentStore struct {
	db *badgerhold.Store // 数据库连接实例
}

// NewUploadSegmentStore 创建一个新的 UploadSegmentStore 实例
// 参数:
//   - db: *badgerhold.Store 数据库连接实例
//
// 返回值:
//   - *UploadSegmentStore: 新创建的 UploadSegmentStore 实例
func NewUploadSegmentStore(db *badgerhold.Store) *UploadSegmentStore {
	// 返回一个新的 UploadSegmentStore 实例，其中包含传入的数据库实例
	return &UploadSegmentStore{db: db}
}

// CreateUploadSegment 创建一个新的上传片段记录
// 参数:
//   - segment: *pb.UploadSegmentRecord 要创建的上传片段记录
//
// 返回值:
//   - error: 如果创建成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) CreateUploadSegment(segment *pb.UploadSegmentRecord) error {
	// 验证片段记录的完整性
	if err := s.ValidateSegment(segment); err != nil {
		logger.Errorf("片段记录验证失败: %v", err) // 记录验证失败的错误日志
		return err
	}

	// 将片段记录插入或更新到数据库
	err := s.db.Upsert(segment.SegmentId, segment)
	if err != nil {
		logger.Errorf("创建上传片段记录失败: %v", err) // 记录创建失败的错误日志
		return err
	}

	logger.Infof("成功创建上传片段记录: %s", segment.SegmentId) // 记录创建成功的日志
	return nil
}

// GetUploadSegment 根据片段ID获取上传片段记录
// 参数:
//   - segmentID: string 片段的唯一标识符
//
// 返回值:
//   - *pb.UploadSegmentRecord: 获取到的上传片段记录
//   - error: 如果获取成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) GetUploadSegment(segmentID string) (*pb.UploadSegmentRecord, error) {
	var segment pb.UploadSegmentRecord // 定义一个UploadSegmentRecord变量用于存储查询结果

	// 从数据库中获取指定segmentID的片段记录
	err := s.db.Get(segmentID, &segment)
	if err != nil {
		logger.Errorf("获取上传片段记录失败: %v", err) // 记录获取失败的错误日志
		return nil, err
	}

	return &segment, nil
}

// UpdateUploadSegment 更新上传片段记录
// 参数:
//   - segment: *pb.UploadSegmentRecord 要更新的上传片段记录
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) UpdateUploadSegment(segment *pb.UploadSegmentRecord) error {
	// 更新数据库中的片段记录
	err := s.db.Update(segment.SegmentId, segment)
	if err != nil {
		logger.Errorf("更新上传片段记录失败: %v", err) // 记录更新失败的错误日志
		return err
	}

	logger.Infof("成功更新上传片段记录: %s", segment.SegmentId) // 记录更新成功的日志
	return nil
}

// UpdateSegmentStatus 更新片段状态
// 参数:
//   - segmentID: string 片段ID
//   - status: pb.SegmentUploadStatus 新的状态
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) UpdateSegmentStatus(segmentID string, status pb.SegmentUploadStatus) error {
	// 获取片段记录
	segment, err := s.GetUploadSegment(segmentID)
	if err != nil {
		logger.Errorf("获取片段记录失败: segmentID=%s, err=%v", segmentID, err)
		return err
	}

	// 更新状态
	segment.Status = status

	// 保存更新
	if err := s.UpdateUploadSegment(segment); err != nil {
		logger.Errorf("更新片段状态失败: segmentID=%s, status=%s, err=%v",
			segmentID, status.String(), err)
		return err
	}

	logger.Infof("成功更新片段状态: segmentID=%s, status=%s",
		segmentID, status.String())
	return nil
}

// DeleteUploadSegment 删除上传片段记录
// 参数:
//   - segmentID: string 要删除的片段ID
//
// 返回值:
//   - error: 如果删除成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) DeleteUploadSegment(segmentID string) error {
	// 从数据库中删除指定segmentID的片段记录
	err := s.db.Delete(segmentID, &pb.UploadSegmentRecord{})
	if err != nil {
		logger.Errorf("删除上传片段记录失败: %v", err) // 记录删除失败的错误日志
		return err
	}

	logger.Infof("成功删除上传片段记录: %s", segmentID) // 记录删除成功的日志
	return nil
}

// ListUploadSegments 列出所有上传片段记录
// 返回值:
//   - []*pb.UploadSegmentRecord: 所有上传片段记录的切片
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) ListUploadSegments() ([]*pb.UploadSegmentRecord, error) {
	var segments []*pb.UploadSegmentRecord // 定义切片用于存储所有片段记录

	// 查询所有片段记录
	err := s.db.Find(&segments, nil)
	if err != nil {
		logger.Errorf("列出所有上传片段记录失败: %v", err) // 记录查询失败的错误日志
		return nil, err
	}

	return segments, nil
}

// GetUploadSegmentsByTaskID 根据任务ID获取上传片段记录
// 参数:
//   - taskID: string 要查询的任务ID
//
// 返回值:
//   - []*pb.UploadSegmentRecord: 符合任务ID条件的片段记录切片
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) GetUploadSegmentsByTaskID(taskID string) ([]*pb.UploadSegmentRecord, error) {
	var segments []*pb.UploadSegmentRecord // 定义切片用于存储查询结果

	// 查询指定任务ID的片段记录，并按片段索引排序
	err := s.db.Find(&segments,
		badgerhold.
			Where("TaskId").Eq(taskID).
			SortBy("SegmentIndex").
			Index("TaskId"))
	if err != nil {
		logger.Errorf("根据任务ID获取上传片段记录失败: %v", err) // 记录查询失败的错误日志
		return nil, err
	}

	return segments, nil
}

// GetUploadSegmentBySegmentID 根据片段ID获取上传片段记录
// 参数:
//   - segmentID: string 片段的唯一标识符
//
// 返回值:
//   - *pb.UploadSegmentRecord: 获取到的上传片段记录
//   - bool: 记录是否存在
//   - error: 如果发生系统错误返回错误信息，记录不存在则返回nil
func (s *UploadSegmentStore) GetUploadSegmentBySegmentID(segmentID string) (*pb.UploadSegmentRecord, bool, error) {
	var segment pb.UploadSegmentRecord

	// 从数据库中获取指定segmentID的片段记录
	err := s.db.Get(segmentID, &segment)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, false, nil
		}
		logger.Errorf("根据片段ID获取上传片段记录失败: %v", err)
		return nil, false, err
	}

	return &segment, true, nil
}

// GetUploadSegmentByTaskIDAndIndex 根据任务ID和片段索引获取上传片段记录
// 参数:
//   - taskID: string 任务的唯一标识符
//   - index: int64 片段索引
//
// 返回值:
//   - *pb.UploadSegmentRecord: 获取到的上传片段记录
//   - bool: 记录是否存在
//   - error: 如果发生系统错误返回错误信息，记录不存在则返回nil
func (s *UploadSegmentStore) GetUploadSegmentByTaskIDAndIndex(taskID string, index int64) (*pb.UploadSegmentRecord, bool, error) {
	var segments []*pb.UploadSegmentRecord // 定义切片用于存���查询结果

	// 查询指定任务ID和片段索引的记录
	if err := s.db.Find(&segments, badgerhold.
		Where("TaskId").Eq(taskID).
		And("SegmentIndex").Eq(index).
		Index("TaskId").
		Index("SegmentIndex")); err != nil {
		logger.Errorf("根据任务ID和片段索引获取上传片段记录失败: %v", err) // 记录查询失败的错误日志
		return nil, false, err
	}

	// 检查是否找到记录
	if len(segments) == 0 {
		return nil, false, nil
	}

	// 检查是否存在多条记录
	if len(segments) > 1 {
		logger.Warnf("发现任务 %s 索引 %d 存在多个片段记录", taskID, index) // 记录警告日志
	}

	return segments[0], true, nil
}

// GetUploadSegmentByChecksum 根据校验和和任务ID获取上传片段记录
// 参数:
//   - taskID: string 要查询的任务ID
//   - checksum: uint32 要查询的CRC32校验和
//
// 返回值:
//   - *pb.UploadSegmentRecord: 获取到的上传片段记录
//   - bool: 记录是否存在
//   - error: 如果发生系统错误返回错误信息，记录不存在则返回nil
func (s *UploadSegmentStore) GetUploadSegmentByChecksum(taskID string, checksum uint32) (*pb.UploadSegmentRecord, bool, error) {
	var segments []*pb.UploadSegmentRecord

	// 查询指定校验和和任务ID的片段记录
	if err := s.db.Find(&segments, badgerhold.
		Where("TaskId").Eq(taskID).
		And("Crc32Checksum").Eq(checksum).
		Index("TaskId").
		Index("Crc32Checksum")); err != nil {
		logger.Errorf("根据校验和和任务ID获取上传片段记录失败: %v", err)
		return nil, false, err
	}

	// 检查是否找到记录
	if len(segments) == 0 {
		return nil, false, nil
	}

	// 检查是否存在多条记录
	if len(segments) > 1 {
		logger.Warnf("发现任务 %s 校验和 %d 存在多个片段记录", taskID, checksum)
	}

	return segments[0], true, nil
}

// GetUploadSegmentsByStatus 根据状态和任务ID获取上传片段记录
// 参数:
//   - taskID: string 要查询的任务ID
//   - status: pb.SegmentUploadStatus 要查询的上传状态
//
// 返回值:
//   - []*pb.UploadSegmentRecord: 符合状态和任务ID条件的片段记录切片
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) GetUploadSegmentsByStatus(taskID string, status pb.SegmentUploadStatus) ([]*pb.UploadSegmentRecord, error) {
	var segments []*pb.UploadSegmentRecord

	// 查询指定状态和任务ID的片段记录
	if err := s.db.Find(&segments,
		badgerhold.
			Where("TaskId").Eq(taskID).
			And("Status").Eq(status).
			Index("TaskId").
			Index("Status")); err != nil {
		logger.Errorf("根据状态和任务ID获取上传片段记录失败: %v", err)
		return nil, err
	}

	return segments, nil
}

// BatchCreateUploadSegments 批量创建上传片段记录
// 参数:
//   - segments: []*pb.UploadSegmentRecord 要批量创建的片段记录切片
//
// 返回值:
//   - error: 如果批量创建成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) BatchCreateUploadSegments(segments []*pb.UploadSegmentRecord) error {
	// 开启事务
	tx := s.db.Badger().NewTransaction(true)
	defer tx.Discard() // 确保事务最终会被丢弃

	// 遍历所有片段记录进行创建
	for _, segment := range segments {
		err := s.db.TxUpsert(tx, segment.SegmentId, segment)
		if err != nil {
			logger.Errorf("批量创建上传片段记录失败: %v", err) // 记录创建失败的错误日志
			return err
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		logger.Errorf("提交事务失败: %v", err) // 记录提交失败的错误日志
		return err
	}

	logger.Infof("成功批量创建 %d 条上传片段记录", len(segments)) // 记录创建成功的日志
	return nil
}

// BatchDeleteUploadSegments 批量删除上传片段记录
// 参数:
//   - segmentIDs: []string 要批量删除的片段ID切片
//
// 返回值:
//   - error: 如果批量删除成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) BatchDeleteUploadSegments(segmentIDs []string) error {
	// 遍历所有片段ID进行删除
	for _, segmentID := range segmentIDs {
		err := s.DeleteUploadSegment(segmentID) // 删除单个片段记录
		if err != nil {
			logger.Errorf("批量删除上传片段记录失败: %v", err) // 记录删除失败的错误日志
			return err
		}
	}

	logger.Infof("成功批量删除 %d 条上传片段记录", len(segmentIDs)) // 记录删除成功的日志
	return nil
}

// ClearAllUploadSegments 清空所有上传片段记录
// 返回值:
//   - error: 如果清空成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) ClearAllUploadSegments() error {
	// 删除所有片段记录
	err := s.db.DeleteMatching(&pb.UploadSegmentRecord{}, nil)
	if err != nil {
		logger.Errorf("清空所有上传片段记录失败: %v", err) // 记录清空失败的错误日志
		return err
	}

	logger.Info("所有上传片段记录已成功清空") // 记录清空成功的日志
	return nil
}

// ValidateSegment 验证片段记录的完整性
// 参数:
//   - segment: *pb.UploadSegmentRecord 要验证的片段记录
//
// 返回值:
//   - error: 如果验证通过返回nil，否则返回错误��息
func (s *UploadSegmentStore) ValidateSegment(segment *pb.UploadSegmentRecord) error {
	// 检查片段记录是否为空
	if segment == nil {
		return fmt.Errorf("片段记录为空")
	}

	// 检查片段ID是否为空
	if segment.SegmentId == "" {
		return fmt.Errorf("片段ID为空")
	}

	// 检查任务ID是否为空
	if segment.TaskId == "" {
		return fmt.Errorf("任务ID为空")
	}

	// 检查片段索引是否有效
	if segment.SegmentIndex < 0 {
		return fmt.Errorf("片段索引无效")
	}

	// 检查片段内容是否为空
	if len(segment.SegmentContent) == 0 {
		return fmt.Errorf("片段内容为空")
	}

	return nil
}

// DeleteUploadSegmentByTaskID 删除上传切片文件记录
// 参数:
//   - taskID: string 要删除的任务ID
//
// 返回值:
//   - error: 如果删除成功返回nil，否则返回错误信息
func (s *UploadSegmentStore) DeleteUploadSegmentByTaskID(taskID string) error {
	// 从数据库中删除指定taskID的文件记录
	if err := s.db.DeleteMatching(&pb.UploadSegmentRecord{}, badgerhold.
		Where("TaskId").Eq(taskID).
		Index("TaskId")); err != nil {
		logger.Errorf("删除上传文件记录失败: %v", err) // 记录错误日志
		return err
	}
	logger.Infof("成功删除上传文件记录: %s", taskID) // 记录成功日志
	return nil
}

// GetUploadProgress 获取上传进度信息
// 参数:
//   - taskID: string 任务的唯一标识符
//
// 返回值:
//   - totalSegments: int 总片段数
//   - completedSegments: int 已完成片段数
//   - error: 如果查询失败返回错误信息
func (s *UploadSegmentStore) GetUploadProgress(taskID string) (totalSegments, completedSegments int, err error) {
	// 查询指定任务的所有片段记录
	segments, err := s.GetUploadSegmentsByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取上传片段记录失败: taskID=%s, err=%v", taskID, err)
		return 0, 0, err
	}

	// 统计总数和已完成数
	totalSegments = len(segments)
	for _, segment := range segments {
		if segment.Status == pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_COMPLETED {
			completedSegments++
		}
	}

	return totalSegments, completedSegments, nil
}
