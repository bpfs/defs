package downloads

import (
	"crypto/ecdsa"
	"fmt"
	"strings"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"

	logging "github.com/dep2p/log"
)

var logger = logging.Logger("downloads")

// NewDownload 创建新的下载任务
// 参数:
//   - fileID: 文件唯一标识,要下载的文件ID
//   - ownerPriv: 所有者的私钥,用于签名和权限验证
//
// 返回值:
//   - string: 下载任务ID
//   - error: 如果创建失败则返回错误原因
func (m *DownloadManager) NewDownload(
	ownerPriv *ecdsa.PrivateKey,
	fileID string,
) (string, error) {
	// 检查服务端节点数量是否足够
	minNodes := m.opt.GetMinDownloadServerNodes()
	if m.routingTable.Size(2) < minNodes {
		logger.Warnf("下载时所需服务端节点不足: 当前%d, 需要%d", m.routingTable.Size(2), minNodes)
		return "", fmt.Errorf("下载时所需服务端节点不足: 当前%d, 需要%d", m.routingTable.Size(2), minNodes)
	}

	// 删除文件ID两端的空白字符
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		logger.Error("文件唯一标识不可为空")
		return "", fmt.Errorf("文件唯一标识不可为空")
	}

	// 如果未提供所有者私钥，则使用默认私钥
	if ownerPriv == nil {
		ownerPriv = m.Options().GetDefaultOwnerPriv()
		if ownerPriv == nil {
			logger.Error("用户密钥不可为空")
			return "", fmt.Errorf("用户密钥不可为空")
		}
	}

	// 检查指定文件是否正在下载中（包括获取信息、等待下载、下载中和暂停状态）
	if m.IsFileDownloading(fileID) {
		logger.Errorf("文件 %s 正在下载中", fileID)
		return "", fmt.Errorf("文件 %s 正在下载中", fileID)
	}

	// 生成任务ID,使用所有者私钥签名生成唯一标识
	taskID, err := files.GenerateTaskID(ownerPriv)
	if err != nil {
		logger.Errorf("生成任务ID失败: %v", err)
		return "", err
	}

	// 将私钥转换为字节数组格式,用于后续处理
	privateKeyBytes, err := files.MarshalPrivateKey(ownerPriv)
	if err != nil {
		logger.Errorf("将私钥转换为字节数组失败: %v", err)
		return "", err
	}

	// 通过私钥字节生成公钥哈希,用于身份验证
	pubkeyHash, err := files.PrivateKeyBytesToPublicKeyHash(privateKeyBytes)
	if err != nil {
		logger.Errorf("通过私钥字节生成公钥哈希失败: %v", err)
		return "", err
	}

	// 生成密钥分片,用于文件加密和访问控制
	shares, err := files.GenerateKeyShares(ownerPriv, fileID)
	if err != nil {
		logger.Errorf("生成密钥分片失败: %v", err)
		return "", err
	}

	// 创建新的下载文件任务
	downloadInfo, err := NewDownloadFile(
		m.ctx,      // 上下文
		m.db,       // 数据库实例
		m.host,     // 主机实例
		m.nps,      // 发布订阅系统
		taskID,     // 任务ID
		fileID,     // 文件ID
		pubkeyHash, // 公钥哈希
		shares[0],  // 第一个密钥分片
	)
	if err != nil {
		logger.Errorf("创建下载文件任务失败: %v", err)
		return "", err
	}

	// 触发下载操作
	if err := m.TriggerDownload(downloadInfo.TaskId); err != nil {
		logger.Errorf("触发下载操作失败: %v", err)
		return "", err
	}

	return downloadInfo.TaskId, nil
}

