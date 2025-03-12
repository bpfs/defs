package downloads

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/bitset"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/segment"

	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
)

// GetDownloadProgressAndPending 获取下载任务的进度和待下载片段信息
// 功能: 根据任务ID获取下载进度位图和待下载片段集合,用于跟踪下载状态
//
// 参数:
// - db: 数据库存储实例
// - taskID: 下载任务ID
//
// 返回值:
// - *bitset.BitSet: 下载进度位图,每个位表示一个数据片段的下载状态(1表示已完成,0表示未完成)
// - map[int64]struct{}: 待下载片段的索引集合,key为片段索引
// - error: 错误信息
func GetDownloadProgressAndPending(db *badgerhold.Store, taskID string) (*bitset.BitSet, map[int64]struct{}, error) {
	// 创建片段记录存储实例
	store := database.NewDownloadSegmentStore(db)

	// 获取任务的所有分片记录
	segments, err := store.FindByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取分片记录失败: %v", err)
		return nil, nil, err
	}

	// 检查分片记录是否存在
	if len(segments) == 0 {
		logger.Errorf("任务 %s 没有分片记录", taskID)
		return nil, nil, fmt.Errorf("任务 %s 没有分片记录", taskID)
	}

	// 初始化统计变量
	maxIndex := int64(-1)                           // 记录最大分片索引
	indexMap := make(map[int64]bool, len(segments)) // 用于验证分片连续性
	dataSegmentCount := 0                           // 数据片段计数

	// 第一次遍历:统计基本信息
	for _, segment := range segments {
		if segment.SegmentIndex > maxIndex {
			maxIndex = segment.SegmentIndex
		}
		indexMap[segment.SegmentIndex] = true
		if !segment.IsRsCodes {
			dataSegmentCount++
		}
	}

	// 验证分片索引连续性
	expectedSize := int64(len(segments))
	if maxIndex != expectedSize-1 {
		logger.Errorf("分片索引不连续: 最大索引=%d, 期望大小=%d", maxIndex, expectedSize)
		return nil, nil, fmt.Errorf("分片索引不连续")
	}

	// 验证分片完整性
	for i := int64(0); i < expectedSize; i++ {
		if !indexMap[i] {
			logger.Errorf("缺少分片索引: %d", i)
			return nil, nil, fmt.Errorf("缺少分片索引")
		}
	}

	// 创建进度位图和待下载集合
	progress := bitset.New(uint(dataSegmentCount))
	pendingSegments := make(map[int64]struct{})

	// 第二次遍历:更新进度信息
	dataSegmentIndex := uint(0)
	for _, segment := range segments {
		if segment.IsRsCodes {
			continue
		}

		if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED {
			progress.Set(dataSegmentIndex)
		} else {
			if !segment.IsRsCodes {
				pendingSegments[segment.SegmentIndex] = struct{}{}
			}
		}
		dataSegmentIndex++
	}

	return progress, pendingSegments, nil
}

// GetListDownloadSegments 获取下载任务的所有片段记录
// 功能: 根据任务ID获取所有下载片段的详细信息
//
// 参数:
// - db: 数据库存储实例
// - taskID: 下载任务ID
//
// 返回值:
// - []*pb.DownloadSegmentRecord: 下载片段记录列表,包含每个片段的详细信息
// - error: 错误信息
func GetListDownloadSegments(db *badgerhold.Store, taskID string) ([]*pb.DownloadSegmentRecord, error) {
	// 创建片段记录存储实例
	store := database.NewDownloadSegmentStore(db)

	// 获取所有分片记录
	segments, err := store.FindByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取分片记录失败: %v", err)
		return nil, err
	}

	// 检查分片记录是否存在
	if len(segments) == 0 {
		logger.Errorf("任务 %s 没有分片记录", taskID)
		return nil, fmt.Errorf("任务 %s 没有分片记录", taskID)
	}

	return segments, nil
}

