package downloads

import (
	"crypto/md5"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"

	defsproto "github.com/bpfs/defs/v2/utils/protocol"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/pointsub"
)

const (
	maxWorkersPerPeer = 10                     // 每个节点的最大工作协程数
	maxTotalWorkers   = 50                     // 总的最大工作协程数
	segmentsPerWorker = 10                     // 每个工作协程处理的片段数
	maxVerifyRetries  = 3                      // 最大验证重试次数
	verifyRetryDelay  = time.Second * 5        // 重试等待时间
	batchWindow       = 100 * time.Millisecond // 请求批处理窗口时间
)

// NetworkErrorType 定义网络错误类型
type NetworkErrorType int

const (
	// 临时性错误，可以重试
	TempError NetworkErrorType = iota
	// 严重错误，需要终止任务
	CriticalError
)

// NetworkError 自定义网络错误结构
type NetworkError struct {
	errType NetworkErrorType
	message string
	err     error
}

// Error 实现 error 接口
func (e *NetworkError) Error() string {
	errTypeStr := "临时错误"
	if e.errType == CriticalError {
		errTypeStr = "严重错误"
	}
	return fmt.Sprintf("%s - %s: %v", errTypeStr, e.message, e.err)
}

// handleSegmentIndex 处理片段索引请求
// 从网络获取文件片段的索引信息
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleSegmentIndex() error {
	// 加锁保护共享资源
	t.mu.Lock()
	defer t.mu.Unlock()
	logger.Infof("处理索引清单更新: taskID=%s", t.TaskID())

	// 创建存储实例
	fileStore := database.NewDownloadFileStore(t.db)

	// 获取当前文件记录
	fileRecord, exists, err := fileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, error=%v", t.TaskID(), err)
		return err
	}
	if !exists {
		logger.Errorf("文件记录不存在: taskID=%s", t.taskId)
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 从数据库获取未完成片段列表
	pendingSegmentIds, err := GetPendingSegments(t.db, t.TaskID())
	if err != nil {
		logger.Errorf("获取未完成片段失败: taskID=%s, error=%v", t.TaskID(), err)
		return err
	}

	// 如果没有未完成的片段，检查是否所有片段都已完成
	if len(pendingSegmentIds) == 0 {
		return t.ForceSegmentVerify()
	}

	// 发送索引清单请求到网络
	if err := RequestManifestPubSub(
		t.ctx,
		t.host,
		t.nps,
		t.TaskID(),
		fileRecord.FileId,
		fileRecord.PubkeyHash,
		pendingSegmentIds,
	); err != nil {
		logger.Errorf("发送索引清单请求失败: taskID=%s, error=%v", t.TaskID(), err)
		return err
	}

	// logger.Infof("已发送索引清单请求: taskID=%s, pendingSegments=%d",
	// 	t.TaskID(),
	// 	len(pendingSegmentIds),
	// )

	// 在 HandleDownloadManifestResponsePubSub 中处理索引清单响应
	// 通过 ForceNodeDispatch 强制触发节点分发
	return nil
}

