// Package database 提供数据库操作相关功能
package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/utils/logger"
)

var (
	// fileSegmentStorageTableName 定义文件片段存储表名
	fileSegmentStorageTableName = "file_segment_storage"
)

// FileSegmentStorageSqlStore 文件片段存储的SQLite实现
type FileSegmentStorageSqlStore struct {
	sqliteDB *sql.DB // SQLite数据库连接实例
}

// NewFileSegmentStorageSqlStore 创建新的文件片段存储实例
// 参数:
//   - sqlite: SQLite数据库连接实例
//
// 返回值:
//   - *FileSegmentStorageSqlStore: 新创建的文件片段存储实例
func NewFileSegmentStorageSqlStore(sqlite *sql.DB) *FileSegmentStorageSqlStore {
	return &FileSegmentStorageSqlStore{
		sqliteDB: sqlite,
	}
}

// CreateFileSegmentStorageTable 创建文件片段存储表
// 参数:
//   - db: SQLite数据库连接实例
//
// 返回值:
//   - error: 如果创建成功返回nil,否则返回错误信息
func CreateFileSegmentStorageTable(db *sql.DB) error {
	// 定义表字段
	table := []string{
		"id_ INTEGER PRIMARY KEY AUTOINCREMENT", // 自增长主键
		"file_id TEXT",                          // 文件唯一标识
		"name TEXT",                             // 文件原始名称
		"extension TEXT",                        // 文件扩展名
		"size INTEGER",                          // 文件总大小
		"content_type TEXT",                     // MIME类型
		"sha256_hash BLOB",                      // 文件内容的SHA256哈希值
		"upload_time INTEGER",                   // 文件首次上传的Unix时间戳
		"p2pkh_script BLOB",                     // P2PKH脚本
		"p2pk_script BLOB",                      // P2PK脚本
		"slice_table BLOB",                      // 文件分片哈希表
		"segment_id TEXT",                       // 当前片段的唯一标识
		"segment_index INTEGER",                 // 当前片段在文件中的索引位置
		"crc32_checksum INTEGER",                // 当前片段的CRC32校验和
		"segment_content BLOB",                  // 当前片段的加密后内容
		"encryption_key BLOB",                   // 用于解密segment_content的AES密钥
		"signature BLOB",                        // 文件所有者对片段内容的数字签名
		"shared INTEGER",                        // 是否允许其他节点访问该片段
		"version TEXT",                          // 片段的版本号
	}

	// 创建表
	if err := CreateTable(db, fileSegmentStorageTableName, table); err != nil {
		logger.Errorf("创建表时失败: %v", err)
		return err
	}

	return nil
}

// CreateFileSegmentStorage 创建新的文件片段存储记录
// 参数:
//   - fileSegmentStorage: 要存储的文件片段信息
//
// 返回值:
//   - error: 如果创建成功返回nil,否则返回错误信息
func (rss *FileSegmentStorageSqlStore) CreateFileSegmentStorage(fileSegmentStorage *pb.FileSegmentStorageSql) error {
	// 确保size字段有效
	size := fileSegmentStorage.Size_
	if size == 0 {
		size = fileSegmentStorage.Size_
	}

	// 构建数据映射
	data := map[string]interface{}{
		"file_id":         fileSegmentStorage.FileId,         // 文件唯一标识
		"name":            fileSegmentStorage.Name,           // 文件原始名称
		"extension":       fileSegmentStorage.Extension,      // 文件扩展名
		"size":            size,                              // 文件总大小
		"content_type":    fileSegmentStorage.ContentType,    // MIME类型
		"sha256_hash":     fileSegmentStorage.Sha256Hash,     // 文件内容的SHA256哈希值
		"upload_time":     fileSegmentStorage.UploadTime,     // 文件首次上传的Unix时间戳
		"p2pkh_script":    fileSegmentStorage.P2PkhScript,    // P2PKH脚本
		"p2pk_script":     fileSegmentStorage.P2PkScript,     // P2PK脚本
		"slice_table":     fileSegmentStorage.SliceTable,     // 文件分片哈希表
		"segment_id":      fileSegmentStorage.SegmentId,      // 当前片段的唯一标识
		"segment_index":   fileSegmentStorage.SegmentIndex,   // 当前片段在文件中的索引位置
		"crc32_checksum":  fileSegmentStorage.Crc32Checksum,  // 当前片段的CRC32校验和
		"segment_content": fileSegmentStorage.SegmentContent, // 当前片段的加密后内容
		"encryption_key":  fileSegmentStorage.EncryptionKey,  // 用于解密segment_content的AES密钥
		"signature":       fileSegmentStorage.Signature,      // 文件所有者对片段内容的数字签名
		"shared":          fileSegmentStorage.Shared,         // 是否允许其他节点访问该片段
		"version":         fileSegmentStorage.Version,        // 片段的版本号
	}

	// 执行插入操作
	if err := Insert(rss.sqliteDB, fileSegmentStorageTableName, data); err != nil {
		logger.Errorf("插入数据时失败: %v", err)
		return err
	}

	return nil
}

