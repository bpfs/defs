package downloads

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
	"golang.org/x/time/rate"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/pubsub"
)

// DownloadTask 描述一个文件下载任务
type DownloadTask struct {
	ctx          context.Context       // 上下文用于管理协程的生命周期
	cancel       context.CancelFunc    // 取消函数，用于取消上下文
	opt          *fscfg.Options        // 文件存储选项配置
	db           *badgerhold.Store     // 持久化存储
	fs           afero.Afero           // 文件系统接口，提供跨平台的文件操作能力
	host         host.Host             // libp2p网络主机实例
	routingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	nps          *pubsub.NodePubSub    // 发布订阅系统，用于之间的消息传递

	mu           sync.RWMutex               // 用于保护队列操作的互斥锁
	taskId       string                     // 任务唯一标识
	distribution *files.SegmentDistribution // 分片分配管理器，用于管理文件分片在节点间的分配

	// 内部操作通道
	chSegmentIndex     chan struct{}             // 片段索引：请求文件片段的索引信息
	chSegmentProcess   chan struct{}             // 处理文件片段：将文件片段整合并写入队列
	chNodeDispatch     chan struct{}             // 节点分发：以节点为单位从队列中读取文件片段
	chNetworkTransfer  chan map[peer.ID][]string // 网络传输：key为节点ID，value为该节点负责的分片ID列表
	chSegmentVerify    chan struct{}             // 片段验证：验证已传输片段的完整性
	chSegmentMerge     chan struct{}             // 片段合并：合并已下载的文件片段
	chFileFinalize     chan struct{}             // 文件完成：处理文件下载完成后的操作
	chRecoverySegments chan struct{}             // 片段恢复通道

	// 外部通知通道
	chSegmentStatus chan *pb.DownloadChan // 片段状态：通知文件片段的处理状态
	chError         chan error            // 错误通知：向外部传递错误信息

	// 添加验证重试相关字段
	verifyRetryCount int         // 验证重试次数
	lastVerifyTime   time.Time   // 上次验证时间
	verifyMutex      sync.Mutex  // 验证互斥锁
	verifyInProgress atomic.Bool // 使用原子操作追踪验证状态

	// 索引请求控制
	indexTicker      *time.Ticker // 索引请求定时器
	indexTickerMutex sync.Mutex   // 保护定时器操作的互斥锁
	indexInProgress  atomic.Bool  // 标记索引请求是否正在进行中

	// 索引请求状态跟踪
	lastIndexInfo struct {
		pendingIDs    string        // 上次请求的片段ID列表的hash
		timestamp     time.Time     // 上次请求的时间
		retryCount    int           // 连续重复请求次数
		lastProgress  int64         // 上次的下载进度
		noProgressFor time.Duration // 无进度持续时间
	}
	indexInfoMutex sync.Mutex // 保护索引信息的互斥锁

	requestLimiter *rate.Limiter

	// 合并相关字段
	mergeMutex      sync.Mutex  // 合并操作互斥锁
	mergeInProgress atomic.Bool // 合并状态标记
	mergeRetryCount int         // 合并重试次数
	lastMergeTime   time.Time   // 上次合并时间
}

