package downloads

import (
	"fmt"

	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/pb"

	"github.com/gogo/protobuf/proto"
)

// GetInitialDownloadResponse 根据文件ID获取首次下载响应
// 参数:
//   - db: 数据库存储接口
//   - taskID: 任务的唯一标识符
//   - fileID: 文件的唯一标识符
//   - pubkeyHash: 公钥哈希
//
// 返回值:
//   - *pb.DownloadPubSubFileInfoResponse: 文件摘要响应
//   - *pb.DownloadPubSubManifestResponse: 索引清单响应
//   - error: 操作过程中可能发生的错误
//
// 功能:
//   - 获取文件片段存储记录
//   - 构建文件元数据
//   - 生成文件信息响应
//   - 生成索引清单响应
func GetInitialDownloadResponse(db *database.DB, taskID string, fileID string, pubkeyHash []byte) (*pb.DownloadPubSubFileInfoResponse, *pb.DownloadPubSubManifestResponse, error) {
	var storages []*pb.FileSegmentStorageSql
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 获取所有相关的文件片段存储记录
	storages, err := store.GetFileSegmentStoragesByFileID(fileID, pubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败: %v", err)
		return nil, nil, err
	}

	// 如果没有找到记录，返回错误
	if len(storages) == 0 {
		logger.Errorf("未找到文件ID为 %s 的存储记录", fileID)
		return nil, nil, fmt.Errorf("未找到文件ID为 %s 的存储记录", fileID)
	}

	// 创建FileMeta对象，使用找到的存储记录的信息
	fileMeta := &pb.FileMeta{
		Name:        storages[0].Name,        // 文件原始名称,不包含扩展名
		Extension:   storages[0].Extension,   // 文件扩展名,不包含点号(.)
		Size_:       storages[0].Size_,       // 文件总大小,单位:字节
		ContentType: storages[0].ContentType, // MIME类型,用于标识文件格式,如"application/pdf"
		Sha256Hash:  storages[0].Sha256Hash,  // 文件内容的SHA256哈希值,用于校验文件完整性
	}

	// 反序列化 SliceTable
	var wrapper database.SliceTableWrapper
	if err := proto.Unmarshal(storages[0].SliceTable, &wrapper); err != nil {
		logger.Errorf("反序列化分片哈希表失败: %v", err)
		return nil, nil, err
	}

	sliceTable := make(map[int64]*pb.HashTable)
	for _, entry := range wrapper.Entries {
		value := entry.Value
		sliceTable[entry.Key] = &value
	}

	// 创建文件信息响应对象
	fileInfoResponse := &pb.DownloadPubSubFileInfoResponse{
		TaskId:     taskID,     // 任务ID
		FileId:     fileID,     // 文件ID
		FileMeta:   fileMeta,   // 文件元数据
		SliceTable: sliceTable, // 文件片段的哈希表
	}

	// 创建一个切片来存储可用的分片索引
	availableSlices := make(map[int64]string)
	// 遍历所有存储记录，将分片索引添加到availableSlices中
	for _, storage := range storages {
		availableSlices[storage.SegmentIndex] = storage.SegmentId
	}

	// 创建索引清单响应对象
	manifestResponse := &pb.DownloadPubSubManifestResponse{
		TaskId:          taskID,          // 任务ID
		AvailableSlices: availableSlices, // 本地可用的分片索引数组
	}

	// 返回响应对象和nil错误
	return fileInfoResponse, manifestResponse, nil
}

