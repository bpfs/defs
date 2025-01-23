package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/reedsolomon"

	"github.com/bpfs/defs/v2/utils/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
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

	// 遍历所有待下载的片段
	for _, segment := range segments {
		// 从分片分配管理器中获取可用节点
		if distribution, ok := t.distribution.GetNextDistribution(); ok {
			// 将片段分配给节点
			for peerID, segmentIDs := range distribution {
				// 检查该节点是否有这个片段
				for _, segmentID := range segmentIDs {
					if segmentID == segment.SegmentId {
						// 将片段添加到对应节点的列表中
						peerSegments[peerID] = append(peerSegments[peerID], segmentID)
						break
					}
				}
			}
		}
	}

	// 如果没有可用的节点分配，返回错误
	if len(peerSegments) == 0 {
		logger.Errorf("没有可用的节点分配: taskID=%s", t.taskId)
		return fmt.Errorf("没有可用的节点分配")
	}

	// 将分配好的节点-片段映射添加到分配管理器
	t.distribution.AddDistribution(peerSegments)

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

		logger.Infof("获取到新的分配任务: taskID=%s, peerSegments=%v", t.taskId, peerSegments)

		// 计算映射中的总片段数
		segmentCount := countSegments(peerSegments)
		totalProcessed += segmentCount
		logger.Infof("当前处理的片段数: %d, taskID=%s", segmentCount, t.taskId)

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
		logger.Infof("成功处理的片段数: %d, taskID=%s", segmentCount, t.taskId)
	}

	// 记录处理结果统计
	logger.Infof("完成片段分发处理: 总数=%d, 成功=%d, 失败=%d, taskID=%s",
		totalProcessed, totalSuccess, totalFailed, t.taskId)

	// 如果有失败的片段，返回错误
	if totalFailed > 0 {
		logger.Errorf("部分片段分发失败: 总数=%d, 失败=%d, taskID=%s",
			totalProcessed, totalFailed, t.taskId)
		return fmt.Errorf("部分片段分发失败: 总数=%d, 失败=%d",
			totalProcessed, totalFailed)
	}

	// 强制触发片段验证
	logger.Infof("强制触发片段验证: taskID=%s", t.taskId)
	return t.ForceSegmentVerify()
}

// countSegments 计算映射中的总片段数
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

	// 遍历所有节点及其对应的片段
	for peerID, segmentIDs := range peerSegments {

		// 获取本地节点的完整地址信息
		addrInfo := peer.AddrInfo{
			ID:    t.host.ID(),    // 设置节点ID
			Addrs: t.host.Addrs(), // 设置节点地址列表
		}

		// 序列化地址信息
		addrInfoBytes, err := addrInfo.MarshalJSON()
		if err != nil {
			logger.Errorf("序列化 AddrInfo 失败: %v", err)
			return err
		}

		// 处理每个片段
		for _, segmentID := range segmentIDs {
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			default:
			}

			// 创建片段存储实例
			store := database.NewDownloadSegmentStore(t.db)

			// 获取片段记录
			segment, exists, err := store.GetDownloadSegmentBySegmentID(segmentID)
			if err != nil {
				logger.Errorf("获取片段记录失败: segmentID=%s, err=%v", segmentID, err)
				return err
			}
			if !exists {
				logger.Warnf("片段记录不存在: segmentID=%s", segmentID)
				continue
			}

			// 构造片段内容请求
			request := &pb.SegmentContentRequest{
				TaskId:     t.taskId,              // 任务唯一标识
				FileId:     fileRecord.FileId,     // 文件唯一标识
				PubkeyHash: fileRecord.PubkeyHash, // 所有者的公钥哈希
				AddrInfo:   addrInfoBytes,         // 请求者的 AddrInfo，包含 ID 和地址信息
				SegmentId:  segmentID,             // 请求下载的文件片段唯一标识数
			}
			t.host.Addrs()

			// 序列化请求
			data, err := request.Marshal()
			if err != nil {
				logger.Errorf("序列化请求失败: %v", err)
				return err
			}
			// 创建新的流
			stream, err := t.host.NewStream(t.ctx, peerID, protocol.ID(StreamRequestSegmentProtocol))
			if err != nil {
				logger.Errorf("创建新流失败: %v", err)
				return err
			}
			defer stream.Close()
			// 发送请求并获取响应
			res, err := network.SendStreamWithExistingStream(stream, data)
			if err != nil || res == nil {
				logger.Errorf("发送请求失败: err=%v", err)
				continue
			}

			// 解析响应
			response := &pb.SegmentContentResponse{}
			if err := response.Unmarshal(res.Data); err != nil {
				logger.Errorf("解析响应失败: %v", err)
				continue
			}
			// 验证响应数据的有效性
			if len(response.SegmentContent) == 0 {
				logger.Errorf("收到无效的片段内容响应: taskID=%s, segmentID=%s",
					t.TaskID(), segmentID)
				continue
			}

			// 验证并存储下载的文件片段
			if err := ValidateAndStoreSegment(t.db, fileRecord.FirstKeyShare, response); err != nil {
				logger.Errorf("验证响应数据失败: %v", err)
				continue
			}

			logger.Infof("成功接收片段: peerID=%s, segmentID=%s", peerID, response.SegmentId)

			progress, err := t.GetProgress()
			if err != nil {
				logger.Errorf("获取进度失败: %v", err)
				continue
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
			// 去判断下进度
			go t.checkProress(fileRecord.SliceTable)

		}
	}

	// 强制触发片段验证
	// return t.ForceSegmentVerify()
	// 在 handleNodeDispatch() 中强制触发片段验证
	return nil
}