// NewShareDownload 创建新的共享下载任务
// 参数:
//   - fileID: 文件唯一标识,要下载的文件ID
//   - ownerPriv: 所有者的私钥,用于签名和权限验证
//
// 返回值:
//   - string: 下载任务ID
//   - error: 如果创建失败则返回错误原因
func (m *DownloadManager) NewShareDownload(
	ownerPriv *ecdsa.PrivateKey,
	fileID string,
	firstKeyShare []byte,
) (string, error) {
	// 检查服务端节点数量是否足够
	minNodes := m.opt.GetMinDownloadServerNodes()
	if m.routingTable.Size(2) < minNodes {
		logger.Warnf("下载时所需服务端节点不足: 当前%d, 需要%d", m.routingTable.Size(2), minNodes)
		return "", fmt.Errorf("下载时所需服务端节点不足: 当前%d, 需要%d", m.routingTable.Size(2), minNodes)
	}

	// 删除文件ID两端的空白字符
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		logger.Error("文件唯一标识不可为空")
		return "", fmt.Errorf("文件唯一标识不可为空")
	}

	// 如果未提供所有者私钥，则使用默认私钥
	if ownerPriv == nil {
		ownerPriv = m.Options().GetDefaultOwnerPriv()
		if ownerPriv == nil {
			logger.Error("用户密钥不可为空")
			return "", fmt.Errorf("用户密钥不可为空")
		}
	}

	// 检查指定文件是否正在下载中（包括获取信息、等待下载、下载中和暂停状态）
	if m.IsFileDownloading(fileID) {
		logger.Errorf("文件 %s 正在下载中", fileID)
		return "", fmt.Errorf("文件 %s 正在下载中", fileID)
	}

	// 生成任务ID,使用所有者私钥签名生成唯一标识
	taskID, err := files.GenerateTaskID(ownerPriv)
	if err != nil {
		logger.Errorf("生成任务ID失败: %v", err)
		return "", err
	}

	// 创建新的下载文件任务
	downloadInfo, err := NewDownloadFile(
		m.ctx,         // 上下文
		m.db,          // 数据库实例
		m.host,        // 主机实例
		m.nps,         // 发布订阅系统
		taskID,        // 任务ID
		fileID,        // 文件ID
		nil,           // 公钥哈希
		firstKeyShare, // 第?个密钥分片
	)
	if err != nil {
		logger.Errorf("创建下载文件任务失败: %v", err)
		return "", err
	}

	// 触发下载操作
	if err := m.TriggerDownload(downloadInfo.TaskId); err != nil {
		logger.Errorf("触发下载操作失败: %v", err)
		return "", err
	}

	return downloadInfo.TaskId, nil
}

// TriggerDownload 触发指定任务ID的下载操作
// 参数:
//   - taskID: 要触发下载的任务ID
//
// 返回值:
//   - error: 如果触发失败则返回错误原因
func (m *DownloadManager) TriggerDownload(taskID string) error {
	logger.Infof("开始触发下载任务: %s", taskID)

	// 检查服务端节点数量是否足够
	minNodes := m.opt.GetMinDownloadServerNodes()
	if m.routingTable.Size(2) < minNodes {
		logger.Warnf("下载时所需服务端节点不足: 当前%d, 需要%d", m.routingTable.Size(2), minNodes)
		return fmt.Errorf("下载时所需服务端节点不足: 当前%d, 需要%d", m.routingTable.Size(2), minNodes)
	}

	// 检查是否达到下载允许的最大并发数
	if m.IsMaxConcurrencyReached() {
		logger.Errorf("已达到下载允许的最大并发数 %d", MaxSessions)
		return fmt.Errorf("已达到下载允许的最大并发数 %d", MaxSessions)
	}

	// 移除指定任务ID的下载任务，如果存在
	m.removeTask(taskID)

	// 创建并初始化新的下载任务
	downloadTask, err := NewDownloadTask(
		m.ctx, // 上下文
		m.opt, // 配置选项
		m.db,  // 数据库实例
		m.fs,
		m.host,         // 主机实例
		m.routingTable, // 路由表
		m.nps,          // 发布订阅系统
		m.statusChan,
		m.errChan,
		taskID, // 任务ID
	)
	if err != nil {
		logger.Errorf("初始化下载实例时失败: %v", err)
		return err
	}

	// 添加一个新的下载任务
	if err := m.addTask(downloadTask); err != nil {
		logger.Errorf("添加下载任务失败: %v", err)
		return err
	}

	// 使用 select 语句尝试将任务ID发送到下载通道
	select {
	case m.downloadChan <- taskID:
		// 成功将任务ID发送到下载通道
		logger.Infof("已触发任务 %s 的下载", taskID)
		return nil
	case <-time.After(5 * time.Second):
		m.removeTask(taskID)
		// 果 5 秒内无法发送，则返回超时错误
		logger.Errorf("触发任务 %s 下载超时", taskID)
		return fmt.Errorf("触发任务 %s 下载超时", taskID)
	}
}