// handleSegmentProcess 处理文件片段
// 主要步骤：
// 1. 从数据库查询待上传的片段
// 2. 为每个片段找到合适的临近节点
// 3. 将片段分配给这些节点
// 4. 更新分片分配管理器
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleSegmentProcess() error {
	logger.Infof("处理片段请求: taskID=%s", t.taskId)

	// 创建下载片段存储实例
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)

	// 获取当前任务中状态为待下载的所有片段
	segments, err := downloadSegmentStore.FindByTaskIDAndStatus(
		t.taskId,
		pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_PENDING,
	)
	if err != nil {
		logger.Errorf("获取待下载片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 如果没有待下载的片段，直接触发片段验证
	if len(segments) == 0 {
		logger.Infof("没有待下载的片段: taskID=%s", t.taskId)
		return t.ForceNodeDispatch()
	}

	// 创建节点到片段ID的映射
	peerSegments := make(map[peer.ID][]string)

	// 获取所有可用的分配信息
	var allDistributions []map[peer.ID][]string
	for {
		if distribution, ok := t.distribution.GetNextDistribution(); ok {
			allDistributions = append(allDistributions, distribution)
		} else {
			break
		}
	}

	// 遍历所有待下载的片段
	for _, segment := range segments {
		// 检查所有可用的分配信息
		for _, distribution := range allDistributions {
			for peerID, segmentIDs := range distribution {
				for _, segmentID := range segmentIDs {
					if segmentID == segment.SegmentId {
						peerSegments[peerID] = append(peerSegments[peerID], segmentID)
						break
					}
				}
			}
		}
	}

	// 将未使用的分配信息重新添加回分配管理器
	for _, distribution := range allDistributions {
		t.distribution.AddDistribution(distribution)
	}

	// 触发网络传输
	if err := t.ForceNetworkTransfer(peerSegments); err != nil {
		logger.Errorf("触发网络传输失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 添加延迟，等待网络传输完成
	time.Sleep(500 * time.Millisecond)

	// 再触发验证
	if err := t.ForceSegmentVerify(); err != nil {
		logger.Errorf("触发验证失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	return nil
}

// handleNodeDispatch 处理节点分发
// 主要步骤：
// 1. 循环从分片分配管理器获取待处理的分配
// 2. 通过 ForceNetworkTransfer 发送到网络传输通道
// 3. 直到队列为空时退出
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleNodeDispatch() error {
	logger.Infof("处理节点分发: taskID=%s", t.taskId)

	totalProcessed := 0 // 处理的总片段数
	totalSuccess := 0   // 成功处理的片段数
	totalFailed := 0    // 处理失败的片段数

	// 循环处理队列中的所有分配任务
	for {
		select {
		case <-t.ctx.Done():
			logger.Warnf("任务已取消: taskID=%s", t.taskId)
			return fmt.Errorf("任务已取消")
		default:
		}

		// 获取下一个待处理的分配
		peerSegments, ok := t.distribution.GetNextDistribution()
		if !ok {
			logger.Infof("没有更多的分配任务: taskID=%s", t.taskId)
			break
		}

		// logger.Infof("获取到新的分配任务: taskID=%s, peerSegments=%v", t.taskId, peerSegments)

		// 计算映射中的总片段数
		segmentCount := countSegments(peerSegments)
		totalProcessed += segmentCount
		// logger.Infof("当前处理的片段数: %d, taskID=%s", segmentCount, t.taskId)

		// 强制触发网络传输
		if err := t.ForceNetworkTransfer(peerSegments); err != nil {
			logger.Errorf("发送片段到网络通道失败: err=%v, taskID=%s", err, t.taskId)
			totalFailed += segmentCount
			if err.Error() == "任务已取消" {
				return err
			}
			continue
		}

		totalSuccess += segmentCount
		// logger.Infof("成功处理的片段数: %d, taskID=%s", segmentCount, t.taskId)
	}

	// 记录处理结果统计
	logger.Infof("完成片段分发处理: 总数=%d, 成功=%d, 失败=%d, taskID=%s",
		totalProcessed, totalSuccess, totalFailed, t.taskId)

	// 强制触发片段验证
	return t.ForceSegmentVerify()
}

// countSegments 计算映射中的总片段数
// 参数:
//   - peerSegments: map[peer.ID][]string 节点与其负责的片段ID映射
//
// 返回值:
//   - int: 映射中的总片段数
func countSegments(peerSegments map[peer.ID][]string) int {
	total := 0
	for _, segments := range peerSegments {
		total += len(segments)
	}
	return total
}

// handleNetworkTransfer 处理网络传输请求
// 主要步骤：
// 1. 获取文件和片段信息
// 2. 建立网络连接
// 3. 发送数据并处理响应
// 4. 更新片段状态
//
// 参数:
//   - peerSegments: map[peer.ID][]string 节点与其负责的片段ID映射
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleNetworkTransfer(peerSegments map[peer.ID][]string) error {
	logger.Infof("处理网络传输: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)

	// 获取下载文件记录
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取下载文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("下载文件记录不存在: taskID=%s", t.taskId)
	}

	// logger.Infof("处理网络传输: taskID=%s, peerCount=%d", t.taskId, len(peerSegments))

	// 创建一个信号量来限制总并发数
	sem := make(chan struct{}, maxTotalWorkers)
	var globalWg sync.WaitGroup
	var errors []error
	errChan := make(chan error, len(peerSegments))

	// 记录已处理的片段，避免重复发送
	processedSegments := make(map[string]bool)
	var processedMu sync.Mutex

	// 遍历所有需要发送的节点
	for peerID, segmentIDs := range peerSegments {
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		default:
		}

		// 过滤掉已处理的片段
		var unprocessedSegments []string
		processedMu.Lock()
		for _, segID := range segmentIDs {
			if !processedSegments[segID] {
				unprocessedSegments = append(unprocessedSegments, segID)
				processedSegments[segID] = true
			}
		}
		processedMu.Unlock()

		// 如果该节点的所有片段都已处理，跳过
		if len(unprocessedSegments) == 0 {
			continue
		}

		globalWg.Add(1)
		go func(peerID peer.ID, segments []string) {
			defer globalWg.Done()
			// 发送数据到节点
			if err := t.sendToPeer(peerID, fileRecord, segments, sem); err != nil {
				logger.Errorf("向节点发送数据失败: peerID=%s, err=%v", peerID, err)
				// 发送失败时，将片段标记为未处理
				processedMu.Lock()
				for _, segID := range segments {
					delete(processedSegments, segID)
				}
				processedMu.Unlock()
				errChan <- &SegmentError{
					PeerID:   peerID,
					Err:      err,
					Segments: segments,
				}
			}
		}(peerID, unprocessedSegments)
	}

	// 等待所有发送完成
	globalWg.Wait()
	close(errChan)

	// 收集所有错误
	for err := range errChan {
		if segErr, ok := err.(*SegmentError); ok {
			errors = append(errors, segErr)
		}
	}

	// 等待片段实际写入完成
	time.Sleep(100 * time.Millisecond)

	// 如果所有节点都失败了，才返回错误
	if len(errors) == len(peerSegments) {
		return fmt.Errorf("所有节点下载失败: %v", errors)
	}

	// 有部分成功就继续
	if len(errors) > 0 {
		logger.Warnf("部分节点下载失败: %v", errors)
	}

	return nil
}

// sendToPeer 向单个节点发送数据
// 主要步骤：
// 1. 计算分片数量
// 2. 动态计算worker数量
// 3. 计算每个worker处理的分片数
// 4. 遍历每个worker，发送分片
// 5. 等待所有发送完成并收集错误
// 参数:
//   - peerID: 目标节点ID
//   - segments: 需要发送的分片列表
//   - sem: 信号量，用于限制并发数
//
// 返回值:
//   - error: 如果发送过程中发生错误，返回相应的错误信息
func (t *DownloadTask) sendToPeer(peerID peer.ID, fileRecord *pb.DownloadFileRecord, segments []string, sem chan struct{}) error {
	segmentCount := len(segments)
	logger.Infof("向节点发送数据: peerID=%s, segmentCount=%d", peerID, segmentCount)

	// 动态计算worker数量
	workerCount := min(
		min(maxWorkersPerPeer, (segmentCount+segmentsPerWorker-1)/segmentsPerWorker),
		maxTotalWorkers,
	)

	if segmentCount == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, workerCount)

	// 计算每个worker处理的分片数
	segmentsPerGoroutine := (segmentCount + workerCount - 1) / workerCount

	for i := 0; i < workerCount; i++ {
		startIdx := i * segmentsPerGoroutine
		endIdx := min((i+1)*segmentsPerGoroutine, segmentCount)

		// 如果startIdx大于segmentCount，则退出
		if startIdx >= segmentCount {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // 获取全局信号量

		go func(workerID int, start, end int) {
			defer wg.Done()
			defer func() { <-sem }()

			// 发送数据到节点
			if err := t.workerSendSegments(peerID, fileRecord, segments[start:end]); err != nil {
				logger.Errorf("worker %d 发送失败: %v", workerID, err)
				errChan <- err
				return
			}
			logger.Infof("worker %d 完成发送: start=%d, end=%d", workerID, start, end)
		}(i, startIdx, endIdx)
	}

	// 等待并收集错误
	go func() {
		wg.Wait()
		close(errChan)
	}()

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("节点 %s 发送过程中出现 %d 个错误: %v", peerID, len(errs), errs)
	}

	return nil
}

// workerSendSegments 工作协程发送片段
// 主要步骤：
// 1. 创建连接
// 2. 设置更大的缓冲区
// 3. 处理每个分片
// 4. 返回错误信息
// 参数:
//   - peerID: 目标节点ID
//   - segments: 需要发送的分片列表
//
// 返回值:
//   - error: 如果发送过程中发生错误，返回相应的错误信息
func (t *DownloadTask) workerSendSegments(peerID peer.ID, fileRecord *pb.DownloadFileRecord, segments []string) error {
	// 创建连接
	conn, err := pointsub.Dial(t.ctx, t.host, peerID, protocol.ID(StreamRequestSegmentProtocol))
	if err != nil {
		logger.Errorf("创建连接失败: %v", err)
		return err
	}
	defer conn.Close()

	// 设置更大的缓冲区
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetWriteBuffer(MaxBlockSize)
		tcpConn.SetReadBuffer(MaxBlockSize)
	}

	// 处理每个分片
	var errors []error
	for _, segmentID := range segments {
		// 发送分片
		if err := t.sendSegment(fileRecord, peerID, segmentID, conn); err != nil {
			logger.Errorf("发送分片 %s 失败: %v", segmentID, err)
			// 下载失败时设置状态为失败
			if updateErr := t.updateSegmentStatus(segmentID, pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_FAILED, peerID.String()); updateErr != nil {
				logger.Warnf("更新片段状态为失败状态失败: %v", updateErr)
			}
			errors = append(errors, err)
		}
	}

	// 如果有可恢复的错误，返回组合错误
	if len(errors) > 0 {
		return &SegmentErrors{
			PeerID: peerID,
			Errors: errors,
		}
	}

	return nil
}

// sendSegment 发送单个分片
// 主要步骤：
// 1. 获取分片数据
// 2. 打印请求消息的详细内容
// 3. 发送请求
// 4. 接收响应
// 5. 检查错误响应
// 6. 验证并存储下载的文件片段
// 7. 更新分片状态
// 8. 获取进度
// 9. 发送成功后通知片段完成
// 参数:
//   - fileRecord: 文件记录
//   - peerID: 目标节点ID
//   - segmentID: 分片ID
//   - conn: 连接
//
// 返回值:
//   - error: 如果发送过程中发生错误，返回相应的错误信息
func (t *DownloadTask) sendSegment(fileRecord *pb.DownloadFileRecord, peerID peer.ID, segmentID string, conn net.Conn) error {
	// 开始下载时设置状态为下载中
	if err := t.updateSegmentStatus(segmentID, pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_DOWNLOADING, peerID.String()); err != nil {
		logger.Warnf("更新片段状态为下载中失败: %v", err)
	}

	// 获取分片数据
	segmentContentRequest, segment, err := t.getSegmentData(fileRecord, segmentID)
	if err != nil {
		logger.Errorf("获取分片数据失败: %v", err)
		return err
	}

	// 打印请求消息的详细内容
	logger.Infof("准备发送请求消息: %+v", segmentContentRequest)

	// 发送请求
	reqMsg := &SegmentRequestMessage{
		Request: segmentContentRequest,
	}

	// 序列化测试
	testData, err := reqMsg.Marshal()
	if err != nil {
		logger.Errorf("测试序列化失败: %v", err)
		return err
	}
	logger.Infof("测试序列化数据(hex): %x", testData)

	// 添加详细的请求日志
	logger.Infof("发送分片请求: taskID=%s, fileID=%s, segmentID=%s, peerID=%s",
		segmentContentRequest.TaskId,
		segmentContentRequest.FileId,
		segmentContentRequest.SegmentId,
		peerID)

	if err := defsproto.NewHandler(conn, &defsproto.Options{
		MaxRetries:     3,
		RetryDelay:     time.Second,
		WriteTimeout:   ConnTimeout,
		ReadTimeout:    ConnTimeout,
		ProcessTimeout: ConnTimeout,
		Rate:           1024 * 1024,      // 1MB/s
		Window:         1024 * 1024 * 10, // 10MB window
		Threshold:      1024 * 1024 * 5,  // 5MB threshold
	}).SendMessage(reqMsg); err != nil {
		logger.Errorf("发送请求失败: taskID=%s, segmentID=%s, err=%v",
			t.TaskID(), segmentID, err)
		return err
	}

	// 接收响应
	respMsg := &SegmentResponseMessage{}
	if err := defsproto.NewHandler(conn, &defsproto.Options{
		MaxRetries:     3,
		RetryDelay:     time.Second,
		WriteTimeout:   ConnTimeout,
		ReadTimeout:    ConnTimeout,
		ProcessTimeout: ConnTimeout,
		Rate:           1024 * 1024,      // 1MB/s
		Window:         1024 * 1024 * 10, // 10MB window
		Threshold:      1024 * 1024 * 5,  // 5MB threshold
	}).ReceiveMessage(respMsg); err != nil {
		logger.Errorf("接收响应失败: taskID=%s, segmentID=%s, err=%v",
			t.TaskID(), segmentID, err)
		return err
	}

	// 处理响应消息
	if respMsg.Response.HasError {
		// 根据错误码进行不同的处理
		switch respMsg.Response.ErrorCode {
		case pb.SegmentError_SEGMENT_ERROR_SEGMENT_NOT_FOUND:
			// 分片不存在，可以尝试从其他节点获取
			logger.Warnf("分片不存在，将尝试其他节点: %s", respMsg.Response.ErrorMessage)
			return fmt.Errorf("分片不存在: %s", respMsg.Response.ErrorMessage)

		case pb.SegmentError_SEGMENT_ERROR_FILE_PERMISSION:
			// 权限错误，可能需要重新验证
			logger.Errorf("文件访问权限错误: %s", respMsg.Response.ErrorMessage)
			return fmt.Errorf("权限错误: %s", respMsg.Response.ErrorMessage)

		case pb.SegmentError_SEGMENT_ERROR_SEGMENT_CORRUPTED:
			// 分片损坏，需要从其他节点重新下载
			logger.Errorf("分片数据损坏: %s", respMsg.Response.ErrorMessage)
			return fmt.Errorf("分片损坏: %s", respMsg.Response.ErrorMessage)

		case pb.SegmentError_SEGMENT_ERROR_SYSTEM:
			// 系统错误(存储空间不足、速率限制、节点忙等)
			switch {
			case strings.Contains(respMsg.Response.ErrorMessage, "存储空间不足"):
				logger.Errorf("节点存储空间不足: %s", respMsg.Response.ErrorMessage)
				return fmt.Errorf("存储空间不足: %s", respMsg.Response.ErrorMessage)
			case strings.Contains(respMsg.Response.ErrorMessage, "速率限制"):
				logger.Warnf("节点速率限制: %s", respMsg.Response.ErrorMessage)
				return fmt.Errorf("速率限制: %s", respMsg.Response.ErrorMessage)
			case strings.Contains(respMsg.Response.ErrorMessage, "节点忙"):
				logger.Warnf("节点忙: %s", respMsg.Response.ErrorMessage)
				return fmt.Errorf("节点忙: %s", respMsg.Response.ErrorMessage)
			default:
				logger.Errorf("系统错误: %s", respMsg.Response.ErrorMessage)
				return fmt.Errorf("系统错误: %s", respMsg.Response.ErrorMessage)
			}

		case pb.SegmentError_SEGMENT_ERROR_NETWORK,
			pb.SegmentError_SEGMENT_ERROR_TIMEOUT:
			// 系统级错误，可以重试
			logger.Errorf("系统错误: %s (错误码: %v)",
				respMsg.Response.ErrorMessage,
				respMsg.Response.ErrorCode)
			return fmt.Errorf("系统错误: %s", respMsg.Response.ErrorMessage)

		default:
			// 其他错误
			logger.Errorf("未知错误: %s (错误码: %v)",
				respMsg.Response.ErrorMessage,
				respMsg.Response.ErrorCode)
			return fmt.Errorf("远程节点返回错误: %s (错误码: %v)",
				respMsg.Response.ErrorMessage,
				respMsg.Response.ErrorCode)
		}
	}

	// 如果没有错误，继续处理正常响应
	if respMsg.Response.SegmentId == "" {
		logger.Errorf("响应中缺少分片ID")
		return fmt.Errorf("响应中缺少分片ID")
	}

	// 验证响应数据
	if err := validateResponse(respMsg.Response); err != nil {
		logger.Errorf("响应数据验证失败: %v", err)
		return fmt.Errorf("响应数据无效: %v", err)
	}

	// 验证并存储下载的文件片段
	if err := ValidateAndStoreSegment(t.db, fileRecord.FirstKeyShare, respMsg.Response); err != nil {
		logger.Errorf("验证并存储下载的文件片段失败: %v", err)
		return err
	}

	// 更新分片状态
	if err := t.updateSegmentStatus(segmentID, pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED, peerID.String()); err != nil {
		logger.Errorf("更新分片状态失败: %v", err)
		return err
	}

	// 异步通知状态更新
	go func() {
		// 获取进度
		progress, err := t.GetProgress()
		if err != nil {
			logger.Errorf("获取进度失败: %v", err)
			return
		}

		// 发送通知
		t.NotifySegmentStatus(&pb.DownloadChan{
			TaskId:           t.taskId,
			IsComplete:       progress == 100,
			DownloadProgress: progress,
			TotalShards:      int64(len(fileRecord.SliceTable)),
			SegmentId:        respMsg.Response.SegmentId,
			SegmentIndex:     respMsg.Response.SegmentIndex,
			SegmentSize:      segment.Size_,
			IsRsCodes:        segment.IsRsCodes,
			NodeId:           peerID.String(),
			DownloadTime:     time.Now().Unix(),
		})
	}()

	return nil
}

// validateResponse 验证响应数据完整性
func validateResponse(resp *pb.SegmentContentResponse) error {
	if resp.FileMeta == nil {
		return fmt.Errorf("文件元数据为空")
	}
	if resp.P2PkScript == nil {
		return fmt.Errorf("P2PK脚本为空")
	}
	if len(resp.Signature) == 0 {
		return fmt.Errorf("签名数据为空")
	}
	return nil
}

// getSegmentData 获取分片数据
// 参数:
//   - segmentID: 分片ID
//
// 返回值:
//   - *pb.FileSegmentStorage: 分片数据
//   - error: 如果获取过程中发生错误，返回相应的错误信息
func (t *DownloadTask) getSegmentData(fileRecord *pb.DownloadFileRecord, segmentID string) (*pb.SegmentContentRequest, *pb.DownloadSegmentRecord, error) {
	// 创建片段存储实例
	store := database.NewDownloadSegmentStore(t.db)

	// 获取片段记录
	segment, exists, err := store.GetDownloadSegmentBySegmentID(segmentID)
	if err != nil {
		logger.Errorf("获取片段记录失败: segmentID=%s, err=%v", segmentID, err)
		return nil, nil, err
	}
	if !exists {
		logger.Warnf("片段记录不存在: segmentID=%s", segmentID)
		return nil, nil, fmt.Errorf("片段记录不存在: segmentID=%s", segmentID)
	}

	// 构造片段内容请求
	// 获取本地节点的完整地址信息
	addrInfo := peer.AddrInfo{
		ID:    t.host.ID(),    // 设置节点ID
		Addrs: t.host.Addrs(), // 设置节点地址列表
	}

	// 序列化地址信息
	addrInfoBytes, err := addrInfo.MarshalJSON()
	if err != nil {
		logger.Errorf("序列化 AddrInfo 失败: %v", err)
		return nil, segment, err
	}

	return &pb.SegmentContentRequest{
		TaskId:     t.taskId,
		FileId:     fileRecord.FileId,
		PubkeyHash: fileRecord.PubkeyHash,
		AddrInfo:   addrInfoBytes,
		SegmentId:  segmentID,
	}, segment, err
}

// handleSegmentVerify 处理片段验证
// 主要步骤：
// 1. 获取文件记录中的切片表信息
// 2. 获取已完成下载的片段数量
// 3. 比较两者是否一致
// 4. 根据验证结果触发后续操作
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleSegmentVerify() error {
	// 获取验证锁
	t.verifyMutex.Lock()
	defer t.verifyMutex.Unlock()

	// 设置验证状态
	if !t.verifyInProgress.CompareAndSwap(false, true) {
		logger.Debug("验证已在进行中，跳过本次验证")
		return nil
	}
	defer t.verifyInProgress.Store(false)

	// 检查重试次数和时间间隔
	if t.verifyRetryCount > 0 {
		if time.Since(t.lastVerifyTime) < verifyRetryDelay {
			logger.Infof("验证请求太频繁，跳过本次验证: taskID=%s, retryCount=%d",
				t.taskId, t.verifyRetryCount)
			return nil
		}
	}

	// 更新验证时间
	t.lastVerifyTime = time.Now()

	logger.Infof("验证片段下载状态: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)

	// 获取文件记录
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 获取文件总片段数
	totalSegments := len(fileRecord.SliceTable)
	if totalSegments == 0 {
		return fmt.Errorf("文件切片表为空: taskID=%s", t.taskId)
	}

	// 获取所有已完成的片段
	segments, err := downloadSegmentStore.FindByTaskIDAndStatus(
		t.taskId,
		pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED,
	)
	if err != nil {
		logger.Errorf("获取已完成片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 获取所需的数据片段数量
	requiredShards := getRequiredDataShards(fileRecord.SliceTable)

	// 打印片段统计信息
	logger.Infof("片段统计: 已下载数=%d, 所需片段数=%d, 总片段数=%d",
		len(segments), requiredShards, len(fileRecord.SliceTable))

	// 检查是否有足够的片段
	if len(segments) < requiredShards {
		logger.Warnf("分片未全部下载完成: 已下载=%d, 所需数据片段=%d",
			len(segments), requiredShards)
		// 触发恢复操作
		if err := t.ForceRecoverySegments(); err != nil {
			logger.Errorf("触发片段恢复失败: %v", err)
		}
		return fmt.Errorf("数据片段未下载完成: 已下载=%d, 所需数据片段=%d",
			len(segments), requiredShards)
	}

	// 只有在有足够的片段时才进行合并
	return t.ForceSegmentMerge()
}

// handleSegmentMerge 处理片段合并
// 主要步骤：
// 1. 获取文件记录和所有已下载片段
// 2. 创建Reed-Solomon解码器
// 3. 合并文件片段
// 4. 验证合并结果
//
// 返回值:
//   - error: 如果合并过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleSegmentMerge() error {
	// 添加合并锁
	t.mergeMutex.Lock()
	defer t.mergeMutex.Unlock()

	// 检查合并状态
	if !t.mergeInProgress.CompareAndSwap(false, true) {
		logger.Debug("合并已在进行中，跳过本次合并")
		return nil
	}
	defer t.mergeInProgress.Store(false)

	logger.Infof("开始合并文件片段: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)

	// 获取文件记录
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 获取所有已完成的片段
	segments, err := downloadSegmentStore.FindByTaskIDAndStatus(
		t.taskId,
		pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED,
	)
	if err != nil {
		logger.Errorf("获取已完成片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 获取所需的数据片段数量
	requiredShards := getRequiredDataShards(fileRecord.SliceTable)

	// 检查是否有足够的片段
	if len(segments) < requiredShards {
		logger.Errorf("没有足够的片段进行合并: 需要 %d 个, 当前有 %d 个",
			len(segments), requiredShards)
		// 如果片段不足，触发继续下载
		return t.ForceSegmentProcess()
	}

	// 按片段索引排序
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].SegmentIndex < segments[j].SegmentIndex
	})

	// 创建临时文件用于存储合并后的数据
	tempFilePath := filepath.Join(fileRecord.TempStorage, fileRecord.FileId+".defs")
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return err
	}
	defer tempFile.Close()

	// 逐个处理片段
	for _, segment := range segments {
		// 读取加密数据
		encryptedData, err := os.ReadFile(segment.StoragePath)
		if err != nil {
			logger.Errorf("读取临时文件失败: %v", err)
			return err
		}

		// 解密并解压数据
		decryptedData, err := DecompressAndDecryptSegmentContent(
			fileRecord.FirstKeyShare,
			segment.EncryptionKey, // 使用存储的加密密钥
			encryptedData,
			segment.Crc32Checksum,
		)
		if err != nil {
			logger.Errorf("解密解压片段失败: %v", err)
			return err
		}

		// 写入解密后的数据
		if _, err := tempFile.Write(decryptedData); err != nil {
			logger.Errorf("写入临时文件失败: %v", err)
			return err
		}

		// 清理临时文件
		os.Remove(segment.StoragePath)
	}

	// 构建基础文件路径
	basePath := filepath.Join(fileRecord.TempStorage, fileRecord.FileMeta.Name)

	// 生成唯一的最终文件路径
	finalFilePath := generateUniqueFilePath(basePath, fileRecord.FileMeta.Extension)

	// 将临时文件重命名为最终文件
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		logger.Errorf("重命名文件失败: %v", err)
		// 清理临时文件
		os.Remove(tempFilePath)
		return err
	}

	// 清理临时目录
	os.RemoveAll(filepath.Join(fileRecord.TempStorage, t.taskId))

	logger.Infof("文件 %s 合并完成，保存在 %s", fileRecord.FileMeta.Name, finalFilePath)

	// 触发文件完成处理
	return t.ForceFileFinalize()
}

// handleFileFinalize 处理文件完成
// 主要步骤：
// 1. 更新文件状态为完成
// 2. 删除所有已完成的文件片段
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleFileFinalize() error {
	logger.Infof("处理文件完成: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)

	// 1. 更新文件状态为完成
	record := &pb.DownloadFileRecord{
		TaskId:     t.taskId,
		Status:     pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED,
		FinishedAt: time.Now().Unix(),
	}
	if err := downloadFileStore.Update(record); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 2. 删除所有已完成的文件片段
	segments, err := downloadSegmentStore.FindByTaskID(t.taskId)
	if err != nil {
		logger.Errorf("获取文件片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有已完成的文件片段
	for _, segment := range segments {
		if err := downloadSegmentStore.Delete(segment.SegmentId); err != nil {
			logger.Errorf("删除文件片段失败: segmentID=%s, err=%v", segment.SegmentId, err)
			return err
		}
	}

	logger.Infof("文件下载完成: taskID=%s", t.taskId)
	// 发送成功后通知片段完成
	t.NotifySegmentStatus(&pb.DownloadChan{
		TaskId:           t.taskId,          // 设置任务ID
		IsComplete:       true,              // 检查是否所有分片都已完成
		DownloadProgress: 100,               // 获取当前上传进度(0-100)
		DownloadTime:     time.Now().Unix(), // 设置当前时间戳
	})
	return nil
}

// generateUniqueFilePath 生成唯一文件路径
// 参数:
//   - basePath: 基础文件路径
//   - extension: 文件扩展名
//
// 返回值:
//   - string: 生成的唯一文件路径
//
// 功能:
//   - 检查文件是否已存在
//   - 如果存在则在文件名后添加递增数字
//   - 生成不冲突的文件路径
func generateUniqueFilePath(basePath, extension string) string {
	// 获取基础路径(不含扩展名)
	basePathWithoutExt := strings.TrimSuffix(basePath, "."+extension)
	finalPath := fmt.Sprintf("%s.%s", basePathWithoutExt, extension)

	// 检查文件是否存在
	counter := 1
	for {
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			// 文件不存在，可以使用这个路径
			return finalPath
		}
		// 文件存在，生成新的文件名
		finalPath = fmt.Sprintf("%s_%d.%s", basePathWithoutExt, counter, extension)
		counter++
	}
}

// updateSegmentStatus 更新分片状态
// 参数:
//   - segmentID: 分片ID
//   - status: 要更新的状态(下载中/已完成/失败)
//   - peerID: 可选的节点ID，用于标记节点状态
//
// 返回值:
//   - error: 如果更新状态失败，返回相应的错误信息
func (t *DownloadTask) updateSegmentStatus(segmentID string, status pb.SegmentDownloadStatus, peerID ...string) error {
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)
	if err := downloadSegmentStore.UpdateSegmentStatus(segmentID, status, peerID...); err != nil {
		logger.Errorf("更新状态失败: %v", err)
		return err
	}
	return nil
}

// TriggerSegmentIndexRequest 触发片段索引请求
// 主要功能：
// 1. 检查是否有正在进行的索引请求
// 2. 检查下载是否已完成
// 3. 获取当前下载进度
// 4. 检查进度变化和重试次数
// 5. 发送索引清单请求
func (t *DownloadTask) TriggerSegmentIndexRequest() error {
	// 检查任务是否已完成
	progress, err := t.GetProgress()
	if err != nil {
		logger.Errorf("获取下载进度失败: %v", err)
		return err
	}

	// 如果已完成,停止定时器并返回
	if progress >= 100 {
		t.stopIndexTicker()
		return nil
	}

	// 使用原子操作检查是否已有请求在进行中
	if !t.indexInProgress.CompareAndSwap(false, true) {
		logger.Debug("索引请求正在进行中，跳过本次请求")
		return nil
	}
	defer t.indexInProgress.Store(false)

	t.indexInfoMutex.Lock()
	defer t.indexInfoMutex.Unlock()

	// 获取当前进度
	currentProgress, err := t.GetProgress()
	if err != nil {
		logger.Errorf("获取当前进度失败: %v", err)
		return err
	}

	// 检查进度变化
	if currentProgress == t.lastIndexInfo.lastProgress {
		t.lastIndexInfo.noProgressFor += time.Since(t.lastIndexInfo.timestamp)
	} else {
		// 有进度更新，重置计时
		t.lastIndexInfo.noProgressFor = 0
		t.lastIndexInfo.lastProgress = currentProgress
	}

	// 获取未完成片段列表
	pendingSegmentIds, err := GetPendingSegments(t.db, t.TaskID())
	if err != nil {
		logger.Errorf("获取未完成片段列表失败: %v", err)
		return err
	}

	// 如果没有未完成的片段，验证并可能完成任务
	if len(pendingSegmentIds) == 0 {
		t.stopIndexTicker() // 停止定时器
		return t.ForceSegmentVerify()
	}

	// 计算当前请求的特征值
	sort.Strings(pendingSegmentIds)
	currentHash := fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(pendingSegmentIds, ","))))

	now := time.Now()
	if currentHash == t.lastIndexInfo.pendingIDs {
		t.lastIndexInfo.retryCount++

		// 根据无进度时间和重试次数调整策略
		if t.lastIndexInfo.noProgressFor > 5*time.Minute || t.lastIndexInfo.retryCount >= 5 {
			logger.Warnf("下载可能存在问题: taskID=%s, 无进度持续时间=%v, 重试次数=%d",
				t.TaskID(), t.lastIndexInfo.noProgressFor, t.lastIndexInfo.retryCount)

			// 调整定时器间隔
			t.adjustIndexInterval()

			if t.lastIndexInfo.retryCount >= 10 {
				return fmt.Errorf("下载似乎已停滞，请检查网络连接")
			}
		}
	} else {
		// 新的请求内容，重置状态
		t.lastIndexInfo.pendingIDs = currentHash
		t.lastIndexInfo.retryCount = 0
		t.resetIndexTicker() // 恢复正常的定时间隔
	}

	t.lastIndexInfo.timestamp = now

	// 等待一小段时间，合并可能的多个请求
	select {
	case <-time.After(batchWindow):
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	}

	// 发送索引清单请求
	return t.ForceSegmentIndex()
}