// GetSegmentStorageData 获取文件片段的存储数据
// 功能: 根据给定的标识信息获取文件片段的完整内容和元数据
//
// 参数:
// - db: 数据库实例
// - hostID: 主机ID
// - taskID: 任务ID
// - fileID: 文件ID
// - segmentID: 片段ID
//
// 返回值:
// - *pb.SegmentContentResponse: 包含片段内容和元数据的完整响应对象
// - error: 错误信息
func GetSegmentStorageData(db *database.DB, hostID string, taskID string, fileID string, segmentID string) (*pb.SegmentContentResponse, error) {
	// 创建SQL存储实例
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 获取片段存储记录
	segmentStorage, err := store.GetFileSegmentStorage(segmentID)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败: %v", err)
		return nil, err
	}

	// 构建文件元数据
	fileMeta := &pb.FileMeta{
		Name:        segmentStorage.Name,
		Extension:   segmentStorage.Extension,
		Size_:       segmentStorage.Size_,
		ContentType: segmentStorage.ContentType,
		Sha256Hash:  segmentStorage.Sha256Hash,
	}

	// 构建片段存储路径
	subDir := filepath.Join(paths.GetSlicePath(), hostID, fileID)

	// 打开片段文件
	file, err := os.Open(filepath.Join(subDir, segmentStorage.SegmentId))
	if err != nil {
		// 过滤掉文件路径,只返回基本错误信息
		logger.Errorf("打开文件失败: %v", err)
		return nil, fmt.Errorf("打开文件失败")
	}
	defer file.Close()

	// 定义需要读取的片段类型
	segmentTypes := []string{"SEGMENTID", "SEGMENTINDEX", "SEGMENTCONTENT", "SIGNATURE"}

	// 读取文件片段
	segmentResults, _, err := segment.ReadFileSegments(file, segmentTypes)
	if err != nil {
		logger.Errorf("读取文件片段失败: %v", err)
		return nil, err
	}

	// 获取并验证片段ID
	id, exists := segmentResults["SEGMENTID"]
	if !exists {
		logger.Error("片段ID不存在")
		return nil, fmt.Errorf("片段ID不存在")
	}

	// 获取并验证片段索引
	index, exists := segmentResults["SEGMENTINDEX"]
	if !exists {
		logger.Error("片段索引不存在")
		return nil, fmt.Errorf("片段索引不存在")
	}

	// 创建类型解码器
	codec := segment.NewTypeCodec()

	// 解码片段ID
	idDecode, err := codec.Decode(id.Data)
	if err != nil {
		logger.Errorf("解码片段ID失败: %v", err)
		return nil, err
	}

	// 解码片段索引
	indexDecode, err := codec.Decode(index.Data)
	if err != nil {
		logger.Errorf("解码片段索引失败: %v", err)
		return nil, err
	}

	// 验证片段标识和索引
	if !reflect.DeepEqual(idDecode, segmentStorage.SegmentId) || !reflect.DeepEqual(indexDecode, segmentStorage.SegmentIndex) {
		logger.Errorf("文件片段标识或索引不匹配")
		return nil, fmt.Errorf("文件片段标识或索引不匹配")
	}

	// 获取并验证片段内容
	content, exists := segmentResults["SEGMENTCONTENT"]
	if !exists {
		logger.Error("片段内容不存在")
		return nil, fmt.Errorf("片段内容不存在")
	}

	// 解码片段内容
	contentDecodeDecode, err := codec.Decode(content.Data)
	if err != nil {
		logger.Errorf("解码片段内容失败: %v", err)
		return nil, err
	}

	// 获取解码后的内容
	segmentContent := contentDecodeDecode.([]byte)

	// 打印片段信息
	// logger.Infof("读取分片[%d] - 从存储读取: 大小=%d bytes, 校验和=%d, 片段ID=%s",
	// 	segmentStorage.SegmentIndex,
	// 	len(segmentContent),
	// 	segmentStorage.Crc32Checksum,
	// 	segmentStorage.SegmentId)

	// 获取并验证签名
	signature, exists := segmentResults["SIGNATURE"]
	if !exists {
		logger.Error("签名不存在")
		return nil, fmt.Errorf("签名不存在")
	}

	// 解码签名
	signatureDecode, err := codec.Decode(signature.Data)
	if err != nil {
		logger.Errorf("解码签名失败: %v", err)
		return nil, err
	}

	// 反序列化切片表
	var wrapper database.SliceTableWrapper
	if err := proto.Unmarshal(segmentStorage.SliceTable, &wrapper); err != nil {
		return nil, err
	}

	// 构建切片表映射
	sliceTable := make(map[int64]*pb.HashTable)
	for _, entry := range wrapper.Entries {
		value := entry.Value
		sliceTable[entry.Key] = &value
	}

	// 构建响应对象
	response := &pb.SegmentContentResponse{
		TaskId:         taskID,                       // 任务ID
		FileId:         fileID,                       // 文件ID
		FileMeta:       fileMeta,                     // 文件元数据
		P2PkScript:     segmentStorage.P2PkScript,    // P2PK脚本
		SegmentId:      segmentStorage.SegmentId,     // 片段ID
		SegmentIndex:   segmentStorage.SegmentIndex,  // 片段索引
		Crc32Checksum:  segmentStorage.Crc32Checksum, // 校验和
		SegmentContent: segmentContent,               // 片段内容
		EncryptionKey:  segmentStorage.EncryptionKey, // 加密密钥
		Signature:      signatureDecode.([]byte),     // 签名
		SliceTable:     sliceTable,                   // 切片表
	}

	return response, nil
}

