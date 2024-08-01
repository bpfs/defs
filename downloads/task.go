package downloads

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/network"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/defs/wallets"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

const (
	// 定时任务，索引清单相关时间参数
	CheckForNewChecklistInitialDelay   = 1 * time.Second  // 初次调用的延迟时间
	CheckForNewChecklistInterval       = 40 * time.Second // 定时循环的常规间隔时间
	CheckForNewChecklistContextTimeout = 60 * time.Minute // 上下文超时时间

	// 定时任务，下载文件片段相关时间参数
	CheckForDownSnippetInterval       = 1 * time.Minute   // 定时循环的常规间隔时间
	CheckForDownSnippetContextTimeout = 180 * time.Minute // 上下文超时时间

	// 定时任务，合并文件相关时间参数
	CheckForMergeFilesInterval       = 10 * time.Second  // 定时循环的常规间隔时间
	CheckForMergeFilesContextTimeout = 200 * time.Minute // 上下文超时时间

	// 通道操作的超时设置
	TickerChannelTimeout = 20 * time.Second // 定时任务通道操作的超时时间
	TickerDownTimeout    = 50 * time.Second // 文件片段下载通道的超时时间

	// 通过令牌桶机制限制同时发送的片段数量，避免通道满载和过多并发请求导致的压力。
	TickerChannelBufferSize     = 20                     // 通道缓冲区大小
	EventDownSnippetChannelSize = 100                    // 通道容量常量
	TokenBucketSize             = 50                     // 令牌桶容量常量
	TokenBucketRefillInterval   = 100 * time.Millisecond // 令牌桶补充间隔
)

// DownloadTask 描述一个文件下载任务
type DownloadTask struct {
	ctx    context.Context    // 上下文用于管理协程的生命周期
	cancel context.CancelFunc // 取消函数

	rwmu sync.RWMutex // 控制对Progress的并发访问的读写互斥锁

	TaskID       string            // 任务唯一标识
	File         *DownloadFile     // 待下载的文件信息
	TotalPieces  int               // 文件总片数（数据片段和纠删码片段的总数）
	DataPieces   int               // 数据片段的数量
	OwnerPriv    *ecdsa.PrivateKey // 所有者的私钥
	Secret       []byte            // 文件加密密钥
	UserPubHash  []byte            // 用户的公钥哈希
	Progress     util.BitSet       // 下载任务的进度，表示为0到100之间的百分比
	CreatedAt    int64             // 任务创建的时间戳
	UpdatedAt    int64             // 最后一次下载成功的时间戳
	MergeCounter int               // 用于跟踪文件合并操作的计数器

	TickerChecklist   chan struct{} // 定时任务，通知检查是否需要下载新的索引清单的通道
	TickerDownSnippet chan struct{} // 定时任务，通知检查是否需要下载新的文件片段的通道
	TickerMergeFile   chan struct{} // 定时任务，通知检查是否需要执行文件合并操作的通道

	ChecklistDone   chan struct{} // 用于通知索引清单完成，关闭定时任务的通道
	DownSnippetDone chan struct{} // 用于通知下载片段完成，关闭定时任务的通道
	MergeFileDone   chan struct{} // 用于通知文件合并完成，关闭定时任务的通道

	EventChecklist   chan struct{} // 通道事件，通知下载新的索引清单的通道
	EventDownSnippet chan int      // 通道事件，通知下载新的文件片段的通道
	EventMergeFile   chan struct{} // 通道事件，通知执行文件合并操作的通道

	DownloadTaskDone chan struct{} // 用于通知文件下载任务完成的通道

	DownloadStatus DownloadStatus // 下载任务的状态
	StatusCond     *sync.Cond     // 用于状态变化的条件变量

	ChecklistTimeout    bool       `optional:"false" default:"false"` // 索引清单超时，默认为 false，且为必填项
	ChecklistStatusCond *sync.Cond // 用于索引清单状态变化的条件变量

	DownSnippetTimeout    bool       `optional:"false" default:"false"` // 下载片段超时，默认为 false，且为必填项
	DownSnippetStatusCond *sync.Cond // 用于片段下载状态变化的条件变量
}

