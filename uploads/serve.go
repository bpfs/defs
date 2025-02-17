package uploads

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"

	"github.com/dep2p/go-dep2p/core/protocol"
	logging "github.com/dep2p/log"
)

var logger = logging.Logger("uploads")

// NewUpload 创建一个新的上传任务
// 参数:
//   - path: 文件路径,要上传的文件的完整路径
//   - ownerPriv: 所有者的私钥,用于签名和权限验证
//   - immediate: 是否立即执行上传（可选参数,默认为 false）
//
// 返回:
//   - *pb.UploadOperationInfo: 上传操作信息,包含任务ID等信息
//   - error: 错误信息,如果创建失败则返回错误原因
func (m *UploadManager) NewUpload(
	path string,
	ownerPriv *ecdsa.PrivateKey,
	immediate ...bool,
) (*pb.UploadOperationInfo, error) {

	// 检查是否存在可用的发送节点
	if !m.ps.Client().HasAvailableNodes(protocol.ID(SendingToNetworkProtocol)) {
		logger.Warn("没有可用的发送节点")
		return nil, fmt.Errorf("没有可用的发送节点")
	}

	// 删除路径两端的空格
	path = strings.TrimSpace(path)
	if path == "" {
		logger.Error("文件路径不可为空")
		return nil, fmt.Errorf("文件路径不可为空")
	}

	// 检查路径有效性
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Errorf("文件不存在: %s", path)
			return nil, fmt.Errorf("文件不存在")
		}
		logger.Errorf("检查文件路径失败: %v", err)
		return nil, err
	}

	// 检查是否为目录
	if fileInfo.IsDir() {
		logger.Errorf("不支持上传目录: %s", path)
		return nil, fmt.Errorf("不支持上传目录")
	}

	// 检查文件大小
	if fileInfo.Size() == 0 {
		logger.Error("不能上传空文件")
		return nil, fmt.Errorf("不能上传空文件")
	}

	// 如果未提供所有者私钥,则使用默认私钥
	if ownerPriv == nil {
		ownerPriv = m.Options().GetDefaultOwnerPriv()
		if ownerPriv == nil {
			logger.Error("用户密钥不可为空")
			return nil, fmt.Errorf("用户密钥不可为空")
		}
	}

	// 先创建状态对象
	taskStatus := NewSegmentStatus(&m.mu)

	// 创建回调函数
	onSegmentsReady := func(taskID string) {
		logger.Infof("进入分片准备完成回调函数, taskID: %s", taskID)

		m.mu.Lock()
		// 在回调函数中设置状态
		m.segmentStatuses[taskID] = taskStatus
		m.mu.Unlock()

		// 检查是否为立即上传模式
		isImmediate := len(immediate) > 0 && immediate[0]
		if isImmediate {
			go func() {
				if err := m.TriggerUpload(taskID, true); err != nil {
					logger.Errorf("触发上传失败: %v, taskID: %s", err, taskID)
					m.errChan <- err
				}
			}()
		}
	}

	// 创建并初始化UploadFile实例
	uploadInfo, err := NewUploadFile(m.opt, m.db, m.scheme, path, ownerPriv, onSegmentsReady, taskStatus, m.errChan)
	if err != nil {
		logger.Errorf("创建上传文件任务失败: %v", err)
		return nil, err
	}

	return uploadInfo, nil
}