// GetPendingSegments 获取未完成的下载片段ID列表
// 功能: 获取指定任务中所有未完成下载的非校验片段ID
//
// 参数:
// - db: 数据库存储实例
// - taskID: 下载任务ID
//
// 返回值:
// - []string: 未完成片段的ID列表
// - error: 错误信息
func GetPendingSegments(db *badgerhold.Store, taskID string) ([]string, error) {
	// 创建片段记录存储实例
	store := database.NewDownloadSegmentStore(db)

	// 获取所有分片记录
	segments, err := store.FindByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取分片记录失败: %v", err)
		return nil, err
	}

	// 检查分片记录是否存在
	if len(segments) == 0 {
		logger.Errorf("任务 %s 没有分片记录", taskID)
		return nil, fmt.Errorf("任务 %s 没有分片记录", taskID)
	}

	// 收集未完成的片段ID
	pendingSegmentIds := make([]string, 0)
	for _, segment := range segments {
		// 跳过校验片段
		if segment.IsRsCodes {
			continue
		}

		// 跳过已完成的片段
		if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED {
			continue
		}

		// 检查是否有活跃的下载节点
		hasActiveNode := false
		if segment.SegmentNode != nil {
			for _, isAvailable := range segment.SegmentNode {
				if isAvailable {
					hasActiveNode = true
					break
				}
			}
		}

		// 如果片段正在下载或有活跃节点，跳过
		if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_DOWNLOADING || hasActiveNode {
			continue
		}

		// 以下情况需要重新下载:
		// 1. 状态为 UNSPECIFIED 且没有活跃节点
		// 2. 状态为 PENDING 且没有活跃节点
		// 3. 状态为 FAILED 且没有活跃节点
		pendingSegmentIds = append(pendingSegmentIds, segment.SegmentId)
	}

	return pendingSegmentIds, nil
}