// GetFileInfoResponse 根据文件ID获取文件信息响应
// 参数:
//   - db: 数据库存储接口
//   - taskID: 任务的唯一标识符
//   - fileID: 文件的唯一标识符
//   - pubkeyHash: 公钥哈希
//
// 返回值:
//   - *pb.FileInfoResponse: 文件信息响应
//   - error: 操作过程中可能发生的错误
//
// 功能:
//   - 获取文件片段存储记录
//   - 构建文件元数据
//   - 生成文件信息响应
func GetFileInfoResponse(db *database.DB, taskID string, fileID string, pubkeyHash []byte) (*pb.DownloadPubSubFileInfoResponse, error) {
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 获取所有相关的文件片段存储记录
	storages, err := store.GetFileSegmentStoragesByFileID(fileID, pubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败: %v", err)
		return nil, err
	}

	if len(storages) == 0 {
		logger.Errorf("未找到文件ID为 %s 的存储记录", fileID)
		return nil, fmt.Errorf("未找到文件ID为 %s 的存储记录", fileID)
	}

	// 创建FileMeta对象
	fileMeta := &pb.FileMeta{
		Name:        storages[0].Name,        // 文件原始名称
		Extension:   storages[0].Extension,   // 文件扩展名
		Size_:       storages[0].Size_,       // 文件大小
		ContentType: storages[0].ContentType, // 内容类型
		Sha256Hash:  storages[0].Sha256Hash,  // SHA256哈希值
	}

	// 反序列化 SliceTable
	var wrapper database.SliceTableWrapper
	if err := proto.Unmarshal(storages[0].SliceTable, &wrapper); err != nil {
		logger.Errorf("反序列化分片哈希表失败: %v", err)
		return nil, err
	}

	sliceTable := make(map[int64]*pb.HashTable)
	for _, entry := range wrapper.Entries {
		value := entry.Value
		sliceTable[entry.Key] = &value
	}

	// 创建文件信息响应对象
	fileInfoResponse := &pb.DownloadPubSubFileInfoResponse{
		TaskId:     taskID,     // 任务ID
		FileId:     fileID,     // 文件ID
		FileMeta:   fileMeta,   // 文件元数据
		SliceTable: sliceTable, // 分片哈希表
	}

	return fileInfoResponse, nil
}

// GetManifestResponse 根据文件ID获取索引清单响应
// 参数:
//   - db: 数据库存储接口
//   - taskID: 任务的唯一标识符
//   - fileID: 文件的唯一标识符
//   - pubkeyHash: 公钥哈希
//   - requestedSegmentIds: 请求下载的片段ID列表
//
// 返回值:
//   - *pb.DownloadManifestResponse: 索引清单响应
//   - error: 如果获取过程中发生错误则返回相应错误
//
// 功能:
//   - 获取文件片段存储记录
//   - 过滤请求的片段
//   - 构建可用片段映射
//   - 生成索引清单响应
func GetManifestResponse(
	db *database.DB,
	taskID string,
	fileID string,
	pubkeyHash []byte,
	requestedSegmentIds []string,
) (*pb.DownloadPubSubManifestResponse, error) {
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 获取所有相关的文件片段存储记录
	storages, err := store.GetFileSegmentStoragesByFileID(fileID, pubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败: %v", err)
		return nil, err
	}

	if len(storages) == 0 {
		logger.Errorf("未找到文件ID为 %s 的存储记录", fileID)
		return nil, fmt.Errorf("未找到文件ID为 %s 的存储记录", fileID)
	}

	// 创建请求片段ID的映射,用于快速查找
	requestedSegments := make(map[string]bool)
	for _, id := range requestedSegmentIds {
		requestedSegments[id] = true
	}

	// 构建可用分片映射
	availableSlices := make(map[int64]string)
	for _, storage := range storages {
		// 如果指定了请求片段列表且当前片段不在请求列表中,则跳过
		if len(requestedSegmentIds) > 0 {
			if _, ok := requestedSegments[storage.SegmentId]; !ok {
				continue
			}
		}
		availableSlices[storage.SegmentIndex] = storage.SegmentId
	}

	// 构建索引清单响应
	response := &pb.DownloadPubSubManifestResponse{
		TaskId:          taskID,          // 任务ID
		AvailableSlices: availableSlices, // 可用片段列表
	}

	return response, nil
}