// TriggerUpload 触发上传操作
// 参数:
//   - taskID: 要上传的任务ID,用于标识具体的上传任务
//
// 返回值:
//   - error: 如果触发过程中发生错误,返回错误信息;否则返回 nil
func (m *UploadManager) TriggerUpload(taskID string, checkNodesAndSend bool) error {
	logger.Infof("开始触发上传任务: %s", taskID)

	// 检查任务状态是否存在
	if status, exists := m.segmentStatuses[taskID]; exists {
		// 等待任务状态变为就绪(true)
		status.WaitForSpecificState(true)

	} else {
		// 如果找不到任务状态,记录警告日志并返回错误
		logger.Warnf("未找到任务状态, taskID: %s", taskID)
		return fmt.Errorf("未找到任务状态, taskID: %s", taskID)
	}

	if checkNodesAndSend {
		// 检查是否存在可用的发送节点
		if !m.ps.Client().HasAvailableNodes(protocol.ID(SendingToNetworkProtocol)) {
			logger.Warn("没有可用的发送节点")
			return fmt.Errorf("没有可用的发送节点")
		}
	}

	// 检查是否达到上传允许的最大并发数
	if m.IsMaxConcurrencyReached() {
		logger.Errorf("已达到上传允许的最大并发数 %d", MaxSessions)
		return fmt.Errorf("已达到上传允许的最大并发数 %d", MaxSessions)
	}

	// 移除指定任务ID的上传任务,如果存在
	m.removeTask(taskID)
	// 判断statusChan是否已经关闭
	m.EnsureChannelOpen()

	// 创建新的任务实例
	uploadTask := NewUploadTask(
		m.ctx,        // 上下文对象,用于控制任务的生命周期
		m.opt,        // 配置选项,包含上传相关的各种参数设置
		m.db,         // 数据库实例,用于持久化存储任务信息
		m.fs,         // 文件系统实例,用于文件读写操作
		m.host,       // 主机信息,包含节点的网络地址等
		m.ps,         // 点对点传输实例,用于文件分片的发送和转发
		m.nps,        // 发布订阅系统,用于任务状态变更通知
		m.scheme,     // 加密方案,用于文件加密和解密
		m.statusChan, // 状态通知通道,用于向上层反馈任务进度
		m.errChan,    // 错误通道,用于向上层反馈错误信息
		taskID,       // 任务唯一标识符,用于区分不同的上传任务
	)

	// 添加一个新的上传任务
	if err := m.addTask(uploadTask); err != nil {
		logger.Errorf("添加上传任务失败: %v", err)
		return err
	}

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(m.db.BadgerDB)

	// 1. 更新文件状态为完成
	if err := uploadFileStore.UpdateUploadFileStatus(
		taskID,
		pb.UploadStatus_UPLOAD_STATUS_UPLOADING,
		time.Now().Unix(),
	); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", taskID, err)
		return err
	}

	if checkNodesAndSend {
		// 使用 select 语句来避免阻塞,尝试将任务ID发送到上传通道
		select {
		case m.uploadChan <- taskID:
			// 成功将任务ID发送到上传通道
			logger.Infof("已触发任务 %s 的上传", taskID)
			return nil
		case <-time.After(5 * time.Second):
			// 发送超时,需要清理已创建的任务
			m.removeTask(taskID)
			logger.Errorf("触发任务 %s 上传超时", taskID)
			return fmt.Errorf("触发任务 %s 上传超时", taskID)
		}
	} else {
		return nil

	}

}

// 检查通道是否关闭，并在关闭时重新初始化
func (m *UploadManager) EnsureChannelOpen() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 确保通道未关闭
	select {
	case _, ok := <-m.statusChan:
		if !ok {
			// 通道已关闭
			m.statusChan = make(chan *pb.UploadChan, 1)
		}
	default:

	}
}

// PauseUpload 暂停上传
// 参数:
//   - taskID: 要暂停的任务ID,用于标识具体的上传任务
//
// 返回值:
//   - error: 如果暂停过程中发生错误,返回错误信息;否则返回 nil
func (m *UploadManager) PauseUpload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*UploadTask)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(task.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到上传文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {
	case pb.UploadStatus_UPLOAD_STATUS_PENDING,
		pb.UploadStatus_UPLOAD_STATUS_UPLOADING:

		// 取消上下文
		task.cancel()
		// 更新文件状态为暂停
		uploadFileStore := database.NewUploadFileStore(task.db)
		if err := uploadFileStore.UpdateUploadFileStatus(
			task.taskId,
			pb.UploadStatus_UPLOAD_STATUS_PAUSED,
			time.Now().Unix(),
		); err != nil {
			logger.Errorf("更新文件状态失败: taskID=%s, err=%v", task.taskId, err)
			return err
		}

		return nil

	case pb.UploadStatus_UPLOAD_STATUS_PAUSED:
		return fmt.Errorf("任务已经处于暂停状态: %s", taskID)

	case pb.UploadStatus_UPLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法暂停: %s", taskID)

	case pb.UploadStatus_UPLOAD_STATUS_CANCELED:
		return fmt.Errorf("任务已取消，无法暂停: %s", taskID)

	default:
		return fmt.Errorf("任务状态(%s)不支持暂停操作: %s",
			fileRecord.Status.String(), taskID)
	}
}