// ValidateAndStoreSegment 验证并存储下载的文件片段
// 功能: 对下载的文件片段进行完整性验证,包括签名验证、解密、校验和验证,并将验证通过的片段存储到数据库
//
// 参数:
// - db: 数据库存储实例
// - shareOne: 第一个密钥分片
// - response: 下载响应数据
//
// 返回值:
// - error: 错误信息
func ValidateAndStoreSegment(db *badgerhold.Store, shareOne []byte, response *pb.SegmentContentResponse) error {
	// 1. 验证签名
	if err := VerifySegmentSignature(response); err != nil {
		logger.Errorf("验证片段签名失败: %v", err)
		return err
	}

	// 2. 创建临时文件存储加密数据
	tempDir := filepath.Join(os.TempDir(), "bpfs_downloads")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		logger.Errorf("创建临时目录失败: %v", err)
		return err
	}

	// 使用片段ID和校验和创建唯一的临时文件名
	tempFileName := fmt.Sprintf("%s_%d", response.SegmentId, response.Crc32Checksum)
	tempFilePath := filepath.Join(tempDir, tempFileName)

	// 使用流式写入替代一次性写入
	f, err := os.OpenFile(tempFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return err
	}
	defer f.Close()

	// 使用缓冲写入器
	bufWriter := bufio.NewWriterSize(f, 1024*1024) // 1MB buffer

	// 分块写入数据
	reader := bytes.NewReader(response.SegmentContent)
	buf := make([]byte, 1024*1024) // 1MB chunks
	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorf("读取数据失败: %v", err)
			return err
		}

		if _, err := bufWriter.Write(buf[:n]); err != nil {
			logger.Errorf("写入数据失败: %v", err)
			return err
		}
	}

	// 确保所有数据都写入磁盘
	if err := bufWriter.Flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	// 3. 更新片段记录
	store := database.NewDownloadSegmentStore(db)
	record := &pb.DownloadSegmentRecord{
		SegmentId:     response.SegmentId,
		SegmentIndex:  response.SegmentIndex,
		TaskId:        response.TaskId,
		Crc32Checksum: response.Crc32Checksum,
		StoragePath:   tempFilePath,
		EncryptionKey: response.EncryptionKey,
		Status:        pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED,
	}

	// 4. 更新数据库记录
	if err := store.Update(record); err != nil {
		// 清理临时文件
		os.Remove(tempFilePath)
		logger.Errorf("更新片段记录失败: %v", err)
		return err
	}

	return nil
}

// GetSegmentStats 获取下载任务的片段统计信息
// 功能: 统计下载任务中数据片段和校验片段的完成情况
//
// 参数:
// - db: 数据库存储实例
// - taskID: 下载任务ID
//
// 返回值:
// - *struct{}: 包含数据片段和校验片段统计信息的结构体,包括总数、完成数、失败数等
// - error: 错误信息
func GetSegmentStats(db *badgerhold.Store, taskID string) (*struct {
	DataSegments struct {
		Total     int
		Completed int
		Failed    map[int64]string
	}
	ParitySegments struct {
		Completed int
		Pending   map[int64]string
	}
}, error) {
	// 创建片段记录存储实例
	store := database.NewDownloadSegmentStore(db)

	// 初始化统计结构
	stats := &struct {
		DataSegments struct {
			Total     int
			Completed int
			Failed    map[int64]string
		}
		ParitySegments struct {
			Completed int
			Pending   map[int64]string
		}
	}{}

	// 初始化映射
	stats.DataSegments.Failed = make(map[int64]string)
	stats.ParitySegments.Pending = make(map[int64]string)

	// 获取所有片段记录
	segments, err := store.FindByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取片段记录失败: %v", err)
		return nil, err
	}

	// 遍历统计片段状态
	for _, segment := range segments {
		if !segment.IsRsCodes {
			// 统计数据片段
			stats.DataSegments.Total++
			switch segment.Status {
			case pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED:
				if segment.StoragePath != "" {
					stats.DataSegments.Completed++
				}
			case pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_FAILED:
				stats.DataSegments.Failed[segment.SegmentIndex] = segment.SegmentId
			}
		} else {
			// 统计校验片段
			if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED &&
				segment.StoragePath != "" {
				stats.ParitySegments.Completed++
			} else {
				stats.ParitySegments.Pending[segment.SegmentIndex] = segment.SegmentId
			}
		}
	}

	return stats, nil
}

