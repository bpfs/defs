package downloads

import (
	"context"
	"fmt"
	"sync"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/dgraph-io/badger/v4"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/pubsub"
	"go.uber.org/fx"
)

const (
	MaxSessions = 3 // 允许的最大并发会话数
)

// DownloadManager 管理所有下载任务，提供文件下载的统一入口和管理功能
type DownloadManager struct {
	ctx          context.Context       // 上下文，用于管理 DownloadManager 的生命周期和取消操作
	cancel       context.CancelFunc    // 取消函数，用于取消上下文，停止所有相关的goroutine
	mu           sync.Mutex            // 互斥锁，用于保护并发访问共享资源，确保线程安全
	opt          *fscfg.Options        // 文件存储选项配置，包含各种存储相关的设置参数
	db           *database.DB          // 数据库存储，用于持久化下载任务和相关元数据
	fs           afero.Afero           // 文件系统接口，提供跨平台的文件操作能力
	host         host.Host             // libp2p网络主机实例
	routingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	nps          *pubsub.NodePubSub    // 发布订阅系统，用于节点之间的消息传递

	tasks sync.Map // 下载任务映射，键为任务ID (string)，值为下载任务指针 (*DownloadTask)

	downloadChan chan string           // 下载操作通知通道，传递任务ID以触发新的下载任务
	statusChan   chan *pb.DownloadChan // 下载状态和进度通知通道，用于实时更新下载进度
	errChan      chan error            // 错误通道，用于将错误信息传递到外部
}

// Context 获取任务的上下文
// 返回值:
//   - context.Context: 任务的上下文对象
func (m *DownloadManager) Context() context.Context {
	return m.ctx
}

// Cancel 获取任务的取消函数
// 返回值:
//   - context.CancelFunc: 任务的取消函数
func (m *DownloadManager) Cancel() context.CancelFunc {
	return m.cancel
}

// Options 返回文件存储选项配置
// 返回值:
//   - *fscfg.Options: 文件存储选项配置
func (m *DownloadManager) Options() *fscfg.Options {
	return m.opt
}

// DB 返回数据库存储
// 返回值:
//   - *badgerhold.Store: 数据库存储
func (m *DownloadManager) DB() *database.DB {
	return m.db
}

// FS 返回文件系统接口
// 返回值:
//   - afero.Afero: 文件系统接口
func (m *DownloadManager) FS() afero.Afero {
	return m.fs
}

// Host 获取网络主机实例
// 返回值:
//   - host.Host: 网络主机实例
func (m *DownloadManager) Host() host.Host {
	return m.host
}

// RoutingTable 获取客户端实例
// 返回值:
//   - *kbucket.RoutingTable : 路由表实例
func (m *DownloadManager) RoutingTable() *kbucket.RoutingTable {
	return m.routingTable
}

// NodePubSub 返回发布订阅系统
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统
func (m *DownloadManager) NodePubSub() *pubsub.NodePubSub {
	return m.nps
}

// StatusChan 返回下载状态和进度通知通道
// 返回值:
//   - <-chan *pb.UploadChan: 只读的通道，用于接收下载状态和进度通知
func (m *DownloadManager) StatusChan() <-chan *pb.DownloadChan {
	return m.statusChan
}

// ErrChan 返回错误通知通道
// 返回值:
//   - <-chan error: 只读的通道，用于接收错误通知
func (m *DownloadManager) ErrChan() <-chan error {
	return m.errChan
}

// addTask 添加一个新的上传任务(内部方法)
// 参数:
//   - task: 要添加的上传任务
//
// 返回值:
//   - error: 如果添加过程中出现错误，返回相应的错误信息
func (m *DownloadManager) addTask(task *DownloadTask) error {
	// 检查任务是否已存在
	if _, exists := m.tasks.Load(task.TaskID()); exists {
		logger.Errorf("添加任务失败: 任务ID %s 已存在", task.TaskID())
		return fmt.Errorf("任务ID %s 已存在", task.TaskID())
	}

	// 存储任务
	m.tasks.Store(task.TaskID(), task)
	// logger.Infof("成功添加任务: taskID=%s", task.TaskID())
	return nil
}

// getTask 获取指定任务ID的上传任务(内部方法)
// 参数:
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - *UploadTask: 如果找到则返回 UploadTask 实例，否则为 nil
//   - bool: 如果找到则为 true，否则为 false
func (m *DownloadManager) getTask(taskID string) (*DownloadTask, bool) {
	task, exists := m.tasks.Load(taskID)
	if !exists {
		return nil, false
	}
	return task.(*DownloadTask), true
}

// removeTask 移除指定任务ID的上传任务(内部方法)
// 参数:
//   - taskID: 任务的唯一标识符
func (m *DownloadManager) removeTask(taskID string) {
	if task, exists := m.tasks.Load(taskID); exists {
		task.(*DownloadTask).Close() // 关闭任务并清理资源
		m.tasks.Delete(taskID)
		// logger.Infof("成功移除任务: %s", taskID)
	}
}