// GetFileSegmentStorage 根据片段ID获取文件片段存储记录
// 参数:
//   - segmentID: 片段唯一标识
//
// 返回值:
//   - *pb.FileSegmentStorageSql: 查询到的文件片段存储记录
//   - error: 如果查询成功返回nil,否则返回错误信息
func (rss *FileSegmentStorageSqlStore) GetFileSegmentStorage(segmentID string) (*pb.FileSegmentStorageSql, error) {
	m := new(pb.FileSegmentStorageSql)

	// 定义查询字段
	columns := []string{
		"file_id",         // 文件唯一标识
		"name",            // 文件原始名称
		"extension",       // 文件扩展名
		"size",            // 文件总大小
		"content_type",    // MIME类型
		"sha256_hash",     // 文件内容的SHA256哈希值
		"upload_time",     // 文件首次上传的Unix时间戳
		"p2pkh_script",    // P2PKH脚本
		"p2pk_script",     // P2PK脚本
		"slice_table",     // 文件分片哈希表
		"segment_id",      // 当前片段的唯一标识
		"segment_index",   // 当前片段在文件中的索引位置
		"crc32_checksum",  // 当前片段的CRC32校验和
		"segment_content", // 当前片段的加密后内容
		"encryption_key",  // 用于解密segment_content的AES密钥
		"signature",       // 文件所有者对片段内容的数字签名
		"shared",          // 是否允许其他节点访问该片段
		"version",         // 片段的版本号
	}

	// 设置查询条件
	conditions := []string{"segment_id=?"} // 查询条件
	args := []interface{}{segmentID}       // 查询条件对应的值

	// 执行查询
	row, err := SelectOne(rss.sqliteDB, "file_segment_storage", columns, conditions, args)
	if err != nil {
		if err == sql.ErrNoRows {
			return m, nil
		}
		logger.Errorf("数据操作时失败: %v", err)
		return m, err
	}

	// 扫描结果到结构体
	if err := row.Scan(
		&m.FileId,         // 文件唯一标识
		&m.Name,           // 文件原始名称
		&m.Extension,      // 文件扩展名
		&m.Size_,          // 文件总大小
		&m.ContentType,    // MIME类型
		&m.Sha256Hash,     // 文件内容的SHA256哈希值
		&m.UploadTime,     // 文件首次上传的Unix时间戳
		&m.P2PkhScript,    // P2PKH脚本
		&m.P2PkScript,     // P2PK脚本
		&m.SliceTable,     // 文件分片哈希表
		&m.SegmentId,      // 当前片段的唯一标识
		&m.SegmentIndex,   // 当前片段在文件中的索引位置
		&m.Crc32Checksum,  // 当前片段的CRC32校验和
		&m.SegmentContent, // 当前片段的加密后内容
		&m.EncryptionKey,  // 用于解密segment_content的AES密钥
		&m.Signature,      // 文件所有者对片段内容的数字签名
		&m.Shared,         // 是否允许其他节点访问该片段
		&m.Version,        // 片段的版本号
	); err != nil {
		if err == sql.ErrNoRows {
			return m, nil
		}
		logger.Errorf("数据操作时失败: %v", err)
		return m, err
	}

	return m, nil
}