// GetSegmentsForRecovery 获取用于数据恢复的片段信息
// 功能: 根据下载失败的数据片段数量,选择合适的校验片段用于数据恢复
//
// 参数:
// - db: 数据库存储实例
// - taskID: 下载任务ID
//
// 返回值:
// - map[int64]string: 需要下载的片段映射,key为片段索引,value为片段ID
// - error: 错误信息
func GetSegmentsForRecovery(db *badgerhold.Store, taskID string) (map[int64]string, error) {
	// 创建片段记录存储实例
	store := database.NewDownloadSegmentStore(db)

	// 获取所有片段记录
	segments, err := store.FindByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取片段记录失败: %v", err)
		return nil, err
	}

	// 初始化结果映射
	segmentsToDownload := make(map[int64]string)

	// 统计各类片段状态
	failedDataCount := 0
	failedDataSegments := make(map[int64]string)
	pendingParitySegments := make(map[int64]string)
	completedParityCount := 0
	failedParitySegments := make(map[int64]string)

	// 遍历统计片段状态
	for _, segment := range segments {
		if !segment.IsRsCodes {
			// 统计失败的数据片段
			if segment.Status == pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_FAILED {
				failedDataCount++
				failedDataSegments[segment.SegmentIndex] = segment.SegmentId
			}
		} else {
			// 统计校验片段状态
			switch segment.Status {
			case pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED:
				if len(segment.StoragePath) > 0 {
					completedParityCount++
				}
			case pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_FAILED:
				failedParitySegments[segment.SegmentIndex] = segment.SegmentId
			default:
				pendingParitySegments[segment.SegmentIndex] = segment.SegmentId
			}
		}
	}

	// 计算需要恢复的片段数量
	neededCount := failedDataCount - completedParityCount
	if neededCount <= 0 {
		return nil, nil
	}

	// 优先选择待下载的校验片段
	if len(pendingParitySegments) > 0 {
		for index, id := range pendingParitySegments {
			segmentsToDownload[index] = id
			neededCount--
			if neededCount == 0 {
				break
			}
		}
	}

	// 如果还需要更多片段,选择失败的片段
	if neededCount > 0 {
		// 合并所有失败片段
		failedSegments := make(map[int64]string)
		for k, v := range failedDataSegments {
			failedSegments[k] = v
		}
		for k, v := range failedParitySegments {
			failedSegments[k] = v
		}

		// 选择所需数量的失败片段
		for index, id := range failedSegments {
			if _, exists := segmentsToDownload[index]; !exists {
				segmentsToDownload[index] = id
				neededCount--
				if neededCount == 0 {
					break
				}
			}
		}
	}

	return segmentsToDownload, nil
}

// UpdateSegmentNodes 更新片段的节点信息并返回未完成的片段索引
// 参数:
//   - db: 数据库存储实例
//   - taskID: 任务ID
//   - peerID: 节点ID
//   - availableSlices: 节点可用的片段映射，key为片段索引，value为片段ID
//
// 返回值:
//   - map[int64]string: 未完成片段的映射，key为片段索引，value为片段ID
//   - error: 错误信息
func UpdateSegmentNodes(db *badgerhold.Store, taskID string, peerID string, availableSlices map[int64]string) (map[int64]string, error) {
	// 创建片段记录存储实例
	store := database.NewDownloadSegmentStore(db)

	// 开启事务
	err := db.Badger().Update(func(txn *badger.Txn) error {
		// 获取任务的所有片段记录
		segments, err := store.FindByTaskIDTx(txn, taskID)
		if err != nil {
			logger.Errorf("获取片段记录失败: %v", err)
			return err
		}

		// 遍历可用片段映射
		for segmentIndex := range availableSlices {
			// 查找对应的片段记录
			var found bool
			for _, segment := range segments {
				if segment.SegmentIndex == segmentIndex {
					found = true

					// 如果 SegmentNode 为空，初始化map
					if segment.SegmentNode == nil {
						segment.SegmentNode = make(map[string]bool)
					}

					// 直接设置节点状态为true
					if !segment.SegmentNode[peerID] {
						segment.SegmentNode[peerID] = true
						// 更新片段记录
						if err := store.UpdateTx(txn, segment); err != nil {
							logger.Errorf("更新片段记录失败: %v", err)
							return err
						}
					}
					break
				}
			}

			if !found {
				logger.Warnf("片段记录不存在: taskID=%s, segmentIndex=%d", taskID, segmentIndex)
			}
		}
		return nil
	})

	if err != nil {
		logger.Errorf("更新节点信息失败: %v", err)
		return nil, err
	}

	// 获取未完成的片段
	pendingSlices := make(map[int64]string)
	segments, err := store.FindByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取片段记录失败: %v", err)
		return nil, err
	}

	// 筛选未完成的片段
	for _, segment := range segments {
		if segment.Status != pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED {
			if segmentID, ok := availableSlices[segment.SegmentIndex]; ok {
				pendingSlices[segment.SegmentIndex] = segmentID
			}
		}
	}

	return pendingSlices, nil
}
