package downloads

import "github.com/libp2p/go-libp2p/core/peer"

const (
	version = "1.0.0"
)

// 下载任务的状态
type DownloadStatus string

const (
	StatusPending     DownloadStatus = "pending"     // 待下载
	StatusDownloading DownloadStatus = "downloading" // 下载中
	StatusCompleted   DownloadStatus = "completed"   // 下载完成
	StatusFailed      DownloadStatus = "failed"      // 下载失败
	StatusPaused      DownloadStatus = "paused"      // 下载暂停
)

// 文件片段的下载状态
type SegmentDownloadStatus string

const (
	SegmentStatusPending     SegmentDownloadStatus = "pending"     // 待下载
	SegmentStatusDownloading SegmentDownloadStatus = "downloading" // 下载中
	SegmentStatusCompleted   SegmentDownloadStatus = "completed"   // 下载完成
	SegmentStatusFailed      SegmentDownloadStatus = "failed"      // 下载失败
)

// DownloadChan 用于刷新下载任务的通道
type DownloadChan struct {
	TaskID           string  // 任务唯一标识
	TotalPieces      int     // 文件总分片数
	DownloadProgress int     // 下载进度百分比
	IsComplete       bool    // 下载任务是否完成
	SegmentID        string  // 文件片段唯一标识
	SegmentIndex     int     // 文件片段索引，表示该片段在文件中的顺序
	SegmentSize      int     // 文件片段大小，单位为字节
	UsesErasureCodes bool    // 是否使用纠删码技术
	NodeID           peer.ID // 存储该文件片段的节点ID
	DownloadTime     int64   // 下载完成时间的时间戳
}

type DownloadToLocal struct {
	TaskID string // 任务唯一标识
	Index  int    // 分片索引，表示该片段在文件中的顺序
}

// HashTable 描述分片的校验和是否属于纠删码
type HashTable struct {
	Checksum  []byte // 分片的校验和，用于校验分片数据的完整性和一致性
	IsRsCodes bool   // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
}