// NewDownloadTask 创建并初始化一个新的DownloadTask实例。
// 参数：
//   - ctx: context.Context 上下文用于管理协程的生命周期。
//   - mu: *sync.Mutex 互斥锁用于保护状态。
//   - taskID: string 任务的唯一标识符。
//   - fileID: string 待下载的文件信息。
//   - ownerPriv: *ecdsa.PrivateKey 所有者的私钥。
//
// 返回值：
//   - *DownloadTask: 新创建的DownloadTask实例。
//   - error: 如果发生错误，返回错误信息。
func NewDownloadTask(ctx context.Context, taskID string, fileID string, ownerPriv *ecdsa.PrivateKey) (*DownloadTask, error) {
	// 使用私钥和文件校验和生成秘密
	secret, err := util.GenerateSecretFromPrivateKeyAndChecksum(ownerPriv, []byte(fileID))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 通过私钥生成公钥哈希
	pubKeyHash, ok := wallets.PrivateKeyToPublicKeyHash(ownerPriv) // 从ECDSA私钥中提取公钥哈希
	if !ok {
		return nil, fmt.Errorf("通过私钥生成公钥哈希时失败")
	}

	c, cancel := context.WithCancel(ctx)

	downloadFile := &DownloadFile{
		FileID:   fileID,     // 文件唯一标识
		Segments: sync.Map{}, // 初始化文件分片信息
	}

	task := &DownloadTask{
		ctx:    c,      // 上下文用于管理协程的生命周期
		cancel: cancel, // 取消函数

		rwmu: sync.RWMutex{}, // 初始化读写互斥锁

		TaskID:       taskID,             // 任务唯一标识
		File:         downloadFile,       // 待下载的文件信息
		TotalPieces:  0,                  // 文件总片数，初始时未知
		DataPieces:   0,                  // 数据片段的数量，初始时未知
		OwnerPriv:    ownerPriv,          // 所有者的私钥
		Secret:       secret,             // 文件加密密钥
		UserPubHash:  pubKeyHash,         // 用户的公钥哈希
		Progress:     *util.NewBitSet(0), // 下载任务的进度
		CreatedAt:    time.Now().Unix(),  // 任务创建的时间戳
		UpdatedAt:    time.Time{}.Unix(), // 最后一次下载成功的时间戳
		MergeCounter: 0,                  // 用于跟踪文件合并操作的计数器

		TickerChecklist:   make(chan struct{}, TickerChannelBufferSize), // 初始化用于检查是否需要下载新的索引清单的通道，缓冲区大小为1，只保存最新的信息
		TickerDownSnippet: make(chan struct{}, TickerChannelBufferSize), // 初始化用于检查是否需要下载新的文件片段的通道，缓冲区大小为1，只保存最新的信息
		TickerMergeFile:   make(chan struct{}, TickerChannelBufferSize), // 初始化用于检查是否需要执行文件合并操作的通道，缓冲区大小为1，只保存最新的信息

		ChecklistDone:   make(chan struct{}, TickerChannelBufferSize), // 初始化用于通知索引清单完成，关闭定时任务的通道，缓冲区大小为1，只保存最新的信息
		DownSnippetDone: make(chan struct{}, TickerChannelBufferSize), // 初始化用于通知下载片段完成，关闭定时任务的通道，缓冲区大小为1，只保存最新的信息
		MergeFileDone:   make(chan struct{}, TickerChannelBufferSize), // 初始化用于通知文件合并完成，关闭定时任务的通道，缓冲区大小为1，只保存最新的信息

		EventChecklist:   make(chan struct{}, TickerChannelBufferSize), // 初始化用于通知下载新的索引清单的通道，缓冲区大小为1，只保存最新的信息
		EventDownSnippet: make(chan int, EventDownSnippetChannelSize),  // 初始化用于通知下载新的文件片段的通道，缓冲区大小为100
		EventMergeFile:   make(chan struct{}, TickerChannelBufferSize), // 初始化用于通知执行文件合并操作的通道，缓冲区大小为1，只保存最新的信息
		DownloadTaskDone: make(chan struct{}, TickerChannelBufferSize), // 初始化用于通知文件下载任务完成的通道，缓冲区大小为1，只保存最新的信息

		DownloadStatus:     StatusPending, // 初始化下载任务的当前状态为"待下载"
		ChecklistTimeout:   false,         // 初始化用于索引清单状态变化的条件变量
		DownSnippetTimeout: false,         // 初始化用于片段下载状态变化的条件变量
	}

	task.StatusCond = sync.NewCond(&task.rwmu)            // 初始化用于状态变化的条件变量
	task.ChecklistStatusCond = sync.NewCond(&task.rwmu)   // 初始化用于索引清单状态变化的条件变量
	task.DownSnippetStatusCond = sync.NewCond(&task.rwmu) // 初始化用于片段下载状态变化的条件变量

	return task, nil
}

