package uploads

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
)

// UploadSuccessInfo 用于封装文件上传成功后的返回信息。
type UploadSuccessInfo struct {
	TaskID      string // 任务唯一标识，用于区分和管理不同的上传任务。
	FileID      string // 文件唯一标识，用于在系统内部唯一区分文件。
	Checksum    string // 文件的校验和
	Name        string // 文件名，包括扩展名，描述文件的名称。
	Size        int64  // 文件大小，单位为字节，描述文件的总大小。
	TotalSlices int    // 文件总切片总数，描述文件的总切片数。
	Extension   string // 文件的扩展名
	ContentType string // MIME类型，表示文件的内容类型，如"text/plain"。
	UploadTime  int64  // 文件开始上传的时间。

}

// NewUpload 新上传操作
// 参数：
//   - opt: *opts.Options 文件存储选项配置。
//   - afe: afero.Afero 文件系统接口。
//   - p2p: *dep2p.DeP2P 网络主机。
//   - pubsub: *pubsub.DeP2PPubSub 网络订阅。
//   - path: string 文件路径。
//   - ownerPriv: *ecdh.PrivateKey 所有者的私钥。
//
// 返回值：
//   - *UploadSuccessInfo: 文件上传成功后的返回信息。
//   - error: 如果发生错误，返回错误信息。
func (manager *UploadManager) NewUpload(
	opt *opts.Options, // 文件存储选项配置
	afe afero.Afero, // 文件系统接口
	p2p *dep2p.DeP2P, // 网络主机
	pubsub *pubsub.DeP2PPubSub, // 网络订阅
	path string, // 文件路径
	ownerPriv *ecdh.PrivateKey, // 所有者的私钥
) (*UploadSuccessInfo, error) {
	path = strings.TrimSpace(path) // 删除了所有前导和尾随空格
	if path == "" {
		return nil, fmt.Errorf("文件路径不可为空")
	}
	if ownerPriv == nil {
		ownerPriv = opt.GetDefaultOwnerPriv() // 获取默认所有者的私钥
		if ownerPriv == nil {
			return nil, fmt.Errorf("所有者密钥不可为空")
		}
	}

	// 检查是否达到上传允许的最大并发数
	if manager.IsMaxConcurrencyReached() {
		return nil, fmt.Errorf("已达到上传允许的最大并发数")
	}

	// 打开一个文件，返回该文件或错误（如果发生）。
	file, err := afero.NewOsFs().Open(path)
	if err != nil {
		logrus.Errorf("[%s]打开文件时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}
	defer file.Close()

	// 生成taskID
	taskID, err := util.GenerateTaskID(ownerPriv)
	if err != nil {
		logrus.Errorf("[%s]生成任务ID时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 创建并初始化一个新的文件上传任务实例
	task, err := NewUploadTask(manager.ctx, opt, &manager.Mu, manager.Scheme, taskID, file, ownerPriv)
	if err != nil {
		logrus.Errorf("[%s]初始化上传实例时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 向管理器注册一个新的上传任务
	go manager.RegisterTask(opt, afe, p2p, pubsub, task)

	// 保存任务至文件
	go manager.SaveTasksToFileSingleChan()

	return &UploadSuccessInfo{
		TaskID:      taskID,                                 // 任务唯一标识
		FileID:      task.File.FileID,                       // 文件唯一标识
		Name:        task.File.Name,                         // 文件名
		Size:        task.File.Size,                         // 文件大小
		TotalSlices: len(task.File.SliceTable),              // 文件总切片数
		Extension:   task.File.Extension,                    // 文件的扩展名
		ContentType: task.File.ContentType,                  // MIME类型
		UploadTime:  time.Now().UTC().Unix(),                // 文件开始上传的时间
		Checksum:    hex.EncodeToString(task.File.Checksum), // 文件的校验和
	}, nil
}

// PauseUpload 暂停上传操作
func (manager *UploadManager) PauseUpload(taskID string) error {
	task, ok := manager.Tasks[taskID]

	if !ok {
		// TODO: 失败事件
		return fmt.Errorf("未找到上传任务: %s", taskID)
	}
	task.Mu.Lock()
	defer task.Mu.Unlock()
	task.Status = StatusPaused
	return nil
}

// ClearUpload 清空上传记录
func (manager *UploadManager) ClearUpload() error {
	manager.Tasks = make(map[string]*UploadTask)
	return nil
}

// CancelUpload 取消上传操作
func (manager *UploadManager) CancelUpload(taskID string) error {
	task, ok := manager.Tasks[taskID]
	if !ok {
		// TODO: 失败事件
		return fmt.Errorf("未找到上传任务: %s", taskID)
	}
	delete(manager.Tasks, task.TaskID)
	return nil
}

// ContinueUpload 继续上传操作
func (manager *UploadManager) ContinueUpload(taskID string) error {
	task, ok := manager.Tasks[taskID]
	if !ok {
		// TODO: 失败事件
		return fmt.Errorf("未找到上传任务: %s", taskID)
	}
	task.Mu.Lock()
	defer task.Mu.Unlock()
	task.Status = StatusUploading

	// 准备好本地存储文件片段
	go task.SegmentReadySingleChan()

	return nil
}
