package uploads

import (
	"fmt"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/bitset"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
)

// GetUploadProgress 获取上传任务的进度信息
// 参数:
//   - db: *badgerhold.Store 数据库存储接口
//   - taskID: string 上传任务的唯一标识
//
// 返回值:
//   - *bitset.BitSet: 表示上传进度的位图
//   - error: 如果获取过程中发生错误则返回相应错误，否则返回 nil
func GetUploadProgress(db *badgerhold.Store, taskID string) (*bitset.BitSet, error) {
	// 创建 UploadSegmentStore 实例
	store := database.NewUploadSegmentStore(db)

	// 获取所有分片记录
	segments, err := store.GetUploadSegmentsByTaskID(taskID)
	if err != nil {
		logger.Errorf("获取分片记录失败: taskID=%s, err=%v", taskID, err)
		return nil, err
	}

	// 检查分片记录是否为空
	if len(segments) == 0 {
		logger.Errorf("任务没有分片记录: taskID=%s", taskID)
		return nil, fmt.Errorf("任务没有分片记录: taskID=%s", taskID)
	}

	// 找出最大的分片索引并验证分片连续性
	maxIndex := int64(-1)
	indexMap := make(map[int64]bool, len(segments))
	for _, segment := range segments {
		if segment.SegmentIndex > maxIndex {
			maxIndex = segment.SegmentIndex
		}
		indexMap[segment.SegmentIndex] = true
	}

	// 验证分片索引的连续性和范围
	expectedSize := int64(len(segments))
	if maxIndex != expectedSize-1 {
		logger.Errorf("分片索引不连续: taskID=%s, maxIndex=%d, expectedSize=%d", taskID, maxIndex, expectedSize)
		return nil, fmt.Errorf("分片索引不连续: maxIndex=%d, expectedSize=%d", maxIndex, expectedSize)
	}

	// 验证所有索引是否存在
	for i := int64(0); i < expectedSize; i++ {
		if !indexMap[i] {
			logger.Errorf("缺少分片索引: taskID=%s, index=%d", taskID, i)
			return nil, fmt.Errorf("缺少分片索引: %d", i)
		}
	}

	// 创建位图用于记录进度
	progress := bitset.New(uint(expectedSize))

	// 根据已完成的分片更新进度位图
	for _, segment := range segments {
		if segment.Status == pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_COMPLETED {
			progress.Set(uint(segment.SegmentIndex))
		}
	}

	return progress, nil
}

// CreateUploadSegmentRecord 创建上传分片记录
// 参数:
//   - db: *badgerhold.Store 数据库实例
//   - taskID: string 任务ID
//   - segmentID: string 分片ID
//   - segmentIndex: int64 分片索引
//   - size: int64 分片大小
//   - checksum: uint32 CRC32校验和
//   - readKey: string 临时文件读取标识
//   - isRsCodes: bool 是否为纠删码分片
//   - status: SegmentUploadStatus 分片状态
//
// 返回值:
//   - error: 如果创建过程中发生错误，返回错误信息
func CreateUploadSegmentRecord(
	db *badgerhold.Store,
	taskID string,
	segmentID string,
	segmentIndex int64,
	size int64,
	checksum uint32,
	readKey string,
	isRsCodes bool,
	status pb.SegmentUploadStatus,
) error {
	// 创建分片记录
	record := &pb.UploadSegmentRecord{
		// 分片ID
		SegmentId: segmentID,
		// 分片索引
		SegmentIndex: segmentIndex,
		// 任务ID
		TaskId: taskID,
		// 分片大小
		Size_: size,
		// CRC32校验和
		Crc32Checksum: checksum,
		// 临时文件读取标识
		ReadKey: readKey,
		// 是否为纠删码分片
		IsRsCodes: isRsCodes,
		// 分片状态
		Status: status,
		// 过滤的节点ID列表
		FilteredPeerIds: make([]string, 0),
	}

	// 将记录插入数据库
	if err := db.Insert(segmentID, record); err != nil {
		logger.Errorf("创建分片记录失败: segmentID=%s err=%v", segmentID, err)
		return err
	}

	// logger.Infof("创建分片记录成功: segmentID=%s", segmentID)
	return nil
}

// UpdateSegmentUploadInfo 更新文件片段的上传信息
// 该方法用于更新文件片段的上传状态、节点ID和上传时间
//
// 参数:
//   - db: *badgerhold.Store 数据库存储接口
//   - taskID: string 上传任务的唯一标识
//   - index: int64 片段索引
//   - status: pb.SegmentUploadStatus 片段上传状态
//
// 返回值:
//   - error: 如果更新过程中发生错误则返回相应错误，否则返回 nil
func UpdateSegmentUploadInfo(
	db *badgerhold.Store,
	taskID string,
	index int64,
	status pb.SegmentUploadStatus,
) error {
	// 创建分片存储接口
	store := database.NewUploadSegmentStore(db)

	// 获取分片记录
	segment, exists, err := store.GetUploadSegmentByTaskIDAndIndex(taskID, index)
	if err != nil {
		logger.Errorf("获取分片记录失败: taskID=%s, index=%d, err=%v", taskID, index, err)
		return err
	}
	if !exists {
		logger.Errorf("分片记录不存在: taskID=%s, index=%d", taskID, index)
		return fmt.Errorf("分片记录不存在: taskID=%s, index=%d", taskID, index)
	}

	// 更新分片信息
	segment.Status = status

	// 保存更新
	if err := store.UpdateUploadSegment(segment); err != nil {
		logger.Errorf("更新分片记录失败: taskID=%s, index=%d, status=%s, err=%v",
			taskID, index, status.String(), err)
		return err
	}

	// logger.Infof("成功更新分片上传信息: taskID=%s, index=%d, status=%s",
	// 	taskID, index, status.String())

	return nil
}
