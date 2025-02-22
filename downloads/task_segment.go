package downloads

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/reedsolomon"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/pointsub"
)

const (
	maxWorkersPerPeer = 10 // 每个节点的最大工作协程数
	maxTotalWorkers   = 50 // 总的最大工作协程数
	segmentsPerWorker = 10 // 每个工作协程处理的片段数
)

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

	logger.Infof("已发送索引清单请求: taskID=%s, pendingSegments=%d",
		t.TaskID(),
		len(pendingSegmentIds),
	)

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
			if err := t.sendToPeer(peerID, fileRecord, segments, sem); err != nil {
				logger.Errorf("向节点发送数据失败: peerID=%s, err=%v", peerID, err)
				// 发送失败时，将片段标记为未处理
				processedMu.Lock()
				for _, segID := range segments {
					delete(processedSegments, segID)
				}
				processedMu.Unlock()
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

		if startIdx >= segmentCount {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // 获取全局信号量

		go func(workerID int, start, end int) {
			defer wg.Done()
			defer func() { <-sem }()

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
// 3. 创建带缓冲的读写器
// 4. 处理每个分片
// 5. 返回错误信息
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

	// 创建带缓冲的读写器
	writer := bufio.NewWriterSize(conn, MaxBlockSize)
	reader := bufio.NewReaderSize(conn, MaxBlockSize)

	// 处理每个分片
	for _, segmentID := range segments {
		if err := t.sendSegment(fileRecord, peerID, segmentID, conn, reader, writer); err != nil {
			logger.Errorf("发送分片 %s 失败: %v", segmentID, err)
			return err
		}
	}

	return nil
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
func (t *DownloadTask) sendSegment(fileRecord *pb.DownloadFileRecord, peerID peer.ID, segmentID string, conn net.Conn, reader *bufio.Reader, writer *bufio.Writer) error {
	// 获取分片数据
	segmentContentRequest, segment, err := t.getSegmentData(fileRecord, segmentID)
	if err != nil {
		logger.Errorf("获取分片数据失败: %v", err)
		return err
	}

	// 序列化数据
	data, err := segmentContentRequest.Marshal()
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

	// 读取响应长度
	lenBuf = make([]byte, 4)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		logger.Errorf("读取响应长度失败: %v", err)
		return err
	}

	// 解析响应长度
	msgLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])

	// 验证响应长度
	if msgLen <= 0 || msgLen > MaxBlockSize {
		logger.Errorf("无效的响应长度: %d", msgLen)
		return err
	}

	// 读取响应数据
	responseBuf := make([]byte, msgLen)
	if _, err := io.ReadFull(reader, responseBuf); err != nil {
		logger.Errorf("读取响应数据失败: %v", err)
		return err
	}

	// 解析响应
	response := &pb.SegmentContentResponse{}
	if err := response.Unmarshal(responseBuf); err != nil {
		logger.Errorf("解析响应失败: %v", err)
		return err
	}

	// 验证并存储下载的文件片段
	if err := ValidateAndStoreSegment(t.db, fileRecord.FirstKeyShare, response); err != nil {
		logger.Errorf("验证响应数据失败: %v", err)
		return err
	}
	if err := t.updateSegmentStatus(segmentID); err != nil {
		logger.Errorf("更新分片状态失败: %v", err)
		return err
	}

	progress, err := t.GetProgress()
	if err != nil {
		logger.Errorf("获取进度失败: %v", err)
		return nil
	}
	// 发送成功后通知片段完成
	t.NotifySegmentStatus(&pb.DownloadChan{
		TaskId:           t.taskId,                          // 设置任务ID
		IsComplete:       progress == 100,                   // 检查是否所有分片都已完成
		DownloadProgress: progress,                          // 获取当前上传进度(0-100)
		TotalShards:      int64(len(fileRecord.SliceTable)), // 设置总分片数
		SegmentId:        response.SegmentId,                // 设置分片ID
		SegmentIndex:     response.SegmentIndex,             // 设置分片索引
		SegmentSize:      segment.Size_,                     // 设置分片大小
		IsRsCodes:        segment.IsRsCodes,                 // 设置是否使用纠删码
		NodeId:           peerID.String(),                   // 设置存储节点ID
		DownloadTime:     time.Now().Unix(),                 // 设置当前时间戳
	})

	// 更新状态
	return err
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

	// 获取已完成的片段
	completedSegments, err := downloadSegmentStore.FindByTaskIDAndStatus(
		t.taskId,
		pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED,
	)
	if err != nil {
		logger.Errorf("获取已完成片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	completedCount := len(completedSegments)

	// 获取所需的数据片段数量
	requiredShards := getRequiredDataShards(fileRecord.SliceTable)

	if completedCount >= requiredShards {
		if err := t.ForceSegmentMerge(); err != nil {
			logger.Errorf("强制触发片段合并 合并已下载的文件片段: taskID=%s, err=%v", t.taskId, err)
			return err
		}
		return nil
	}

	// 如果完成数量不等于总片段数，说明存在未下载的片段
	if completedCount != totalSegments {
		logger.Infof("存在未下载的片段，触发片段处理: taskID=%s, 已完成=%d, 总数=%d",
			t.taskId, completedCount, totalSegments)
		// 强制触发片段处理
		return t.ForceSegmentProcess()
	}

	// 如果完成数量等于总片段数，说明所有片段都已下载完成
	logger.Infof("所有片段下载完成，触发文件合并处理: taskID=%s", t.taskId)
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
	logger.Infof("开始合并文件片段: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)

	// 获取文件记录
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 获取所需的数据片段数量
	requiredShards := getRequiredDataShards(fileRecord.SliceTable)

	// 创建分片文件数组
	shards := make([][]byte, len(fileRecord.SliceTable))
	shardsPresent := 0

	// 获取下载任务所有的切片信息
	segments, err := GetListDownloadSegments(t.db, t.TaskID())
	if err != nil {
		logger.Errorf("获取下载片段列表失败: %v", err)
		return err
	}

	// 读取所有下载的片段
	for index := range fileRecord.SliceTable {
		for _, v := range segments {
			if v.SegmentIndex == index {

				shards[index] = v.SegmentContent
				shardsPresent++
				break
			}
		}
	}

	// 检查是否有足够的分片来重构文件
	if shardsPresent < requiredShards {
		err := fmt.Errorf("没有足够的分片来重构文件，需要 %d 个，但只有 %d 个",
			requiredShards, shardsPresent)
		logger.Errorf(err.Error())
		return err
	}

	// 创建Reed-Solomon编码器
	rs, err := reedsolomon.New(requiredShards, len(shards)-requiredShards)
	if err != nil {
		logger.Errorf("创建Reed-Solomon编码器失败: %v", err)
		return err
	}

	// 创建临时文件用于存储合并后的数据
	tempFilePath := filepath.Join(fileRecord.TempStorage, fileRecord.FileId+".defs")
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return err
	}
	defer tempFile.Close()

	// 使用JoinFile方法将分片合并为完整文件
	if err := rs.Join(tempFile, shards, int(fileRecord.FileMeta.Size_)); err != nil {
		os.Remove(fileRecord.TempStorage)
		logger.Errorf("合并分片失败: %v", err)
		return err
	}

	// 构建基础文件路径
	basePath := filepath.Join(fileRecord.TempStorage, fileRecord.FileMeta.Name)

	// 生成唯一的最终文件路径
	finalFilePath := generateUniqueFilePath(basePath, fileRecord.FileMeta.Extension)

	// 将临时文件重命名为最终文件
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		os.Remove(fileRecord.TempStorage)
		logger.Errorf("重命名文件失败: %v", err)
		return err
	}

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
//
// 返回值:
//   - error: 如果更新状态失败，返回相应的错误信息
func (t *DownloadTask) updateSegmentStatus(segmentID string) error {
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)
	if err := downloadSegmentStore.UpdateSegmentStatus(segmentID, pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED); err != nil {
		logger.Errorf("更新状态失败: %v", err)
		return err
	}
	return nil
}