// 判断进度
func (t *DownloadTask) checkProress(sliceTable map[int64]*pb.HashTable) {
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)
	// 获取已完成的片段
	completedSegments, err := downloadSegmentStore.FindByTaskIDAndStatus(
		t.taskId,
		pb.SegmentDownloadStatus_SEGMENT_DOWNLOAD_STATUS_COMPLETED,
	)
	if err != nil {
		logger.Errorf("获取已完成片段失败: taskID=%s, err=%v", t.taskId, err)
		return
	}
	completedCount := len(completedSegments)

	// 获取所需的数据片段数量
	requiredShards := getRequiredDataShards(sliceTable)

	if completedCount >= requiredShards {
		if err := t.ForceSegmentMerge(); err != nil {
			logger.Errorf("强制触发片段合并 合并已下载的文件片段: taskID=%s, err=%v", t.taskId, err)
			return
		}
		return
	}
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

// handlePause 处理暂停请求
// 通过取消上下文来暂停任务
//
// 返回值:
//   - error: 如果暂停过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handlePause() error {
	logger.Infof("处理暂停请求: taskID=%s", t.taskId)

	// 更新文件状态为暂停
	downloadFileStore := database.NewDownloadFileStore(t.db)
	record := &pb.DownloadFileRecord{
		TaskId: t.taskId,
		Status: pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED,
	}

	// 更新文件状态为暂停
	if err := downloadFileStore.Update(record); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	return nil
}

// handleCancel 处理取消请求
// 删除该任务下的所有文件片段
//
// 返回值:
//   - error: 如果取消过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleCancel() error {
	logger.Infof("处理取消请求: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)

	// 更新文件状态为已取消
	record := &pb.DownloadFileRecord{
		TaskId: t.taskId,
		Status: pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED,
	}
	// 更新文件状态为已取消
	if err := downloadFileStore.Update(record); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有文件片段
	segments, err := downloadSegmentStore.FindByTaskID(t.taskId)
	if err != nil {
		logger.Errorf("获取文件片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有文件片段
	for _, segment := range segments {
		if err := downloadSegmentStore.Delete(segment.SegmentId); err != nil {
			logger.Errorf("删除文件片段失败: segmentID=%s, err=%v", segment.SegmentId, err)
			return err
		}
	}

	return nil
}

// handleDelete 处理删除请求
// 删除该任务下的文件记录和所有文件片段
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func (t *DownloadTask) handleDelete() error {
	logger.Infof("处理删除请求: taskID=%s", t.taskId)

	// 创建存储实例
	downloadFileStore := database.NewDownloadFileStore(t.db)
	downloadSegmentStore := database.NewDownloadSegmentStore(t.db)

	// 删除文件记录
	if err := downloadFileStore.Delete(t.taskId); err != nil {
		logger.Errorf("删除文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有文件片段
	segments, err := downloadSegmentStore.FindByTaskID(t.taskId)
	if err != nil {
		logger.Errorf("获取文件片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有文件片段
	for _, segment := range segments {
		if err := downloadSegmentStore.Delete(segment.SegmentId); err != nil {
			logger.Errorf("删除文件片段失败: segmentID=%s, err=%v", segment.SegmentId, err)
			return err
		}
	}

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