// ResumeUpload 继续上传
// 参数:
//   - taskID: 要继续上传的任务ID,用于标识具体的上传任务
//
// 返回值:
//   - error: 如果继续上传过程中发生错误,返回相应的错误信息
func (m *UploadManager) ResumeUpload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*UploadTask)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(task.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到上传文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {
	case pb.UploadStatus_UPLOAD_STATUS_PENDING, // 待上传
		pb.UploadStatus_UPLOAD_STATUS_UPLOADING, // 上传中
		pb.UploadStatus_UPLOAD_STATUS_PAUSED,    // 已暂停
		pb.UploadStatus_UPLOAD_STATUS_FAILED:    // 失败
		// 这些状态允许继续上传
		if err := m.TriggerUpload(taskID, true); err != nil {
			logger.Errorf("继续上传失败: %v", err)
			return err
		}
		return nil

	case pb.UploadStatus_UPLOAD_STATUS_COMPLETED: // 已完成
		return fmt.Errorf("任务已完成，无法继续上传: %s", taskID)

	case pb.UploadStatus_UPLOAD_STATUS_CANCELED: // 已取消
		return fmt.Errorf("任务已取消，无法继续上传: %s", taskID)

	default: // 其他状态（异常状态）
		return fmt.Errorf("任务状态异常(%s)，无法继续上传: %s",
			fileRecord.Status.String(), taskID)
	}
}

// CancelUpload 取消上传
// 参数:
//   - taskID: 要取消的任务ID,用于标识具体的上传任务
//
// 返回值:
//   - error: 如果取消过程中发生错误,返回错误信息;否则返回 nil
func (m *UploadManager) CancelUpload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*UploadTask)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(task.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到上传文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {
	case pb.UploadStatus_UPLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法取消: %s", taskID)

	case pb.UploadStatus_UPLOAD_STATUS_CANCELED:
		return fmt.Errorf("任务已经处于取消状态: %s", taskID)

	default:

		// 更新任务状态为取消
		fileRecord.Status = pb.UploadStatus_UPLOAD_STATUS_CANCELED
		if err := uploadFileStore.UpdateUploadFile(fileRecord); err != nil {
			logger.Errorf("更新任务状态失败: taskID=%s, err=%v", taskID, err)
			return err
		}

		// 从任务管理器中移除任务
		//m.tasks.Delete(taskID)
		return nil
	}
}

// DeleteUpload 删除上传任务
// 参数:
//   - taskID: 要删除的任务ID,用于标识具体的上传任务
//
// 返回值:
//   - error: 如果删除过程中发生错误,返回错误信息;否则返回 nil
func (m *UploadManager) DeleteUpload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*UploadTask)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(task.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到上传文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {

	case pb.UploadStatus_UPLOAD_STATUS_UPLOADING:
		return fmt.Errorf("任务正在上传中，无法删除: %s", taskID)
	default:
		// 创建存储实例
		uploadFileStore := database.NewUploadFileStore(task.db)
		uploadSegmentStore := database.NewUploadSegmentStore(task.db)

		// 删除文件记录
		if err := uploadFileStore.DeleteUploadFile(task.taskId); err != nil {
			logger.Errorf("删除文件记录失败: taskID=%s, err=%v", task.taskId, err)
			return err
		}
		// 删除所有文件片段
		if err := uploadSegmentStore.DeleteUploadSegmentByTaskID(task.taskId); err != nil {
			logger.Errorf("删除文件片段失败: taskID=%s, err=%v", task.taskId, err)
			return err
		}
		// 从任务管理器中移除任务
		m.tasks.Delete(taskID)
		return nil
	}
}

