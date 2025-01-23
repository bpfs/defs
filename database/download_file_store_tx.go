package database

import (
	"bytes"
	"fmt"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/pb"

	"github.com/dgraph-io/badger/v4"
)

// InsertTx 在事务中插入一条新的下载文件记录
// 参数:
//   - txn: Badger 事务对象
//   - record: 要插入的下载文件记录
//
// 返回值:
//   - error: 如果插入过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) InsertTx(txn *badger.Txn, record *pb.DownloadFileRecord) error {
	// 首先尝试插入记录到数据库
	err := s.store.TxInsert(txn, record.TaskId, record)
	if err != nil {
		// 如果错误是因为记录已存在
		if err == badgerhold.ErrKeyExists {
			// 尝试更新已存在的记录
			updateErr := s.store.TxUpdate(txn, record.TaskId, record)
			if updateErr != nil {
				// 如果更新也失败,记录错误日志并返回错误信息
				logger.Errorf("更新交易记录 %v 时出错: %v", record.TaskId, updateErr)
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
//   - txn: Badger 事务对象
//   - record: 要更新的下载文件记录
//
// 返回值:
//   - error: 如果更新过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) UpdateTx(txn *badger.Txn, record *pb.DownloadFileRecord) error {
	if record == nil || record.TaskId == "" {
		return fmt.Errorf("无效的更新记录")
	}

	// 获取现有记录
	var existingRecord pb.DownloadFileRecord
	err := s.store.TxGet(txn, record.TaskId, &existingRecord)
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
		if err := s.store.TxUpdate(txn, record.TaskId, &existingRecord); err != nil {
			return fmt.Errorf("更新记录失败: %v", err)
		}
		logger.Debugf("更新下载文件记录: TaskID=%s, Status=%s", record.TaskId, existingRecord.Status)
	}

	return nil
}

// Get 根据任务ID获取下载文件记录
// 参数:
//   - txn: Badger 事务对象
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - *pb.DownloadFileRecord: 找到的下载文件记录
//   - bool: 记录是否存在
//   - error: 如果查找过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) GetTx(txn *badger.Txn, taskID string) (*pb.DownloadFileRecord, bool, error) {
	var record pb.DownloadFileRecord
	err := s.store.TxGet(txn, taskID, &record)
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
//   - txn: Badger 事务对象
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - error: 如果删除过程中出现错误，返回相应的错误信息
func (s *DownloadFileStore) DeleteTx(txn *badger.Txn, taskID string) error {
	return s.store.TxDelete(txn, taskID, &pb.DownloadFileRecord{})
}

// QueryDownloadTaskRecordsTx 在事务中执行下载任务记录的查询
// 参数:
//   - txn: *badger.Txn 数据库事务对象
//   - start: int 查询的起始位置
//   - limit: int 返回的最大记录数
//   - options: ...QueryOption 查询选项(如状态过滤、时间范围等)
//
// 返回值:
//   - []*pb.DownloadFileRecord: 下载任务记录列表
//   - uint64: 符合查询条件的总记录数
//   - error: 如果查询过程中发生错误，返回相应错误
func (s *DownloadFileStore) QueryDownloadTaskRecordsTx(txn *badger.Txn, start, limit int, options ...QueryOption) ([]*pb.DownloadFileRecord, uint64, error) {
	// 创建基础查询对象
	query := &badgerhold.Query{}

	// 应用所有查询选项
	for _, option := range options {
		query = option(query)
	}

	// 执行分页查询
	var tasks []*pb.DownloadFileRecord
	err := s.store.TxFind(txn, &tasks, query.Skip(start).Limit(limit))
	if err != nil {
		logger.Errorf("查询下载任务记录失败: %v", err)
		return nil, 0, err
	}

	// 获取总记录数
	totalCount, err := s.store.TxCount(txn, &pb.DownloadFileRecord{}, query)
	if err != nil {
		logger.Errorf("查询下载任务总数失败: %v", err)
		return nil, 0, err
	}

	return tasks, uint64(totalCount), nil
}