// ChannelEvents 通道事件处理
// 参数：
//   - opt: *opts.Options 文件存储选项配置，用于指定文件存储相关的选项。
//   - afe: afero.Afero 文件系统接口，用于文件操作。
//   - p2p: *dep2p.DeP2P DeP2P 网络主机，用于网络操作。
//   - pubsub: *pubsub.DeP2PPubSub DeP2P 网络订阅系统，用于发布和订阅消息。
//   - manager: *DownloadManager 管理下载任务的管理器，用于管理下载任务的相关操作。
func (task *DownloadTask) ChannelEvents(
	opt *opts.Options, // 文件存储选项配置
	afe afero.Afero, // 文件系统接口
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	manager *DownloadManager, // 管理下载任务
) {
	for {
		select {
		case <-task.ctx.Done():
			return

		case <-task.TickerChecklist:
			// 通知检查是否需要下载新的索引清单的通道
			task.ChannelEventsTickerChecklist()

		case <-task.TickerDownSnippet:
			// 通知检查是否需要下载新的文件片段的通道
			task.ChannelEventsTickerDownSnippet()

		case <-task.TickerMergeFile:
			// 通知检查是否需要执行文件合并操作的通道
			task.ChannelEventsTickerMergeFile()

		case <-task.EventChecklist:
			// 通知下载新的索引清单的通道
			task.ChannelEventsEventChecklist(p2p, pubsub)

		case index := <-task.EventDownSnippet:
			// 通知下载新的文件片段的通道
			task.ChannelEventsEventDownSnippet(opt, afe, p2p, pubsub, opt.GetDownloadMaximumSize(), index, manager.DownloadChan)

		case <-task.EventMergeFile:
			// 通知执行文件合并操作的通道
			task.ChannelEventsEventMergeFile(opt, afe, p2p, manager.DownloadChan)

		case <-task.DownloadTaskDone:
			manager.SaveTasksToFileSingleChan() // 保存任务至文件的通知通道

			// 通知文件下载任务完成
			task.cancel()                      // 取消任务，确保所有子 Goroutine 退出
			time.Sleep(100 * time.Millisecond) // 适当延时退出
			return
		}
	}
}

// CheckForNewChecklist 定时任务，检查是否需要下载新的索引清单。
func (task *DownloadTask) CheckForNewChecklist() {
	// 创建一个60分钟超时的上下文
	ctx, cancel := context.WithTimeout(task.ctx, CheckForNewChecklistContextTimeout)
	defer cancel()

	// 初始化定时器，初始延迟时间为1秒
	ticker := time.NewTicker(CheckForNewChecklistInitialDelay)
	defer ticker.Stop()

	for {
		select {
		case <-task.ctx.Done():
			return

		case <-ctx.Done():
			logrus.Warnf("定时任务超时 %v ，退出下载索引清单子进程", CheckForNewChecklistContextTimeout)
			return

		case <-task.ChecklistDone:
			logrus.Info("索引清单完成，退出预处理子进程")
			return

		case <-ticker.C:
			// 修改定时器为常规间隔时间为40秒
			ticker.Reset(CheckForNewChecklistInterval)

			// 向任务的通知检查是否需要下载新的索引清单的通道发送通知
			task.TickerChecklistSingleChan(ctx)
		}
	}
}

// CheckForDownSnippet 定时任务，检查是否需要下载文件片段。
func (task *DownloadTask) CheckForDownSnippet() {
	// 创建一个180分钟超时的上下文
	ctx, cancel := context.WithTimeout(task.ctx, CheckForDownSnippetContextTimeout)
	defer cancel()

	ticker := time.NewTicker(CheckForDownSnippetInterval) // 设置1分钟的定时循环
	defer ticker.Stop()

	for {
		select {
		case <-task.ctx.Done():
			return

		case <-ctx.Done():
			logrus.Warnf("定时任务超时 %v ，退出下载文件片段子进程", CheckForDownSnippetContextTimeout)
			return

		case <-task.DownSnippetDone:
			logrus.Info("片段下载完成，退出下载子进程")
			return

		case <-ticker.C:
			// 向任务的通知检查是否需要下载新的文件片段的通道发送通知
			task.TickerDownSnippetSingleChan(ctx)
		}
	}
}

// CheckForMergeFiles 定时任务，检查是否需要合并文件。
func (task *DownloadTask) CheckForMergeFiles() {
	// 创建一个200分钟超时的上下文
	ctx, cancel := context.WithTimeout(task.ctx, CheckForMergeFilesContextTimeout)
	defer cancel()

	ticker := time.NewTicker(CheckForMergeFilesInterval)
	defer ticker.Stop()

	for {
		select {
		case <-task.ctx.Done():
			return

		case <-task.MergeFileDone:
			logrus.Info("文件合并完成，退出合并子进程")
			return

		case <-ticker.C:
			// 向任务的通知检查是否需要执行文件合并操作的通道发送通知
			task.TickerMergeFileSingleChan(ctx)
		}
	}
}

