package downloads

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// DownloadSuccessInfo 用于封装文件下载成功后的返回信息
type DownloadSuccessInfo struct {
	TaskID       string // 任务唯一标识，用于区分和管理不同的下载任务
	FileID       string // 文件唯一标识，用于在系统内部唯一区分文件
	DownloadTime int64  // 文件开始下载的时间
}

// NewDownload 新下载操作
// 参数：
//   - opt: *opts.Options 文件存储选项配置。
//   - afe: afero.Afero 文件系统接口。
//   - p2p: *dep2p.DeP2P 网络主机。
//   - pubsub: *pubsub.DeP2PPubSub 网络订阅。
//   - fileID: string 文件唯一标识。
//   - ownerPriv: *ecdsa.PrivateKey 所有者的私钥。
//   - segmentNodes: ...map[int][]peer.ID 文件片段所在节点。
//
// 返回值：
//   - *DownloadSuccessInfo: 文件下载成功后的返回信息。
//   - error: 如果发生错误，返回错误信息。
func (manager *DownloadManager) NewDownload(
	opt *opts.Options, // 文件存储选项配置
	afe afero.Afero, // 文件系统接口
	p2p *dep2p.DeP2P, // 网络主机
	pubsub *pubsub.DeP2PPubSub, // 网络订阅
	fileID string, // 文件唯一标识
	ownerPriv *ecdsa.PrivateKey, // 所有者的私钥
	segmentNodes ...map[int][]peer.ID, // 文件片段所在节点
) (*DownloadSuccessInfo, error) {
	fileID = strings.TrimSpace(fileID) // 删除了所有前导和尾随空格
	if fileID == "" {
		return nil, fmt.Errorf("文件唯一标识不可为空")
	}
	if ownerPriv == nil {
		ownerPriv = opt.GetDefaultOwnerPriv() // 获取默认所有者的私钥
		if ownerPriv == nil {
			return nil, fmt.Errorf("所有者密钥不可为空")
		}
	}

	// 过滤重复下载
	for _, task := range manager.Tasks {
		if task.File.FileID == fileID {
			return nil, fmt.Errorf("文件 %s 正在下载中", fileID)
		}
	}

	// 确保文件目录存在
	if err := os.MkdirAll(filepath.Dir(opt.GetDownloadPath()), 0755); err != nil {
		logrus.Errorf("[%s]文件目录不存在: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 创建临时文件
	tempFilePath := filepath.Join(opt.GetDownloadPath(), fileID+".defs")
	if _, err := os.Create(tempFilePath); err != nil {
		logrus.Errorf("[%s]创建临时文件时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 生成taskID
	taskID, err := util.GenerateTaskID(ownerPriv)
	if err != nil {
		logrus.Errorf("[%s]生成任务ID时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 创建并初始化一个新的文件下载任务实例
	task, err := NewDownloadTask(manager.ctx, taskID, fileID, ownerPriv)
	if err != nil {
		logrus.Errorf("[%s]初始化下载实例时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 更新节点ID
	if len(segmentNodes) > 0 {
		for idx, peers := range segmentNodes[0] {
			// 添加文件片段所在的节点信息
			task.File.AddSegmentNodes(idx, peers)
		}
	}

	// 向管理器注册一个新的下载任务
	go manager.RegisterTask(opt, afe, p2p, pubsub, task)

	// 保存任务至文件
	go manager.SaveTasksToFileSingleChan()

	return &DownloadSuccessInfo{
		TaskID:       taskID,                  // 任务唯一标识
		FileID:       fileID,                  // 文件唯一标识
		DownloadTime: time.Now().UTC().Unix(), // 文件下载时间
	}, nil
}

// PauseDownload 暂停下载操作
func (manager *DownloadManager) PauseDownload(
	taskID string, // 任务唯一标识，用于区分和管理不同的上传任务
) error {
	task, ok := manager.Tasks[taskID]
	if !ok {
		logrus.Errorf("[%s]找不到下载任务: %s", debug.WhereAmI(), taskID)
		return fmt.Errorf("下载任务不存在")
	}

	task.SetDownloadStatus(StatusPaused) // 设置下载任务的状态为"下载暂停"

	go manager.SaveTasksToFileSingleChan() // 保存任务至文件的通知通道
	return nil
}

// CancelDownload 取消下载操作
func (manager *DownloadManager) CancelDownload(
	taskID string, // 任务唯一标识，用于区分和管理不同的上传任务
) error {
	task, ok := manager.Tasks[taskID]
	if !ok {
		logrus.Errorf("[%s]找不到下载任务: %s", debug.WhereAmI(), taskID)
		return fmt.Errorf("下载任务不存在")
	}

	task.cancel() // 取消任务

	delete(manager.Tasks, taskID)

	go manager.SaveTasksToFileSingleChan() // 保存任务至文件的通知通道
	return nil
}

// ContinueDownload 继续下载操作
// 参数：
//   - taskID: string 任务唯一标识，用于区分和管理不同的下载任务
//
// 返回值：
//   - error 如果发生错误，返回错误信息
func (manager *DownloadManager) ContinueDownload(taskID string) error {
	// 从下载任务管理器中获取任务
	task, ok := manager.Tasks[taskID]
	if !ok {
		logrus.Errorf("[%s]找不到下载任务: %s", debug.WhereAmI(), taskID)
		return fmt.Errorf("下载任务不存在")
	}

	// 启动协程继续下载
	go func() {
		// 更新每个文件片段的节点信息和纠删码信息
		task.File.Segments.Range(func(key, value interface{}) bool {
			index := key.(int)
			segment := value.(*FileSegment)
			// 下载文件片段，排除纠删码片段且下载未完成的片段
			if !segment.IsRsCodes && !segment.IsStatus(SegmentStatusCompleted) {
				// 通知将对应文件片段下载到本地
				task.EventDownSnippetChan(index)
			}
			return true
		})
	}()

	task.SetDownloadStatus(StatusDownloading) // 设置下载任务的状态为"下载中"

	// 启动协程保存任务状态至文件
	go manager.SaveTasksToFileSingleChan()

	return nil
}

// // ContinueDownload 继续下载操作
// func (manager *DownloadManager) ContinueDownload(
// 	taskID string, // 任务唯一标识，用于区分和管理不同的上传任务
// ) error {
// 	// TODO: 从本地文件夹读取已经下载的文件片段
// 	task, ok := manager.Tasks[taskID]
// 	if !ok {
// 		logrus.Errorf("[%s]找不到下载任务: %s", debug.WhereAmI(), taskID)
// 		return fmt.Errorf("下载任务不存在")
// 	}

// 	task.RWMu.Lock()
// 	defer task.RWMu.Unlock()

// 	go func() {
// 		// 再继续下载
// 		// 更新每个文件片段的节点信息和纠删码信息
// 		for index := range task.File.Segments {
// 			// 下载文件片段
// 			// 不是纠删码，且不等于下载完成
// 			if !task.File.Segments[index].IsRsCodes && task.File.Segments[index].Status != SegmentStatusCompleted {
// 				// 通知将对应文件片段下载到本地
// 				task.EventDownSnippetChan(index) // 传递需要下载的片段索引
// 			}
// 		}
// 	}()

// 	task.SetDownloadStatus(StatusDownloading) // 设置下载任务的状态为"下载中"

// 	go manager.SaveTasksToFileSingleChan() // 保存任务至文件的通知通道

// 	return nil
// }

// ClearDownloadTask 清空文件的下载任务
func (manager *DownloadManager) ClearDownloadTask() {
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	manager.Tasks = make(map[string]*DownloadTask)

	go manager.SaveTasksToFileSingleChan() // 保存任务至文件的通知通道
}
