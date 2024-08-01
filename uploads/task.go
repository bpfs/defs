package uploads

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/shamir"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/kbucket"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
)

// UploadTask 描述一个文件上传任务，包括文件信息和上传状态
// 它封装了单个文件上传的细节，如文件元数据、上传进度、状态控制等
type UploadTask struct {
	ctx    context.Context    // 上下文用于管理协程的生命周期
	cancel context.CancelFunc // 取消函数

	Mu         sync.RWMutex // 控制对Progress的并发访问
	Status     UploadStatus // 上传任务的当前状态，如进行中、暂停、完成等
	StatusCond *sync.Cond

	TaskID   string      // 任务唯一标识，用于区分和管理不同的上传任务
	File     *UploadFile // 待上传的文件信息，包含文件的元数据和分片信息
	Progress util.BitSet // 上传任务的进度，表示为0到100之间的百分比

	SegmentReady    chan struct{}         // 用于通知准备好本地存储文件片段的通道
	SendToNetwork   chan int              // 用于触发向网络发送已存储文件片段的动作的通道
	UploadDone      chan struct{}         // 用于通知上传完成的通道
	NetworkReceived chan *NetworkResponse // 用于接收网络返回的接受方节点地址信息的通道
}

// NewUploadTask 创建并初始化一个新的文件上传任务实例。
// taskID 为任务的唯一标识符。
// file 为待上传的文件信息。
// maxConcurrency 为任务允许的最大并发上传数。
func NewUploadTask(ctx context.Context, opt *opts.Options, mu *sync.Mutex, scheme *shamir.ShamirScheme, taskID string, file afero.File, ownerPriv *ecdh.PrivateKey) (*UploadTask, error) {
	// 创建并初始化一个新的UploadFile实例
	f, err := NewUploadFile(opt, ownerPriv, file, scheme)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	if f.Size < opt.GetMinUploadSize() {
		return nil, fmt.Errorf("文件不可小于最小上传大小 %d", opt.GetMinUploadSize())
	}
	if f.Size > opt.GetMaxUploadSize() {
		return nil, fmt.Errorf("文件不可最大上传大小 %d", opt.GetMaxUploadSize())
	}

	ct, cancel := context.WithCancel(ctx)

	return &UploadTask{
		ctx:        ct,
		cancel:     cancel,
		Mu:         sync.RWMutex{}, // 任务允许的最大并发上传数
		Status:     StatusPending,  // 上传任务的当前状态: 待上传
		StatusCond: sync.NewCond(mu),

		TaskID:   taskID,                             // 任务唯一标识
		File:     f,                                  // 待上传的文件信息
		Progress: *util.NewBitSet(len(f.SliceTable)), // 上传任务的进度

		SegmentReady:    make(chan struct{}, 1),
		SendToNetwork:   make(chan int, MaxConcurrency), // 通道缓冲为任务允许的最大并发上传数
		UploadDone:      make(chan struct{}, 1),
		NetworkReceived: make(chan *NetworkResponse),
	}, nil
}

// ChannelEvents 通道事件处理
func (task *UploadTask) ChannelEvents(
	opt *opts.Options, // 文件存储选项配置
	afe afero.Afero, // 文件系统接口
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	uploadChan chan *UploadChan, // 上传对外通道
) {
	for {
		select {
		case <-task.ctx.Done():
			return

		case <-task.SegmentReady:
			// 文件片段存储为本地文件
			if err := sliceLocalFileHandle(task); err != nil {
				logrus.Errorf("[%s]文件片段存储为本地文件时失败: %v", debug.WhereAmI(), err)
			}

		// 用于触发向网络发送已存储文件片段的动作的通道
		case index := <-task.SendToNetwork:
			logrus.Printf("开始将 %d 发送到网络", index)
			// 发送文件片段到网络
			go task.SendingSliceToNetwork(afe, p2p, index)

		// 网络接收通道，用于接收网络返回的接受方节点地址信息，以及进行下一步的发送操作。
		case response := <-task.NetworkReceived:
			// 处理网络接收通道的响应
			go task.handleNetworkResponse(response, uploadChan)
		}
	}
}

