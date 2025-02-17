package uploads

import (
	"context"
	"fmt"
	"sync"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/shamir"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/pointsub"
	"github.com/dep2p/pubsub"
	"go.uber.org/fx"
)

const (
	MaxSessions = 3 // 允许的最大并发会话数
)

// UploadManager 管理所有上传任务，提供文件上传的统一入口和管理功能
// 它负责协调上传任务的执行，管理任务的生命周期，以及通知任务的状态更新和错误事件
type UploadManager struct {
	ctx    context.Context    // 上下文，用于管理 UploadManager 的生命周期和取消操作
	cancel context.CancelFunc // 取消函数，用于取消上下文，停止所有相关的goroutine
	mu     sync.Mutex         // 互斥锁，用于保护并发访问共享资源，确保线程安全
	opt    *fscfg.Options     // 文件存储选项配置，包含各种存储相关的设置参数
	db     *database.DB       // 数据库存储，用于持久化上传任务和相关元数据
	fs     afero.Afero        // 文件系统接口，提供跨平台的文件操作能力
	host   host.Host          // libp2p网络主机实例
	ps     *pointsub.PointSub // 点对点传输实例
	nps    *pubsub.NodePubSub // 发布订阅系统，用于节点之间的消息传递

	scheme          *shamir.ShamirScheme      // Shamir秘密共享方案，用于文件加密和分片
	tasks           sync.Map                  // 上传任务映射，存储所有正在进行的上传任务
	segmentStatuses map[string]*SegmentStatus // 使用 taskID 作为键存储状态

	uploadChan  chan string                 // 上传操作通知通道，传递任务ID以触发新的上传任务
	forwardChan chan *pb.FileSegmentStorage // 用于接收转发请求的通道
	statusChan  chan *pb.UploadChan         // 上传状态和进度通知通道，用于实时更新上传进度
	errChan     chan error                  // 错误通道，用于将错误信息传递到外部
}

// Context 获取任务的上下文
// 返回值:
//   - context.Context: 任务的上下文对象
func (m *UploadManager) Context() context.Context {
	return m.ctx
}

// Cancel 获取任务的取消函数
// 返回值:
//   - context.CancelFunc: 任务的取消函数
func (m *UploadManager) Cancel() context.CancelFunc {
	return m.cancel
}

// Options 返回文件存储选项配置
// 返回值:
//   - *fscfg.Options: 文件存储选项配置
func (m *UploadManager) Options() *fscfg.Options {
	return m.opt
}

// DB 返回数据库存储
// 返回值:
//   - *badgerhold.Store: 数据库存储
func (m *UploadManager) DB() *database.DB {
	return m.db
}

// FS 返回文件系统接口
// 返回值:
//   - afero.Afero: 文件系统接口
func (m *UploadManager) FS() afero.Afero {
	return m.fs
}

// Host 获取网络主机实例
// 返回值:
//   - host.Host: 网络主机实例
func (m *UploadManager) Host() host.Host {
	return m.host
}

// PS 获取点对点传输实例
// 返回值:
//   - *pointsub.PointSub: 点对点传输实例
func (m *UploadManager) PS() *pointsub.PointSub {
	return m.ps
}

// NodePubSub 返回发布订阅系统

// NodePubSub 返回发布订阅系统
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统
func (m *UploadManager) NodePubSub() *pubsub.NodePubSub {
	return m.nps
}

// StatusChan 返回上传状态和进度通知通道
// 返回值:
//   - <-chan *pb.UploadChan: 只读的通道，用于接收上传状态和进度通知
func (m *UploadManager) StatusChan() <-chan *pb.UploadChan {
	return m.statusChan
}

// ErrChan 返回错误通知通道
// 返回值:
//   - <-chan error: 只读的通道，用于接收错误通知
func (m *UploadManager) ErrChan() <-chan error {
	return m.errChan
}

// addTask 添加一个新的上传任务(内部方法)
// 参数:
//   - task: 要添加的上传任务
//
// 返回值:
//   - error: 如果添加过程中出现错误，返回相应的错误信息
func (m *UploadManager) addTask(task *UploadTask) error {
	// 检查任务是否已存在
	if _, exists := m.tasks.Load(task.TaskID()); exists {
		logger.Errorf("添加任务失败: 任务ID %s 已存在", task.TaskID())
		return fmt.Errorf("任务ID %s 已存在", task.TaskID())
	}

	// 存储任务
	m.tasks.Store(task.TaskID(), task)
	logger.Infof("成功添加任务: taskID=%s", task.TaskID())
	return nil
}