// TickerChecklistSingleChan 向任务的通知检查是否需要下载新的索引清单的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) TickerChecklistSingleChan(parentCtx context.Context) {
	go func() {
		timeoutCtx, cancel := context.WithTimeout(parentCtx, TickerChannelTimeout)
		defer cancel()

		select {
		case <-timeoutCtx.Done():
			// 超时处理，记录日志并退出，不需要再操作通道
			// logrus.Warnf("[TickerChecklistSingleChan] 处理超时")
			return

		// 通知检查是否需要下载新的索引清单
		case task.TickerChecklist <- struct{}{}:
			logrus.Infof("[检查索引清单] 发送通知成功")

		default:
			// 如果通道已满，记录日志，丢弃旧消息再写入新消息
			// logrus.Warnf("[TickerChecklistSingleChan] 通道已满，丢弃旧消息")
			select {
			case <-task.TickerChecklist:
				// logrus.Infof("[检查索引清单] 丢弃旧消息成功")
			default:
			}
			select {
			case task.TickerChecklist <- struct{}{}:
				// logrus.Infof("[检查索引清单] 发送新通知成功")
			case <-timeoutCtx.Done():
				// logrus.Warnf("[TickerChecklistSingleChan] 处理超时")
			}
		}
	}()
}

// TickerDownSnippetSingleChan 向任务的通知检查是否需要下载新的文件片段的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) TickerDownSnippetSingleChan(parentCtx context.Context) {
	go func() {
		timeoutCtx, cancel := context.WithTimeout(parentCtx, TickerChannelTimeout)
		defer cancel()

		select {
		case <-timeoutCtx.Done():
			// 超时处理，记录日志并退出，不需要再操作通道
			// logrus.Warnf("[TickerDownSnippetSingleChan] 处理超时")
			return

		// 通知检查是否需要下载新的文件片段
		case task.TickerDownSnippet <- struct{}{}:
			// logrus.Infof("[检查文件片段] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.TickerDownSnippet:
			default:
			}
			task.TickerDownSnippet <- struct{}{}
			// logrus.Infof("[检查文件片段] 丢弃旧消息并发送新通知")
		}
	}()
}

// TickerMergeFileSingleChan 向任务的通知检查是否需要执行文件合并操作的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) TickerMergeFileSingleChan(parentCtx context.Context) {
	go func() {
		timeoutCtx, cancel := context.WithTimeout(parentCtx, TickerChannelTimeout)
		defer cancel()

		select {
		case <-timeoutCtx.Done():
			// 超时处理，记录日志并退出，不需要再操作通道
			// logrus.Warnf("[TickerMergeFileSingleChan] 处理超时")
			return

		// 通知检查是否需要执行文件合并操作
		case task.TickerMergeFile <- struct{}{}:
			// logrus.Infof("[检查文件合并] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.TickerMergeFile:
			default:
			}
			task.TickerMergeFile <- struct{}{}
			// logrus.Infof("[检查文件合并] 丢弃旧消息并发送新通知")
		}
	}()
}

// ChecklistDoneSingleChan 向任务的通知索引清单完成，关闭定时任务的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) ChecklistDoneSingleChan() {
	go func() {
		select {
		case task.ChecklistDone <- struct{}{}:
			// logrus.Infof("[索引清单完成] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.ChecklistDone:
			default:
			}
			task.ChecklistDone <- struct{}{}
			// logrus.Infof("[索引清单完成] 丢弃旧消息并发送新通知")
		}
	}()
}

// DownSnippetDoneSingleChan 向任务的通知下载片段完成，关闭定时任务的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) DownSnippetDoneSingleChan() {
	go func() {
		select {
		case task.DownSnippetDone <- struct{}{}:
			// logrus.Infof("[下载片段完成] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.DownSnippetDone:
			default:
			}
			task.DownSnippetDone <- struct{}{}
			// logrus.Infof("[下载片段完成] 丢弃旧消息并发送新通知")
		}
	}()
}

// MergeFileDoneSingleChan 向任务的通知文件合并完成，关闭定时任务的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) MergeFileDoneSingleChan() {
	go func() {
		select {
		case task.MergeFileDone <- struct{}{}:
			// logrus.Infof("[文件合并完成] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.MergeFileDone:
			default:
			}
			task.MergeFileDone <- struct{}{}
			// logrus.Infof("[文件合并完成] 丢弃旧消息并发送新通知")
		}
	}()
}

// EventChecklistSingleChan 向任务的通知下载新的索引清单的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) EventChecklistSingleChan() {
	go func() {
		select {
		case task.EventChecklist <- struct{}{}:
			// logrus.Infof("[下载新的索引清单] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.EventChecklist:
			default:
			}
			task.EventChecklist <- struct{}{}
			// logrus.Infof("[下载新的索引清单] 丢弃旧消息并发送新通知")
		}
	}()
}

// EventDownSnippetChan 向任务的通知下载新的文件片段的通道写入索引，并处理通道已满的情况。
// 参数：
//   - index: int 文件片段的索引。
func (task *DownloadTask) EventDownSnippetChan(index int) {
	go func() {
		success := false
		for !success {
			select {
			case task.EventDownSnippet <- index:
				// 非阻塞发送成功
				success = true

			case <-time.After(100 * time.Millisecond):
				// 延迟一段时间后重试
				// logrus.Warnf("通道已满，正在重试索引 %d", index)
			}
		}
	}()
}