// PeriodicSend 定时任务，发送数据到网络
func (task *UploadTask) PeriodicSend() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-task.ctx.Done():
			return
		case <-task.UploadDone:
			logrus.Info("文件上传完成，退出上传子进程")
			task.cancel() // 取消任务，确保所有子 Goroutine 退出
			return
		case <-ticker.C:
			// 检查是否需要发送到网络
			go task.CheckSegmentsStatus()
		}
	}
}

// IsUploadComplete 检查指定文件的上传是否完成
func (task *UploadTask) IsUploadComplete() bool {
	for i := 0; i < len(task.File.SliceTable); i++ {
		if !task.Progress.IsSet(i) {
			return false
		}
	}
	return true
}

// UploadCompleteCount 检查已经完成的数量
func (task *UploadTask) UploadCompleteCount() int {
	var count = 0
	for i := 0; i < len(task.File.SliceTable); i++ {
		if task.Progress.IsSet(i) {
			count++
		}
	}
	return count
}

// SendingSliceToNetwork 发送文件片段到网络。
// 参数：
//   - afe: 文件系统接口，用于读取文件片段。
//   - p2p: P2P网络对象。
//   - index: 文件片段索引。
//   - networkReceivedChan: 网络响应通道，用于发送网络响应结果。
func (task *UploadTask) SendingSliceToNetwork(afe afero.Afero, p2p *dep2p.DeP2P, index int) {
	// 获取指定索引的文件片段信息
	segment, exists := task.File.Segments[index]
	if !exists {
		logrus.Errorf("索引 %d 的文件片段不存在", index)
		return
	}

	// TODO: 怀疑这里读取不到内容，没有指定具体读取那个文件片段
	// 读取指定文件的内容并返回
	sliceByte, err := afero.ReadFile(afe, filepath.Join(paths.GetUploadPath(), task.File.FileID, segment.SegmentID))
	if err != nil {
		logrus.Errorf("读取文件片段时失败: %v", err)
		return
	}

	if len(sliceByte) == 0 {
		logrus.Errorf("读取文件片段时失败")
		return
	}

	// 设置文件片段的状态为上传中
	segment.SetStatusUploading()

	for i := 0; ; {
		// 节点不足
		if p2p.RoutingTable(2).Size() < 1 {
			// 延时1秒后退出
			logrus.Warnf("上传时所需节点不足:%d", p2p.RoutingTable(2).Size())
			time.Sleep(1 * time.Second)
			return
		}
		receiverPeers := p2p.RoutingTable(2).NearestPeers(kbucket.ConvertKey(segment.SegmentID), i+1)
		if len(receiverPeers) == i {
			logrus.Errorf("临近节点资源不足")

			// 设置文件片段的状态为失败
			segment.SetStatusFailed()

			return
		}

		if len(receiverPeers) == 0 {
			i++
			continue
		}

		if i >= len(receiverPeers) {
			i = 0
		}

		// 通知准备发送到网络
		segmentInfo := &FileSegmentInfo{
			TaskID:        task.TaskID,                                         // 任务ID
			FileID:        task.File.FileID,                                    // 文件唯一标识
			TempStorage:   path.Join(task.File.TempStorage, segment.SegmentID), // 文件的临时存储位置
			SegmentID:     segment.SegmentID,                                   // 文件片段的唯一标识
			TotalSegments: len(task.File.Segments),                             // 文件总分片数
			Index:         index,                                               // 分片索引
			Size:          segment.Size,                                        // 分片大小
			IsRsCodes:     task.File.SliceTable[index].IsRsCodes,               // 是否使用纠删码
		}

		node := receiverPeers[i]
		// 向目标节点发送文件片段
		if err := sendSliceToNode(p2p, segmentInfo, node, sliceByte, task.NetworkReceived); err != nil {
			i++
			continue
		}

		// 设置文件片段的状态为已完成
		segment.SetStatusCompleted()

		return
	}
}

