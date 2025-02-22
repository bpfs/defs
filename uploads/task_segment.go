package uploads

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/pointsub"
)

const (
	maxWorkersPerPeer = 10 // 每个节点的最大工作协程数
	maxTotalWorkers   = 50 // 总的最大工作协程数
	segmentsPerWorker = 10 // 每个工作协程处理的片段数
)

// handleSegmentProcess 处理文件片段
// 主要步骤：
// 1. 从数据库查询待上传的片段
// 2. 为每个片段找到合适的临近节点
// 3. 将片段分配给这些节点
// 4. 更新分片分配管理器
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) handleSegmentProcess() error {
	logger.Infof("处理片段请求: taskID=%s", t.taskId)

	// 创建上传片段存储实例
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 获取当前任务中状态为待上传的所有片段
	segments, err := uploadSegmentStore.GetUploadSegmentsByStatus(
		t.taskId,
		pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_PENDING,
	)
	if err != nil {
		logger.Errorf("获取待上传片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 如果没有待上传的片段，直接触发片段验证
	if len(segments) == 0 {
		logger.Infof("没有待上传的片段: taskID=%s", t.taskId)
		// 强制触发片段验证
		return t.ForceSegmentVerify()
	}

	// 创建节点到片段ID的映射
	peerSegments := make(map[peer.ID][]string)

	// 遍历所有待上传片段
	for _, segment := range segments {
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		default:
		}

		// 获取更多的候选节点 (过滤节点数量 + 1)
		extraNodes := len(segment.FilteredPeerIds) + 1
		closestPeers := t.routingTable.NearestPeers(kbucket.ConvertKey(segment.SegmentId), extraNodes, 2)
		if len(closestPeers) == 0 {
			logger.Warnf("未找到合适的节点处理片段: segmentID=%s", segment.SegmentId)
			continue
		}

		// 过滤掉不需要的节点
		var nearestPeer peer.ID
		// 遍历最近的节点列表
		for _, p := range closestPeers {
			peerIDStr := p.String()
			isFiltered := false
			// 检查当前节点是否在过滤列表中
			for _, filteredID := range segment.FilteredPeerIds {
				if peerIDStr == filteredID {
					isFiltered = true
					break
				}
			}
			// 如果节点不在过滤列表中，选择该节点
			if !isFiltered {
				nearestPeer = p
				break
			}
		}

		// 如果所有节点都被过滤，记录警告并继续
		if nearestPeer.String() == "" {
			logger.Warnf("所有候选节点都在过滤列表中: segmentID=%s", segment.SegmentId)
			continue
		}

		// 将片段ID添加到对应节点的切片中
		peerSegments[nearestPeer] = append(
			peerSegments[nearestPeer],
			segment.SegmentId,
		)

		// logger.Infof("分配片段到节点: segmentID=%s, peerID=%s",
		// 	segment.SegmentId, nearestPeer)
	}

	// 检查是否有成功分配的片段
	if len(peerSegments) == 0 {
		logger.Warnf("没有找到合适的节点处理任何片段: taskID=%s", t.taskId)
		// 触发片段验证以检查整体状态
		return t.ForceSegmentVerify()
	}

	// 将分配结果添加到分片分配管理器
	t.distribution.AddDistribution(peerSegments)

	// logger.Infof("完成片段分配: taskID=%s, totalSegments=%d, totalPeers=%d",
	// 	t.taskId, len(segments), len(peerSegments))

	// 强制触发节点分发
	return t.ForceNodeDispatch()
}

// handleNodeDispatch 处理节点分发
// 主要步骤：
// 1. 循环从分片分配管理器获取待处理的分配
// 2. 通过 ForceNetworkTransfer 发送到网络传输通道
// 3. 直到队列为空时退出
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) handleNodeDispatch() error {
	logger.Infof("处理节点分发: taskID=%s", t.taskId)

	totalProcessed := 0 // 处理的总片段数
	totalSuccess := 0   // 成功处理的片段数
	totalFailed := 0    // 处理失败的片段数

	// 循环处理队列中的所有分配任务
	for {
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		default:
		}

		// 获取下一个待处理的分配
		peerSegments, ok := t.distribution.GetNextDistribution()
		if !ok {
			break
		}

		// 计算映射中的总片段数
		totalProcessed += countSegments(peerSegments)

		// 强制触发网络传输
		if err := t.ForceNetworkTransfer(peerSegments); err != nil {
			logger.Errorf("发送片段到网络通道失败: err=%v", err)
			// 更新失败处理的片段数
			totalFailed += countSegments(peerSegments)
			if err.Error() == "任务已取消" {
				logger.Errorf("任务已取消: %v", err)
				return err
			}
			continue
		}

		// 更新成功处理的片段数
		totalSuccess += countSegments(peerSegments)
	}

	// 记录处理结果统计
	logger.Infof("完成片段分发处理: 总数=%d, 成功=%d, 失败=%d, taskID=%s",
		totalProcessed, totalSuccess, totalFailed, t.taskId)

	// 强制触发片段验证
	// return t.ForceSegmentVerify()
	return nil
}

