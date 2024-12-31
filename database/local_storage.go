package database

import (
	"github.com/bpfs/defs/badgerhold"
)

// FileSegmentStorageStore 处理 FileSegmentStorage 的数据库操作
type FileSegmentStorageStore struct {
	db *badgerhold.Store // 数据库存储实例
}

// NewFileSegmentStorageStore 创建一个新的 FileSegmentStorageStore 实例
// 参数:
//   - db: badgerhold.Store 数据库实例
//
// 返回值:
//   - *FileSegmentStorageStore: 新创建的 FileSegmentStorageStore 实例
func NewFileSegmentStorageStore(db *badgerhold.Store) *FileSegmentStorageStore {
	return &FileSegmentStorageStore{db: db}
}

// // CreateFileSegmentStorage 创建一个新的文件片段存储记录
// // 参数:
// //   - storage: *pb.FileSegmentStorage 要创建的文件片段存储记录
// //
// // 返回值:
// //   - error: 操作过程中可能发生的错误
// func (s *FileSegmentStorageStore) CreateFileSegmentStorage(storage *pb.FileSegmentStorage) error {
// 	err := s.db.Upsert(storage.SegmentId, storage)
// 	if err != nil {
// 		logger.Errorf("创建文件片段存储记录失败: %v", err)
// 	}
// 	return err
// }

// // GetFileSegmentStorage 根据片段ID获取文件片段存储记录
// // 参数:
// //   - segmentID: string 文件片段的唯一标识符
// //
// // 返回值:
// //   - *pb.FileSegmentStorage: 获取到的文件片段存储记录
// //   - error: 操作过程中可能发生的错误
// func (s *FileSegmentStorageStore) GetFileSegmentStorage(segmentID string) (*pb.FileSegmentStorage, error) {
// 	var storage pb.FileSegmentStorage
// 	err := s.db.Get(segmentID, &storage)
// 	if err != nil {
// 		logger.Errorf("获取文件片段存储记录失败: %v", err)
// 		return nil, err
// 	}
// 	return &storage, nil
// }

// // UpdateFileSegmentStorage 更新文件片段存储记录
// // 参数:
// //   - storage: *pb.FileSegmentStorage 要更新的文件片段存储记录
// //
// // 返回值:
// //   - error: 操作过程中可能发生的错误
// func (s *FileSegmentStorageStore) UpdateFileSegmentStorage(storage *pb.FileSegmentStorage) error {
// 	err := s.db.Update(storage.SegmentId, storage)
// 	if err != nil {
// 		logger.Errorf("更新文件片段存储记录失败: %v", err)
// 	}
// 	return err
// }

// // DeleteFileSegmentStorage 删除文件片段存储记录
// // 参数:
// //   - segmentID: string 要删除的文件片段的唯一标识符
// //
// // 返回值:
// //   - error: 操作过程中可能发生的错误
// func (s *FileSegmentStorageStore) DeleteFileSegmentStorage(segmentID string) error {
// 	err := s.db.Delete(segmentID, &pb.FileSegmentStorage{})
// 	if err != nil {
// 		logger.Errorf("删除文件片段存储记录失败: %v", err)
// 	}
// 	return err
// }

// // ListFileSegmentStorages 列出所有文件片段存储记录
// // 返回值:
// //   - []*pb.FileSegmentStorage: 所有文件片段存储记录的切片
// //   - error: 操作过程中可能发生的错误
// func (s *FileSegmentStorageStore) ListFileSegmentStorages() ([]*pb.FileSegmentStorage, error) {
// 	var storages []*pb.FileSegmentStorage
// 	err := s.db.Find(&storages, nil)
// 	if err != nil {
// 		logger.Errorf("列出所有文件片段存储记录失败: %v", err)
// 	}
// 	return storages, err
// }

// // GetFileSegmentStoragesByFileID 根据文件ID和公钥哈希获取所有相关的文件片段存储记录
// func (s *FileSegmentStorageStore) GetFileSegmentStoragesByFileID(fileID string, pubkeyHash []byte) ([]*pb.FileSegmentStorage, error) {
// 	// 构建P2PKH脚本
// 	p2pkhScript, err := script.NewScriptBuilder().
// 		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).          // 复制栈顶元素并计算哈希
// 		AddData(pubkeyHash).                                    // 添加公钥哈希
// 		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG). // 验证相等并检查签名
// 		Script()
// 	if err != nil {
// 		logger.Errorf("构建P2PKH脚本失败: %v", err)
// 		return nil, err
// 	}

// 	// 使用文件ID和P2PKH脚本作为查询条件
// 	var storages []*pb.FileSegmentStorage
// 	err = s.db.Find(&storages, badgerhold.Where("FileId").Eq(fileID).And("P2PkhScript").Eq(p2pkhScript))
// 	if err != nil {
// 		logger.Errorf("根据文件ID和P2PKH脚本获取文件片段存储记录失败: %v", err)
// 		return nil, err
// 	}

// 	return storages, nil
// }

// // GetFileSegmentStorageByFileIDAndSegment 获取单个文件片段存储记录，根据文件ID、公钥哈希、片段ID和片段索引
// func (s *FileSegmentStorageStore) GetFileSegmentStorageByFileIDAndSegment(fileID string, pubkeyHash []byte, segmentID string, segmentIndex int64) (*pb.FileSegmentStorage, bool, error) {
// 	// 构建P2PKH脚本
// 	p2pkhScript, err := script.NewScriptBuilder().
// 		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).
// 		AddData(pubkeyHash).
// 		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG).
// 		Script()
// 	if err != nil {
// 		logger.Errorf("构建P2PKH脚本失败: %v", err)
// 		return nil, false, err
// 	}

// 	// 使用文件ID、P2PKH脚本、片段ID和片段索引作为查询条件
// 	var storage pb.FileSegmentStorage
// 	err = s.db.FindOne(&storage, badgerhold.Where("FileId").Eq(fileID).And("P2PkhScript").Eq(p2pkhScript).And("SegmentId").Eq(segmentID).And("SegmentIndex").Eq(segmentIndex))

// 	if err == badgerhold.ErrNotFound {
// 		return nil, false, nil // 记录不存在，返回 (nil, false, nil)
// 	}

// 	if err != nil {
// 		logger.Errorf("根据文件ID、P2PKH脚本、片段ID和片段索引获取文件片段存储记录失败: %v", err)
// 		return nil, false, err
// 	}

// 	return &storage, true, nil
// }

// // DeleteFileSegmentStoragesByFileID 根据文件ID和公钥哈希删除所有相关的文件片段存储记录
// func (s *FileSegmentStorageStore) DeleteFileSegmentStoragesByFileID(fileID string, pubkeyHash []byte) error {
// 	// 构建P2PKH脚本
// 	// p2pkhScript, err := script.NewScriptBuilder().
// 	// 	AddOp(script.OP_DUP).AddOp(script.OP_HASH160).          // 复制栈顶元素并计算哈希
// 	// 	AddData(pubkeyHash).                                    // 添加公钥哈希
// 	// 	AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG). // 验证相等并检查签名
// 	// 	Script()
// 	// if err != nil {
// 	// 	logger.Errorf("构建P2PKH脚本失败: %v", err)
// 	// 	return err
// 	// }

// 	// 使用文件ID和P2PKH脚本作为查询条件进行删除
// 	err := s.db.DeleteMatching(&pb.FileSegmentStorage{}, badgerhold.Where("FileId").Eq(fileID))
// 	if err != nil {
// 		logger.Errorf("根据文件ID和P2PKH脚本删除文件片段存储记录失败: %v", err)
// 		return err
// 	}

// 	return nil
// }
