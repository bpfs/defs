package download

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/reedsolomon"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/defs/util/crypto/gcm"
	"github.com/bpfs/defs/util/sign/rsa"

	"github.com/bpfs/defs/util/zip/gzip"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
)

// registerFileDownloadStartEvent 注册文件下载开始事件
func RegisterFileDownloadStartEvent(
	opt *opts.Options,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	registry *eventbus.EventRegistry,
	pool *pool.MemoryPool,
) error {
	// 获取事件总线
	bus := registry.GetEventBus(config.EventFileDownloadStart)
	if bus == nil {
		return fmt.Errorf("无法获取文件下载开始事件总线")
	}

	// 注册
	return bus.Subscribe(config.EventFileDownloadStart, func(
		fileID string, // 文件的唯一标识
		userPubHash []byte, // 用户的公钥哈希
	) error {
		// 插入文件下载数据
		if err := sqlite.InsertFilesDatabase(db,
			fileID,     // 文件的唯一标识
			0,          // 文件片段的总量
			0,          // 操作(0:下载)
			2,          // 状态(2:待开始)
			time.Now(), // 时间(当前时间)
		); err != nil {
			return err
		}

		// 确保文件目录存在
		if err := os.MkdirAll(filepath.Dir(opt.GetDownloadPath()), 0755); err != nil {
			return err
		}

		// 创建临时文件
		tempFilePath := filepath.Join(opt.GetDownloadPath(), fileID+".dep2p")
		if _, err := os.Create(tempFilePath); err != nil {
			return err
		}

		var retries int64 = 0
		for retries < opt.GetMaxRetries() {
			// 文件下载请求(清单)
			requestChecklistPayload := &FileDownloadRequestChecklistPayload{
				FileID:      fileID,      // 文件的唯一标识
				UserPubHash: userPubHash, // 用户的公钥哈希
			}

			// 向指定的全网节点发送文件下载请求订阅消息
			if err := network.SendPubSub(p2p, pubsub, config.PubsubFileDownloadRequestTopic, "checklist", "", requestChecklistPayload); err != nil {
				return err
			}

			// 等待响应
			time.Sleep(opt.GetRetryInterval())

			// 检查内存池中是否已接收到文件片段的哈希表
			pool.Mu.RLock()
			task, exists := pool.DownloadTasks[fileID]
			pool.Mu.RUnlock()
			if exists && len(task.PieceInfo) > 0 {
				// 已经收到文件片段的哈希表
				return nil
			}

			retries++
		}

		// 更新文件下载数据对象的状态
		if err := sqlite.UpdateFileDatabaseStatus(db, fileID,
			0, // 操作(0:下载)
			0, // 状态(0:失败)
		); err != nil {
			return err
		}

		return nil
	})
}

// registerFileDownloadCheckEvent 注册文件下载检查事件
//  1. 先从文件夹读取文件片段:
//     在尝试恢复数据之前，首先应从指定的子目录中读取所有文件片段。
//     检查每个文件片段是否真的存在于文件系统中。
//  2. 内存池的数据一致性检查:
//     如果内存池中某个片段被标记为已下载，但实际上在文件系统中不存在，需要提供一种方法来处理这种不一致。
//     这可以是一个修复程序，它清除内存池中的错误标记，并可能重新触发下载这些丢失的片段。
//  3. 增加一步从文件夹读取文件片段的逻辑:
//     在尝试进行数据恢复之前，应该验证所有必要的片段是否都在本地文件夹中可用。
//  4. 如果发现不一致，进行处理:
//     如果发现任何不一致（如文件系统中缺少标记为已下载的片段），应触发相应的修复逻辑。
func RegisterFileDownloadCheckEvent(
	opt *opts.Options,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	registry *eventbus.EventRegistry,
	pool *pool.MemoryPool,
) error {
	// 获取事件总线
	bus := registry.GetEventBus(config.EventFileDownloadCheck)
	if bus == nil {
		return fmt.Errorf("无法获取文件下载检查事件总线")
	}

	// 注册
	return bus.Subscribe(config.EventFileDownloadCheck, func(
		fileID string, // 文件的唯一标识
	) error {

		// 检查下载是否完成
		if !pool.IsDownloadComplete(fileID) {
			return nil // 下载未完成
		}

		// 新建文件存储
		fs, err := afero.NewFileStore(paths.DownloadPath)
		if err != nil {
			logrus.Errorf("创建文件存储失败:\t%v", err)
			return err
		}
		// 子目录当前主机+文件hash
		subDir := filepath.Join(p2p.Host().ID().String(), string(fileID)) // 设置子目录

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)
		if err != nil {
			logrus.Errorf("[registerFileDownloadCheckEvent] 获取切片失败:\t%v", err)
			return err
		}

		// 文件不存在
		if len(slices) == 0 {
			// 如果文件夹中没有任何切片文件，则重置下载任务
			if err := pool.ResetDownloadTask(fileID); err != nil {
				logrus.Errorf("重置下载任务失败: %v", err)
				return err
			}
		}

		// 从下载的切片中恢复文件数据
		// if err := recoverDataFromSlices(fs, p2p, pool, opt, fileID); err != nil {
		// 	// 如果恢复过程中出错，可能需要回退某些片段的进度
		// 	return err
		// }
		if recoverDataFromSlices(fs, p2p, pool, opt, fileID) {
			// 更新文件下载数据对象的状态
			if err := sqlite.UpdateFileDatabaseStatus(db, fileID,
				0, // 操作(0:下载)
				1, // 状态(1:成功)
			); err != nil {
				return err
			}
			logrus.Printf("合并成功！！！！！！！")
			// TODO: 更新数据库状态、发布完成事件等
		}

		return nil
	})
}