// EventMergeFileSingleChan 向任务的通知执行文件合并操作的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) EventMergeFileSingleChan() {
	go func() {
		select {
		case task.EventMergeFile <- struct{}{}:
			// logrus.Infof("[执行文件合并操作] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.EventMergeFile:
			default:
			}
			task.EventMergeFile <- struct{}{}
			// logrus.Infof("[执行文件合并操作] 丢弃旧消息并发送新通知")
		}
	}()
}

// DownloadTaskDoneSingleChan 向任务的通知文件下载任务完成的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *DownloadTask) DownloadTaskDoneSingleChan() {
	go func() {
		select {
		case task.DownloadTaskDone <- struct{}{}:
			// logrus.Infof("[文件下载任务完成] 发送通知成功")

		default:
			// 如果通道已满，丢弃旧消息再写入新消息
			select {
			case <-task.DownloadTaskDone:
			default:
			}
			task.DownloadTaskDone <- struct{}{}
			// logrus.Infof("[文件下载任务完成] 丢弃旧消息并发送新通知")
		}
	}()
}

// SetDownloadStatus 设置下载任务的状态，并通知所有等待区块同步状态变化的goroutine。
func (task *DownloadTask) SetDownloadStatus(status DownloadStatus) {
	task.rwmu.Lock() // 使用写锁
	defer task.rwmu.Unlock()

	task.DownloadStatus = status
	task.StatusCond.Broadcast()
}

// GetDownloadStatus 获取当前下载任务的状态
func (task *DownloadTask) GetDownloadStatus() DownloadStatus {
	task.rwmu.RLock() // 使用读锁
	defer task.rwmu.RUnlock()
	return task.DownloadStatus
}

// WaitFoDownloadStatusChange 阻塞当前goroutine，直到下载任务的状态发生变化。
func (task *DownloadTask) WaitFoDownloadStatusChange(targetStatuses ...DownloadStatus) {
	util.WaitForStatusChange(task.StatusCond, func() bool {
		// 获取当前状态
		currentStatus := task.GetDownloadStatus()

		// 如果没有提供任何期望状态，那么任何状态变化都应该唤醒等待
		if len(targetStatuses) == 0 {
			return true // 返回true，结束等待
		}

		// 检查当前状态是否是期望状态之一
		for _, s := range targetStatuses {
			if currentStatus == s {
				return true // 返回true，结束等待
			}
		}

		// 如果当前状态不是任何期望状态，继续等待
		return false
	})
}

// SetChecklistTimeout 设置索引清单状态，并通知所有等待索引清单状态变化的goroutine。
func (task *DownloadTask) SetChecklistTimeout(status bool) {
	task.rwmu.Lock()
	defer task.rwmu.Unlock()

	task.ChecklistTimeout = status
	task.ChecklistStatusCond.Broadcast()
}

// GetChecklistTimeout 获取当前的索引清单状态。
func (task *DownloadTask) GetChecklistTimeout() bool {
	task.rwmu.RLock()
	defer task.rwmu.RUnlock()

	return task.ChecklistTimeout
}

// WaitForChecklistTimeoutChange 阻塞当前goroutine，直到索引清单状态发生变化。
func (task *DownloadTask) WaitForChecklistTimeoutChange() {
	task.rwmu.Lock()
	defer task.rwmu.Unlock()

	task.ChecklistStatusCond.Wait()
}

// WaitForSpecificChecklistTimeoutStatus 阻塞当前goroutine，直到索引清单状态发生变化并达到期望值。
func (task *DownloadTask) WaitForSpecificChecklistTimeoutStatus(targetStatus bool) {
	task.rwmu.Lock()
	defer task.rwmu.Unlock()

	for task.ChecklistTimeout != targetStatus {
		task.ChecklistStatusCond.Wait()
	}
	// 当循环退出时，说明状态已经是期望的值了，此时继续执行后续操作
}

// SetDownSnippetTimeout 设置片段下载状态，并通知所有等待片段下载状态变化的goroutine。
func (task *DownloadTask) SetDownSnippetTimeout(status bool) {
	task.rwmu.Lock()
	defer task.rwmu.Unlock()

	task.DownSnippetTimeout = status
	task.DownSnippetStatusCond.Broadcast()
}

// GetDownSnippetTimeout 获取当前的片段下载状态。
func (task *DownloadTask) GetDownSnippetTimeout() bool {
	task.rwmu.RLock()
	defer task.rwmu.RUnlock()

	return task.DownSnippetTimeout
}