// NewDownloadManagerInput 定义了创建 DownloadManager 所需的输入参数
type NewDownloadManagerInput struct {
	fx.In

	Ctx          context.Context       // 上下文
	Opt          *fscfg.Options        // 文件存储选项配置
	DB           *database.DB          // 数据库存储
	FS           afero.Afero           // 文件系统接口
	Host         host.Host             // libp2p网络主机实例
	RoutingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	NPS          *pubsub.NodePubSub    // 发布订阅系统，用于节点之间的消息传递
}

// NewDownloadManagerOutput 定义了 NewDownloadManager 函数的输出
type NewDownloadManagerOutput struct {
	fx.Out

	Download *DownloadManager // 下载管理器实例
}

// NewDownloadManager 创建并初始化一个新的 DownloadManager 实例
// 功能: 根据提供的参数创建并初始化一个新的下载管理器实例
// 参数:
//   - lc: 生命周期管理器
//   - input: 创建下载管理器所需的输入参数
//
// 返回值:
//   - NewDownloadManagerOutput: 包含创建的下载管理器实例的输出结构
//   - error: 创建过程中的错误信息，如果成功则为nil
func NewDownloadManager(lc fx.Lifecycle, input NewDownloadManagerInput) (out NewDownloadManagerOutput, err error) {
	// 创建一个新的上下文和取消函数
	ctx, cancel := context.WithCancel(input.Ctx)

	// 创建并初始化 DownloadManager 实例
	download := &DownloadManager{
		ctx:          ctx,
		cancel:       cancel,
		mu:           sync.Mutex{},
		opt:          input.Opt,
		nps:          input.NPS,
		db:           input.DB,
		fs:           input.FS,
		host:         input.Host,
		routingTable: input.RoutingTable,
		tasks:        sync.Map{},
		downloadChan: make(chan string, 5),
		statusChan:   make(chan *pb.DownloadChan, 100),
		errChan:      make(chan error, 100),
	}

	// 将创建的 DownloadManager 实例赋值给输出
	out.Download = download
	return out, nil
}

// InitializeDownloadManagerInput 定义了初始化 DownloadManager 所需的输入参数
type InitializeDownloadManagerInput struct {
	fx.In

	Download *DownloadManager // 下载管理器实例
}

// InitializeDownloadManager 初始化 DownloadManager 并设置相关的生命周期钩子
// 功能: 初始化下载管理器并设置其生命周期钩子函数
// 参数:
//   - lc: 生命周期管理器
//   - input: 初始化下载管理器所需的输入参数
//
// 返回值:
//   - error: 初始化过程中的错误信息，如果成功则为nil
func InitializeDownloadManager(lc fx.Lifecycle, input InitializeDownloadManagerInput) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("正在启动下载管理器...")

			// 启动通道事件处理
			go input.Download.ManagerChannelEvents()

			// 加载现有的下载任务
			if err := input.Download.LoadExistingTasks(); err != nil {
				logger.Errorf("加载现有任务失败: %v", err)
				return err
			}

			logger.Info("下载管理器已启动")
			return nil
		},

		// 停止钩子
		OnStop: func(ctx context.Context) error {
			logger.Info("正在停止下载管理器...")
			input.Download.cancel()
			return nil
		},
	})

	return nil
}

// IsMaxConcurrencyReached 检查是否达到下载允许的最大并发数
// 返回值:
//   - bool: 如果达到最大并发数则返回true，否则返回false
func (m *DownloadManager) IsMaxConcurrencyReached() bool {
	// 加锁以确保线程安全
	m.mu.Lock()
	defer m.mu.Unlock()

	// 创建下载文件存储实例
	downloadFileStore := database.NewDownloadFileStore(m.db.BadgerDB)

	// 统计活跃任务数量
	activeCount := 0
	m.tasks.Range(func(_, value interface{}) bool {
		task := value.(*DownloadTask)

		// 获取下载文件记录
		record, exists, err := downloadFileStore.Get(task.TaskID())
		if err != nil {
			logger.Errorf("获取下载文件记录失败: taskID=%s, err=%v", task.TaskID(), err)
			return true // 继续遍历
		}
		if !exists {
			return true // 继续遍历
		}

		// 检查任务是否处于下载中状态
		if record.Status == pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING {
			activeCount++
		}

		// 如果活跃任务数量达到或超过最大会话数，提前结束遍历
		return activeCount < MaxSessions
	})

	// 如果活跃任务数量达到或超过最大会话数，记录警告日志
	if activeCount >= MaxSessions {
		logger.Warnf("已达到最大并发下载数: %d", MaxSessions)
	}

	return activeCount >= MaxSessions
}

