package downloads

import (
	"sync"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// DownloadFile 包含待下载文件的详细信息及其分片信息
type DownloadFile struct {
	FileID      string   // 文件唯一标识
	Name        string   // 文件名，包括扩展名，描述文件的名称
	Size        int64    // 文件大小，单位为字节，描述文件的总大小
	ContentType string   // MIME类型，表示文件的内容类型，如"text/plain"
	Segments    sync.Map // 使用并发安全的 sync.Map 存储文件分片信息，键是分片索引 (int)，值是指向 FileSegment 结构体的指针 (*FileSegment)
}

// AddSegment 添加文件分片信息
// 参数：
//   - index: int 分片索引
//   - segment: *FileSegment 文件分片信息
func (df *DownloadFile) AddSegment(index int, segment *FileSegment) {
	if segment != nil {
		df.Segments.Store(index, segment)
	}
}

// GetSegment 获取文件分片信息
// 参数：
//   - index: int 分片索引
//
// 返回值：
//   - *FileSegment 文件分片信息
//   - bool 表示是否找到该索引的分片
func (df *DownloadFile) GetSegment(index int) (*FileSegment, bool) {
	value, ok := df.Segments.Load(index)
	if ok {
		return value.(*FileSegment), true
	}
	return nil, false
}

// DeleteSegment 删除文件分片信息
// 参数：
//   - index: int 分片索引
func (df *DownloadFile) DeleteSegment(index int) {
	df.Segments.Delete(index)
}

// UpdateSegmentStatus 更新文件分片的下载状态
// 参数：
//   - index: int 分片索引
//   - status: SegmentDownloadStatus 新的下载状态
func (df *DownloadFile) UpdateSegmentStatus(index int, status SegmentDownloadStatus) {
	if segment, ok := df.GetSegment(index); ok {
		segment.Status = status
		df.Segments.Store(index, segment)
	}
}

// UpdateSegmentNodes 更新文件分片的节点信息
// 参数：
//   - index: int 分片索引
//   - peers: []peer.ID 节点ID列表
func (df *DownloadFile) UpdateSegmentNodes(index int, peers []peer.ID) {
	if segment, ok := df.GetSegment(index); ok {
		segment.UpdateNodes(peers)
		df.Segments.Store(index, segment)
	}
}

// ListAllSegments 列出所有文件分片信息
// 返回值：
//   - map[int]*FileSegment 所有文件分片信息的映射
func (df *DownloadFile) ListAllSegments() map[int]*FileSegment {
	allSegments := make(map[int]*FileSegment)
	df.Segments.Range(func(key, value interface{}) bool {
		allSegments[key.(int)] = value.(*FileSegment)
		return true
	})
	return allSegments
}

// GetSegmentsToDownload 获取需要下载的文件片段索引
// 返回值：
//   - []int: 需要下载的文件片段索引数组
func (df *DownloadFile) GetSegmentsToDownload() []int {
	var segmentsToDownload []int // 用于存储需要下载的文件片段索引

	df.Segments.Range(func(key, value interface{}) bool {
		index := key.(int)
		segment := value.(*FileSegment)
		if segment.IsStatus(SegmentStatusPending) || segment.IsStatus(SegmentStatusFailed) {
			if segment.HasActiveNodes() {
				segmentsToDownload = append(segmentsToDownload, index)
			}
		}
		return true
	})

	return segmentsToDownload
}

// GetPendingSegmentsForNode 获取特定节点下的待下载文件片段索引和唯一标识的映射
// 参数：
//   - ID: peer.ID 节点ID
//
// 返回值：
//   - map[int]string: 待下载文件片段的索引和唯一标识的映射
func (df *DownloadFile) GetPendingSegmentsForNode(ID peer.ID) map[int]string {
	pendingSegments := make(map[int]string) // 初始化待下载文件片段的索引和唯一标识的映射

	df.Segments.Range(func(key, value interface{}) bool {
		index := key.(int)
		segment := value.(*FileSegment)
		if segment.IsStatus(SegmentStatusPending) && segment.IsNodeActive(ID) {
			pendingSegments[index] = segment.GetSegmentID()
		}
		return true
	})

	return pendingSegments
}

// SetNodeInactive 将特定节点下的所有文件片段的节点状态设置为不可用
// 参数：
//   - ID: peer.ID 节点ID
func (df *DownloadFile) SetNodeInactive(ID peer.ID) {
	df.Segments.Range(func(key, value interface{}) bool {
		segment := value.(*FileSegment)
		segment.SetNodeInactive(ID)
		return true
	})
}

// DownloadCompleteCount 检查已经完成的数量
// 返回值：
//   - int: 已经完成的文件片段数量
func (df *DownloadFile) DownloadCompleteCount() int {
	count := 0
	df.Segments.Range(func(key, value interface{}) bool {
		segment := value.(*FileSegment)
		if segment.Status == SegmentStatusCompleted {
			count++
		}
		return true
	})
	return count
}

// SetSegmentStatus 设置文件片段的下载状态
// 参数：
//   - index: int 文件片段的索引
//   - status: SegmentDownloadStatus 文件片段的下载状态
func (df *DownloadFile) SetSegmentStatus(index int, status SegmentDownloadStatus) {
	if segment, ok := df.GetSegment(index); ok {
		segment.Status = status
		df.Segments.Store(index, segment)
	}
}

// IsSegmentCompleted 检查文件片段是否存在以及是否已经下载完成
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - index: int 文件片段的索引
//   - subDir: string 子目录路径
//
// 返回值：
//   - bool: 文件片段是否存在
//   - bool: 文件片段是否下载完成
//   - error: 错误信息
func (df *DownloadFile) IsSegmentCompleted(opt *opts.Options, afe afero.Afero, index int, subDir string) (bool, bool, error) {
	segment, exists := df.GetSegment(index)
	if !exists {
		return false, false, nil
	}

	// 下载状态
	if segment.Status != SegmentStatusCompleted {
		return true, false, nil
	}

	// 读取文件
	sliceContent, err := util.Read(opt, afe, subDir, segment.SegmentID)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return true, false, err
	}

	// 如果文件片段为空，则设置为下载失败
	if len(sliceContent) == 0 {
		df.SetSegmentStatus(index, SegmentStatusFailed)
		return true, false, nil
	}

	return true, true, nil
}

// CheckMissingNodes 检查文件信息中不是纠删码片段的片段，是否存在节点ID是空的
// 如果有存在节点ID为空的片段，返回true，表示还有切片的节点ID没有拿到，否则返回false
// 返回值：
//   - bool: 是否存在节点ID为空的片段
func (df *DownloadFile) CheckMissingNodes() bool {
	missingNodes := false
	df.Segments.Range(func(key, value interface{}) bool {
		segment := value.(*FileSegment)
		if !segment.IsRsCodes && !segment.HasNodes() {
			missingNodes = true
			return false
		}
		return true
	})
	return missingNodes
}

// AddSegmentNodes 添加文件片段所在的节点信息
// 参数：
//   - idx: int 文件片段的索引。
//   - peers: []peer.ID 文件片段所在的节点ID。
func (df *DownloadFile) AddSegmentNodes(idx int, peers []peer.ID) {
	if segment, ok := df.GetSegment(idx); ok {
		segment.UpdateNodes(peers)
		df.Segments.Store(idx, segment)
	}
}

// SegmentCount 返回当前文件的分片数量
// 返回值：
//   - int: 当前文件的分片数量
func (df *DownloadFile) SegmentCount() int {
	count := 0
	df.Segments.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}