// UpdateFileSegmentStorage 更新文件片段存储记录
// 参数:
//   - storage: 要更新的文件片段存储记录
//
// 返回值:
//   - error: 如果更新成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) UpdateFileSegmentStorage(storage *pb.FileSegmentStorageSql) error {
	// 构建更新数据
	data := map[string]interface{}{
		"file_id":         storage.FileId,         // 文件唯一标识
		"name":            storage.Name,           // 文件原始名称
		"extension":       storage.Extension,      // 文件扩展名
		"size":            storage.Size_,          // 文件总大小
		"content_type":    storage.ContentType,    // MIME类型
		"sha256_hash":     storage.Sha256Hash,     // 文件内容的SHA256哈希值
		"upload_time":     storage.UploadTime,     // 文件首次上传的Unix时间戳
		"p2pkh_script":    storage.P2PkhScript,    // P2PKH脚本
		"p2pk_script":     storage.P2PkScript,     // P2PK脚本
		"slice_table":     storage.SliceTable,     // 文件分片哈希表
		"segment_id":      storage.SegmentId,      // 当前片段的唯一标识
		"segment_index":   storage.SegmentIndex,   // 当前片段在文件中的索引位置
		"crc32_checksum":  storage.Crc32Checksum,  // 当前片段的CRC32校验和
		"segment_content": storage.SegmentContent, // 当前片段的加密后内容
		"encryption_key":  storage.EncryptionKey,  // 用于解密segment_content的AES密钥
		"signature":       storage.Signature,      // 文件所有者对片段内容的数字签名
		"shared":          storage.Shared,         // 是否允许其他节点访问该片段
		"version":         storage.Version,        // 片段的版本号
	}

	// 设置更新条件
	conditions := []string{"segment_id=?"}   // 更新条件
	args := []interface{}{storage.SegmentId} // 更新条件对应的值

	// 执行更新操作
	if err := Update(s.sqliteDB, fileSegmentStorageTableName, data, conditions, args); err != nil {
		logger.Errorf("更新数据时失败: %v", err)
		return err
	}
	return nil
}

// DeleteFileSegmentStorage 删除文件片段存储记录
// 参数:
//   - segmentID: 要删除的片段ID
//
// 返回值:
//   - error: 如果删除成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) DeleteFileSegmentStorage(segmentID string) error {
	conditions := []string{"segment_id=?"} // 删除条件
	args := []interface{}{segmentID}       // 删除条件对应的值
	return Delete(s.sqliteDB, fileSegmentStorageTableName, conditions, args)
}