// countSegments 计算映射中的总片段数
// 主要步骤：
// 1. 遍历映射中的所有片段
// 2. 计算总片段数
//
// 参数:
//   - peerSegments: 映射中的片段
//
// 返回值:
//   - int: 总片段数
func countSegments(peerSegments map[peer.ID][]string) int {
	total := 0
	for _, segments := range peerSegments {
		total += len(segments)
	}
	return total
}

// handleNetworkTransfer 处理网络传输
// 主要步骤：
// 1. 记录处理网络传输的开始时间
// 2. 创建一个全局等待组
// 3. 创建错误通道
// 4. 记录已处理的片段，避免重复发送
// 5. 遍历所有需要发送的节点
// 6. 过滤掉已处理的片段
// 7. 如果该节点的所有片段都已处理，跳过
// 8. 添加一个协程发送片段到节点
// 9. 等待所有发送完成并收集错误
// 10. 记录错误但不中断流程
// 11. 无论是否有错误都触发片段验证
//
// 参数:
//   - peerSegments: 需要发送的节点和片段
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) handleNetworkTransfer(peerSegments map[peer.ID][]string) error {
	logger.Infof("处理网络传输: taskID=%s, peerCount=%d", t.taskId, len(peerSegments))

	// 创建一个全局等待组
	var globalWg sync.WaitGroup
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
			if err := t.sendToPeer(peerID, segments); err != nil {
				logger.Errorf("向节点发送数据失败: peerID=%s, err=%v", peerID, err)
				// 发送失败时，将片段标记为未处理
				processedMu.Lock()
				for _, segID := range segments {
					delete(processedSegments, segID)
				}
				processedMu.Unlock()

				// 根据错误类型决定是否需要重试
				if isConnectionError(err) {
					// 如果是连接错误，可以考虑将该节点暂时标记为不可用
					logger.Warnf("节点暂时不可用: peerID=%s", peerID)
				}

				errChan <- fmt.Errorf("节点 %s 发送失败: %v", peerID, err)
			}
		}(peerID, unprocessedSegments)
	}

	// 等待所有发送完成并收集错误
	go func() {
		globalWg.Wait()
		close(errChan)
	}()

	// 收集所有错误
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	// 记录错误但不中断流程
	if len(errs) > 0 {
		logger.Errorf("发送过程中出现 %d 个错误: %v", len(errs), errs)
		// 这里不再直接返回错误
	}

	// 无论是否有错误都触发片段验证
	// 验证会重新处理失败的片段
	return t.ForceSegmentVerify()
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
//
// 返回值:
//   - error: 如果发送过程中发生错误，返回相应的错误信息
func (t *UploadTask) sendToPeer(peerID peer.ID, segments []string) error {
	segmentCount := len(segments)
	logger.Infof("向节点发送数据: peerID=%s, segmentCount=%d", peerID, segmentCount)

	// 计算合适的worker数量，但不超过最大限制
	workerCount := min(maxWorkersPerPeer, (segmentCount+segmentsPerWorker-1)/segmentsPerWorker)

	// 创建错误通道
	errChan := make(chan error, workerCount)
	var wg sync.WaitGroup

	// 计算每个worker处理的分片数
	segmentsPerGoroutine := (segmentCount + workerCount - 1) / workerCount

	// 启动worker处理分片
	for i := 0; i < workerCount; i++ {
		startIdx := i * segmentsPerGoroutine
		endIdx := min((i+1)*segmentsPerGoroutine, segmentCount)

		if startIdx >= segmentCount {
			break
		}

		wg.Add(1)
		go func(workerID int, start, end int) {
			defer wg.Done()

			// 使用重试机制建立连接
			conn, err := t.establishConnection(peerID)
			if err != nil {
				errChan <- err
				return
			}
			defer conn.Close()

			// 处理分片
			if err := t.processSegments(peerID, conn, segments[start:end]); err != nil {
				errChan <- err
			}
		}(i, startIdx, endIdx)
	}

	// 等待所有worker完成
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// 收集错误
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("节点 %s 发送过程中出现 %d 个错误: %v", peerID, len(errs), errs)
	}

	return nil
}

