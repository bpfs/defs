package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/reedsolomon"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/utils"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sirupsen/logrus"
)

// UpdateDownloadPieceInfo 用于更新下载任务中特定片段的节点信息。
// 参数：
//   - payload: *FileDownloadResponseChecklistPayload 下载响应的有效载荷。
//   - peerID: peer.ID 节点的ID。
func (task *DownloadTask) UpdateDownloadPieceInfo(payload *FileDownloadResponseChecklistPayload, peerID peer.ID) {
	// 已经下载完成了，直接退出
	if task.GetDownloadStatus() == StatusCompleted {
		return
	}

	// 更新文件信息
	task.updateFileInfo(payload)

	// 更新每个文件片段的节点信息和纠删码信息
	for _, index := range payload.AvailableSlices {
		// 添加节点信息
		task.File.AddSegmentNodes(index, []peer.ID{peerID})

		// 获取文件片段
		segment, ok := task.File.GetSegment(index)
		if !ok {
			continue
		}

		// 不是纠删码，且不等于下载完成
		if !segment.IsRsCodes && !segment.IsCompleted() {
			// 使用子方法通知将对应文件片段下载到本地
			task.EventDownSnippetChan(index)
		}
	}
}

// CheckDataSegmentsCompleted 检查数据片段是否下载完成
// 参数：无
// 返回值：
//   - bool: 如果数据片段下载完成返回true，否则返回false
func (task *DownloadTask) CheckDataSegmentsCompleted() bool {
	// 如果DataPieces为0，表示尚未拿到索引清单，直接返回false
	if task.DataPieces == 0 {
		return false
	}

	// 使用DownloadCompleteCount方法统计已完成下载的片段数量
	completedCount := task.File.DownloadCompleteCount()

	// 已完成下载的片段数量是否达到DataPieces
	return completedCount >= task.DataPieces
}

// CheckDataSegmentsCompleted 检查数据片段是否下载完成
// 参数：无
// 返回值：
//   - bool: 如果数据片段下载完成返回true，否则返回false
// func (task *DownloadTask) CheckDataSegmentsCompleted() bool {
// 	// 加锁以控制并发访问
// 	// task.RWMu.RLock()
// 	// defer task.RWMu.RUnlock()

// 	// 如果DataPieces为0，表示尚未拿到索引清单，直接返回false
// 	if task.DataPieces == 0 {
// 		return false
// 	}

// 	// 统计已完成下载的片段数量
// 	completedCount := 0
// 	for _, segment := range task.File.Segments {
// 		// 加锁以控制对Nodes的并发访问
// 		// segment.Mu.Lock()
// 		if segment.Status == SegmentStatusCompleted {
// 			completedCount++
// 		}
// 		// segment.Mu.Unlock()

// 		// 如果已完成下载的片段数量达到DataPieces，表示数据片段下载完成
// 		if completedCount >= task.DataPieces {
// 			return true
// 		}
// 	}

// 	// 已完成下载的片段数量不足，返回false
// 	return false
// }

// hasEnoughDataPieces 检查是否有足够的数据片段进行合并
// 参数：
//   - task: *DownloadTask 当前下载任务
//   - fileList: []string 文件列表
//
// 返回值：
//   - bool 是否有足够的数据片段
func (task *DownloadTask) hasEnoughDataPieces(fileList []string) bool {
	// 检查数据片段数量是否足够
	return task.DataPieces != 0 && len(fileList) >= task.DataPieces
}

// handleShardError 处理切片读取错误
// 参数：
//   - task: *DownloadTask 当前下载任务
func (task *DownloadTask) handleShardError() {
	if task.MergeCounter > 1 {
		// 如果有多个并行请求，重置计数器
		task.MergeCounter = 0
	} else {
		// 如果只有一个请求，重置计数器
		task.MergeCounter = 0
	}
}

// recoverShards 使用纠删码进行恢复
// 参数：
//   - task: *DownloadTask 当前下载任务
//   - shards: [][]byte 切片数据
//
// 返回值：
//   - bool 是否恢复成功
func (task *DownloadTask) recoverShards(shards [][]byte) bool {
	// 创建纠删码编码器
	enc, err := reedsolomon.New(task.DataPieces, task.TotalPieces-task.DataPieces)
	if err != nil {
		// 如果发生错误，记录错误日志
		logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
		return false
	}

	// 验证数据
	ok, _ := enc.Verify(shards)
	if !ok {
		// 如果验证失败，尝试纠删码恢复数据
		if err := enc.Reconstruct(shards); err != nil {
			// 如果恢复失败，记录错误日志
			logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
			return false
		}
		// 再次验证分片
		ok, _ = enc.Verify(shards)
		if !ok {
			// 如果再次验证失败，返回失败
			return false
		}
	}
	return true
}

// combineAndDecodeData 合并和解码数据
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - task: *DownloadTask 当前下载任务
//   - shards: [][]byte 切片数据
//
// 返回值：
//   - bool 是否合并和解码成功
func (task *DownloadTask) combineAndDecodeData(opt *opts.Options, shards [][]byte) bool {
	// 设置临时文件路径
	tempFilePath := filepath.Join(opt.GetDownloadPath(), task.File.FileID+".defs")
	// 创建纠删码编码器
	enc, _ := reedsolomon.New(task.DataPieces, task.TotalPieces-task.DataPieces)
	// 合并和解码数据
	if err := combineAndDecode(tempFilePath, enc, shards, int(task.File.Size)); err != nil {
		// 如果发生错误，记录错误日志
		logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
		return false
	}

	// 获取最终文件路径
	finalFilePath := task.getFinalFilePath(opt)
	// 重命名临时文件为最终文件
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		// 如果发生错误，记录错误日志
		logrus.Errorf("[%s]重命名文件时发生错误: %v", utils.WhereAmI(), err)
		return false
	}

	return true
}