// ListFileSegmentStorages 列出所有文件片段存储记录
// 返回值:
//   - []*pb.FileSegmentStorageSql: 文件片段存储记录列表
//   - error: 如果查询成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) ListFileSegmentStorages() ([]*pb.FileSegmentStorageSql, error) {
	// 定义查询字段
	columns := []string{
		"file_id",         // 文件唯一标识
		"name",            // 文件原始名称
		"extension",       // 文件扩展名
		"size",            // 文件总大小
		"content_type",    // MIME类型
		"sha256_hash",     // 文件内容的SHA256哈希值
		"upload_time",     // 文件首次上传的Unix时间戳
		"p2pkh_script",    // P2PKH脚本
		"p2pk_script",     // P2PK脚本
		"slice_table",     // 文件分片哈希表
		"segment_id",      // 当前片段的唯一标识
		"segment_index",   // 当前片段在文件中的索引位置
		"crc32_checksum",  // 当前片段的CRC32校验和
		"segment_content", // 当前片段的加密后内容
		"encryption_key",  // 用于解密segment_content的AES密钥
		"signature",       // 文件所有者对片段内容的数字签名
		"shared",          // 是否允许其他节点访问该片段
		"version",         // 片段的版本号
	}

	// 构建查询语句
	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columns, ","), fileSegmentStorageTableName)
	rows, err := s.sqliteDB.Query(query)
	if err != nil {
		logger.Errorf("数据库操作失败: %v", err)
		return nil, err
	}
	defer rows.Close()

	// 遍历结果集
	var storages []*pb.FileSegmentStorageSql
	for rows.Next() {
		m := new(pb.FileSegmentStorageSql)
		err := rows.Scan(
			&m.FileId,         // 文件唯一标识
			&m.Name,           // 文件原始名称
			&m.Extension,      // 文件扩展名
			&m.Size_,          // 文件总大小
			&m.ContentType,    // MIME类型
			&m.Sha256Hash,     // 文件内容的SHA256哈希值
			&m.UploadTime,     // 文件首次上传的Unix时间戳
			&m.P2PkhScript,    // P2PKH脚本
			&m.P2PkScript,     // P2PK脚本
			&m.SliceTable,     // 文件分片哈希表
			&m.SegmentId,      // 当前片段的唯一标识
			&m.SegmentIndex,   // 当前片段在文件中的索引位置
			&m.Crc32Checksum,  // 当前片段的CRC32校验和
			&m.SegmentContent, // 当前片段的加密后内容
			&m.EncryptionKey,  // 用于解密segment_content的AES密钥
			&m.Signature,      // 文件所有者对片段内容的数字签名
			&m.Shared,         // 是否允许其他节点访问该片段
			&m.Version,        // 片段的版本号
		)
		if err != nil {
			logger.Errorf("数据库操作失败: %v", err)
			return nil, err
		}
		storages = append(storages, m)
	}
	return storages, nil
}