// handleNetworkResponse 处理网络接收通道的响应
func (task *UploadTask) handleNetworkResponse(response *NetworkResponse, uploadChan chan *UploadChan) {
	task.Mu.Lock()
	defer task.Mu.Unlock()

	segment, ok := task.File.Segments[response.Index]
	if !ok {
		logrus.Errorf("[%s]无法找到索引为 %d 的文件片段", debug.WhereAmI(), response.Index)
		return
	}

	// 设置任务进度
	task.Progress.Set(response.Index)
	// 检查指定文件的上传是否完成
	isComplete := task.IsUploadComplete()
	// 更新任务状态
	if isComplete {
		task.SetStatusCompleted() // 设置为已完成
	} else if task.Status != StatusPaused {
		task.SetStatusUploading() // 设置为上传中
	}

	// 向上传通道发送更新信息
	go func() {
		uploadChan <- &UploadChan{
			TaskID:           task.TaskID,                // 任务唯一标识
			UploadProgress:   task.UploadCompleteCount(), // 上传进度百分比
			IsComplete:       isComplete,                 // 上传任务是否完成
			SegmentID:        segment.SegmentID,          // 文件片段唯一标识
			SegmentIndex:     segment.Index,              // 文件片段索引
			SegmentSize:      segment.Size,               // 文件片段大小
			UsesErasureCodes: segment.IsRsCodes,          // 是否使用纠删码技术
			NodeID:           response.ReceiverPeerID,    // 存储该文件片段的节点ID
			UploadTime:       time.Now().UTC().Unix(),    // 上传完成时间
		}
	}()
}

// CheckSegmentsStatus 检查是否有待上传或失败的文件片段
// 返回值：bool 是否存在待上传或失败的文件片段
func (task *UploadTask) CheckSegmentsStatus() {
	for _, segment := range task.File.Segments {
		// 上传状态为"待上传"、"失败"的分片，通知发送至网络
		if segment.Status == SegmentStatusPending || segment.Status == SegmentStatusFailed {
			task.SendToNetworkChan(segment.Index)
		}
	}
}

// SegmentReadySingleChan 向任务的通知准备好本地存储文件片段的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *UploadTask) SegmentReadySingleChan() {
	select {
	case task.SegmentReady <- struct{}{}:
	default:
		// 如果通道已满，丢弃旧消息再写入新消息
		<-task.SegmentReady
		task.SegmentReady <- struct{}{}
	}
}

// SendToNetworkChan 向网络发送已存储文件片段的动作的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *UploadTask) SendToNetworkChan(index int) {
	select {
	case task.SendToNetwork <- index:
	default:
		// 如果通道已满，丢弃旧消息再写入新消息
		<-task.SendToNetwork
		task.SendToNetwork <- index
	}
}

// UploadDoneSingleChan 向任务的通知上传完成的通道发送通知。
// 如果通道已满，会先丢弃旧消息再写入新消息，确保通道始终保持最新的通知。
func (task *UploadTask) UploadDoneSingleChan() {
	select {
	case task.UploadDone <- struct{}{}:
	default:
		// 如果通道已满，丢弃旧消息再写入新消息
		<-task.UploadDone
		task.UploadDone <- struct{}{}
	}
}

// SetStatusPending 设置上传任务的状态为待上传
func (task *UploadTask) SetStatusPending() {
	task.Status = StatusPending
}

// SetStatusUploading 设置上传任务的状态为上传中
func (task *UploadTask) SetStatusUploading() {
	task.Status = StatusUploading
}

// SetStatusPaused 设置上传任务的状态为已暂停
func (task *UploadTask) SetStatusPaused() {
	task.Status = StatusPaused
}

// SetStatusCompleted 设置上传任务的状态为已完成
func (task *UploadTask) SetStatusCompleted() {
	task.Status = StatusCompleted
	// 向任务的通知上传完成的通道发送通知
	task.UploadDoneSingleChan()
}

// SetStatusFailed 设置上传任务的状态为失败
func (task *UploadTask) SetStatusFailed() {
	task.Status = StatusFailed
}