// IsFileDownloading 检查指定文件是否正在下载中
// 功能: 检查指定文件是否处于下载相关状态（包括获取信息、等待下载、下载中和暂停状态）
// 参数:
//   - fileID: 要检查的文件ID
//
// 返回值:
//   - bool: 如果文件正在下载中则返回true，否则返回false
func (m *DownloadManager) IsFileDownloading(fileID string) bool {
	// 加锁以确保线程安全
	m.mu.Lock()
	defer m.mu.Unlock()

	// 创建下载文件存储实例
	downloadFileStore := database.NewDownloadFileStore(m.db.BadgerDB)

	// 使用 FindByFileID 查找与指定文件ID相关的所有下载记录
	records, err := downloadFileStore.FindByFileID(fileID)
	if err != nil {
		logger.Errorf("查找文件下载记录失败: fileID=%s, err=%v", fileID, err)
		return false
	}

	// 检查是否存在正在进行的下载任务
	for _, record := range records {
		// 检查文件是否处于以下状态：获取信息中、待下载、下载中、已暂停
		switch record.Status {
		case pb.DownloadStatus_DOWNLOAD_STATUS_FETCHING_INFO, // 获取文件信息中
			pb.DownloadStatus_DOWNLOAD_STATUS_PENDING,     // 待下载
			pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING, // 下载中
			pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED:      // 已暂停
			// logger.Infof("文件 %s 正在下载中，状态: %s, 任务ID: %s",
			// 	fileID, record.Status.String(), record.TaskId)
			return true
		}
	}

	logger.Debugf("文件 %s 当前没有正在进行的下载任务", fileID)
	return false
}

// LoadExistingTasks 从数据库加载现有的下载任务
// 功能: 从数据库中加载并恢复所有现有的下载任务
// 返回值:
//   - error: 加载过程中的错误信息，如果成功则为nil
func (m *DownloadManager) LoadExistingTasks() error {
	// 创建下载任务存储对象
	downloadTaskStore := database.NewDownloadFileStore(m.db.BadgerDB)

	// 从数据库加载现有的下载任务
	// TODO: 这里需要考虑量的问题
	tasks, _, err := QueryDownloadTask(m.db.BadgerDB, 0, 1000) // 设置较大的页面大小以加载所有任务
	if err != nil {
		if err == badgerhold.ErrNotFound {
			logger.Info("没有找到现有的下载任务")
			return nil
		}
		logger.Errorf("加载下载任务时出错: %v", err)
		return err
	}

	// 处理每个下载任务
	for _, downloadFile := range tasks {
		switch downloadFile.Status {
		case pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING, // 下载中
			pb.DownloadStatus_DOWNLOAD_STATUS_FETCHING_INFO, // 获取文件信息中
			pb.DownloadStatus_DOWNLOAD_STATUS_PENDING:       // 待下载
			// 如果状态为下载中、获取信息中或待下载，修改为暂停状态
			err := m.db.BadgerDB.Badger().Update(func(txn *badger.Txn) error {
				downloadFile.Status = pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED // 已暂停
				return downloadTaskStore.UpdateTx(txn, downloadFile)
			})
			if err != nil {
				logger.Errorf("更新文件状态为暂停失败: taskID=%s, error=%v",
					downloadFile.TaskId, err)
				continue
			}

		// TODO: 已完成的不应该再次加入
		case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED: // 已完成
			continue

		case pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED: // 已暂停
			// 已完成或已暂停状态的任务无需修改
			// logger.Infof("文件状态为已完成或已暂停，无需修改: taskID=%s, status=%s",
			// 	downloadFile.TaskId, downloadFile.Status)

		default:
			// 其他状态（未指定、失败等）修改为异常状态
			err := m.db.BadgerDB.Badger().Update(func(txn *badger.Txn) error {
				downloadFile.Status = pb.DownloadStatus_DOWNLOAD_STATUS_FAILED // 失败
				return downloadTaskStore.UpdateTx(txn, downloadFile)
			})
			if err != nil {
				logger.Errorf("更新文件状态为失败状态失败: taskID=%s, error=%v",
					downloadFile.TaskId, err)
				continue
			}
		}

		// 移除指定任务ID的下载任务，如果存在
		m.removeTask(downloadFile.TaskId)

		// 创建新的下载任务实例
		downloadTask, err := NewDownloadTask(
			m.ctx,
			m.opt,
			m.db,
			m.fs,
			m.host,
			m.routingTable,
			m.nps,
			m.statusChan,
			m.errChan,
			downloadFile.TaskId,
		)
		if err != nil {
			logger.Errorf("创建下载任务失败: taskID=%s, error=%v",
				downloadFile.TaskId, err)
			continue
		}

		// 添加一个新的下载任务
		if err := m.addTask(downloadTask); err != nil {
			continue
		}

		// logger.Infof("已加载下载任务: taskID=%s", downloadFile.TaskId)
	}

	var taskCount int
	m.tasks.Range(func(_, _ interface{}) bool {
		taskCount++
		return true
	})
	// logger.Infof("成功加载 %d 个下载任务", taskCount)

	return nil
}