// GetFileSegmentStoragesByFileID 根据文件ID和公钥哈希获取文件片段存储记录
// 参数:
//   - fileID: 文件唯一标识
//   - pubkeyHash: 公钥哈希
//
// 返回值:
//   - []*pb.FileSegmentStorageSql: 文件片段存储记录列表
//   - error: 如果查询成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) GetFileSegmentStoragesByFileID(fileID string, pubkeyHash []byte) ([]*pb.FileSegmentStorageSql, error) {
	// 定义查询字段
	columns := []string{
		"file_id",         // 文件唯一标识
		"name",            // 文件原始名称
		"extension",       // 文件扩展名
		"size",            // 文件总大小
		"content_type",    // MIME类型
		"sha256_hash",     // 文件内容的SHA256哈希值
		"upload_time",     // 文件首次上传的Unix时间戳
		"p2pkh_script",    // P2PKH脚本
		"p2pk_script",     // P2PK脚本
		"slice_table",     // 文件分片哈希表
		"segment_id",      // 当前片段的唯一标识
		"segment_index",   // 当前片段在文件中的索引位置
		"crc32_checksum",  // 当前片段的CRC32校验和
		"segment_content", // 当前片段的加密后内容
		"encryption_key",  // 用于解密segment_content的AES密钥
		"signature",       // 文件所有者对片段内容的数字签名
		"shared",          // 是否允许其他节点访问该片段
		"version",         // 片段的版本号
	}

	// 设置查询条件
	var conditions []string
	var args []interface{}

	if pubkeyHash != nil {
		// 如果提供了pubkeyHash，构建P2PKH脚本并按文件ID和P2PKH脚本查询
		p2pkhScript, err := script.NewScriptBuilder().
			AddOp(script.OP_DUP).AddOp(script.OP_HASH160).          // 复制栈顶元素并计算哈希
			AddData(pubkeyHash).                                    // 添加公钥哈希
			AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG). // 验证相等并检查签名
			Script()
		if err != nil {
			logger.Errorf("构建P2PKH脚本失败: %v", err)
			return nil, err
		}
		conditions = []string{"file_id=?", "p2pkh_script=?"}
		args = []interface{}{fileID, p2pkhScript}
	} else {
		// 如果未提供pubkeyHash，按文件ID和shared标志查询
		conditions = []string{"file_id=?", "shared=?"}
		args = []interface{}{fileID, true} // 允许其他节点访问
	}

	// 执行查询
	rows, err := Select(s.sqliteDB, fileSegmentStorageTableName, columns, conditions, args, 0, 0, "")
	if err != nil {
		logger.Errorf("数据操作时失败: %v", err)
		return nil, err
	}
	defer rows.Close()

	// 遍历结果集
	var storages []*pb.FileSegmentStorageSql
	for rows.Next() {
		m := new(pb.FileSegmentStorageSql)
		err := rows.Scan(
			&m.FileId,         // 文件唯一标识
			&m.Name,           // 文件原始名称
			&m.Extension,      // 文件扩展名
			&m.Size_,          // 文件总大小
			&m.ContentType,    // MIME类型
			&m.Sha256Hash,     // 文件内容的SHA256哈希值
			&m.UploadTime,     // 文件首次上传的Unix时间戳
			&m.P2PkhScript,    // P2PKH脚本
			&m.P2PkScript,     // P2PK脚本
			&m.SliceTable,     // 文件分片哈希表
			&m.SegmentId,      // 当前片段的唯一标识
			&m.SegmentIndex,   // 当前片段在文件中的索引位置
			&m.Crc32Checksum,  // 当前片段的CRC32校验和
			&m.SegmentContent, // 当前片段的加密后内容
			&m.EncryptionKey,  // 用于解密segment_content的AES密钥
			&m.Signature,      // 文件所有者对片段内容的数字签名
			&m.Shared,         // 是否允许其他节点访问该片段
			&m.Version,        // 片段的版本号
		)
		if err != nil {
			logger.Errorf("数据库操作失败: %v", err)
			return nil, err
		}
		storages = append(storages, m)
	}
	return storages, nil
}