// getTask 获取指定任务ID的上传任务(内部方法)
// 参数:
//   - taskID: 任务的唯一标识符
//
// 返回值:
//   - *UploadTask: 如果找到则返回 UploadTask 实例，否则为 nil
//   - bool: 如果找到则为 true，否则为 false
func (m *UploadManager) getTask(taskID string) (*UploadTask, bool) {
	task, exists := m.tasks.Load(taskID)
	if !exists {
		return nil, false
	}
	return task.(*UploadTask), true
}

// removeTask 移除指定任务ID的上传任务(内部方法)
// 参数:
//   - taskID: 任务的唯一标识符
func (m *UploadManager) removeTask(taskID string) {
	if task, exists := m.tasks.Load(taskID); exists {
		task.(*UploadTask).Close() // 关闭任务并清理资源
		m.tasks.Delete(taskID)
		logger.Infof("成功移除任务: %s", taskID)
	}
}

// NewUploadManagerInput 定义了创建 UploadManager 所需的输入参数
type NewUploadManagerInput struct {
	fx.In

	Ctx  context.Context    // 全局上下文，用于管理整个应用的生命周期和取消操作
	Opt  *fscfg.Options     // 文件存储选项配置，包含各种系统设置和参数
	DB   *database.DB       // 持久化存储，用于本地数据的存储和检索
	FS   afero.Afero        // 文件文件系统接口，提供跨平台的文件操作能力统接口
	Host host.Host          // libp2p网络主机实例
	PS   *pointsub.PointSub // 点对点传输实例
	// RoutingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	NPS *pubsub.NodePubSub // 发布订阅系统，用于节点之间的消息传递
}

// NewUploadManagerOutput 定义了 NewUploadManager 函数的输出
type NewUploadManagerOutput struct {
	fx.Out

	Upload *UploadManager // 上传管理器，用于处理和管理文件上传任务，包括任务调度、状态跟踪等
}

// NewUploadManager 创建并初始化一个新的 UploadManager 实例
// 参数:
//   - lc: fx.Lifecycle 用于管理应用生命周期的对象
//   - input: NewUploadManagerInput 包含创建 UploadManager 所需的输入参数
//
// 返回值:
//   - out: NewUploadManagerOutput 包含创建的 UploadManager 实例
//   - err: error 如果创建过程中发生错误，则返回相应的错误信息
func NewUploadManager(lc fx.Lifecycle, input NewUploadManagerInput) (out NewUploadManagerOutput, err error) {
	// 创建一个新的上下文和取消函数
	ctx, cancel := context.WithCancel(input.Ctx)

	// 创建并初始化 UploadManager 实例
	upload := &UploadManager{
		ctx:    ctx,
		cancel: cancel,
		mu:     sync.Mutex{},
		opt:    input.Opt,
		nps:    input.NPS,
		db:     input.DB,
		fs:     input.FS,
		host:   input.Host,
		ps:     input.PS,
		// routingTable:    input.RoutingTable,
		tasks:           sync.Map{},
		segmentStatuses: make(map[string]*SegmentStatus),
		uploadChan:      make(chan string, 5),                   // 上传操作通知通道，传递任务ID以触发新的上传任务
		forwardChan:     make(chan *pb.FileSegmentStorage, 100), // 用于接收转发请求的通道
		statusChan:      make(chan *pb.UploadChan, 1),           // 上传状态和进度通知通道，用于实时更新上传进度
		errChan:         make(chan error, 1),                    // 初始化错误通道，设置合适的缓冲区大小
	}

	// 将创建的 UploadManager 实例赋值给输出
	out.Upload = upload
	return out, nil
}

// InitializeUploadManagerInput 定义了初始化 UploadManager 所需的输入参数
type InitializeUploadManagerInput struct {
	fx.In

	Upload *UploadManager
}

// InitializeUploadManager 初始化 UploadManager 并设置相关的生命周期钩子
// 参数:
//   - lc: fx.Lifecycle 用于管理应用生命周期的对象
//   - input: InitializeUploadManagerInput 包含初始化 UploadManager 所需的输入参数
//
// 返回值:
//   - error 如果初始化过程中发生错误，则返回相应的错误信息
func InitializeUploadManager(lc fx.Lifecycle, input InitializeUploadManagerInput) error {
	// 添加生命周期钩子
	lc.Append(fx.Hook{
		// 启动钩子
		OnStart: func(ctx context.Context) error {
			logger.Info("正在启动上传管理器...")

			// 启动通道事件处理
			go input.Upload.ManagerChannelEvents()

			// 加载有的上传任务
			if err := input.Upload.LoadExistingTasks(); err != nil {
				logger.Errorf("加载现有上传任务失败: %v", err)
				return err
			}

			// 记录上传管理器启动日志
			logger.Info("上传管理器已启动")
			return nil
		},

		// 停止钩子
		OnStop: func(ctx context.Context) error {
			// 记录上传管理器停止日志
			logger.Info("正在停止上传管理器...")
			// 取消上传管理器的上下文
			input.Upload.cancel()
			return nil
		},
	})

	return nil
}

