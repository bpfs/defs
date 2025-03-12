package uploads

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/pointsub"
)

// ForwardTask 文件片段转发任务
type ForwardTask struct {
	SegmentID  string                 // 分片ID
	Payload    *pb.FileSegmentStorage // 存储原始 payload（不包含内容）
	RetryCount int                    // 重试计数
	Timestamp  time.Time              // 创建时间戳
}

// ForwardQueueManager 转发队列管理器
type ForwardQueueManager struct {
	queue        chan *ForwardTask     // 使用带缓冲的通道作为队列
	processing   sync.Map              // 记录正在处理的任务
	workerLimit  chan struct{}         // 限制并发工作协程数
	db           *database.DB          // 数据库连接
	routingTable *kbucket.RoutingTable // 路由表
	host         host.Host             // 主机实例
	ctx          context.Context       // 上下文
}

// NewForwardQueueManager 创建转发队列管理器
func NewForwardQueueManager(ctx context.Context, workerCount int, db *database.DB,
	routingTable *kbucket.RoutingTable, host host.Host) *ForwardQueueManager {
	fqm := &ForwardQueueManager{
		queue:        make(chan *ForwardTask, 1000), // 队列容量设置更大
		workerLimit:  make(chan struct{}, workerCount),
		db:           db,
		routingTable: routingTable,
		host:         host,
		ctx:          ctx,
	}

	// 启动工作协程
	for i := 0; i < workerCount; i++ {
		go fqm.worker()
	}

	// 启动清理协程
	go fqm.startCleaner()

	logger.Info("转发队列管理器已启动，工作协程数: ", workerCount)
	return fqm
}

// Submit 提交转发任务
func (fqm *ForwardQueueManager) Submit(payload *pb.FileSegmentStorage) {
	if payload == nil || payload.SegmentId == "" {
		logger.Warn("无法提交转发任务：payload为空或SegmentID为空")
		return
	}

	segmentID := payload.SegmentId

	// 检查是否已在处理中
	if _, exists := fqm.processing.LoadOrStore(segmentID, true); exists {
		// 已在处理队列中，无需重复提交
		return
	}

	// 创建转发任务，复制所有必要的元数据，但不复制内容
	clonedPayload := &pb.FileSegmentStorage{
		SegmentId:     payload.SegmentId,
		FileId:        payload.FileId,
		Name:          payload.Name,
		Extension:     payload.Extension,
		Size_:         payload.Size_,
		ContentType:   payload.ContentType,
		Sha256Hash:    payload.Sha256Hash,
		UploadTime:    payload.UploadTime,
		P2PkhScript:   payload.P2PkhScript,
		P2PkScript:    payload.P2PkScript,
		SliceTable:    payload.SliceTable, // 关键字段：存储文件哈希表
		SegmentIndex:  payload.SegmentIndex,
		Crc32Checksum: payload.Crc32Checksum,
		EncryptionKey: payload.EncryptionKey,
		Signature:     payload.Signature,
		Shared:        payload.Shared,
		Version:       payload.Version,
		// SegmentContent 字段不复制，保持为 nil
	}

	// 创建任务
	task := &ForwardTask{
		SegmentID:  segmentID,
		Payload:    clonedPayload,
		RetryCount: 0,
		Timestamp:  time.Now(),
	}

	// 尝试放入队列
	select {
	case fqm.queue <- task:
		// 成功提交
	default:
		// 队列已满，移除处理标记
		fqm.processing.Delete(segmentID)
		logger.Warnf("转发队列已满，无法提交任务：%s", segmentID)

		// 延迟重试
		time.AfterFunc(5*time.Second, func() {
			logger.Infof("尝试重新提交转发任务：%s", segmentID)
			fqm.Submit(clonedPayload)
		})
	}
}

// worker 工作协程
func (fqm *ForwardQueueManager) worker() {
	for {
		select {
		case <-fqm.ctx.Done():
			return
		case task := <-fqm.queue:
			// 获取工作令牌
			fqm.workerLimit <- struct{}{}

			// 执行转发
			err := fqm.processForwardTask(task)

			// 释放令牌
			<-fqm.workerLimit

			// 处理完成，移除处理标记
			fqm.processing.Delete(task.SegmentID)

			// 处理重试逻辑
			if err != nil && task.RetryCount < 3 {
				task.RetryCount++
				// 使用指数退避策略
				delay := time.Duration(1<<task.RetryCount) * time.Second

				time.AfterFunc(delay, func() {
					fqm.Submit(task.Payload)
				})

				logger.Warnf("转发任务失败，将在 %v 后重试: %s, 错误: %v",
					delay, task.SegmentID, err)
			}
		}
	}
}