// GetFileSegmentStorageByFileIDAndSegment 根据文件ID、公钥哈希、片段ID和片段索引获取单个文件片段存储记录
// 参数:
//   - fileID: 文件唯一标识
//   - pubkeyHash: 公钥哈希
//   - segmentID: 片段唯一标识
//   - segmentIndex: 片段索引
//
// 返回值:
//   - *pb.FileSegmentStorageSql: 文件片段存储记录
//   - bool: 是否找到记录
//   - error: 如果查询成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) GetFileSegmentStorageByFileIDAndSegment(fileID string, pubkeyHash []byte, segmentID string, segmentIndex int64) (*pb.FileSegmentStorageSql, bool, error) {
	// 定义查询字段
	columns := []string{
		"file_id",         // 文件唯一标识
		"name",            // 文件原始名称
		"extension",       // 文件扩展名
		"size",            // 文件总大小
		"content_type",    // MIME类型
		"sha256_hash",     // 文件内容的SHA256哈希值
		"upload_time",     // 文件首次上传的Unix时间戳
		"p2pkh_script",    // P2PKH脚本
		"p2pk_script",     // P2PK脚本
		"slice_table",     // 文件分片哈希表
		"segment_id",      // 当前片段的唯一标识
		"segment_index",   // 当前片段在文件中的索引位置
		"crc32_checksum",  // 当前片段的CRC32校验和
		"segment_content", // 当前片段的加密后内容
		"encryption_key",  // 用于解密segment_content的AES密钥
		"signature",       // 文件所有者对片段内容的数字签名
		"shared",          // 是否允许其他节点访问该片段
		"version",         // 片段的版本号
	}

	// 设置查询条件
	var conditions []string
	var args []interface{}

	if pubkeyHash != nil {
		// 如果提供了pubkeyHash，构建P2PKH脚本并按文件ID、P2PKH脚本和片段信息查询
		p2pkhScript, err := script.NewScriptBuilder().
			AddOp(script.OP_DUP).AddOp(script.OP_HASH160).          // 复制栈顶元素并计算哈希
			AddData(pubkeyHash).                                    // 添加公钥哈希
			AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG). // 验证相等并检查签名
			Script()
		if err != nil {
			logger.Errorf("构建P2PKH脚本失败: %v", err)
			return nil, false, err
		}
		conditions = []string{"file_id=?", "p2pkh_script=?", "segment_id=?", "segment_index=?"}
		args = []interface{}{fileID, p2pkhScript, segmentID, segmentIndex}
	} else {
		// 如果未提供pubkeyHash，按文件ID、shared标志和片段信息查询
		conditions = []string{"file_id=?", "shared=?", "segment_id=?", "segment_index=?"}
		args = []interface{}{fileID, true, segmentID, segmentIndex}
	}

	// 执行查询
	row, err := SelectOne(s.sqliteDB, fileSegmentStorageTableName, columns, conditions, args)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		logger.Errorf("数据操作时失败: %v", err)
		return nil, false, err
	}

	// 扫描结果到结构体
	m := new(pb.FileSegmentStorageSql)
	if err := row.Scan(
		&m.FileId,         // 文件唯一标识
		&m.Name,           // 文件原始名称
		&m.Extension,      // 文件扩展名
		&m.Size_,          // 文件总大小
		&m.ContentType,    // MIME类型
		&m.Sha256Hash,     // 文件内容的SHA256哈希值
		&m.UploadTime,     // 文件首次上传的Unix时间戳
		&m.P2PkhScript,    // P2PKH脚本
		&m.P2PkScript,     // P2PK脚本
		&m.SliceTable,     // 文件分片哈希表
		&m.SegmentId,      // 当前片段的唯一标识
		&m.SegmentIndex,   // 当前片段在文件中的索引位置
		&m.Crc32Checksum,  // 当前片段的CRC32校验和
		&m.SegmentContent, // 当前片段的加密后内容
		&m.EncryptionKey,  // 用于解密segment_content的AES密钥
		&m.Signature,      // 文件所有者对片段内容的数字签名
		&m.Shared,         // 是否允许其他节点访问该片段
		&m.Version,        // 片段的版本号
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		logger.Errorf("数据操作时失败: %v", err)
		return nil, false, err
	}

	return m, true, nil
}

// DeleteFileSegmentStoragesByFileID 根据文件ID和公钥哈希删除所有相关的文件片段存储记录
// 参数:
//   - fileID: 文件唯一标识
//   - pubkeyHash: 公钥哈希
//
// 返回值:
//   - error: 如果删除成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) DeleteFileSegmentStoragesByFileID(fileID string, pubkeyHash []byte) error {
	// 构建P2PKH脚本
	p2pkhScript, err := script.NewScriptBuilder().
		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).
		AddData(pubkeyHash).
		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG).
		Script()
	if err != nil {
		logger.Errorf("构建P2PKH脚本失败: %v", err)
		return err
	}

	// 设置删除条件
	conditions := []string{"file_id=?", "p2pkh_script=?"}
	args := []interface{}{fileID, p2pkhScript}

	// 执行删除操作
	if err := Delete(s.sqliteDB, fileSegmentStorageTableName, conditions, args); err != nil {
		logger.Errorf("数据操作时失败: %v", err)
		return err
	}
	return nil
}

