package downloads

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// DownloadManager 管理所有下载任务，提供文件下载的统一入口和管理功能
// 它负责协调下载任务的执行，管理任务的生命周期，以及通知任务的状态更新和错误事件
type DownloadManager struct {
	ctx             context.Context          // 上下文用于管理协程的生命周期
	cancel          context.CancelFunc       // 取消函数
	Mu              sync.Mutex               // 用于保护状态的互斥锁
	Tasks           map[string]*DownloadTask // 下载任务的映射表
	DownloadChan    chan *DownloadChan       // 下载状态更新通道，用于通知外部下载进度和状态
	SaveTasksToFile chan struct{}            // 保存任务至文件通道
	AsyncDownload   chan *AsyncDownload      // 需要异步下载的文件片段信息
}

type NewDownloadManagerInput struct {
	fx.In
	LC  fx.Lifecycle
	Ctx context.Context // 全局上下文
	Opt *opts.Options   // 文件存储选项配置
	Afe afero.Afero     // 文件系统接口
	P2P *dep2p.DeP2P    // 网络主机
}

type NewDownloadManagerOutput struct {
	fx.Out
	Download *DownloadManager // 管理所有下载会话
}

// NewDownloadManager 创建并初始化一个新的 DownloadManager 实例。
// 参数：
//   - input: NewDownloadManagerInput 用于初始化 DownloadManager 的输入结构体。
//
// 返回值：
//   - NewDownloadManagerOutput: 包含 DownloadManager 的输出结构体。
func NewDownloadManager(input NewDownloadManagerInput) (out NewDownloadManagerOutput) {
	ctx, cancel := context.WithCancel(input.Ctx)
	download := &DownloadManager{
		ctx:             ctx,                            // 初始化上下文
		cancel:          cancel,                         // 初始化取消函数
		Mu:              sync.Mutex{},                   // 初始化互斥体
		Tasks:           make(map[string]*DownloadTask), // 初始化任务映射表
		DownloadChan:    make(chan *DownloadChan),       // 下载状态更新通道
		SaveTasksToFile: make(chan struct{}, 1),         // 保存任务至文件通道，缓冲区大小为1，只保存最新的信息
		AsyncDownload:   make(chan *AsyncDownload, 10),  // 需要异步下载的文件片段信息
	}

	filePath := filepath.Join(paths.GetRootPath(), paths.GetDownloadPath(), "tasks") // 设置子目录
	// 加载任务
	tasks, err := LoadTasksFromFile(filePath)
	if err == nil {
		for id, taskSerializable := range tasks {
			task := &DownloadTask{}
			// 从可序列化的结构体恢复
			if err := task.FromSerializable(taskSerializable); err != nil {
				logrus.Errorf("[%s]从可序列化的结构体恢复失败: %v", debug.WhereAmI(), err)
				continue
			}

			download.Tasks[id] = task
		}
	}

	out.Download = download

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 应用启动时的逻辑，例如初始化资源、启动后台服务等
			logrus.Println("下载管理器已启动")
			// 启动定时保存任务的定时器
			go out.Download.PeriodicSave(filePath, time.Minute)

			// 启动通道事件
			go out.Download.ChannelEvents(input.Opt, input.Afe, input.P2P)

			// 保存任务至文件
			go out.Download.SaveTasksToFileSingleChan()

			return nil
		},

		OnStop: func(ctx context.Context) error {
			// 应用停止时的逻辑，例如释放资源、停止后台服务等
			logrus.Println("下载管理器正在停止")
			out.Download.cancel() // 调用取消函数，确保所有协程被正确终止

			// 保存任务至文件
			go out.Download.SaveTasksToFileSingleChan()

			return nil
		},
	})

	return out
}

// PeriodicSave 定时保存任务数据到文件
// 参数：
//   - filePath: string 文件路径
//   - interval: time.Duration 保存间隔
func (manager *DownloadManager) PeriodicSave(filePath string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-manager.ctx.Done():
			return
		case <-ticker.C:
			// 保存任务数据到文件
			go manager.saveTasks(filePath)

		case <-manager.SaveTasksToFile:
			// 保存任务数据到文件
			go manager.saveTasks(filePath)
		}
	}
}

// ChannelEvents 通道事件
func (manager *DownloadManager) ChannelEvents(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P) {
	go func() {
		for {
			select {
			case asyncDownload := <-manager.AsyncDownload:

				// 处理异步下载
				go manager.handleAsyncDownload(opt, afe, p2p, asyncDownload)

			case <-manager.ctx.Done():
				return
			}
		}
	}()
}

// RegisterTask 向管理器注册一个新的下载任务。
// 参数：
//   - task: *DownloadTask 为准备注册的下载任务。
func (manager *DownloadManager) RegisterTask(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, task *DownloadTask) {
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	if _, exists := manager.Tasks[task.TaskID]; !exists {
		manager.Tasks[task.TaskID] = task
		logrus.Printf("添加任务: %s 成功。\n", task.TaskID)

		// 启动通道事件处理
		go task.ChannelEvents(opt, afe, p2p, pubsub, manager)

		// 启动定时任务，检查是否需要下载新的索引清单
		go task.CheckForNewChecklist()

		// 启动定时任务，检查是否需要下载文件片段
		go task.CheckForDownSnippet()

		// 启动定时任务，检查是否需要合并文件
		go task.CheckForMergeFiles()

	} else {
		logrus.Printf("任务: %s 已存在。\n", task.TaskID)
	}
}

// saveTasks 保存任务数据到文件
// 参数：
//   - filePath: string 文件路径
func (manager *DownloadManager) saveTasks(filePath string) {
	manager.Mu.Lock()
	tasks := make(map[string]*DownloadTaskSerializable)
	for id, task := range manager.Tasks {
		task, err := task.ToSerializable()
		if err != nil {
			logrus.Errorf("[%s]序列化结构体失败: %v", debug.WhereAmI(), err)
			continue
		}
		tasks[id] = task
	}
	manager.Mu.Unlock()

	// 将任务保存到文件
	if err := SaveTasksToFile(filePath, tasks); err != nil {
		logrus.Errorf("[%s]保存任务失败: %v", debug.WhereAmI(), err)
	}
}

// SaveTasksToFileChan 保存任务至文件的通知通道
func (manager *DownloadManager) SaveTasksToFileSingleChan() {
	select {
	case manager.SaveTasksToFile <- struct{}{}:
	default:
		// 如果通道已满，丢弃旧消息再写入新消息
		<-manager.SaveTasksToFile
		manager.SaveTasksToFile <- struct{}{}
	}
}

// ReceivePendingSegments 用于接收未回复的文件片段并将其传递给异步下载通道
// 参数：
//   - requesterAddress string 请求方节点地址
//   - pendingSegments map[int]string 剩余的文件片段（索引 -> 文件片段ID）
func (manager *DownloadManager) ReceivePendingSegments(downloadMaximumSize int64, requesterAddress string, taskID, fileID string, segments map[int]string) {
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	targetPeer, err := peer.Decode(requesterAddress)
	if err != nil {
		return
	}

	asyncDownload := &AsyncDownload{
		downloadMaximumSize: downloadMaximumSize,
		receiver:            targetPeer,
		taskID:              taskID,
		fileID:              fileID,
		segments:            segments,
	}

	if len(asyncDownload.segments) > 0 {
		manager.AsyncDownload <- asyncDownload
	}
}

// GetDownloadChan 返回下载状态更新通道
func (manager *DownloadManager) GetDownloadChan() chan *DownloadChan {
	return manager.DownloadChan
}