// getFinalFilePath 获取最终文件路径
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - task: *DownloadTask 当前下载任务
//
// 返回值：
//   - string 最终文件路径
func (task *DownloadTask) getFinalFilePath(opt *opts.Options) string {
	// 设置初始的最终文件路径
	finalFilePath := filepath.Join(opt.GetDownloadPath(), task.File.Name)
	counter := 1
	for {
		// 检查文件是否存在
		if _, err := os.Stat(finalFilePath); os.IsNotExist(err) {
			// 如果文件不存在，则使用该文件名
			break
		} else {
			// 文件存在，生成新的文件名
			newFileName := generateNewFileName(task.File.Name, counter)
			finalFilePath = filepath.Join(opt.GetDownloadPath(), newFileName)
			counter++
		}
	}
	return finalFilePath
}

// setTaskStatusAndNotify 设置下载任务状态和发送通知
// 参数：
//   - task: *DownloadTask 当前下载任务
func (task *DownloadTask) setTaskStatusAndNotify() {
	// 设置下载任务状态为已完成
	go task.SetDownloadStatus(StatusCompleted)

	// 通知文件下载任务完成
	go task.DownloadTaskDoneSingleChan()

	// 重置合并计数器
	task.MergeCounter = 0
}

// readAllShards 读取所有片段数据
func (task *DownloadTask) readAllShards(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P) ([][]byte, error) {
	subDir := filepath.Join(paths.GetDownloadPath(), p2p.Host().ID().String(), task.File.FileID) // 设置子目录
	shards := make([][]byte, task.TotalPieces)
	for i := range shards {
		if segment, ok := task.File.GetSegment(i); ok {
			content, err := util.Read(opt, afe, subDir, segment.SegmentID)
			if err != nil || len(content) == 0 {
				if err != nil {
					logrus.Warnf("[%s]: %v", debug.WhereAmI(), err)
				}
				shards[i] = nil // 报错的片段用nil表示
				continue
			}
			// 计算 content 的哈希值是否与 segment.Checksum 一致，如果不一致，删除并置为nil
			hash := util.CalculateHash(content)
			if !util.CompareHashes(hash, segment.Checksum) {
				err := afe.Remove(filepath.Join(subDir, segment.GetSegmentID()))
				if err != nil {
					logrus.Warnf("[%s]: %v", debug.WhereAmI(), err)
				}
				shards[i] = nil // 哈希值不一致的片段用nil表示
				continue
			}
			shards[i] = content
		} else {
			shards[i] = nil // 缺失的片段用nil表示
		}
	}

	return shards, nil
}

// recoverDataFromSlices 从下载的切片中恢复文件数据
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - task: *DownloadTask 当前下载任务
//   - subDir: string 子目录路径
//
// 返回值：
//   - bool 恢复数据是否成功
func (task *DownloadTask) recoverDataFromSlices(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, subDir string) bool {
	for {
		// 检查任务状态是否已完成
		if task.GetDownloadStatus() == StatusCompleted {
			return false
		}

		// 增加合并计数器
		task.MergeCounter++
		if task.MergeCounter > 1 {
			// 如果存在其他并行合并请求，则跳过本次合并
			return false
		}

		// 获取文件列表
		fileList, err := getFileList(afe, subDir)
		if err != nil || len(fileList) == 0 {
			// 如果文件列表获取失败或者文件列表为空，则重置任务
			task.resetTask()
			return false
		}

		// 检查是否有足够的数据片段进行合并
		if !task.hasEnoughDataPieces(fileList) {
			//task.RWMu.Unlock()
			return false
		}

		// 读取所有切片
		shards, err := task.readAllShards(opt, afe, p2p)
		if err != nil {
			// 处理切片读取错误
			task.handleShardError()
			continue
		}

		// 使用纠删码进行恢复
		if !task.recoverShards(shards) {
			// 处理切片恢复错误
			task.handleShardError()
			continue
		}

		// 合并和解码数据
		if !task.combineAndDecodeData(opt, shards) {
			// 处理数据合并和解码错误
			task.handleShardError()
			continue
		}

		// 设置下载任务状态和发送通知
		task.setTaskStatusAndNotify()

		return true
	}
}

// getFileList 获取文件列表
// 参数：
//   - afe: afero.Afero 文件系统接口
//   - subDir: string 子目录路径
//
// 返回值：
//   - []string 文件列表
//   - error 错误信息
func getFileList(afe afero.Afero, subDir string) ([]string, error) {
	// 获取子目录下的所有文件名
	fileList, err := afero.ListFileNamesRecursively(afe, subDir)
	if err != nil {
		// 如果发生错误，记录错误日志
		logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
	}
	return fileList, err
}

// combineAndDecode 将分片数据合并为原始数据并进行解码。
func combineAndDecode(path string, enc reedsolomon.Encoder, split [][]byte, size int) error {
	// func combineAndDecode(path string, enc reedsolomon.Encoder, split [][]byte, dataShards, parityShards, size int) error {
	file, err := os.Create(path)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}
	defer file.Close()

	// 还原数据
	if err := enc.Join(file, split, size); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}
	return nil
}

// generateNewFileName 生成新文件名
func generateNewFileName(originalName string, counter int) string {
	ext := filepath.Ext(originalName)
	base := strings.TrimSuffix(originalName, ext)
	newName := fmt.Sprintf("%s_副本%d%s", base, counter, ext)
	return newName
}