// UpdateFileSegmentShared 更新文件片段的共享状态
// 参数:
//   - fileID: 文件唯一标识
//   - pubkeyHash: 公钥哈希
//   - shared: 新的共享状态
//
// 返回值:
//   - error: 如果更新成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) UpdateFileSegmentShared(fileID string, pubkeyHash []byte, shared bool) error {
	// 构建P2PKH脚本
	p2pkhScript, err := script.NewScriptBuilder().
		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).
		AddData(pubkeyHash).
		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG).
		Script()
	if err != nil {
		logger.Errorf("构建P2PKH脚本失败: %v", err)
		return err
	}

	// 构建更新数据
	data := map[string]interface{}{
		"shared": shared, // 更新共享状态
	}

	// 设置更新条件
	conditions := []string{"file_id=?", "p2pkh_script=?"}
	args := []interface{}{fileID, p2pkhScript}

	// 执行更新操作
	if err := Update(s.sqliteDB, fileSegmentStorageTableName, data, conditions, args); err != nil {
		logger.Errorf("更新共享状态失败: fileID=%s, err=%v", fileID, err)
		return err
	}
	return nil
}

// UpdateFileSegmentName 更新文件片段的名称
// 参数:
//   - fileID: 文件唯一标识
//   - pubkeyHash: 公钥哈希
//   - newName: 新的文件名
//
// 返回值:
//   - error: 如果更新成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) UpdateFileSegmentName(fileID string, pubkeyHash []byte, newName string) error {
	// 构建P2PKH脚本
	p2pkhScript, err := script.NewScriptBuilder().
		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).
		AddData(pubkeyHash).
		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG).
		Script()
	if err != nil {
		logger.Errorf("构建P2PKH脚本失败: %v", err)
		return err
	}

	// 检查新名称是否与当前名称相同
	segments, err := s.GetFileSegmentStoragesByFileID(fileID, pubkeyHash)
	if err != nil {
		return err
	}
	if len(segments) > 0 && segments[0].Name == newName {
		return nil // 名称相同，无需更新
	}

	// 构建更新数据
	data := map[string]interface{}{
		"name": newName, // 更新文件名
	}

	// 设置更新条件
	conditions := []string{"file_id=?", "p2pkh_script=?"}
	args := []interface{}{fileID, p2pkhScript}

	// 执行更新操作
	if err := Update(s.sqliteDB, fileSegmentStorageTableName, data, conditions, args); err != nil {
		logger.Errorf("更新文件名失败: fileID=%s, err=%v", fileID, err)
		return err
	}
	return nil
}

// GetSharedFileSegmentStorageByFileID 根据文件ID获取共享的文件片段存储记录
// 参数:
//   - fileID: 文件唯一标识
//
// 返回值:
//   - *pb.ResponseSearchFileSegmentPubSub: 检索文件的响应
//   - bool: 是否找到记录
//   - error: 如果查询成功返回nil,否则返回错误信息
func (s *FileSegmentStorageSqlStore) GetSharedFileSegmentStorageByFileID(fileID string) (*pb.ResponseSearchFileSegmentPubSub, bool, error) {
	// 定义查询字段
	columns := []string{
		"file_id",      // 文件唯一标识
		"name",         // 文件原始名称
		"extension",    // 文件扩展名
		"size",         // 文件总大小
		"content_type", // MIME类型
		"upload_time",  // 文件首次上传的Unix时间戳
	}

	// 设置查询条件：文件ID匹配且必须是共享状态
	conditions := []string{"file_id=?", "shared=?"}
	args := []interface{}{fileID, true}

	// 执行查询
	row, err := SelectOne(s.sqliteDB, fileSegmentStorageTableName, columns, conditions, args)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		logger.Errorf("数据操作时失败: %v", err)
		return nil, false, err
	}

	// 扫描结果到结构体
	m := new(pb.ResponseSearchFileSegmentPubSub)
	if err := row.Scan(
		&m.FileId,      // 文件唯一标识
		&m.Name,        // 文件原始名称
		&m.Extension,   // 文件扩展名
		&m.Size_,       // 文件总大小
		&m.ContentType, // MIME类型
		&m.UploadTime,  // 文件首次上传的Unix时间戳
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		logger.Errorf("数据操作时失败: %v", err)
		return nil, false, err
	}

	return m, true, nil
}