// WaitForDownSnippetTimeoutChange 阻塞当前goroutine，直到片段下载状态发生变化。
func (task *DownloadTask) WaitForDownSnippetTimeoutChange() {
	task.rwmu.Lock()
	defer task.rwmu.Unlock()

	task.DownSnippetStatusCond.Wait()
}

// WaitForSpecificDownSnippetTimeoutStatus 阻塞当前goroutine，直到片段下载状态发生变化并达到期望值。
func (task *DownloadTask) WaitForSpecificDownSnippetTimeoutStatus(targetStatus bool) {
	task.rwmu.Lock()
	defer task.rwmu.Unlock()

	for task.DownSnippetTimeout != targetStatus {
		task.DownSnippetStatusCond.Wait()
	}
	// 当循环退出时，说明状态已经是期望的值了，此时继续执行后续操作
}

// ChannelEventsTickerChecklist 通道事件处理检查是否需要下载新的索引清单
func (task *DownloadTask) ChannelEventsTickerChecklist() {
	// 检查是否需要触发新的文件片段索引清单请求的逻辑
	if task.TotalPieces == 0 || task.File.SegmentCount() < task.DataPieces || task.File.CheckMissingNodes() {
		// 通知下载新的索引清单
		go task.EventChecklistSingleChan()
		return
	}

	// 通知索引清单已完成
	go task.ChecklistDoneSingleChan()
}

// ChannelEventsTickerDownSnippet 通道事件处理，检查是否需要下载新的文件片段
// 详细处理逻辑和设计机制如下：
// 1. 创建一个超时上下文，以防止方法执行时间过长。
// 2. 检查数据片段是否下载完成，如果完成则通知并退出。
// 3. 获取需要下载的文件片段索引，并通过令牌桶机制控制并发请求的数量。
// 4. 启动一个令牌桶填充协程，定期向令牌桶中添加令牌。
// 5. 遍历需要下载的文件片段索引，领取令牌后再发送下载通知。
// 6. 如果上下文超时或者任务上下文被取消，则退出当前处理。

func (task *DownloadTask) ChannelEventsTickerDownSnippet() {
	// 创建一个超时上下文，设定最大执行时间
	timeoutCtx, cancel := context.WithTimeout(task.ctx, TickerChannelTimeout)
	defer cancel() // 确保在方法返回前取消上下文

	// 检查数据片段是否已经下载完成
	if task.CheckDataSegmentsCompleted() {
		// 如果已经完成，通知下载片段已完成
		task.DownSnippetDoneSingleChan()
		return // 退出方法
	}

	// 获取需要下载的文件片段索引列表
	segmentsToDownload := task.File.GetSegmentsToDownload()

	// 创建一个令牌桶通道，容量为TokenBucketSize
	tokenBucket := make(chan struct{}, TokenBucketSize)

	// 启动一个协程，用于定期向令牌桶中添加令牌
	go func() {
		// 创建一个定时器，按照TokenBucketRefillInterval的间隔触发
		ticker := time.NewTicker(TokenBucketRefillInterval)
		defer ticker.Stop() // 确保在协程退出前停止定时器

		for {
			select {
			case <-ticker.C:
				// 每次定时器触发，尝试向令牌桶中添加一个令牌
				select {
				case tokenBucket <- struct{}{}: // 如果令牌桶未满，成功添加令牌
				default: // 如果令牌桶已满，忽略当前令牌
				}
			case <-task.ctx.Done():
				// 如果任务上下文被取消，退出协程
				return
			}
		}
	}()

	// 遍历需要下载的文件片段索引
	for _, segment := range segmentsToDownload {
		select {
		case <-tokenBucket:
			// 从令牌桶中领取一个令牌，表示可以处理一个下载任务
			task.EventDownSnippetChan(segment) // 通知下载新的文件片段，传递需要下载的片段索引
			time.Sleep(10 * time.Millisecond)  // 适当延时以缓解网络压力
		case <-task.ctx.Done():
			// 如果任务上下文被取消，退出方法
			return
		case <-timeoutCtx.Done():
			// 如果超时上下文超时，退出方法
			// logrus.Warnf("TickerDownSnippetSingleChan处理超时，退出当前任务")
			return
		}
	}
}

// ChannelEventsTickerMergeFile 通道事件处理检查是否需要执行文件合并操作
func (task *DownloadTask) ChannelEventsTickerMergeFile() {
	// task.rwmu.Lock()
	// defer task.rwmu.Unlock()

	// 检查是否需要合并文件的逻辑
	if task.Progress.All() {
		go task.EventMergeFileSingleChan() // 通知执行文件合并操作
	} else if task.File.DownloadCompleteCount() >= task.DataPieces {
		// 下载任务的状态不等于已完成，并且下载大于文件片段
		go task.EventMergeFileSingleChan() // 通知执行文件合并操作
	}
}

