package uploads

import (
	"context"
	"math/big"
	"path/filepath"
	"sync"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/shamir"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// UploadManager 管理所有上传任务，提供文件上传的统一入口和管理功能
// 它负责协调上传任务的执行，管理任务的生命周期，以及通知任务的状态更新和错误事件
type UploadManager struct {
	ctx             context.Context        // 上下文用于管理协程的生命周期
	cancel          context.CancelFunc     // 取消函数
	Mu              sync.Mutex             // 用于保护状态的互斥锁
	Tasks           map[string]*UploadTask // 上传任务的映射表，键为任务ID，值为上传任务对象
	UploadChan      chan *UploadChan       // 上传状态更新通道，用于通知外部上传进度和状态
	SaveTasksToFile chan struct{}          // 保存任务至文件通道
	Scheme          *shamir.ShamirScheme   // 创建一个新的ShamirScheme实例
}

type NewUploadManagerInput struct {
	fx.In
	LC  fx.Lifecycle
	Ctx context.Context // 全局上下文
}

type NewUploadManagerOutput struct {
	fx.Out
	Upload *UploadManager // 管理所有上传会话
}

// NewUploadManager 创建并初始化一个新的UploadManager实例
func NewUploadManager(input NewUploadManagerInput) (out NewUploadManagerOutput, err error) {
	ctx, cancel := context.WithCancel(input.Ctx)
	prime, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	upload := &UploadManager{
		ctx:             ctx,                                                   // 初始化上下文
		cancel:          cancel,                                                // 初始化取消函数
		Mu:              sync.Mutex{},                                          // 初始化互斥体
		Tasks:           make(map[string]*UploadTask),                          // 上传任务的映射表
		UploadChan:      make(chan *UploadChan),                                // 上传对外通道
		SaveTasksToFile: make(chan struct{}, 1),                                // 保存任务至文件通道，缓冲区大小为1，只保存最新的信息
		Scheme:          shamir.NewShamirScheme(TotalShares, Threshold, prime), // 创建一个新的ShamirScheme实例
	}

	filePath := filepath.Join(paths.GetRootPath(), paths.GetUploadPath(), "tasks") // 设置子目录
	// 加载任务
	tasks, err := LoadTasksFromFile(filePath)
	if err == nil {
		for id, taskSerializable := range tasks {
			task := &UploadTask{}
			task.FromSerializable(taskSerializable)
			upload.Tasks[id] = task
		}
	}

	out.Upload = upload

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 应用启动时的逻辑，例如初始化资源、启动后台服务等
			logrus.Println("上传管理器已启动")
			// 启动定时保存任务的定时器
			go out.Upload.PeriodicSave(filePath, time.Minute)

			// 保存任务
			go out.Upload.SaveTasksToFileSingleChan()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 应用停止时的逻辑，例如释放资源、停止后台服务等
			logrus.Println("上传管理器正在停止")
			out.Upload.cancel() // 调用取消函数，确保所有协程被正确终止

			// 保存任务
			go out.Upload.SaveTasksToFileSingleChan()

			return nil
		},
	})

	return out, nil
}

// IsMaxConcurrencyReached 检查是否达到上传允许的最大并发数
func (manager *UploadManager) IsMaxConcurrencyReached() bool {
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	activeCount := 0
	for _, task := range manager.Tasks {
		if task.Status == StatusPending || task.Status == StatusUploading {
			activeCount++
		}
		if activeCount >= MaxSessions {
			return true
		}
	}
	return false
}

// RegisterTask 向管理器注册一个新的上传任务。
// 参数：
//   - task: *DownloadTask 为准备注册的上传任务。
func (manager *UploadManager) RegisterTask(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, task *UploadTask) {
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	if _, exists := manager.Tasks[task.TaskID]; !exists {
		manager.Tasks[task.TaskID] = task
		logrus.Printf("添加任务: %s 成功。\n", task.TaskID)

		// 启动通道事件处理
		go task.ChannelEvents(opt, afe, p2p, pubsub, manager.UploadChan)

		// 定时任务，发送数据到网络
		go task.PeriodicSend()

		// 通知准备好本地存储文件片段
		go task.SegmentReadySingleChan()

	} else {
		logrus.Printf("任务: %s 已存在。\n", task.TaskID)
	}
}

// PeriodicSave 定时保存任务数据到文件
// 参数：
//   - filePath: string 文件路径
//   - interval: time.Duration 保存间隔
func (manager *UploadManager) PeriodicSave(filePath string, interval time.Duration) {
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

// saveTasks 保存任务数据到文件
// 参数：
//   - filePath: string 文件路径
func (manager *UploadManager) saveTasks(filePath string) {
	manager.Mu.Lock()
	tasks := make(map[string]*UploadTaskSerializable)
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
func (manager *UploadManager) SaveTasksToFileSingleChan() {
	select {
	case manager.SaveTasksToFile <- struct{}{}:
	default:
		// 如果通道已满，丢弃旧消息再写入新消息
		<-manager.SaveTasksToFile
		manager.SaveTasksToFile <- struct{}{}
	}
}

// GetUploadChan 返回上传状态更新通道
func (manager *UploadManager) GetUploadChan() chan *UploadChan {
	return manager.UploadChan
}