// GetAllUploadFilesSummaries 获取所有上传记录的概要信息
// 返回值:
//   - []*pb.UploadFilesSummaries: 包含所有上传记录概要信息的切片,每个元素包含任务ID、文件名、大小等信息
//   - error: 如果获取过程中发生错误,返回错误信息;否则返回 nil
func (m *UploadManager) GetAllUploadFilesSummaries() ([]*pb.UploadFilesSummaries, error) {
	var summaries []*pb.UploadFilesSummaries
	var lastError error

	// 遍历所有上传任务
	m.tasks.Range(func(_, value interface{}) bool {
		task := value.(*UploadTask)

		// 创建存储实例
		uploadFileStore := database.NewUploadFileStore(task.db)

		// 获取上传文件记录
		fileRecord, exists, err := uploadFileStore.GetUploadFile(task.taskId)
		if err != nil {
			logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", task.taskId, err)
			lastError = err
			return false
		}
		if !exists {
			return false
		}

		progress, err := task.GetProgress()
		if err != nil {
			logger.Errorf("获取上传进度失败: taskID=%s, err=%v", task.TaskID(), err)
			lastError = err
			return false
		}

		// 获取原始文件大小
		var parityShards int64

		// 遍历分片表,统计总分片数和冗余分片数
		for _, hashTable := range fileRecord.SliceTable {
			if hashTable.IsRsCodes {
				parityShards++ // 如果是校验分片则增加校验分片计数
			}
		}

		// 计算奇偶校验片段总大小
		paritySize := int64(parityShards) * task.opt.GetShardSize()

		// 为每个任务创建一个摘要对象
		summary := &pb.UploadFilesSummaries{
			TaskId:       task.TaskID(),                          // 任务ID
			Name:         fileRecord.FileMeta.Name,               // 文件名称
			Extension:    fileRecord.FileMeta.Extension,          // 文件扩展名
			TotalSize:    fileRecord.FileMeta.Size_ + paritySize, // 文件总大小（包括原始文件和奇偶校验片段）
			UploadStatus: fileRecord.Status,                      // 上传状态
			Progress:     progress,                               // 上传进度
		}
		summaries = append(summaries, summary)
		return true
	})

	return summaries, lastError
}

// QueryFileAssets 查询文件资产记录
// 参数:
//   - db: BadgerDB存储实例,用于数据持久化
//   - pubkeyHash: 所有者的公钥哈希,用于权限验证
//   - start: 起始记录索引
//   - pageSize: 每页的最大记录数
//   - query: 查询条件字符串
//   - options: 额外的查询选项,用于设置查询条件
//
// 返回值:
//   - []*pb.FileAssetRecord: 查询到的文件资产记录切片
//   - uint64: 符合查询条件的总记录数
//   - int: 当前页数
//   - int: 每页的最大记录数
//   - error: 如果查询过程中发生错误,返回错误信息
func QueryFileAssets(db *badgerhold.Store, pubkeyHash []byte, start, pageSize int, query string, options ...database.QueryOption) ([]*pb.FileAssetRecord, uint64, int, int, error) {
	// 创建 FileAssetStore 实例
	store := database.NewFileAssetStore(db)

	// 查询文件资产记录
	transactionRecords, totalCount, err := store.QueryFileAssets(pubkeyHash, start, pageSize, query, options...)
	if err != nil {
		logger.Errorf("查询文件资产记录失败: %v", err)
		return nil, 0, 0, 0, err
	}

	// 计算当前页数
	currentPage := start/pageSize + 1
	return transactionRecords, totalCount, currentPage, pageSize, nil
}