// ChannelEventsEventChecklist 通道事件处理下载新的索引清单
// 参数：
//   - p2p: *dep2p.DeP2P 表示 DeP2P 网络主机。
//   - pubsub: *pubsub.DeP2PPubSub 表示 DeP2P 网络订阅系统。
func (task *DownloadTask) ChannelEventsEventChecklist(
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
) {
	// 准备文件片段所在节点的映射
	segmentNodes := make(map[int][]peer.ID)
	task.File.Segments.Range(func(key, value interface{}) bool {
		index := key.(int)
		segment := value.(*FileSegment)

		// 文件片段的唯一标识以及校验和均不为空
		if segment.SegmentID != "" && len(segment.Checksum) != 0 {
			nodeIDs := make([]peer.ID, 0)
			segment.Nodes.Range(func(nodeKey, nodeValue interface{}) bool {
				nodeIDs = append(nodeIDs, nodeKey.(peer.ID))
				return true
			})
			segmentNodes[index] = nodeIDs
		}
		return true
	})

	// 描述请求文件片段清单的参数
	segmentListRequest := SegmentListRequest{
		TaskID:       task.TaskID,      // 任务唯一标识
		FileID:       task.File.FileID, // 文件唯一标识
		UserPubHash:  task.UserPubHash, // 用户的公钥哈希
		SegmentNodes: segmentNodes,     // 文件片段所在节点
	}

	// 向指定的全网节点发送文件下载请求订阅消息
	if err := network.SendPubSub(p2p, pubsub, PubSubDownloadChecklistRequestTopic, "requestList", "", segmentListRequest); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
	}
}

// ChannelEventsEventDownSnippet 通道事件处理下载新的文件片段
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 表示 DeP2P 网络主机。
//   - pubsub: *pubsub.DeP2PPubSub 表示 DeP2P 网络订阅系统。
//   - downloadMaximumSize: int64 下载最大回复大小
//   - index: int 需要下载的文件片段索引
//   - downloadChan: chan *DownloadChan 下载状态更新通道
func (task *DownloadTask) ChannelEventsEventDownSnippet(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	downloadMaximumSize int64,
	index int,
	downloadChan chan *DownloadChan,
) {
	segment, ok := task.File.GetSegment(index)
	if !ok {
		// logrus.Warnf("文件片段 %d 不存在", index)
		return
	}

	// 遍历节点信息
	nodes := segment.GetNodes()
	// logrus.Warnf("[测试] %v", nodes)

	for nodeID, active := range nodes {
		switch task.DownloadStatus {
		case StatusCompleted, StatusFailed, StatusPaused:
			return
		}

		// 如果下载中或者下载完成，直接退出
		if segment.IsStatus(SegmentStatusDownloading) || segment.IsStatus(SegmentStatusCompleted) {
			return
		}

		// 如果节点不可用，跳过该节点
		if !active {
			continue
		}

		// 初始化文件片段的索引和唯一标识的映射
		segmentInfo := make(map[int]string)
		segmentInfo[index] = segment.SegmentID

		// 获取特定节点下的待下载文件片段索引和唯一标识的映射
		pendingSegments := task.File.GetPendingSegmentsForNode(nodeID)
		for k, v := range pendingSegments {
			segmentInfo[k] = v
		}

		// 设置文件片段的下载状态:下载中
		task.File.SetSegmentStatus(index, SegmentStatusDownloading)

		// 向指定的节点发送下载请求
		if !task.sendDownloadRequest(opt, afe, p2p, downloadChan, nodeID, downloadMaximumSize, index, segmentInfo) {
			continue
		}

		// 如果文件片段的下载状态是下载完成，直接返回
		if segment.IsStatus(SegmentStatusCompleted) {
			return
		}

		// 如果文件片段的下载状态不是下载完成，设置状态为下载失败
		task.File.SetSegmentStatus(index, SegmentStatusFailed)
	}

	// 如果所有节点尝试后仍未成功下载，请求下载纠删码
	requestErasureCodeDownload(task, index)
}