// establishConnection 建立连接
// 主要步骤：
// 1. 设置最大重试次数
// 2. 设置重试延迟
// 3. 循环尝试建立连接
// 4. 如果连接成功，返回连接
// 5. 如果连接失败，记录错误
//
// 参数:
//   - peerID: 目标节点ID
//
// 返回值:
//   - net.Conn: 建立的连接
//   - error: 如果建立连接失败，返回相应的错误信息
func (t *UploadTask) establishConnection(peerID peer.ID) (net.Conn, error) {
	const (
		maxRetries = 3
		retryDelay = time.Second * 5
	)

	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			time.Sleep(retryDelay)
		}

		conn, err := pointsub.Dial(t.ctx, t.host, peerID, protocol.ID(StreamSendingToNetworkProtocol))
		if err == nil {
			return conn, nil
		}
		lastErr = err

		if t.ctx.Err() != nil {
			return nil, fmt.Errorf("任务已取消")
		}
	}
	return nil, lastErr
}

// processSegments 处理分片
// 主要步骤：
// 1. 设置缓冲区
// 2. 遍历每个分片
// 3. 发送分片
// 4. 如果发送失败，记录错误
//
// 参数:
//   - conn: 连接
//   - segments: 需要发送的分片列表
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) processSegments(peerID peer.ID, conn net.Conn, segments []string) error {
	// 设置缓冲区
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetWriteBuffer(MaxBlockSize)
		tcpConn.SetReadBuffer(MaxBlockSize)
	}

	writer := bufio.NewWriterSize(conn, MaxBlockSize)
	reader := bufio.NewReaderSize(conn, MaxBlockSize)

	for _, segmentID := range segments {
		if err := t.sendSegment(peerID, segmentID, conn, reader, writer); err != nil {
			if isConnectionError(err) {
				logger.Warnf("连接错误, 重试: %v", err)
				return err
			}
			logger.Errorf("发送分片失败: %v", err)
		}

	}
	return nil
}

// isConnectionError 判断是否是连接错误
// 主要步骤：
// 1. 如果错误为nil，返回false
// 2. 检查错误字符串是否包含常见的连接错误字符串
// 3. 返回是否包含常见的连接错误字符串
//
// 参数:
//   - err: 错误
//
// 返回值:
//   - bool: 如果错误为连接错误，返回true，否则返回false
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// 检查是否包含常见的连接错误字符串
	errStr := err.Error()
	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "reset by peer") ||
		strings.Contains(errStr, "timeout")
}

