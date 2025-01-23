package uploads

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"

	"github.com/bpfs/defs/v2/utils/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
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

		logger.Infof("分配片段到节点: segmentID=%s, peerID=%s",
			segment.SegmentId, nearestPeer)
	}

	// 检查是否有成功分配的片段
	if len(peerSegments) == 0 {
		logger.Warnf("没有找到合适的节点处理任何片段: taskID=%s", t.taskId)
		// 触发片段验证以检查整体状态
		return t.ForceSegmentVerify()
	}

	// 将分配结果添加到分片分配管理器
	t.distribution.AddDistribution(peerSegments)

	logger.Infof("完成片段分配: taskID=%s, totalSegments=%d, totalPeers=%d",
		t.taskId, len(segments), len(peerSegments))

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
			totalFailed += countSegments(peerSegments)
			if err.Error() == "任务已取消" {
				return err
			}
			continue
		}

		totalSuccess += countSegments(peerSegments)
	}

	// 记录处理结果统计
	logger.Infof("完成片段分发处理: 总数=%d, 成功=%d, 失败=%d, taskID=%s",
		totalProcessed, totalSuccess, totalFailed, t.taskId)

	// 如果有失败的片段，返回错误
	if totalFailed > 0 {
		logger.Errorf("部分片段分发失败: 总数=%d, 失败=%d",
			totalProcessed, totalFailed)
		return fmt.Errorf("部分片段分发失败: 总数=%d, 失败=%d",
			totalProcessed, totalFailed)
	}

	// 强制触发片段验证
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
func (t *UploadTask) handleNetworkTransfer(peerSegments map[peer.ID][]string) error {
	logger.Infof("处理网络传输: taskID=%s", t.taskId)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 获取上传文件记录
	fileRecord, exists, err := uploadFileStore.GetUploadFile(t.taskId)
	if err != nil {
		logger.Errorf("获取上传文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}
	if !exists {
		logger.Errorf("上传文件记录不存在: taskID=%s", t.taskId)
		return fmt.Errorf("上传文件记录不存在: taskID=%s", t.taskId)
	}

	// 遍历所有节点及其对应的片段
	for peerID, segmentIDs := range peerSegments {
		// // 创建新的流
		// stream, err := t.host.NewStream(t.ctx, peerID, protocol.ID(StreamSendingToNetworkProtocol))
		// if err != nil {
		// 	logger.Errorf("创建新流失败: %v", err)
		// 	return err
		// }
		// defer stream.Close()

		// 处理每个片段
		for _, segmentID := range segmentIDs {
			logger.Infof("切片ID: %s", segmentID)
			select {
			case <-t.ctx.Done():
				logger.Infof("for循环上传接收到任务取消调度")
				return fmt.Errorf("任务已取消")
			default:
			}

			// 获取片段记录
			segment, exists, err := uploadSegmentStore.GetUploadSegmentBySegmentID(segmentID)
			if err != nil {
				logger.Errorf("获取片段记录失败: segmentID=%s, err=%v", segmentID, err)
				return err
			}
			if !exists {
				logger.Warnf("片段记录不存在: segmentID=%s", segmentID)
				continue
			}

			if segment.Status == pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_COMPLETED {
				logger.Infof("片段已经上传完成: segmentID=%s", segmentID)
				continue
			}

			// 更新片段状态为上传中
			if err := uploadSegmentStore.UpdateSegmentStatus(segmentID,
				pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_UPLOADING); err != nil {
				logger.Errorf("更新片段状态失败: segmentID=%s, err=%v", segmentID, err)
				continue
			}

			// 验证分片内容
			if len(segment.SegmentContent) == 0 {
				logger.Errorf("分片内容为空: segmentID=%s", segment.SegmentId)
				continue
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
				return err
			}

			// 生成数字签名
			signature, err := generateSignature(privateKey, signatureData)
			if err != nil {
				logger.Errorf("生成数字签名失败: %v", err)
				return err
			}

			// 验证加密密钥数组长度
			if len(fileRecord.FileSecurity.EncryptionKey) != 3 {
				logger.Errorf("加密密钥数组长度错误: %d", len(fileRecord.FileSecurity.EncryptionKey))
				return fmt.Errorf("加密密钥数组长度错误: %d", len(fileRecord.FileSecurity.EncryptionKey))
			}

			// 获取加密密钥
			encryptionKey := fileRecord.FileSecurity.EncryptionKey[1]
			logger.Infof("Share #%d 十六进制值: %s", 1, hex.EncodeToString(encryptionKey))

			// 构造分片存储对象
			storage := &pb.FileSegmentStorage{
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
			}

			// 序列化存储对象
			data, err := storage.Marshal()
			if err != nil {
				logger.Errorf("序列化存储对象失败: %v", err)
				return err
			}
			// 创建新的流
			stream, err := t.host.NewStream(t.ctx, peerID, protocol.ID(StreamSendingToNetworkProtocol))
			if err != nil {
				logger.Errorf("创建新流失败: %v", err)
				return err
			}
			defer stream.Close()

			// 使用已存在的流发送数据
			res, err := network.SendStreamWithExistingStream(stream, data)

			// 处理发送结果
			if err != nil || res == nil || res.Code != 200 {
				// 发送失败，更新状态并记录日志
				if err := uploadSegmentStore.UpdateSegmentStatus(segmentID,
					pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_FAILED); err != nil {
					logger.Errorf("更新片段状态失败: segmentID=%s, err=%v", segmentID, err)
				}

				logger.Errorf("发送片段到节点失败: peerID=%s, segmentID=%s, err=%v",
					peerID, segmentID, err)
				return fmt.Errorf("发送片段失败")
			}

			// 发送成功，更新状态为已完成
			if err := uploadSegmentStore.UpdateSegmentStatus(segmentID,
				pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_COMPLETED); err != nil {
				logger.Errorf("更新片段状态失败: segmentID=%s, err=%v", segmentID, err)
				return err
			}

			logger.Infof("成功发送片段到节点: peerID=%s, segmentID=%s",
				peerID, segmentID)

			progress, err := t.GetProgress()
			if err != nil {
				logger.Errorf("获取上传进度失败: taskID=%s, err=%v", t.TaskID(), err)

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
		}
	}

	// 强制触发片段验证
	return t.ForceSegmentVerify()
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
		return fmt.Errorf("文件记录不存在: taskID=%s", t.taskId)
	}

	// 获取文件总片段数
	totalSegments := len(fileRecord.SliceTable)
	if totalSegments == 0 {
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

// handlePause 处理暂停请求
// 通过取消上下文来暂停任务
//
// 返回值:
//   - error: 如果暂停过程中发生错误，返回相应的错误信息
func (t *UploadTask) handlePause() error {
	logger.Infof("处理暂停请求: taskID=%s", t.taskId)

	// 更新文件状态为暂停
	uploadFileStore := database.NewUploadFileStore(t.db)
	if err := uploadFileStore.UpdateUploadFileStatus(
		t.taskId,
		pb.UploadStatus_UPLOAD_STATUS_PAUSED,
		time.Now().Unix(),
	); err != nil {
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
func (t *UploadTask) handleCancel() error {
	logger.Infof("处理取消请求: taskID=%s", t.taskId)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 更新文件状态为已取消
	if err := uploadFileStore.UpdateUploadFileStatus(
		t.taskId,
		pb.UploadStatus_UPLOAD_STATUS_CANCELED,
		time.Now().Unix(),
	); err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有文件片段
	if err := uploadSegmentStore.DeleteUploadSegmentByTaskID(t.taskId); err != nil {
		logger.Errorf("删除文件片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	return nil
}

// handleDelete 处理删除请求
// 删除该任务下的文件记录和所有文件片段
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func (t *UploadTask) handleDelete() error {
	logger.Infof("处理删除请求: taskID=%s", t.taskId)

	// 创建存储实例
	uploadFileStore := database.NewUploadFileStore(t.db)
	uploadSegmentStore := database.NewUploadSegmentStore(t.db)

	// 删除文件记录
	if err := uploadFileStore.DeleteUploadFile(t.taskId); err != nil {
		logger.Errorf("删除文件记录失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	// 删除所有文件片段
	if err := uploadSegmentStore.DeleteUploadSegmentByTaskID(t.taskId); err != nil {
		logger.Errorf("删除文件片段失败: taskID=%s, err=%v", t.taskId, err)
		return err
	}

	return nil
}