// PauseDownload 暂停下载任务
// 参数:
//   - taskID: 要暂停的任务ID
//
// 返回值:
//   - error: 如果暂停过程中发生错误，返回错误信息
func (m *DownloadManager) PauseDownload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*DownloadTask)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(m.db.BadgerDB)

	// 获取下载文件记录
	fileRecord, exists, err := downloadFileStore.Get(taskID)
	if err != nil {
		logger.Errorf("获取下载文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到下载文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {
	case pb.DownloadStatus_DOWNLOAD_STATUS_PENDING,
		pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING:
		// 可以暂停的状态
		if err := task.Pause(); err != nil {
			logger.Errorf("暂停下载失败: taskID=%s, err=%v", taskID, err)
			return err
		}
		return nil

	case pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED:
		return fmt.Errorf("任务已经处于暂停状态: %s", taskID)

	case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法暂停: %s", taskID)

	case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED:
		return fmt.Errorf("任务已取消，无法暂停: %s", taskID)

	default:
		return fmt.Errorf("任务状态(%s)不支持暂停操作: %s",
			fileRecord.Status.String(), taskID)
	}
}

// ResumeDownload 继续下载任务
// 参数:
//   - taskID: 要继续下载的任务ID
//
// 返回值:
//   - error: 如果继续下载过程中发生错误，返回错误信息
func (m *DownloadManager) ResumeDownload(taskID string) error {
	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(m.db.BadgerDB)

	// 获取下载文件记录
	fileRecord, exists, err := downloadFileStore.Get(taskID)
	if err != nil {
		logger.Errorf("获取下载文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到下载文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {
	case pb.DownloadStatus_DOWNLOAD_STATUS_PENDING,
		pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING,
		pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED,
		pb.DownloadStatus_DOWNLOAD_STATUS_FAILED:
		// 这些状态允许继续下载
		if err := m.TriggerDownload(taskID); err != nil {
			logger.Errorf("继续下载失败: %v", err)
			return err
		}
		return nil

	case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法继续下载: %s", taskID)

	case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED:
		return fmt.Errorf("任务已取消，无法继续下载: %s", taskID)

	default:
		return fmt.Errorf("任务状态(%s)不支持继续下载: %s",
			fileRecord.Status.String(), taskID)
	}
}

// CancelDownload 取消下载任务
// 参数:
//   - taskID: 要取消的任务ID
//
// 返回值:
//   - error: 如果取消过程中发生错误，返回错误信息
func (m *DownloadManager) CancelDownload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*DownloadTask)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(m.db.BadgerDB)

	// 获取下载文件记录
	fileRecord, exists, err := downloadFileStore.Get(taskID)
	if err != nil {
		logger.Errorf("获取下载文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("未找到下载文件记录: %s", taskID)
	}

	// 检查任务状态
	switch fileRecord.Status {
	case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED:
		return fmt.Errorf("任务已完成，无法取消: %s", taskID)

	case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED:
		return fmt.Errorf("任务已经处于取消状态: %s", taskID)

	default:
		// 其他状态都可以取消
		if err := task.Cancel(); err != nil {
			logger.Errorf("取消下载失败: taskID=%s, err=%v", taskID, err)
			return err
		}

		// 从任务管理器中移除任务
		m.tasks.Delete(taskID)
		return nil
	}
}

// DeleteDownload 删除下载任务
// 参数:
//   - taskID: 要删除的任务ID
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回错误信息
func (m *DownloadManager) DeleteDownload(taskID string) error {
	// 获取任务实例
	taskValue, exists := m.tasks.Load(taskID)
	if !exists {
		logger.Errorf("任务不存在: %s", taskID)
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task := taskValue.(*DownloadTask)

	// 触发删除操作
	if err := task.Delete(); err != nil {
		logger.Errorf("删除下载任务失败: taskID=%s, err=%v", taskID, err)
		return err
	}

	// 从任务管理器中移除任务
	m.tasks.Delete(taskID)
	return nil
}

// QueryDownload 查询下载任务记录并返回分页信息
// 参数:
//   - start: 查询的起始位置(从0开始)
//   - pageSize: 每页显示的记录数
//   - options: 可选的查询条件(如状态过滤、时间范围等)
//
// 返回值:
//   - []*pb.DownloadFileRecord: 下载任务记录列表
//   - uint64: 符合查询条件的总记录数
//   - int: 当前页码(从1开始)
//   - int: 每页大小
//   - error: 如果查询���败则返回错误原因
func (m *DownloadManager) QueryDownload(start, pageSize int, options ...database.QueryOption) ([]*pb.DownloadFileRecord, uint64, int, int, error) {
	// 调用底层查询方法获取记录列表和总数
	tasks, totalCount, err := QueryDownloadTask(m.db.BadgerDB, start, pageSize, options...)
	if err != nil {
		logger.Errorf("查询下载任务失败: %v", err)
		return nil, 0, 0, 0, err
	}

	// 计算当前页码(从1开始)
	currentPage := start/pageSize + 1

	return tasks, totalCount, currentPage, pageSize, nil
}