// recoverDataFromSlices 从下载的切片中恢复文件数据
// func recoverDataFromSlices(fs *afero.FileStore, p2p *dep2p.DeP2P, pool *pool.MemoryPool, opt *opts.Options, fileID string) error {
// 	// 获取下载任务
// 	pool.Mu.RLock()
// 	task, exists := pool.DownloadTasks[fileID]
// 	pool.Mu.RUnlock()
// 	if !exists {
// 		return fmt.Errorf("下载任务不存在: %s", fileID)
// 	}

// 	task.Mu.Lock()
// 	if task.IsMerged {
// 		task.Mu.Unlock()
// 		return nil // 如果文件已经合并，直接返回
// 	}

// 	subDir := filepath.Join(p2p.Host().ID().String(), task.FileID) // 设置子目录
// 	fileList, err := fs.ListFiles(subDir)
// 	if err != nil {
// 		task.Mu.Unlock()
// 		return err
// 	}

// 	// 已下载的文件小于数据片段的数量时，不合并文件片段
// 	if len(fileList) < task.DataPieces {
// 		task.Mu.Unlock()
// 		return err
// 	}

// 	// 从文件存储中读取所有切片
// 	shards, err := readAllShards(fs, opt, p2p, pool, task)
// 	if err != nil {
// 		task.Mu.Unlock()
// 		return err
// 	}

// 	// 验证数据
// 	ok, err := task.ENC.Verify(shards)
// 	if err != nil {
// 		task.Mu.Unlock()
// 		return err
// 	}
// 	if !ok {
// 		// 恢复数据
// 		if err := task.ENC.Reconstruct(shards); err != nil {
// 			task.Mu.Unlock()
// 			return fmt.Errorf("纠删码恢复失败: %v", err)
// 		}
// 	}

// 	// 写入临时文件
// 	tempFilePath := filepath.Join(opt.GetDownloadPath(), fileID+".dep2p")
// 	if err := combineAndDecode(tempFilePath, task.ENC, shards, task.DataPieces, task.TotalPieces-task.DataPieces, int(task.Size)); err != nil {
// 		task.Mu.Unlock()
// 		return err
// 	}
// 	// 重命名为最终文件
// 	finalFilePath := filepath.Join(opt.GetDownloadPath(), task.Name)
// 	// 判断文件是否存在
// 	counter := 1
// 	for {
// 		if _, err := os.Stat(finalFilePath); err == nil {
// 			fmt.Printf("文件 %s 存在\n", finalFilePath)
// 			// 生成新的文件名
// 			newFileName := generateNewFileName(task.Name, counter)
// 			finalFilePath = filepath.Join(opt.GetDownloadPath(), newFileName)
// 			counter++
// 		} else {
// 			if os.IsNotExist(err) {
// 				fmt.Printf("文件 %s 不存在\n", finalFilePath)
// 				if err := os.Rename(tempFilePath, finalFilePath); err != nil {
// 					task.Mu.Unlock()
// 					return err
// 				}