// sendSegment 发送单个分片
// 主要步骤：
// 1. 获取分片数据
// 2. 序列化数据
// 3. 写入长度前缀
// 4. 使用缓冲写入
// 5. 刷新缓冲区
// 参数:
//   - segmentID: 分片ID
//   - conn: 连接
//   - reader: 读取器
//   - writer: 写入器
func (t *UploadTask) sendSegment(peerID peer.ID, segmentID string, conn net.Conn, reader *bufio.Reader, writer *bufio.Writer) error {
	// 获取分片数据
	storage, segment, fileRecord, err := t.getSegmentData(segmentID)
	if err != nil {
		logger.Errorf("获取分片数据失败: %v", err)
		return err
	}

	// 序列化数据
	data, err := storage.Marshal()
	if err != nil {
		logger.Errorf("序列化数据失败: %v", err)
		return err
	}

	// 每次写入前重置超时
	conn.SetDeadline(time.Now().Add(ConnTimeout))

	// 写入长度前缀
	lenBuf := make([]byte, 4)
	lenBuf[0] = byte(len(data) >> 24)
	lenBuf[1] = byte(len(data) >> 16)
	lenBuf[2] = byte(len(data) >> 8)
	lenBuf[3] = byte(len(data))

	// 使用缓冲写入
	if _, err := writer.Write(lenBuf); err != nil {
		logger.Errorf("写入长度失败: %v", err)
		return err
	}
	if _, err := writer.Write(data); err != nil {
		logger.Errorf("写入数据失败: %v", err)
		return err
	}
	if err := writer.Flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	// 重置读取超时
	conn.SetDeadline(time.Now().Add(ConnTimeout))

	// 读取响应
	response, err := reader.ReadString('\n')
	if err != nil {
		logger.Errorf("读取响应失败: %v", err)
		return err
	}

	// 验证响应
	if !strings.Contains(response, "success") {
		logger.Errorf("响应验证失败: %s", response)
		return fmt.Errorf("响应验证失败: %s", response)
	}

	// 获取上传进度
	progress, err := t.GetProgress()
	if err != nil {
		logger.Errorf("获取上传进度失败: taskID=%s, err=%v", t.TaskID(), err)
		return err
	}
	if err := t.updateSegmentStatus(segmentID); err != nil {
		logger.Errorf("更新分片状态失败: segmentID=%s, err=%v", segmentID, err)
		return err
	}
	// 发送成功后通知片段完成
	t.NotifySegmentStatus(&pb.UploadChan{
		TaskId:         t.taskId,                          // 设置任务ID
		IsComplete:     progress == 100,                   // 检查是否所有分片都已完成
		UploadProgress: progress,                          // 获取当前上传进度(0-100)
		TotalShards:    int64(len(fileRecord.SliceTable)), // 设置总分片数
		SegmentId:      segment.SegmentId,                 // 设置分片ID
		SegmentIndex:   segment.SegmentIndex,              // 设置分片索引
		SegmentSize:    segment.Size_,                     // 设置分片大小
		IsRsCodes:      segment.IsRsCodes,                 // 设置是否使用纠删码
		NodeId:         peerID.String(),                   // 设置存储节点ID
		UploadTime:     time.Now().Unix(),                 // 设置当前时间戳
	})
	// 更新状态
	return nil
}

