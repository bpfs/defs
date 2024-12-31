package downloads

import (
	"context"
	"sync"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/files"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/kbucket"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
	"github.com/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
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
	chSegmentIndex    chan struct{}             // 片段索引：请求文件片段的索引信息
	chSegmentProcess  chan struct{}             // 处理文件片段：将文件片段整合并写入队列
	chNodeDispatch    chan struct{}             // 节点分发：以节点为单位从队列中读取文件片段
	chNetworkTransfer chan map[peer.ID][]string // 网络传输：key为节点ID，value为该节点负责的分片ID列表
	chSegmentVerify   chan struct{}             // 片段验证：验证已传输片段的完整性
	chSegmentMerge    chan struct{}             // 片段合并：合并已下载的文件片段
	chFileFinalize    chan struct{}             // 文件完成：处理文件下载完成后的操作

	// 外部通知通道
	chSegmentStatus chan *pb.DownloadChan // 片段状态：通知文件片段的处理状态
	chError         chan error            // 错误通知：向外部传递错误信息

	// 任务控制通道
	chPause  chan struct{} // 暂停：暂停当前下载任务
	chCancel chan struct{} // 取消：取消当前下载任务
	chDelete chan struct{} // 删除：删除当前下载任务及相关资源
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
		chSegmentIndex:    make(chan struct{}, 1),                                           // 片段索引：请求文件片段的索引信息
		chSegmentProcess:  make(chan struct{}, 1),                                           // 处理文件片段：将文件片段整合并写入队列
		chNodeDispatch:    make(chan struct{}, 1),                                           // 节点分发：以节点为单位从队列中读取文件片段
		chNetworkTransfer: make(chan map[peer.ID][]string, opt.GetMaxConcurrentDownloads()), // 网络传输：向目标节点传输文件片段
		chSegmentVerify:   make(chan struct{}, 1),                                           // 片段验证：验证已传输片段的完整性
		chSegmentMerge:    make(chan struct{}, 1),                                           // 片段合并：合并已下载的文件片段
		chFileFinalize:    make(chan struct{}, 1),                                           // 文件完成：处理文件下载完成后的操作

		// 外部通知通道
		chSegmentStatus: statusChan, // 片段状态：通知文件片段的处理状态
		chError:         errChan,    // 错误通知：向外部传递错误信息

		// 任务控制通道
		chPause:  make(chan struct{}, 1), // 暂停：暂停当前下载任务
		chCancel: make(chan struct{}, 1), // 取消：取消当前下载任务
		chDelete: make(chan struct{}, 1), // 删除：删除当前下载任务及相关资源
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

	// 安全关闭外部通知通道
	if t.chSegmentStatus != nil {
		safeClose(t.chSegmentStatus)
		t.chSegmentStatus = nil
	}
	if t.chError != nil {
		safeClose(t.chError)
		t.chError = nil
	}

	// 安全关闭任务控制通道
	if t.chPause != nil {
		safeClose(t.chPause)
		t.chPause = nil
	}
	if t.chCancel != nil {
		safeClose(t.chCancel)
		t.chCancel = nil
	}
	if t.chDelete != nil {
		safeClose(t.chDelete)
		t.chDelete = nil
	}
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
