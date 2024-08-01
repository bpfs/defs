package uploads

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	version = "1.0.0"
)

const (
	MaxSessions    = 3                // 允许的最大并发会话数
	SessionTimeout = 10 * time.Minute // 会话超时时间设置为10分钟
	MaxConcurrency = 20               // 任务允许的最大并发上传数
	TotalShares    = 3                // Shamir秘密共享方案的总份额数
	Threshold      = 2                // Shamir秘密共享方案的阈值，即需要恢复秘密的最小份额数
)

// UploadStatus 表示上传任务的状态
type UploadStatus string

const (
	StatusPending   UploadStatus = "pending"   // 待上传，任务已创建但尚未开始执行
	StatusUploading UploadStatus = "uploading" // 上传中，任务正在执行文件上传操作
	StatusPaused    UploadStatus = "paused"    // 已暂停，任务已被暂停，可通过恢复操作继续执行
	StatusCompleted UploadStatus = "completed" // 已完成，任务已成功完成所有上传操作
	StatusFailed    UploadStatus = "failed"    // 失败，任务由于某些错误未能成功完成
)

// SegmentUploadStatus 表示文件片段的上传状态
type SegmentUploadStatus string

const (
	SegmentStatusNotReady  SegmentUploadStatus = "not_ready" // 尚未准备好，文件片段尚未准备好
	SegmentStatusPending   SegmentUploadStatus = "pending"   // 待上传，文件片段已准备好待上传但尚未开始
	SegmentStatusUploading SegmentUploadStatus = "uploading" // 上传中，文件片段正在上传过程中
	SegmentStatusCompleted SegmentUploadStatus = "completed" // 已完成，文件片段已成功上传
	SegmentStatusFailed    SegmentUploadStatus = "failed"    // 失败，文件片段上传失败
)

// FileSegmentInfo 用于发送文件片段信息到网络
type FileSegmentInfo struct {
	TaskID        string // 任务ID
	FileID        string // 文件唯一标识，用于在系统内部唯一区分文件
	TempStorage   string // 文件的临时存储位置，用于保存文件分片的临时数据
	SegmentID     string // 文件片段的唯一标识
	TotalSegments int    // 文件总分片数
	Index         int    // 分片索引，表示该片段在文件中的顺序
	Size          int    // 分片大小，单位为字节，描述该片段的数据量大小
	IsRsCodes     bool   // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
}

// ////////

// EventType 定义了上传事件的类型，用于区分不同的事件，如任务状态更新和错误通知
type EventType string

// 上传事件枚举值

const (
	EventTypeStatusUpdate EventType = "StatusUpdate" // 表示任务状态更新事件
	EventTypeError        EventType = "Error"        // 表示错误通知事件
)

// ErrorType 定义了错误的类型，用于分类不同来源的错误
type ErrorType string

// 错误类型枚举值

const (
	ErrorTypeSystem  ErrorType = "System"  // 系统错误，指示上传过程中发生的系统级别错误，如资源不足、服务不可达等
	ErrorTypeFile    ErrorType = "File"    // 文件错误，指示与文件相关的错误，如文件损坏、文件格式不支持等
	ErrorTypeNetwork ErrorType = "Network" // 网络错误，指示上传过程中遇到的网络问题，如连接断开、超时等
	ErrorTypeStorage ErrorType = "Storage" // 存储错误，指示存储操作失败的问题，如磁盘空间不足、权限问题等
)

// ControlCommand 定义了控制上传任务的命令类型，如暂停、恢复等
type ControlCommand string

// 控制命令枚举值

const (
	CommandStart  ControlCommand = "start"  // 开始，用于启动一项上传任务或会话
	CommandPause  ControlCommand = "pause"  // 暂停，用于暂停正在进行的上传任务或会话
	CommandResume ControlCommand = "resume" // 恢复，用于继续已暂停的上传任务或会话
	CommandCancel ControlCommand = "cancel" // 取消，用于取消正在进行的上传任务或会话
)

// UploadChan 用于表示上传任务的通道信息
type UploadChan struct {
	TaskID           string  // 任务唯一标识
	UploadProgress   int     // 上传进度百分比
	IsComplete       bool    // 上传任务是否完成
	SegmentID        string  // 文件片段唯一标识
	SegmentIndex     int     // 文件片段索引，表示该片段在文件中的顺序
	SegmentSize      int     // 文件片段大小，单位为字节
	UsesErasureCodes bool    // 是否使用纠删码技术
	NodeID           peer.ID // 存储该文件片段的节点ID
	UploadTime       int64   // 上传完成时间的时间戳
}

// NetworkResponse 表示从网络接收到的响应信息
type NetworkResponse struct {
	Index          int     // 文件片段索引，表示该片段在文件中的顺序
	ReceiverPeerID peer.ID // 接收该文件片段的节点ID
}