// processForwardTask 处理转发任务
func (fqm *ForwardQueueManager) processForwardTask(task *ForwardTask) error {
	// 记录初始信息
	logger.Debugf("开始处理转发任务: segmentID=%s", task.SegmentID)

	// 确保SliceTable字段不为空，这是关键字段
	if len(task.Payload.SliceTable) == 0 {
		logger.Errorf("转发失败: SliceTable为空, segmentID=%s", task.SegmentID)
		return fmt.Errorf("SliceTable为空，无法转发")
	}

	// 从数据库获取数据内容
	store := database.NewFileSegmentStorageSqlStore(fqm.db.SqliteDB)
	segmentStorage, err := store.GetFileSegmentStorage(task.SegmentID)
	if err != nil {
		logger.Errorf("获取文件片段存储记录失败, segmentID:%s, err:%v", task.SegmentID, err)
		return err
	}

	// 验证数据
	if len(segmentStorage.SegmentContent) == 0 {
		logger.Errorf("文件片段内容为空: segmentID=%s", task.SegmentID)
		return fmt.Errorf("文件片段内容为空")
	}

	// 保存原始payload的引用
	payload := task.Payload

	// 只添加内容字段，保留其他所有元数据
	payload.SegmentContent = segmentStorage.SegmentContent

	logger.Debugf("已准备转发数据: segmentID=%s, fileId=%s, 内容大小=%d字节",
		payload.SegmentId, payload.FileId, len(payload.SegmentContent))

	// 根据存储策略确定目标节点数
	targetNodeCount := 3 // 根据存储策略确定

	// 获取候选节点
	closestPeers := fqm.routingTable.NearestPeers(kbucket.ConvertKey(task.SegmentID), targetNodeCount, 2)
	if len(closestPeers) == 0 {
		logger.Warnf("未找到合适的节点处理转发: segmentID=%s", task.SegmentID)
		return fmt.Errorf("未找到合适的节点")
	}

	logger.Infof("找到 %d 个候选节点进行转发: segmentID=%s", len(closestPeers), task.SegmentID)

	// 尝试向可用节点转发
	var lastErr error
	for i, peer := range closestPeers {
		logger.Debugf("正在转发到节点 %d/%d: peer=%s", i+1, len(closestPeers), peer.String())

		if err := fqm.forwardToNode(peer, payload); err != nil {
			logger.Warnf("转发到节点失败: peer=%s, err=%v", peer.String(), err)
			lastErr = err
			continue
		}

		// 转发成功
		logger.Infof("成功转发片段到节点: segmentID=%s, peer=%s",
			task.SegmentID, peer.String())

		// 清理内存
		payload.SegmentContent = nil
		runtime.GC()

		return nil
	}

	// 清理内存
	payload.SegmentContent = nil
	runtime.GC()

	// 所有节点都失败
	logger.Errorf("转发失败: 所有 %d 个候选节点都无法完成转发, segmentID=%s",
		len(closestPeers), task.SegmentID)
	return lastErr
}

// forwardToNode 转发到单个节点
func (fqm *ForwardQueueManager) forwardToNode(peer peer.ID, payload *pb.FileSegmentStorage) error {
	// 建立连接
	conn, err := pointsub.Dial(fqm.ctx, fqm.host, peer, protocol.ID(StreamForwardToNetworkProtocol))
	if err != nil {
		logger.Errorf("连接节点失败: peer=%s, err=%v", peer.String(), err)
		return err
	}
	defer conn.Close()

	// 创建ProtocolHandler实例
	protocolHandler := NewProtocolHandler(conn)

	// 写入数据
	if err := protocolHandler.WriteSegmentData(payload); err != nil {
		logger.Errorf("写入数据失败: peer=%s, err=%v", peer.String(), err)
		return err
	}

	// 读取响应
	return protocolHandler.ReadResponse()
}

// startCleaner 启动清理协程
func (fqm *ForwardQueueManager) startCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-fqm.ctx.Done():
			return
		case <-ticker.C:
			// 释放未使用内存
			runtime.GC()

			// 打印队列状态
			queueLen := len(fqm.queue)
			var processingCount int
			fqm.processing.Range(func(_, _ interface{}) bool {
				processingCount++
				return true
			})

			logger.Infof("转发队列状态: 队列长度=%d, 处理中=%d",
				queueLen, processingCount)
		}
	}
}

// GetStats 获取转发队列的统计信息
func (fqm *ForwardQueueManager) GetStats() map[string]interface{} {
	// 计算处理中的任务数量
	var processingCount int
	fqm.processing.Range(func(_, _ interface{}) bool {
		processingCount++
		return true
	})

	return map[string]interface{}{
		"queue_length":     len(fqm.queue),
		"processing_count": processingCount,
		"worker_limit":     cap(fqm.workerLimit),
	}
}