// stopIndexTicker 停止索引请求定时器
func (t *DownloadTask) stopIndexTicker() {
	t.indexTickerMutex.Lock()
	defer t.indexTickerMutex.Unlock()

	if t.indexTicker != nil {
		t.indexTicker.Stop()
	}
}

// resetIndexTicker 重置索引请求定时器
func (t *DownloadTask) resetIndexTicker() {
	t.indexTickerMutex.Lock()
	defer t.indexTickerMutex.Unlock()

	if t.indexTicker != nil {
		t.indexTicker.Stop()
	}
	t.indexTicker = time.NewTicker(30 * time.Second)
}

// adjustIndexInterval 调整索引请求间隔
func (t *DownloadTask) adjustIndexInterval() {
	t.indexTickerMutex.Lock()
	defer t.indexTickerMutex.Unlock()

	if t.indexTicker != nil {
		t.indexTicker.Stop()
	}

	// 使用指数退避算法计算新间隔
	baseInterval := 30 * time.Second
	maxInterval := 2 * time.Minute
	interval := baseInterval * time.Duration(1<<uint(t.lastIndexInfo.retryCount))

	if interval > maxInterval {
		interval = maxInterval
	}

	t.indexTicker = time.NewTicker(interval)
	logger.Infof("调整索引请求间隔: taskID=%s, 新间隔=%v", t.TaskID(), interval)
}

// 添加自定义错误类型
type SegmentError struct {
	PeerID   peer.ID
	Err      error
	Segments []string
}

func (e *SegmentError) Error() string {
	return fmt.Sprintf("节点 %s 发送失败: %v", e.PeerID, e.Err)
}

type SegmentErrors struct {
	PeerID peer.ID
	Errors []error
}

func (e *SegmentErrors) Error() string {
	return fmt.Sprintf("节点 %s 发送过程中出现 %d 个错误: %v",
		e.PeerID, len(e.Errors), e.Errors)
}