// NewDownloadTask 创建并初始化一个新的文件下载任务实例
// 参数:
//   - ctx: context.Context 用于管理任务生命周期的上下文
//   - opt: *fscfg.Options 文件存储配置选项
//   - db: *database.DB 数据库实例
//   - host: host.Host libp2p网络主机实例
//   - routingTable: *kbucket.RoutingTable 路由表
//   - nps: *pubsub.NodePubSub 发布订阅系统
//   - statusChan: chan *pb.DownloadChan 状态更新通道
//   - errChan: chan error 错误通知通道
//   - taskID: string 任务唯一标识符
//
// 返回值:
//   - *DownloadTask: 创建的下载任务实例
//   - error: 如果创建过程中发生错误，返回相应的错误信息
func NewDownloadTask(ctx context.Context, opt *fscfg.Options, db *database.DB, fs afero.Afero,
	host host.Host, routingTable *kbucket.RoutingTable, nps *pubsub.NodePubSub,
	statusChan chan *pb.DownloadChan, errChan chan error, taskID string,
) (*DownloadTask, error) {
	// 创建带取消功能的上下文
	ct, cancel := context.WithCancel(ctx)

	// 创建新的下载任务实例
	task := &DownloadTask{
		ctx:          ct,                             // 上下文对象
		cancel:       cancel,                         // 取消函数
		opt:          opt,                            // 下载选项
		db:           db.BadgerDB,                    // BadgerDB 数据库实例
		fs:           fs,                             // 文件系统接口
		host:         host,                           // 主机信息
		routingTable: routingTable,                   // 路由表
		nps:          nps,                            // 网络协议服务
		mu:           sync.RWMutex{},                 // 读写互斥锁
		taskId:       taskID,                         // 任务唯一标识
		distribution: files.NewSegmentDistribution(), // 初始化分片分配管理器

		// 内部操作通道
		chSegmentIndex:     make(chan struct{}, 1),                                           // 片段索引：请求文件片段的索引信息
		chSegmentProcess:   make(chan struct{}, 1),                                           // 处理文件片段：将文件片段整合并写入队列
		chNodeDispatch:     make(chan struct{}, 1),                                           // 节点分发：以节点为单位从队列中读取文件片段
		chNetworkTransfer:  make(chan map[peer.ID][]string, opt.GetMaxConcurrentDownloads()), // 网络传输：向目标节点传输文件片段
		chSegmentVerify:    make(chan struct{}, 1),                                           // 片段验证：验证已传输片段的完整性
		chSegmentMerge:     make(chan struct{}, 1),                                           // 片段合并：合并已下载的文件片段
		chFileFinalize:     make(chan struct{}, 1),                                           // 文件完成：处理文件下载完成后的操作
		chRecoverySegments: make(chan struct{}, 1),                                           // 片段恢复通道

		// 外部通知通道
		chSegmentStatus: statusChan, // 片段状态：通知文件片段的处理状态
		chError:         errChan,    // 错误通知：向外部传递错误信息

		// 验证相关字段初始化
		verifyRetryCount: 0,
		lastVerifyTime:   time.Time{},
		verifyMutex:      sync.Mutex{},
		verifyInProgress: atomic.Bool{},

		// 索引请求控制
		indexTicker:     nil, // 初始为nil，在Start时创建
		indexInProgress: atomic.Bool{},

		// 索引请求状态跟踪
		lastIndexInfo: struct {
			pendingIDs    string
			timestamp     time.Time
			retryCount    int
			lastProgress  int64
			noProgressFor time.Duration
		}{
			timestamp:    time.Now(),
			lastProgress: 0,
		},

		requestLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),

		// 合并相关字段初始化
		mergeMutex:      sync.Mutex{},
		mergeInProgress: atomic.Bool{},
		mergeRetryCount: 0,
		lastMergeTime:   time.Time{},
	}

	return task, nil
}

// Context 获取任务的上下文
// 返回值:
//   - context.Context: 任务的上下文对象
func (t *DownloadTask) Context() context.Context {
	return t.ctx
}

// Options 获取文件存储选项配置
// 返回值:
//   - *fscfg.Options: 文件存储选项配置
func (t *DownloadTask) Options() *fscfg.Options {
	return t.opt
}

// DB 获取持久化存储
// 返回值:
//   - *badgerhold.Store: 持久化存储实例
func (t *DownloadTask) DB() *badgerhold.Store {
	return t.db
}

// FS 返回文件系统接口
// 返回值:
//   - afero.Afero: 文件系统接口
func (t *DownloadTask) FS() afero.Afero {
	return t.fs
}

// Host 获取网络主机实例
// 返回值:
//   - host.Host: 网络主机实例
func (t *DownloadTask) Host() host.Host {
	return t.host
}

// RoutingTable 获取端实例
// 返回值:
//   - *kbucket.RoutingTable : 路由表实例
func (t *DownloadTask) RoutingTable() *kbucket.RoutingTable {
	return t.routingTable
}

// NodePubSub 获取存储网络
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统
func (t *DownloadTask) NodePubSub() *pubsub.NodePubSub {
	return t.nps
}

// TaskID 获取任务ID
// 返回值:
//   - string: 任务的唯一标识符
func (t *DownloadTask) TaskID() string {
	return t.taskId
}

// Cleanup 清理任务资源
func (t *DownloadTask) Cleanup() {
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
	if t.chSegmentMerge != nil {
		safeClose(t.chSegmentMerge)
		t.chSegmentMerge = nil
	}
	if t.chFileFinalize != nil {
		safeClose(t.chFileFinalize)
		t.chFileFinalize = nil
	}
	if t.chSegmentIndex != nil {
		safeClose(t.chSegmentIndex)
		t.chSegmentIndex = nil
	}
	if t.chRecoverySegments != nil {
		safeClose(t.chRecoverySegments)
		t.chRecoverySegments = nil
	}

	// 安全关闭外部通知通道
	if t.chSegmentStatus != nil {
		safeClose(t.chSegmentStatus)
		t.chSegmentStatus = nil
	}
	// if t.chError != nil {
	// 	safeClose(t.chError)
	// 	t.chError = nil
	// }

}

// Close 关闭任务并释放资源
func (t *DownloadTask) Close() {
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
