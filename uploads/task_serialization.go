package uploads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/util"
	"github.com/sirupsen/logrus"
)

// UploadTaskSerializable 是 UploadTask 的可序列化版本
type UploadTaskSerializable struct {
	TaskID       string                    `json:"task_id"`       // 任务唯一标识
	FileMeta     *FileMeta                 `json:"file_meta"`     // 文件元数据
	Segments     map[int]*FileSegment      `json:"segments"`      // 文件分片信息
	TempStorage  string                    `json:"temp_storage"`  // 文件的临时存储位置
	SliceTable   map[int]*HashTable        `json:"slice_table"`   // 文件片段的哈希表
	StartedAt    int64                     `json:"started_at"`    // 文件上传的开始时间戳
	FinishedAt   int64                     `json:"finished_at"`   // 文件上传的完成时间戳
	Progress     util.BitSet               `json:"progress"`      // 上传任务的进度
	Status       UploadStatus              `json:"status"`        // 上传任务的状态
	FileSecurity *FileSecuritySerializable `json:"file_security"` // 文件安全信息
}

// FileSecuritySerializable 是 FileSecurity 的可序列化版本
type FileSecuritySerializable struct {
	Secret        []byte   `json:"secret"`         // 文件加密密钥
	EncryptionKey [][]byte `json:"encryption_key"` // 文件加密密钥
	PrivateKey    []byte   `json:"private_key"`    // 文件签名密钥的序列化字节
	P2PKHScript   []byte   `json:"p2pkh_script"`   // P2PKH 脚本
	P2PKScript    []byte   `json:"p2pk_script"`    // P2PK 脚本
}

// ToSerializable 将 UploadTask 转换为可序列化的结构体
// 返回值：
//   - *UploadTaskSerializable: 可序列化的 UploadTask 结构体
//   - error: 如果发生错误，返回错误信息
func (task *UploadTask) ToSerializable() (*UploadTaskSerializable, error) {
	privKeyBytes, err := util.MarshalPrivateKey(task.File.Security.PrivateKey)
	if err != nil {
		logrus.Errorf("[%s]将ECDSA私钥序列化为字节失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	fileSecurity := &FileSecuritySerializable{
		Secret:        task.File.Security.Secret,
		EncryptionKey: task.File.Security.EncryptionKey,
		PrivateKey:    privKeyBytes,
		P2PKHScript:   task.File.Security.P2PKHScript,
		P2PKScript:    task.File.Security.P2PKScript,
	}

	serializable := &UploadTaskSerializable{
		TaskID:       task.TaskID,
		FileMeta:     &task.File.FileMeta, // 使用指针引用
		Segments:     task.File.Segments,
		TempStorage:  task.File.TempStorage,
		SliceTable:   task.File.SliceTable,
		StartedAt:    task.File.StartedAt,
		FinishedAt:   task.File.FinishedAt,
		Progress:     task.Progress,
		Status:       task.Status,
		FileSecurity: fileSecurity,
	}

	return serializable, nil
}

// FromSerializable 从可序列化的结构体恢复 UploadTask
// 参数：
//   - serializable: *UploadTaskSerializable 可序列化的 UploadTask 结构体
func (task *UploadTask) FromSerializable(serializable *UploadTaskSerializable) error {
	task.TaskID = serializable.TaskID

	// 初始化 FileMeta
	if serializable.FileMeta == nil {
		return fmt.Errorf("file meta is nil")
	}

	// 初始化 FileSecurity
	if serializable.FileSecurity == nil {
		return fmt.Errorf("file security is nil")
	}

	privateKey, err := util.UnmarshalPrivateKey(serializable.FileSecurity.PrivateKey)
	if err != nil {
		logrus.Errorf("[%s]反序列化ECDSA私钥时失败: %v", debug.WhereAmI(), err)
		return err
	}

	task.File = &UploadFile{
		FileMeta:    *serializable.FileMeta, // 解引用指针
		Segments:    serializable.Segments,
		TempStorage: serializable.TempStorage,
		SliceTable:  serializable.SliceTable,
		StartedAt:   serializable.StartedAt,
		FinishedAt:  serializable.FinishedAt,
		Security: &FileSecurity{
			Secret:        serializable.FileSecurity.Secret,
			EncryptionKey: serializable.FileSecurity.EncryptionKey,
			PrivateKey:    privateKey,
			P2PKHScript:   serializable.FileSecurity.P2PKHScript,
			P2PKScript:    serializable.FileSecurity.P2PKScript,
		},
	}

	task.Progress = serializable.Progress
	task.Status = serializable.Status

	// 重新初始化通道
	task.SegmentReady = make(chan struct{}, 1)
	task.SendToNetwork = make(chan int, MaxConcurrency)
	task.UploadDone = make(chan struct{}, 1)
	task.NetworkReceived = make(chan *NetworkResponse)

	return nil
}

// LoadTasksFromFile 从文件加载任务
// 参数：
//   - filePath: string 文件路径
//
// 返回值：
//   - map[string]*UploadTaskSerializable: 任务映射表
//   - error: 如果发生错误，返回错误信息
func LoadTasksFromFile(filePath string) (map[string]*UploadTaskSerializable, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logrus.Debugf("[%s]文件不存在: %v", debug.WhereAmI(), err)
		// 如果文件不存在，返回一个空的任务映射表
		return make(map[string]*UploadTaskSerializable), nil
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		logrus.Errorf("[%s]读取文件内容时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 初始化任务映射表
	var tasks map[string]*UploadTaskSerializable

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
//   - tasks: map[string]*UploadTaskSerializable 任务映射表
//
// 返回值：
//   - error: 如果发生错误，返回错误信息
func SaveTasksToFile(filePath string, tasks map[string]*UploadTaskSerializable) error {
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
