package downloads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/defs/wallets"
	"github.com/sirupsen/logrus"
)

// DownloadTaskSerializable 是 DownloadTask 的可序列化版本
type DownloadTaskSerializable struct {
	TaskID       string         `json:"task_id"`       // 任务唯一标识
	File         *DownloadFile  `json:"file"`          // 待下载的文件信息
	TotalPieces  int            `json:"total_pieces"`  // 文件总片数（数据片段和纠删码片段的总数）
	DataPieces   int            `json:"data_pieces"`   // 数据片段的数量
	OwnerPriv    []byte         `json:"owner_priv"`    // 所有者的私钥
	Secret       []byte         `json:"secret"`        // 文件加密密钥
	UserPubHash  []byte         `json:"user_pub_hash"` // 用户的公钥哈希
	Progress     util.BitSet    `json:"progress"`      // 下载任务的进度，表示为0到100之间的百分比
	CreatedAt    int64          `json:"created_at"`    // 任务创建的时间戳
	UpdatedAt    int64          `json:"updated_at"`    // 最后一次下载成功的时间戳
	MergeCounter int            `json:"merge_counter"` // 用于跟踪文件合并操作的计数器
	Status       DownloadStatus `json:"status"`        // 下载任务的状态
}

// ToSerializable 将 DownloadTask 转换为可序列化的结构体
// 返回值：
//   - *DownloadTaskSerializable: 可序列化的 DownloadTask 结构体
func (task *DownloadTask) ToSerializable() (*DownloadTaskSerializable, error) {
	// 将ECDSA私钥序列化为字节
	privKeyBytes, err := wallets.MarshalPrivateKey(task.OwnerPriv)
	if err != nil {
		logrus.Errorf("[%s]将ECDSA私钥序列化为字节失败: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return &DownloadTaskSerializable{
		TaskID:       task.TaskID,
		File:         task.File,
		TotalPieces:  task.TotalPieces,
		DataPieces:   task.DataPieces,
		OwnerPriv:    privKeyBytes,
		Secret:       task.Secret,
		UserPubHash:  task.UserPubHash,
		Progress:     task.Progress,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
		MergeCounter: task.MergeCounter,
		Status:       task.DownloadStatus,
	}, nil
}

// FromSerializable 从可序列化的结构体恢复 DownloadTask
// 参数：
//   - serializable: *DownloadTaskSerializable 可序列化的 DownloadTask 结构体
func (task *DownloadTask) FromSerializable(serializable *DownloadTaskSerializable) error {
	// 将字节序列反序列化为ECDSA私钥
	privateKey, err := wallets.UnmarshalPrivateKey(serializable.OwnerPriv)
	if err != nil {
		logrus.Errorf("[%s]将字节序列反序列化为ECDSA私钥 失败: %v", debug.WhereAmI(), err)
		return err
	}

	task.TaskID = serializable.TaskID
	task.File = serializable.File
	task.TotalPieces = serializable.TotalPieces
	task.DataPieces = serializable.DataPieces
	task.OwnerPriv = privateKey
	task.Secret = serializable.Secret
	task.UserPubHash = serializable.UserPubHash
	task.Progress = serializable.Progress
	task.CreatedAt = serializable.CreatedAt
	task.UpdatedAt = serializable.UpdatedAt
	task.MergeCounter = serializable.MergeCounter
	task.DownloadStatus = serializable.Status

	// 重新初始化通道
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

	// 初始化状态和条件变量
	task.StatusCond = sync.NewCond(&task.rwmu)
	task.ChecklistStatusCond = sync.NewCond(&task.rwmu)
	task.DownSnippetStatusCond = sync.NewCond(&task.rwmu)

	return nil
}

// LoadTasksFromFile 从文件加载任务
// 参数：
//   - filePath: string 文件路径
//
// 返回值：
//   - map[string]*DownloadTaskSerializable: 任务映射表
//   - error: 如果发生错误，返回错误信息
func LoadTasksFromFile(filePath string) (map[string]*DownloadTaskSerializable, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logrus.Debugf("[%s]文件不存在: %v", debug.WhereAmI(), err)
		// 如果文件不存在，返回一个空的任务映射表
		return make(map[string]*DownloadTaskSerializable), nil
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		logrus.Errorf("[%s]读取文件内容时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 初始化任务映射表
	var tasks map[string]*DownloadTaskSerializable

	// 反序列化文件内容到任务映射表
	if err := json.Unmarshal(data, &tasks); err != nil {
		logrus.Errorf("[%s]反序列化文件内容到任务映射表时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	return tasks, nil
}

// SaveTasksToFile 将任务保存到文件
// 参数：
//   - filePath: string 文件路径
//   - tasks: map[string]*DownloadTaskSerializable 任务映射表
//
// 返回值：
//   - error: 如果发生错误，返回错误信息
func SaveTasksToFile(filePath string, tasks map[string]*DownloadTaskSerializable) error {
	// 将任务映射表序列化为 JSON 格式
	data, err := json.Marshal(tasks)
	if err != nil {
		logrus.Errorf("[%s]将任务映射表序列化为时失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 确保文件目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		logrus.Errorf("[%s] 创建目录失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 创建临时文件路径
	tempFilePath := filePath + ".tmp"

	// 检查临时文件目录是否存在
	if _, err := os.Stat(filepath.Dir(tempFilePath)); os.IsNotExist(err) {
		logrus.Errorf("[%s] 临时文件目录不存在: %v", debug.WhereAmI(), err)
		return err
	}

	// 创建文件
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		logrus.Errorf("[%s] 创建临时文件失败: %v", debug.WhereAmI(), err)
		return err
	}
	defer tempFile.Close()

	// 将序列化的数据写入临时文件
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempFilePath) // 清理临时文件
		logrus.Errorf("[%s] 写入临时文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 重命名临时文件为最终文件
	if err := os.Rename(tempFilePath, filePath); err != nil {
		os.Remove(tempFilePath) // 清理临时文件
		logrus.Errorf("[%s] 重命名文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	return nil
}
