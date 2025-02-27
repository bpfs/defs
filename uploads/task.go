package uploads

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/shamir"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/pubsub"
)

// UploadTask 描述一个文件上传任务，包括文件信息和上传状态
type UploadTask struct {
	ctx          context.Context       // 上下文用于管理协程的生命周期
	cancel       context.CancelFunc    // 取消函数，用于取消上下文
	opt          *fscfg.Options        // 文件存储选项配置
	db           *badgerhold.Store     // 持久化存储
	fs           afero.Afero           // 文件系统接口，提供跨平台的文件操作能力
	host         host.Host             // libp2p网络主机实例
	routingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	nps          *pubsub.NodePubSub    // 发布订阅系统，用于之间的消息传递

	mu           sync.Mutex                 // 用于保护队列操作的互斥锁
	taskId       string                     // 任务唯一标识
	scheme       *shamir.ShamirScheme       // Shamir秘密共享方案，用于文件加密和分片
	distribution *files.SegmentDistribution // 分片分配管理器，用于管理文件分片在节点间的分配

	// 内部操作通道
	chSegmentProcess  chan struct{}             // 处理文件片段：将文件片段整合并写入队列
	chNodeDispatch    chan struct{}             // 节点分发：以节点为单位从队列中读取文件片段
	chNetworkTransfer chan map[peer.ID][]string // 网络传输：key为节点ID，value为该节点负责的分片ID列表
	chSegmentVerify   chan struct{}             // 片段验证：验证已传输片段的完整性
	chFileFinalize    chan struct{}             // 文件完成：处理文件上传完成后的操作

	// 外部通知通道
	chSegmentStatus chan *pb.UploadChan // 片段状态：通知文件片段的处理状态
	// chFileStatus    chan *pb.UploadChan // 文件状态：通知整个文件的处理状态
	chError chan error // 错误通知：向外部传递错误信息

	// 添加验证重试相关字段
	verifyRetryCount int         // 验证重试次数
	lastVerifyTime   time.Time   // 上次验证时间
	verifyMutex      sync.Mutex  // 验证互斥锁
	verifyInProgress atomic.Bool // 使用原子操作追踪验证状态
}

const (
	maxVerifyRetries = 3               // 最大验证重试次数
	verifyRetryDelay = time.Second * 5 // 重试等待时间
)

// NewUploadTask 创建并初始化一个新的文件上传任务实例
// 参数:
//   - ctx: context.Context 用于管理任务生命周期的上下文
//   - opt: *fscfg.Options 文件存储配置选项
//   - db: *database.DB 数据库实例
//   - fs: afero.Afero 文件系统接口
//   - host: host.Host libp2p网络主机实例
//   - routingTable: *kbucket.RoutingTable 路由表
//   - nps: *pubsub.NodePubSub 发布订阅系统
//   - scheme: *shamir.ShamirScheme Shamir秘密共享方案实例
//   - statusChan: chan *pb.UploadChan 状态更新通道
//   - taskID: string 任务唯一标识符
//
// 返回值:
//   - *UploadTask: 创建的上传任务实例
func NewUploadTask(ctx context.Context, opt *fscfg.Options, db *database.DB, fs afero.Afero,
	host host.Host, routingTable *kbucket.RoutingTable, nps *pubsub.NodePubSub,
	scheme *shamir.ShamirScheme, statusChan chan *pb.UploadChan, errChan chan error, taskID string,
) *UploadTask {
	// 创建带取消功能的上下文
	ct, cancel := context.WithCancel(ctx)

	// 创建新的上传任务实例
	return &UploadTask{
		ctx:          ct,                             // 上下文对象
		cancel:       cancel,                         // 取消函数
		opt:          opt,                            // 上传选项
		db:           db.BadgerDB,                    // BadgerDB 数据库实例
		fs:           fs,                             // 文件系统接口
		host:         host,                           // 主机信息
		routingTable: routingTable,                   // 路由表
		nps:          nps,                            // 网络协议服务
		mu:           sync.Mutex{},                   // 互斥锁
		taskId:       taskID,                         // 任务唯一标识
		scheme:       scheme,                         // 协议方案
		distribution: files.NewSegmentDistribution(), // 初始化分片分配管理器

		// 内部操作通道
		chSegmentProcess:  make(chan struct{}, 1),                                         // 处理文件片段：将文件片段整合并写入队列
		chNodeDispatch:    make(chan struct{}, 1),                                         // 节点分发：以节点为单位从队列中读取文件片段
		chNetworkTransfer: make(chan map[peer.ID][]string, opt.GetMaxConcurrentUploads()), // 网络传输：向目标节点传输文件片段
		chSegmentVerify:   make(chan struct{}, 1),                                         // 片段验证：验证已传输片段的完整性
		chFileFinalize:    make(chan struct{}, 1),                                         // 文件完成：处理文件上传完成后的操作

		// 外部通知通道
		chSegmentStatus: statusChan, // 片段状态：通知文件片段的处理状态
		// chFileStatus:    statusChan, // 文件状态：通知整个文件的处理状态
		chError: errChan, // 错误通知：向外部传递错误信息

		// 添加验证重试相关字段
		verifyRetryCount: 0,
		lastVerifyTime:   time.Time{},   // 零值
		verifyMutex:      sync.Mutex{},  // 验证互斥锁
		verifyInProgress: atomic.Bool{}, // 使用原子操作追踪验证状态
	}
}