// handleSegmentVerify 处理片段验证
// 主要步骤：
// 1. 获取文件记录中的切片表信息
// 2. 获取已完成上传的片段数量
// 3. 比较两者是否一致
// 4. 根据验证结果触发后续操作
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) handleSegmentVerify() error {
	logger.Infof("验证片段上传状态: taskID=%s", t.taskId)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 获取文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		logger.Errorf("文件记录不存在: taskID=%s", t.taskId)
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 获取文件总片段数
	totalSegments := len(fileRecord.SliceTable)
	if totalSegments == 0 {
		logger.Errorf("文件切片表为空: taskID=%s", t.taskId)
		return fmt.Errorf("文件切片表为空: taskID=%s", t.taskId)
	}

	// 获取已完成的片段
	completedSegments, err := uploadSegmentStore.GetUploadSegmentsByStatus(
		t.taskId,
		pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_COMPLETED,
	)
	if err != nil {
		logger.Errorf("获取已完成片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	completedCount := len(completedSegments)

	// 如果完成数量不等于总片段数，说明存在未上传的片段
	if completedCount != totalSegments {
		logger.Infof("存在未上传的片段，触发片段处理: taskID=%s", t.taskId)
		// 强制触发片段处理
		return t.ForceSegmentProcess()
	}

	// 如果完成数量等于总片段数，说明所有片段都已上传完成
	logger.Infof("所有片段上传完成，触发文件完成处理: taskID=%s", t.taskId)
	return t.ForceFileFinalize()
}

// handleFileFinalize 处理文件完成
// 主要步骤：
// 1. 更新文件状态为完成
// 2. 删除所有已完成的文件片段
// 3. 添加文件资产记录
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) handleFileFinalize() error {
	logger.Infof("处理文件完成: taskID=%s", t.taskId)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 1. 更新文件状态为完成
	if err := uploadFileStore.UpdateUploadFileStatus(
		t.taskId,
		pb.UploadStatus_UPLOAD_STATUS_COMPLETED,
		time.Now().Unix(),
	); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 2. 创建文件资产记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		logger.Errorf("文件记录不存在: taskID=%s", t.taskId)
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 创建文件资产记录
	if err := CreateFileAssetRecord(t.db, fileRecord); err != nil {
		logger.Errorf("创建文件资产记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 3. 删除所有已完成的文件片段
	if err := uploadSegmentStore.DeleteUploadSegmentByTaskID(t.taskId); err != nil {
		logger.Errorf("删除文件片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	logger.Infof("文件上传完成: taskID=%s, fileID=%s", t.taskId, fileRecord.FileId)
	return nil
}

// getSegmentData 获取分片数据
// 参数:
//   - segmentID: 分片ID
//
// 返回值:
//   - *pb.FileSegmentStorage: 分片数据
//   - error: 如果获取过程中发生错误，返回相应的错误信息
func (t *UploadTask) getSegmentData(segmentID string) (*pb.FileSegmentStorage, *pb.UploadSegmentRecord, *pb.UploadFileRecord, error) {
	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 获取分片数据
	segment, exists, err := uploadSegmentStore.GetUploadSegmentBySegmentID(segmentID)
	if err != nil || !exists {
		logger.Errorf("获取分片数据失败: %v", err)
		return nil, nil, nil, err
	}

	// 获取文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil || !exists {
		logger.Errorf("获取文件记录失败: %v", err)
		return nil, nil, nil, err
	}

	// 构造签名数据
	signatureData := &pb.SignatureData{
		FileId:        fileRecord.FileId,                                           // 文件ID
		ContentType:   fileRecord.FileMeta.ContentType,                             // 内容类型
		Sha256Hash:    fileRecord.FileMeta.Sha256Hash,                              // SHA256哈希值
		SliceTable:    files.ConvertSliceTableToSortedSlice(fileRecord.SliceTable), // 切片表
		SegmentId:     segment.SegmentId,                                           // 分片ID
		SegmentIndex:  segment.SegmentIndex,                                        // 分片索引
		Crc32Checksum: segment.Crc32Checksum,                                       // CRC32校验和
		EncryptedData: segment.SegmentContent,                                      // 加密数据
	}

	// 解析私钥
	privateKey, err := files.UnmarshalPrivateKey(fileRecord.FileSecurity.OwnerPriv)
	if err != nil {
		logger.Errorf("解析私钥失败: %v", err)
		return nil, nil, nil, err
	}

	// 生成数字签名
	signature, err := generateSignature(privateKey, signatureData)
	if err != nil {
		logger.Errorf("生成数字签名失败: %v", err)
		return nil, nil, nil, err
	}

	// 验证加密密钥数组长度
	if len(fileRecord.FileSecurity.EncryptionKey) != 3 {
		logger.Errorf("加密密钥数组长度错误: %d", len(fileRecord.FileSecurity.EncryptionKey))
		return nil, nil, nil, fmt.Errorf("加密密钥数组长度错误: %d", len(fileRecord.FileSecurity.EncryptionKey))
	}

	// 获取加密密钥
	encryptionKey := fileRecord.FileSecurity.EncryptionKey[1]
	// logger.Infof("Share #%d 十六进制值: %s", 1, hex.EncodeToString(encryptionKey))

	// 构造分片存储对象
	return &pb.FileSegmentStorage{
		FileId:         fileRecord.FileId,                   // 文件ID
		Name:           fileRecord.FileMeta.Name,            // 文件名
		Size_:          fileRecord.FileMeta.Size_,           // 文件大小
		ContentType:    fileRecord.FileMeta.ContentType,     // 内容类型
		Extension:      fileRecord.FileMeta.Extension,       // 文件扩展名
		Sha256Hash:     fileRecord.FileMeta.Sha256Hash,      // SHA256哈希值
		UploadTime:     time.Now().Unix(),                   // 上传时间
		P2PkhScript:    fileRecord.FileSecurity.P2PkhScript, // P2PKH脚本
		P2PkScript:     fileRecord.FileSecurity.P2PkScript,  // P2PK脚本
		SliceTable:     fileRecord.SliceTable,               // 切片表
		SegmentId:      segment.SegmentId,                   // 分片ID
		SegmentIndex:   segment.SegmentIndex,                // 分片索引
		Crc32Checksum:  segment.Crc32Checksum,               // CRC32校验和
		SegmentContent: segment.SegmentContent,              // 分片内容
		EncryptionKey:  encryptionKey,                       // 加密密钥(传输共享密钥的第2片密钥)
		Signature:      signature,                           // 数字签名
		Shared:         false,                               // 是否共享
		Version:        version,                             // 版本
	}, segment, fileRecord, nil

	// 构造存储对象
	// return &pb.FileSegmentStorage{
	// 	FileId:         fileRecord.FileId,
	// 	Name:           fileRecord.FileMeta.Name,
	// 	Size_:          fileRecord.FileMeta.Size_,
	// 	ContentType:    fileRecord.FileMeta.ContentType,
	// 	Extension:      fileRecord.FileMeta.Extension,
	// 	Sha256Hash:     fileRecord.FileMeta.Sha256Hash,
	// 	UploadTime:     time.Now().Unix(),
	// 	P2PkhScript:    fileRecord.FileSecurity.P2PkhScript,
	// 	P2PkScript:     fileRecord.FileSecurity.P2PkScript,
	// 	SliceTable:     fileRecord.SliceTable,
	// 	SegmentId:      segment.SegmentId,
	// 	SegmentIndex:   segment.SegmentIndex,
	// 	Crc32Checksum:  segment.Crc32Checksum,
	// 	SegmentContent: segment.SegmentContent,
	// 	EncryptionKey:  fileRecord.FileSecurity.EncryptionKey[1], // 传输共享密钥的第2片密钥
	// 	Version:        version,
	// }, nil
}

// updateSegmentStatus 更新分片状态
// 参数:
//   - segmentID: 分片ID
//
// 返回值:
//   - error: 如果更新状态失败，返回相应的错误信息
func (t *UploadTask) updateSegmentStatus(segmentID string) error {
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)
	if err := uploadSegmentStore.UpdateSegmentStatus(segmentID, pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_COMPLETED); err != nil {
		logger.Errorf("更新状态失败: %v", err)
		return err
	}
	return nil
}