// IsMaxConcurrencyReached 检查是否达到上传允许的最大并发数
// 返回值:
//   - bool: 如果达到最大并发数返回 true，否则返回 false
func (m *UploadManager) IsMaxConcurrencyReached() bool {
	activeCount := 0
	m.tasks.Range(func(_, value interface{}) bool {
		task := value.(*UploadTask)
		// 创建存储实例
		uploadFileStore := database.NewUploadFileStore(task.db)

		// 获取上传文件记录
		fileRecord, exists, err := uploadFileStore.GetUploadFile(task.taskId)
		if err != nil {
			logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", task.taskId, err)
			return false
		}
		if !exists {
			return false
		}

		if fileRecord.Status == pb.UploadStatus_UPLOAD_STATUS_UPLOADING {
			activeCount++
		}
		// 如果已经达到最大会话数,提前结束遍历
		return activeCount < MaxSessions
	})
	return activeCount >= MaxSessions
}

// LoadExistingTasks 从数据库加载现有的上传任务
// 返回值:
//   - error: 如果加载过程中发生错误，返回相应的错误信息
func (m *UploadManager) LoadExistingTasks() error {
	// 创建上传任务存储对象
	uploadTaskStore := database.NewUploadFileStore(m.db.BadgerDB)
	// 创建上传任务存储对象
	uploadSegmentStore := database.NewUploadSegmentStore(m.db.BadgerDB)
	// 从数据库加载现有的上传任务
	tasks, err := uploadTaskStore.ListUploadFiles()
	if err != nil {
		if err == badgerhold.ErrNotFound {
			logger.Info("没有找到现有的上传任务")
			return nil
		}
		logger.Errorf("加载上传任务时出错: %v", err)
		return err
	}

	// 如果成功加载任务，将它们添加到上传管理器中
	for _, uploadFile := range tasks {
		switch uploadFile.Status {
		case pb.UploadStatus_UPLOAD_STATUS_PENDING, pb.UploadStatus_UPLOAD_STATUS_UPLOADING:
			// 如果状态等于上传中修改为暂定
			if err := UpdateUploadFileStatus(m.db.BadgerDB, uploadFile.TaskId, pb.UploadStatus_UPLOAD_STATUS_PAUSED); err != nil {
				logger.Errorf("更新文件状态为暂定失败: %v", err)
				continue
			}

		case pb.UploadStatus_UPLOAD_STATUS_COMPLETED, pb.UploadStatus_UPLOAD_STATUS_PAUSED:
			// 如果状态等于已完成或者已暂停，不进行任何操作
			logger.Infof("文件状态已经是已完成或已暂停: taskID=%s", uploadFile.TaskId)

		case pb.UploadStatus_UPLOAD_STATUS_UNSPECIFIED, pb.UploadStatus_UPLOAD_STATUS_ENCODING, pb.UploadStatus_UPLOAD_STATUS_FAILED:
			// 如果状态等于上传中修改为文件异常
			if err := UpdateUploadFileStatus(m.db.BadgerDB, uploadFile.TaskId, pb.UploadStatus_UPLOAD_STATUS_FILE_EXCEPTION); err != nil {
				logger.Errorf("更新文件状态为异常失败: %v", err)
				continue
			}

		default:
			logger.Infof("其他状态正常: taskID=%s, status=%v", uploadFile.TaskId, uploadFile.Status)
		}

		if uploadFile.Status == pb.UploadStatus_UPLOAD_STATUS_COMPLETED {
			// 从数据库中删除任务
			if err := uploadTaskStore.DeleteUploadFile(uploadFile.TaskId); err != nil {
				logger.Errorf("从数据库删除任务失败: %v", err)
				return err
			}

			// 从数据库中删除任务
			if err := uploadSegmentStore.DeleteUploadSegmentByTaskID(uploadFile.TaskId); err != nil {
				logger.Errorf("从数据库删除任务片段失败: %v", err)
				return err
			}
			continue
		}

		// 先创建状态对象
		taskStatus := NewSegmentStatus(&m.mu)
		taskStatus.SetState(true)
		// 在回调函数中设置状态
		m.segmentStatuses[uploadFile.TaskId] = taskStatus

		m.TriggerUpload(uploadFile.TaskId, false)

		logger.Infof("已加载上传任务: taskID=%s", uploadFile.TaskId)
	}

	var taskCount int
	m.tasks.Range(func(_, _ interface{}) bool {
		taskCount++
		return true
	})
	logger.Infof("成功加载 %d 个上传任务", taskCount)

	return nil
}

// GetErrChan 返回错误通道
// 返回值:
//   - <-chan error: 只读的通道，用于接收错误信息
func (m *UploadManager) GetErrChan() <-chan error {
	return m.errChan
}