// 				// 文件合并完成后，设置 IsMerged 为 true
// 				task.IsMerged = true
// 				task.Mu.Unlock()

//					// 完成数据片段合并
//					return nil
//				} else {
//					fmt.Println("发生其他错误:", err)
//					task.Mu.Unlock()
//					return err
//				}
//			}
//		}
//	}
//
// recoverDataFromSlices 从下载的切片中恢复文件数据
func recoverDataFromSlices(fs *afero.FileStore, p2p *dep2p.DeP2P, pool *pool.MemoryPool, opt *opts.Options, fileID string) bool {
	// 获取下载任务
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()
	if !exists {
		return false
	}

	for {
		// 锁定任务以进行并发控制
		task.Mu.Lock()

		// 如果文件已经合并，直接返回
		if task.IsMerged {
			task.Mu.Unlock()
			return false
		}

		// 增加合并计数器
		task.MergeCounter++
		if task.MergeCounter > 1 {
			// 如果存在其他并行合并请求，则跳过本次合并
			task.Mu.Unlock()
			return false
		}

		// 设置子目录，用于存储切片文件
		fileList, err := fs.ListFiles(filepath.Join(p2p.Host().ID().String(), task.FileID))
		if err != nil {
			task.Mu.Unlock()
			return false
		}

		// 如果已下载的文件小于数据片段数量，不进行合并操作
		if len(fileList) < task.DataPieces {
			task.Mu.Unlock()
			return false
		}

		// 从文件存储中读取所有切片
		shards, err := readAllShards(fs, opt, p2p, pool, task)
		if err != nil {
			task.Mu.Unlock()
			if task.MergeCounter > 1 {
				// 如果有多个并行请求，重置计数器并重试
				task.MergeCounter = 0
				continue
			} else {
				// 如果只有一个请求，且恢复失败，直接返回错误
				task.MergeCounter = 0
				return false
			}
		}

		// 使用纠删码编码器验证数据
		ok, _ := task.ENC.Verify(shards)
		// 如果验证失败，尝试纠删码恢复数据
		if !ok {
			// 尝试纠删码恢复数据
			if err := task.ENC.Reconstruct(shards); err != nil {
				task.Mu.Unlock()
				if task.MergeCounter > 1 {
					// 如果有多个并行请求，重置计数器并重试
					task.MergeCounter = 0
					continue
				} else {
					// 如果只有一个请求，且恢复失败，直接返回错误
					task.MergeCounter = 0
					return false
				}
			}
			// 再次验证分片
			ok, _ := task.ENC.Verify(shards)
			if !ok {
				task.Mu.Unlock()
				if task.MergeCounter > 1 {
					// 如果有多个并行请求，重置计数器并重试
					task.MergeCounter = 0
					continue
				} else {
					// 如果只有一个请求，且恢复失败，直接返回错误
					task.MergeCounter = 0
					return false
				}
			}

		}

		// 将恢复的数据写入临时文件
		tempFilePath := filepath.Join(opt.GetDownloadPath(), fileID+".dep2p")
		if err := combineAndDecode(tempFilePath, task.ENC, shards, task.DataPieces, task.TotalPieces-task.DataPieces, int(task.Size)); err != nil {
			task.Mu.Unlock()
			return false
		}

		// 重命名为最终文件
		finalFilePath := filepath.Join(opt.GetDownloadPath(), task.Name)
		// 判断文件是否存在并处理重复文件名
		counter := 1
		for {
			if _, err := os.Stat(finalFilePath); os.IsNotExist(err) {
				// 如果文件不存在，则使用该文件名
				break
			} else if err != nil {
				// 发生除文件不存在之外的其他错误
				fmt.Println("检查文件存在时发生错误:", err)
				task.Mu.Unlock()
				return false
			} else {
				// 文件存在，生成新的文件名
				fmt.Printf("文件 %s 已存在，生成新文件名\n", finalFilePath)
				newFileName := generateNewFileName(task.Name, counter)
				finalFilePath = filepath.Join(opt.GetDownloadPath(), newFileName)
				counter++
			}
		}

		// 重命名临时文件为最终文件名
		if err := os.Rename(tempFilePath, finalFilePath); err != nil {
			fmt.Println("重命名文件时发生错误:", err)
			task.Mu.Unlock()
			if task.MergeCounter > 1 {
				// 如果有多个并行请求，重置计数器并重试
				task.MergeCounter = 0
				continue
			} else {
				// 如果只有一个请求，且恢复失败，直接返回错误
				task.MergeCounter = 0
				return false
			}
		}

		// 文件合并完成后，设置 IsMerged 为 true
		task.IsMerged = true
		task.MergeCounter = 0
		task.Mu.Unlock()

		// 完成数据片段合并
		return true
	}
}