// Context 获取任务的上下文
// 返回值:
//   - context.Context: 任务的上下文对象
func (t *UploadTask) Context() context.Context {
	return t.ctx
}

// Options 获取文件存储选项配置
// 返回值:
//   - *fscfg.Options: 文件存储选项配置
func (t *UploadTask) Options() *fscfg.Options {
	return t.opt
}

// DB 获取持久化存储
// 返回值:
//   - *badgerhold.Store: 持久化存储实例
func (t *UploadTask) DB() *badgerhold.Store {
	return t.db
}

// FS 返回文件系统接口
// 返回值:
//   - afero.Afero: 文件系统接口
func (t *UploadTask) FS() afero.Afero {
	return t.fs
}

// Host 获取网络主机实例
// 返回值:
//   - host.Host: 网络主机实例
func (t *UploadTask) Host() host.Host {
	return t.host
}

// RoutingTable 获取端实例
// 返回值:
//   - *kbucket.RoutingTable : 路由表实例
func (t *UploadTask) RoutingTable() *kbucket.RoutingTable {
	return t.routingTable
}

// NodePubSub 获取存储网络
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统
func (t *UploadTask) NodePubSub() *pubsub.NodePubSub {
	return t.nps
}

// Scheme 返回 Shamir 秘密共享方案
// 返回值:
//   - *shamir.ShamirScheme: Shamir 秘密共享方案
func (t *UploadTask) Scheme() *shamir.ShamirScheme {
	return t.scheme
}

// TaskID 获取任务ID
// 返回值:
//   - string: 任务的唯一标识符
func (t *UploadTask) TaskID() string {
	return t.taskId
}

// Cleanup 清理任务资源
func (t *UploadTask) Cleanup() {
	// 关闭所有通道（使用互斥锁保护）
	t.mu.Lock()
	defer t.mu.Unlock()

	// 清空分片分配列表
	t.distribution.Clear()

	// 安全关闭内部操作通道
	if t.chSegmentProcess != nil {
		safeClose(t.chSegmentProcess)
		t.chSegmentProcess = nil
	}
	if t.chNodeDispatch != nil {
		safeClose(t.chNodeDispatch)
		t.chNodeDispatch = nil
	}
	if t.chNetworkTransfer != nil {
		safeClose(t.chNetworkTransfer)
		t.chNetworkTransfer = nil
	}
	if t.chSegmentVerify != nil {
		safeClose(t.chSegmentVerify)
		t.chSegmentVerify = nil
	}
	if t.chFileFinalize != nil {
		safeClose(t.chFileFinalize)
		t.chFileFinalize = nil
	}

	// 安全关闭外部通知通道
	if t.chSegmentStatus != nil {
		safeClose(t.chSegmentStatus)
		t.chSegmentStatus = nil
	}
	// if t.chFileStatus != nil {
	// 	safeClose(t.chFileStatus)
	// 	t.chFileStatus = nil
	// }
	if t.chError != nil {
		safeClose(t.chError)
		t.chError = nil
	}

}

// Close 关闭任务并释放资源
func (t *UploadTask) Close() {
	// 取消上下文
	if t.cancel != nil {
		t.cancel()
	}

	// 清理资源
	t.Cleanup()
}

// safeClose 安全地关闭一个通道
// 参数:
//   - ch: chan T 需要关闭的通道
func safeClose[T any](ch chan T) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("关闭通道时发生panic", "panic", r)
		}
	}()

	// 确保通道未关闭
	select {
	case _, ok := <-ch:
		if !ok {
			// 通道已关闭
			return
		}
	default:
		// 通道未关闭，可以安全关闭
		close(ch)
	}
}

// SetStatus 设置任务状态
// 参数:
//   - status: pb.UploadStatus 新的任务状态
//
// 返回值:
//   - error: 如果设置状态失败，返回相应的错误信息
func (t *UploadTask) SetStatus(status pb.UploadStatus) error {
	uploadFileStore := database.NewUploadFileStore(t.db)
	return uploadFileStore.UpdateUploadFileStatus(t.taskId, status, time.Now().Unix())
}
