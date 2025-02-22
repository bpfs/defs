package downloads

import (
	"time"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"

	"github.com/dgraph-io/badger/v4"
)

// CreateDownloadFileRecord 创建下载文件记录并保存到数据库
// 参数:
//   - db: 数据库存储接口
//   - taskID: 下载任务的唯一标识
//   - fileID: 文件的唯一标识
//   - pubkeyHash: 所有者的公钥哈希
//   - firstKeyShare: 恢复密钥的第一个分片
//   - tempStorage: 临时存储路径
//   - fileMeta: 文件的元数据信息
//   - sliceTable: 分片哈希表
//   - status: 下载任务的初始状态
//
// 返回值:
//   - *pb.DownloadFileRecord: 创建的下载文件记录对象
//   - error: 如果创建或存储过程中发生错误则返回相应错误
//
// 功能:
//   - 创建新的下载文件记录
//   - 初始化文件和分片记录
//   - 分批处理并保存分片记录
//   - 使用事务确保数据一致性
func CreateDownloadFileRecord(
	db *badgerhold.Store,
	taskID string,
	fileID string,
	pubkeyHash []byte,
	firstKeyShare []byte,
	tempStorage string,
	fileMeta *pb.FileMeta,
	sliceTable map[int64]*pb.HashTable,
	status pb.DownloadStatus,
) (*pb.DownloadFileRecord, error) {
	// 创建文件和分片记录的存储接口实例
	fileStore := database.NewDownloadFileStore(db)
	segmentStore := database.NewDownloadSegmentStore(db)

	// 构建完整的下载文件记录对象，包含所有必要字段
	fileRecord := &pb.DownloadFileRecord{
		TaskId:        taskID,            // 任务唯一标识
		FileId:        fileID,            // 文件唯一标识
		PubkeyHash:    pubkeyHash,        // 所有者公钥哈希
		FirstKeyShare: firstKeyShare,     // 第一个密钥分片
		TempStorage:   tempStorage,       // 临时存储路径
		FileMeta:      fileMeta,          // 文件元数据
		SliceTable:    sliceTable,        // 分片哈希表
		StartedAt:     time.Now().Unix(), // 任务开始时间戳
		Status:        status,            // 初始下载状态
	}

	segments := make([]*pb.DownloadSegmentRecord, 0, len(sliceTable))

	// 预先构建所有片段记录
	for _, hashTable := range sliceTable {
		segmentRecord := &pb.DownloadSegmentRecord{
			SegmentId:     hashTable.SegmentId,
			SegmentIndex:  hashTable.SegmentIndex,
			TaskId:        taskID,
			Crc32Checksum: hashTable.Crc32Checksum,
			IsRsCodes:     hashTable.IsRsCodes,
			Status:        pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_PENDING,
		}
		segments = append(segments, segmentRecord)
	}

	// 使用事务保存文件记录和分片记录
	err := db.Badger().Update(func(txn *badger.Txn) error {
		// 1. 保存文件记录
		if err := fileStore.InsertTx(txn, fileRecord); err != nil {
			logger.Errorf("保存文件记录失败: %v", err)
			return err
		}

		// 2. 批量保存分片记录
		const batchSize = 10
		for i := 0; i < len(segments); i += batchSize {
			end := i + batchSize
			if end > len(segments) {
				end = len(segments)
			}

			// 对于每个批次，开启一个新的事务
			err := db.Badger().Update(func(txn *badger.Txn) error {
				for _, segment := range segments[i:end] {
					if err := segmentStore.InsertTx(txn, segment); err != nil {
						logger.Errorf("保存片段记录失败 [%s]: %v", segment.SegmentId, err)
						return err
					}
				}
				return nil
			})
			if err != nil {
				logger.Errorf("批次处理分片记录失败: %v", err)
				return err
			}
		}

		return nil
	})

	if err != nil {
		logger.Errorf("创建下载任务失败: %v", err)
		return nil, err
	}

	return fileRecord, nil
}

// UpdateDownloadFileStatus 更新下载文件的状态
// 参数:
//   - db: 数据库存储接口
//   - taskID: 下载任务的唯一标识
//   - status: 新的文件状态
//
// 返回值:
//   - error: 如果更新过程中发生错误则返回相应错误，否则返回 nil
//
// 功能:
//   - 获取当前文件记录
//   - 更新文件状态
//   - 保存更新后的记录
func UpdateDownloadFileStatus(db *badgerhold.Store, taskID string, status pb.DownloadStatus) error {
	store := database.NewDownloadFileStore(db)

	// 获取当前文件记录
	fileRecord, exists, err := store.Get(taskID)
	if err != nil {
		logger.Errorf("获取文件记录失败: %v", err)
		return err
	}
	if !exists {
		logger.Errorf("文件记录不存在: %s", taskID)
		return err
	}

	// 更新状态
	fileRecord.Status = status
	logger.Infof("更新文件 %s 状态为: %s", taskID, status.String())

	// 将更新后的记录保存到数据库
	if err := store.Update(fileRecord); err != nil {
		logger.Errorf("更新文件状态失败: %v", err)
		return err
	}

	return nil
}

// QueryDownloadTask 执行下载任务的基础查询
// 参数:
//   - db: *badgerhold.Store 数据库存储实例
//   - start: int 查询的起始位置
//   - pageSize: int 每页显示的记录数
//   - options: ...database.QueryOption 可选的查询条件
//
// 返回值:
//   - []*pb.DownloadFileRecord: 下载任务记录列表
//   - uint64: 符合查询条件的总记录数
//   - error: 如果查询过程中发生错误，返回相应错误
func QueryDownloadTask(db *badgerhold.Store, start, pageSize int, options ...database.QueryOption) ([]*pb.DownloadFileRecord, uint64, error) {
	// 创建文件存储实例
	fileStore := database.NewDownloadFileStore(db)

	var tasks []*pb.DownloadFileRecord
	var totalCount uint64

	// 开启事务执行查询
	err := db.Badger().View(func(txn *badger.Txn) error {
		var err error
		tasks, totalCount, err = fileStore.QueryDownloadTaskRecordsTx(txn, start, pageSize, options...)
		logger.Infof("查询下载任务成功, 总数: %d", totalCount)
		return err
	})

	if err != nil {
		logger.Errorf("查询下载任务失败: %v", err)
		return nil, 0, err
	}

	logger.Infof("成功查询下载任务, 总数: %d", totalCount)
	return tasks, totalCount, nil
}