// readAllShards 读取所有片段数据
func readAllShards(fs *afero.FileStore, opt *opts.Options, p2p *dep2p.DeP2P, pool *pool.MemoryPool, task *pool.DownloadTask) ([][]byte, error) {
	subDir := filepath.Join(p2p.Host().ID().Pretty(), task.FileID) // 设置子目录
	shards := make([][]byte, task.TotalPieces)
	for i := range shards {
		if pieceInfo, ok := task.PieceInfo[i]; ok {
			content, err := fs.Read(subDir, pieceInfo.Hash)
			// content, err := processShard(fs, opt, p2p, pool, task, pieceInfo)
			if err != nil {
				shards[i] = nil // 报错的片段用nil表示
				continue
			}
			// TODO: 计算 content 的哈希值是否与  pieceInfo.Hash 一致，如果不一致，删除并置为nil
			shards[i] = content
		} else {
			shards[i] = nil // 缺失的片段用nil表示
		}
	}

	return shards, nil
}

// processShard 处理单个片段
func processShard(fs *afero.FileStore, opt *opts.Options, p2p *dep2p.DeP2P, pool *pool.MemoryPool, task *pool.DownloadTask, pieceInfo *pool.DownloadPieceInfo) ([]byte, error) {
	subDir := filepath.Join(p2p.Host().ID().String(), task.FileID) // 设置子目录
	sliceFile, err := fs.OpenFile(subDir, pieceInfo.Hash)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "打开文件失败")
	}
	segmentTypes := []string{
		"FILEID",
		"P2PKSCRIPT",
		"SLICETABLE",
		"SLICEHASH",
		"INDEX",
		"CONTENT",
		"SIGNATURE",
	}
	segmentResults, _, err := segment.ReadFileSegments(sliceFile, segmentTypes)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取文件段失败")
	}

	// 检查并提取每个段的数据
	for segmentType, result := range segmentResults {
		if result.Error != nil {
			return handlePieceError(pool, task, pieceInfo, fmt.Sprintf("读取 %s 段失败", segmentType))
		}
	}

	// 提取具体数据
	fileIDData := segmentResults["FILEID"].Data         // 读取文件的唯一标识
	p2pkScriptData := segmentResults["P2PKSCRIPT"].Data // 读取文件的 P2PK 脚本
	sliceTableData := segmentResults["SLICETABLE"].Data // 读取文件片段的哈希表
	sliceHashData := segmentResults["SLICEHASH"].Data   // 读取文件片段的哈希值
	indexData := segmentResults["INDEX"].Data           // 读取文件片段的索引
	contentData := segmentResults["CONTENT"].Data       // 读取文件片段的内容(加密)
	signatureData := segmentResults["SIGNATURE"].Data   // 读取文件和文件片段的数据签名

	///////////////////////////////////////////////////////////////

	/**
	// 打开文件
	sliceFile, err := fs.OpenFile(subDir, pieceInfo.Hash)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "打开文件失败")
	}

	// 加载 xref 表
	xref, err := segment.LoadXref(sliceFile)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "加载xref失败")
	}

	// 读取文件的唯一标识
	fileIDData, err := segment.ReadSegmentToFile(sliceFile, "FILEID", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取文件的唯一标识失败")
	}

	// 读取文件的 P2PK 脚本
	p2pkScriptData, err := segment.ReadSegmentToFile(sliceFile, "P2PKSCRIPT", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取文件的 P2PK 脚本失败")
	}

	// 读取文件片段的哈希表
	sliceTableData, err := segment.ReadSegmentToFile(sliceFile, "SLICETABLE", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取哈希表失败")
	}

	// 读取文件片段的哈希值
	sliceHashData, err := segment.ReadSegmentToFile(sliceFile, "SLICEHASH", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取哈希表失败")
	}

	// 读取文件片段的索引
	indexData, err := segment.ReadSegmentToFile(sliceFile, "INDEX", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取切片索引失败")
	}

	// 读取文件片段的内容(加密)
	contentData, err := segment.ReadSegmentToFile(sliceFile, "CONTENT", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取加密数据失败")
	}

	// 读取文件和文件片段的数据签名
	signatureData, err := segment.ReadSegmentToFile(sliceFile, "SIGNATURE", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取数据签名失败")
	}

	*/

	fileID := string(fileIDData)
	if fileID != task.FileID {
		return handlePieceError(pool, task, pieceInfo, "文件的唯一标识不一致")
	}
	sliceHash := string(sliceHashData)
	if sliceHash != pieceInfo.Hash {
		return handlePieceError(pool, task, pieceInfo, "文件片段不一致")
	}

	pubKey, err := script.ExtractPubKeyFromP2PKScriptToRSA(p2pkScriptData) // 从脚本中提取公钥
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "从 P2PK 中提取公钥失败")
	}

	var sliceTable map[int]core.HashTable
	if err := util.DecodeFromBytes(sliceTableData, &sliceTable); err != nil {
		return handlePieceError(pool, task, pieceInfo, "解析文件哈希表失败")
	}

	index32, err := util.FromBytes[int32](indexData)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "解析切片索引失败")
	}
	index := int(index32)

	var rc bool
	hashTable, exists := sliceTable[index]
	if exists {
		rc = hashTable.IsRsCodes // 是否为纠删码
	} else {
		return handlePieceError(pool, task, pieceInfo, "获取转换数据失败")
	}

	st, err := json.Marshal(sliceTable)
	if err != nil {
		return nil, fmt.Errorf(": %v", err)
	}
	merged, err := util.MergeFieldsForSigning( // 组装签名数据
		task.FileID, // 文件的唯一标识
		st,          // 切片内容的哈希表
		index,       // 文件片段的索引
		sliceHash,   // 文件片段的哈希值
		rc,          // 文件片段的存储模式
	)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "文件数字签名验证失败")
	}

	// 验证签名
	if !rsa.VerifySignature(pubKey, merged, signatureData) {
		return handlePieceError(pool, task, pieceInfo, "文件数字签名验证失败")
	}

	// AES加密的密钥，长度需要是16、24或32字节
	key := md5.Sum([]byte(task.FileKey))

	// 数据解压
	decompressData, err := gzip.DecompressData(contentData)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "文件片段解压失败")
	}

	// 解密验证
	decrypted, err := gcm.DecryptData(decompressData, key[:])
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "文件片段解密失败")
	}

	hash, data, err := util.SeparateHashFromData(decrypted)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "文件片段读取失败")
	}

	fmt.Printf("6======hash\t%s\n", hash)

	return data, nil
}