// ChannelEventsEventMergeFile 通道事件处理执行文件合并操作
// 参数：
//   - p2p: *dep2p.DeP2P 表示 DeP2P 网络主机。
//   - pubsub: *pubsub.DeP2PPubSub 表示 DeP2P 网络订阅系统。
func (task *DownloadTask) ChannelEventsEventMergeFile(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, downloadChan chan *DownloadChan) {
	// 子目录当前主机+文件hash
	subDir := filepath.Join(paths.GetDownloadPath(), p2p.Host().ID().String(), task.File.FileID) // 设置子目录

	// 检查目录是否存在
	exists, err := afero.DirExists(afe, subDir)
	if err != nil {
		logrus.Errorf("[%s]检查目录是否存在时失败: %v", debug.WhereAmI(), err)
		return
	}

	// 目录不存在
	if !exists {
		logrus.Errorf("[%s]目录不存在: %s", debug.WhereAmI(), subDir)
		return
	}

	// 从下载的切片中恢复文件数据
	if task.recoverDataFromSlices(opt, afe, p2p, subDir) {
		// 更新文件下载数据对象的状态
		logrus.Printf("文件合并成功！！！！！")

		// 通知文件合并已完成
		task.MergeFileDoneSingleChan()

		// 对外通道
		go func() {
			downloadChan <- &DownloadChan{
				TaskID:           task.TaskID,                       // 任务唯一标识
				TotalPieces:      task.TotalPieces,                  // 文件总分片数
				DownloadProgress: task.File.DownloadCompleteCount(), // 已完成的数量
				IsComplete:       true,                              // 是否下载完成
				DownloadTime:     time.Now().UTC().Unix(),           // 下载完成时间的时间戳
			}

		}()

		// 通知文件下载任务已完成
		task.DownloadTaskDoneSingleChan()
	}
}

// updateFileInfo 更新下载任务的文件信息
// 参数：
//   - payload: *FileDownloadResponseChecklistPayload 下载响应的有效载荷。
func (task *DownloadTask) updateFileInfo(payload *FileDownloadResponseChecklistPayload) {
	if task.File.FileID == "" {
		task.File.FileID = payload.FileID // 文件ID
	}
	if task.File.Name == "" {
		task.File.Name = payload.Name // 文件名
	}
	if task.File.Size == 0 {
		task.File.Size = payload.Size // 文件大小
	}
	if task.File.ContentType == "" {
		task.File.ContentType = payload.ContentType // MIME类型
	}
	if task.TotalPieces == 0 {
		task.TotalPieces = len(payload.SliceTable) // 通过哈希表获取文件总片数
		// 初始化 BitSet
		task.Progress = *util.NewBitSet(task.TotalPieces)
	}

	if task.DataPieces == 0 {
		// 计算数据片段的数量，不是纠删码的数据片段
		var dataPieceCount int
		// 文件片段的哈希表
		for _, v := range payload.SliceTable {
			if !v.IsRsCodes {
				dataPieceCount++
			}
		}
		task.DataPieces = dataPieceCount // 数据片段的数量
	}

	if task.File.SegmentCount() == 0 {
		// 文件片段的哈希表
		for index, v := range payload.SliceTable {
			segment := &FileSegment{
				Index:     index,                // 分片索引
				Checksum:  v.Checksum,           // 分片的校验和
				IsRsCodes: v.IsRsCodes,          // 是否是纠删码片段
				Status:    SegmentStatusPending, // 下载状态:待下载
			}
			// 文件片段的唯一标识
			segmentID, err := util.GenerateSegmentID(task.File.FileID, index)
			if err != nil {
				logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
				return
			}
			segment.SegmentID = segmentID
			task.File.AddSegment(index, segment)
		}
	}
}

// resetTask 重置下载任务的状态和相关数据。
// 它会清空文件片段信息，并重置任务状态、进度和计数器等。
func (task *DownloadTask) resetTask() {
	task.rwmu.Lock()         // 加写锁，以确保对Progress的并发访问安全
	defer task.rwmu.Unlock() // 确保函数结束后解锁

	// 清空文件片段信息
	task.File.Segments = sync.Map{}

	// 重置任务状态和计数器
	task.Progress = *util.NewBitSet(0)  // 重置进度
	task.UpdatedAt = time.Time{}.Unix() // 重置最后一次下载成功的时间戳
	task.MergeCounter = 0               // 重置合并计数器
	task.DownloadStatus = StatusPending // 重置任务状态为待下载

	// 重新初始化通道，确保通道始终保持最新的信息
	task.TickerChecklist = make(chan struct{}, 20)
	task.TickerDownSnippet = make(chan struct{}, 20)
	task.TickerMergeFile = make(chan struct{}, 20)

	task.ChecklistDone = make(chan struct{}, 20)
	task.DownSnippetDone = make(chan struct{}, 20)
	task.MergeFileDone = make(chan struct{}, 20)

	task.EventChecklist = make(chan struct{}, 20)
	task.EventDownSnippet = make(chan int, 100)
	task.EventMergeFile = make(chan struct{}, 20)
	task.DownloadTaskDone = make(chan struct{}, 20)

	// 重置索引清单和下载片段的状态变量
	task.ChecklistTimeout = false
	task.DownSnippetTimeout = false

	// 重新初始化条件变量
	task.StatusCond = sync.NewCond(&task.rwmu)
	task.ChecklistStatusCond = sync.NewCond(&task.rwmu)
	task.DownSnippetStatusCond = sync.NewCond(&task.rwmu)
}