// GetSegmentContent 根据文件ID、片段ID和片段索引获取单个文件片段的内容
// 参数:
//   - db: 数据库存储接口
//   - taskID: 任务的唯一标识符
//   - fileID: 文件的唯一标识符
//   - segmentId: 请求下载的片段ID
//   - segmentIndex: 请求下载的片段索引
//   - pubkeyHash: 公钥哈希
//   - requestedSegmentIds: 请求下载的片段ID列表
//
// 返回值:
//   - *pb.SegmentContentResponse: 包含请求片段内容的响应对象
//   - error: 如果获取过程中发生错误则返回相应错误
//
// 功能:
//   - 获取指定片段的存储记录
//   - 构建可用片段映射
//   - 生成片段内容响应
func GetSegmentContent(
	db *database.DB,
	taskID string,
	fileID string,
	segmentId string,
	segmentIndex int64,
	pubkeyHash []byte,
	requestedSegmentIds []string,
) (*pb.SegmentContentResponse, error) {
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 获取指定片段的存储记录
	storage, exists, err := store.GetFileSegmentStorageByFileIDAndSegment(fileID, pubkeyHash, segmentId, segmentIndex)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败: %v", err)
		return nil, err
	}
	if !exists {
		logger.Errorf("分片记录不存在: taskID=%s, index=%d", taskID, segmentIndex)
		return nil, fmt.Errorf("分片记录不存在: taskID=%s, index=%d", taskID, segmentIndex)
	}

	// 获取所有相关的文件片段存储记录
	storages, err := store.GetFileSegmentStoragesByFileID(fileID, pubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败: %v", err)
		return nil, err
	}

	if len(storages) == 0 {
		logger.Errorf("未找到文件ID为 %s 的存储记录", fileID)
		return nil, fmt.Errorf("未找到文件ID为 %s 的存储记录", fileID)
	}

	// 创建请求片段ID的映射,用于快速查找
	requestedSegments := make(map[string]bool)
	for _, id := range requestedSegmentIds {
		requestedSegments[id] = true
	}

	// 构建可用分片映射
	availableSlices := make(map[int64]string)
	for _, storage := range storages {
		// 如果指定了请求片段列表且当前片段不在请求列表中,则跳过
		if len(requestedSegmentIds) > 0 {
			if _, ok := requestedSegments[storage.SegmentId]; !ok {
				continue
			}
		}
		availableSlices[storage.SegmentIndex] = storage.SegmentId
	}

	// 反序列化 SliceTable
	var wrapper database.SliceTableWrapper
	if err := proto.Unmarshal(storage.SliceTable, &wrapper); err != nil {
		logger.Errorf("反序列化分片哈希表失败: %v", err)
		return nil, err
	}

	sliceTable := make(map[int64]*pb.HashTable)
	for _, entry := range wrapper.Entries {
		value := entry.Value
		sliceTable[entry.Key] = &value
	}
	// 构建片段内容响应
	response := &pb.SegmentContentResponse{
		TaskId:          taskID,                 // 任务ID
		FileId:          storage.FileId,         // 文件ID
		P2PkScript:      storage.P2PkScript,     // P2PK脚本
		SegmentId:       storage.SegmentId,      // 片段ID
		SegmentIndex:    storage.SegmentIndex,   // 片段索引
		Crc32Checksum:   storage.Crc32Checksum,  // CRC32校验和
		SegmentContent:  storage.SegmentContent, // 片段内容
		EncryptionKey:   storage.EncryptionKey,  // 加密密钥
		Signature:       storage.Signature,      // 数字签名
		SliceTable:      sliceTable,             // 分片哈希表
		AvailableSlices: availableSlices,        // 可用片段列表
		FileMeta: &pb.FileMeta{
			Name:        storage.Name,        // 文件名
			Extension:   storage.Extension,   // 扩展名
			Size_:       storage.Size_,       // 文件大小
			ContentType: storage.ContentType, // 内容类型
			Sha256Hash:  storage.Sha256Hash,  // SHA256哈希值
			ModifiedAt:  storage.UploadTime,  // 上传时间
		},
	}
	return response, nil
}