// handlePieceError 处理片段错误
func handlePieceError(pool *pool.MemoryPool, task *pool.DownloadTask, pieceInfo *pool.DownloadPieceInfo, errMsg string) ([]byte, error) {
	log.Printf("%s: %v", errMsg, pieceInfo.Hash)
	pool.RevertDownloadPieceProgress(task.FileID, pieceInfo.Hash)
	if !pool.IsDownloadComplete(task.FileID) {
		return nil, fmt.Errorf("%s，等待下一次检查", errMsg)
	}
	return nil, fmt.Errorf("%s", errMsg)
}

// combineShards 合并片段
// func combineShards(shards [][]byte) []byte {
// 	var buf bytes.Buffer
// 	for _, shard := range shards {
// 		buf.Write(shard)
// 	}
// 	return buf.Bytes()
// }

// combineAndDecode 将分片数据合并为原始数据并进行解码。
func combineAndDecode(path string, enc reedsolomon.Encoder, split [][]byte, dataShards, parityShards, size int) error {
	// enc, err := reedsolomon.New(dataShards, parityShards)
	// if err != nil {
	// 	fmt.Println("创建文件失败:", err)
	// 	return err
	// }
	file, err := os.Create(path)
	if err != nil {
		fmt.Println("创建文件失败:", err)
		return err
	}
	defer file.Close()

	// 还原数据
	if err := enc.Join(file, split, size); err != nil {
		fmt.Println("创建文件失败:", err)
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
