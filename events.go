package defs

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/filestore"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/klauspost/reedsolomon"
	"github.com/sirupsen/logrus"

	"github.com/bpfs/dep2p"
	"github.com/dgraph-io/ristretto"
	"go.uber.org/fx"
)

// 事件协议
const (
	// 文件上传检查事件
	EventFileUploadCheck = "defs@event:file/upload/check/1.0.0"
	// 文件片段上传事件
	EventFileSliceUpload = "defs@event:file/slice/upload/1.0.0"
	// 文件下载开始事件
	EventFileDownloadStart = "defs@event:file/download/start/1.0.0"
	// 文件下载检查事件
	EventFileDownloadCheck = "defs@event:file/download/check/1.0.0"
)

type NewEventRegistryOutput struct {
	fx.Out
	Registry *eventbus.EventRegistry // 事件总线
}

// NewEventRegistry 新的事件总线
func NewEventRegistry(lc fx.Lifecycle) (out NewEventRegistryOutput, err error) {
	// 创建事件注册器
	registry := eventbus.NewEventRegistry()

	// 注册文件上传检查事件总线
	registry.RegisterEvent(EventFileUploadCheck, eventbus.New())

	// 注册文件片段上传事件总线
	registry.RegisterEvent(EventFileSliceUpload, eventbus.New())

	// 注册文件下载开始事件总线
	registry.RegisterEvent(EventFileDownloadStart, eventbus.New())

	// 注册文件下载检查事件总线
	registry.RegisterEvent(EventFileDownloadCheck, eventbus.New())

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})

	out.Registry = registry
	return out, nil
}

type RegisterEventProtocolInput struct {
	fx.In
	Ctx          context.Context     // 全局上下文
	Opt          *Options            // 文件存储选项配置
	P2P          *dep2p.DeP2P        // DeP2P网络主机
	PubSub       *pubsub.DeP2PPubSub // DeP2P网络订阅
	DB           *sqlites.SqliteDB   // sqlite数据库服务
	UploadChan   chan *uploadChan    // 用于刷新上传的通道
	DownloadChan chan *downloadChan  // 用于刷新下载的通道

	Registry *eventbus.EventRegistry // 事件总线
	Cache    *ristretto.Cache        // 缓存实例
	Pool     *MemoryPool             // 内存池
}

// RegisterEvents 注册事件
func RegisterEventProtocol(lc fx.Lifecycle, input RegisterEventProtocolInput) error {
	// 注册文件上传检查事件
	if err := registerFileUploadCheckEvent(
		input.P2P,
		input.PubSub,
		input.DB,
		input.Registry,
		input.Pool,
	); err != nil {
		return err
	}

	// 注册文件片段上传事件
	if err := registerFileSliceUploadEvent(
		input.Opt,
		input.P2P,
		input.PubSub,
		input.DB,
		input.UploadChan,
		input.Registry,
		input.Cache,
		input.Pool,
	); err != nil {
		return err
	}

	// 注册文件下载开始事件
	if err := registerFileDownloadStartEvent(
		input.Opt,
		input.P2P,
		input.PubSub,
		input.DB,
		input.Registry,
		input.Pool,
	); err != nil {
		return err
	}

	// 注册文件下载检查事件
	if err := registerFileDownloadCheckEvent(
		input.Opt,
		input.P2P,
		input.PubSub,
		input.DB,
		input.Registry,
		input.Pool,
	); err != nil {
		return err
	}

	return nil
}

// registerFileUploadCheckEvent 注册文件上传检查事件
func registerFileUploadCheckEvent(
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	registry *eventbus.EventRegistry,
	pool *MemoryPool,
) error {

	// 获取事件总线
	bus := registry.GetEventBus(EventFileUploadCheck)
	if bus == nil {
		return fmt.Errorf("无法获取切片上传事件总线")
	}

	// 注册
	return bus.Subscribe(EventFileUploadCheck, func(
		assetID string, // 文件资产的唯一标识
		dataShards int64, // 数据分片
		parityShards int64, // 奇偶分片
		uploadTime time.Time, // 上传时间
	) error {
		// 插入文件上传数据
		if err := db.InsertFilesDatabase(
			assetID,                        // 文件资产的唯一标识
			int64(dataShards+parityShards), // 文件片段的总量
			1,                              // 操作(1:上传)
			2,                              // 状态(2:待开始)
			time.Now(),                     // 时间(当前时间)
		); err != nil {
			return err
		}

		log.Printf("[文件网络检查] assetID:\t%s\n", assetID)
		// 本地数据存储
		// 发送本地上传的切片
		// go SendLocalUploadSlice(db, registry, pool, assetID, sliceHash, total, current)

		// 文件上传请求(检查)
		requestCheckPayload := &FileUploadRequestCheckPayload{
			AssetID:    assetID,
			UploadTime: uploadTime,
		}

		// 向指定的全网节点发送文件上传请求的订阅消息
		if err := SendPubSub(p2p, pubsub, PubsubFileUploadRequestTopic, "check", "", requestCheckPayload); err != nil {
			return err
		}

		return nil
	})
}

// registerFileSliceUploadEvent 注册文件片段上传事件
func registerFileSliceUploadEvent(
	opt *Options,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	uploadChan chan *uploadChan,
	registry *eventbus.EventRegistry,
	cache *ristretto.Cache,
	pool *MemoryPool,
) error {
	// 获取事件总线
	bus := registry.GetEventBus(EventFileSliceUpload)
	if bus == nil {
		return fmt.Errorf("无法获取文件片段上传事件总线")
	}

	// 注册
	return bus.Subscribe(EventFileSliceUpload, func(
		assetID string, // 文件资产的唯一标识(外部标识)
		sliceHash string, // 文件片段的哈希值(外部标识)
		totalPieces int, // 文件片段总量
		current int, // 当前序列
	) error {
		// 插入文件片段数据，状态(0:失败)
		if err := db.InsertSlicesDatabase(assetID, sliceHash, current, 0); err != nil {
			return err
		}

	undone:

		// 发送本地上传的切片
		if err := SendFileSliceToNetwork(opt, p2p, uploadChan, registry, cache, pool, assetID, sliceHash, totalPieces, current); err != nil {
			// 更新文件片段数据对象的状态
			if err := db.UpdateSlicesDatabaseStatus(assetID, sliceHash, 0); err != nil { // 状态(0:失败)
				return err
			}
			// 更新文件上传数据对象的状态
			if err := db.UpdateFileDatabaseStatus(assetID, 1, 0); err != nil { // 状态(0:失败)
				return err
			}

			return err
		}

		// 标记上传任务中的一个片段为完成
		pool.MarkUploadPieceComplete(assetID, current)

		// 更新文件片段数据对象的状态(1:成功)
		if err := db.UpdateSlicesDatabaseStatus(assetID, sliceHash, 1); err != nil {
			return err
		}
		// 更新文件上传数据对象的状态
		if current == 1 { // 第一个数据片段
			if err := db.UpdateFileDatabaseStatus(assetID, 1, 3); err != nil { // 状态(3:进行中)
				return err
			}
		} else if current == totalPieces { // 最后一个数据片段
			// 检查指定文件资产是否上传完成
			if !pool.IsUploadComplete(assetID) {
				// 获取未完成的上传片段
				undoneAssetID := pool.GetIncompleteUploadPieces(assetID)
				if len(undoneAssetID) != 0 {
					s, err := db.SelectOneSlicesDatabaseStatus(undoneAssetID[0])
					if err != nil {
						sliceHash = s.SliceHash // 文件片段的哈希值
						current = s.SliceIndex  // 文件片段的索引

						goto undone
					}
				}
			}
			if err := db.UpdateFileDatabaseStatus(assetID, 1, 1); err != nil { // 状态(1:成功)
				return err
			}

			// 开启本地存储
			if !opt.localStorage {
				// 新建文件存储
				fs, err := filestore.NewFileStore(UploadPath)
				if err == nil {
					// for _, v := range pool.GetAllPieceHashes(assetID) {
					// 	if err := fs.Delete(assetID, v); err != nil { // 删除文件
					// 		logrus.Errorf("删除文件失败: %v", err)
					// 	}
					// }
					if err := fs.DeleteAll(assetID); err != nil { // 删除所有文件
						logrus.Errorf("删除 %s 的所有文件失败: %v", assetID, err)
					}
				}
				pool.DeleteUploadTask(assetID) // 删除指定资产的上传任务信息
			}

		}

		return nil
	})
}

// registerFileDownloadStartEvent 注册文件下载开始事件
func registerFileDownloadStartEvent(
	opt *Options,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	registry *eventbus.EventRegistry,
	pool *MemoryPool,
) error {
	// 获取事件总线
	bus := registry.GetEventBus(EventFileDownloadStart)
	if bus == nil {
		return fmt.Errorf("无法获取文件下载开始事件总线")
	}

	// 注册
	return bus.Subscribe(EventFileDownloadStart, func(
		assetID string, // 文件资产的唯一标识
	) error {
		// 插入文件下载数据
		if err := db.InsertFilesDatabase(
			assetID,    // 文件资产的唯一标识
			0,          // 文件片段的总量
			0,          // 操作(0:下载)
			2,          // 状态(2:待开始)
			time.Now(), // 时间(当前时间)
		); err != nil {
			return err
		}

		// 确保文件目录存在
		if err := os.MkdirAll(filepath.Dir(opt.downloadPath), 0755); err != nil {
			return err
		}

		// 创建临时文件
		tempFilePath := filepath.Join(opt.downloadPath, assetID+".dep2p")
		if _, err := os.Create(tempFilePath); err != nil {
			return err
		}

		var retries int64 = 0
		for retries < opt.maxRetries {
			// 文件下载请求(清单)
			requestChecklistPayload := &FileDownloadRequestChecklistPayload{
				AssetID: assetID, // 文件资产的唯一标识
			}

			// 向指定的全网节点发送文件下载请求订阅消息
			if err := SendPubSub(p2p, pubsub, PubsubFileDownloadRequestTopic, "checklist", "", requestChecklistPayload); err != nil {
				return err
			}

			// 等待响应
			time.Sleep(opt.retryInterval)

			// 检查内存池中是否已接收到文件片段的哈希表
			pool.Mu.RLock()
			task, exists := pool.DownloadTasks[assetID]
			pool.Mu.RUnlock()
			if exists && len(task.PieceInfo) > 0 {
				// 已经收到文件片段的哈希表
				return nil
			}

			retries++
		}

		// 更新文件下载数据对象的状态
		if err := db.UpdateFileDatabaseStatus(assetID, 0, 0); err != nil { // 状态(0:失败)
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
func registerFileDownloadCheckEvent(
	opt *Options,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	registry *eventbus.EventRegistry,
	pool *MemoryPool,
) error {
	// 获取事件总线
	bus := registry.GetEventBus(EventFileDownloadCheck)
	if bus == nil {
		return fmt.Errorf("无法获取文件下载检查事件总线")
	}

	// 注册
	return bus.Subscribe(EventFileDownloadCheck, func(
		assetID string, // 文件资产的唯一标识
	) error {
		// 检查下载是否完成
		if !pool.IsDownloadComplete(assetID) {
			return nil // 下载未完成
		}

		// 新建文件存储
		fs, err := filestore.NewFileStore(DownloadPath)
		if err != nil {
			logrus.Errorf("创建文件存储失败:\t%v", err)
			return err
		}
		// 子目录当前主机+文件hash
		subDir := filepath.Join(p2p.Host().ID().String(), string(assetID)) // 设置子目录

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)
		if err != nil {
			logrus.Errorf("[registerFileDownloadCheckEvent] 获取切片失败:\t%v", err)
			return err
		}

		// 文件资产不存在
		if len(slices) == 0 {
			// 如果文件夹中没有任何切片文件，则重置下载任务
			if err := pool.ResetDownloadTask(assetID); err != nil {
				logrus.Errorf("重置下载任务失败: %v", err)
				return err
			}

			// return fmt.Errorf("no slices found in directory for assetID: %s", assetID)
		}

		if err := recoverDataFromSlices(fs, p2p, pool, opt, assetID); err != nil {
			// 如果恢复过程中出错，可能需要回退某些片段的进度
			return err
		}
		logrus.Printf("合并成功！！！！！！！")
		// TODO: 更新数据库状态、发布完成事件等

		return nil
	})
}

// recoverDataFromSlices从下载的切片中恢复文件数据
func recoverDataFromSlices(fs *filestore.FileStore, p2p *dep2p.DeP2P, pool *MemoryPool, opt *Options, assetID string) error {
	// 获取下载任务
	task, exists := pool.DownloadTasks[assetID]
	if !exists {
		return fmt.Errorf("下载任务不存在: %s", assetID)
	}

	// 初始化纠删码编码器
	enc, err := reedsolomon.New(task.DataPieces, task.TotalPieces-task.DataPieces)
	if err != nil {
		return fmt.Errorf("初始化纠删码编码器失败: %v", err)
	}

	// 从文件存储中读取所有切片
	shards, err := readAllShards(fs, p2p, pool, task)
	if err != nil {
		return err
	}

	// 验证数据
	ok, err := enc.Verify(shards)
	if err != nil {
		return err
	}
	if ok {
		logrus.Println("无需重建")
	} else {
		logrus.Println("验证失败。重建数据")
		// 恢复数据
		if err := enc.Reconstruct(shards); err != nil {
			return fmt.Errorf("纠删码恢复失败: %v", err)
		}
	}

	// 确保文件目录存在
	if err := os.MkdirAll(filepath.Dir(opt.downloadPath), 0755); err != nil {
		return err
	}

	// 写入临时文件
	tempFilePath := filepath.Join(opt.downloadPath, assetID+".defsdownload")

	if err := combineAndDecode(tempFilePath, shards, task.DataPieces, task.TotalPieces-task.DataPieces, int(task.Size)); err != nil {
		return err
	}
	// 重命名为最终文件
	finalFilePath := filepath.Join(opt.downloadPath, task.Name)

	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		return err
	}

	// 合并数据片段
	return nil
}

// readAllShards 读取所有片段数据
func readAllShards(fs *filestore.FileStore, p2p *dep2p.DeP2P, pool *MemoryPool, task *DownloadTask) ([][]byte, error) {
	shards := make([][]byte, task.TotalPieces)

	for i := range shards {

		if pieceInfo, ok := task.PieceInfo[i+1]; ok {

			content, err := processShard(fs, p2p, pool, task, pieceInfo)
			if err != nil {
				return nil, err
			}
			shards[i] = content
		} else {
			shards[i] = nil // 缺失的片段用nil表示
		}

	}
	return shards, nil
}

// processShard 处理单个片段
func processShard(fs *filestore.FileStore, p2p *dep2p.DeP2P, pool *MemoryPool, task *DownloadTask, pieceInfo *DownloadPieceInfo) ([]byte, error) {
	assetID := hex.EncodeToString((CalculateHash([]byte(task.FileHash))))

	subDir := filepath.Join(p2p.Host().ID().String(), assetID) // 设置子目录

	// 打开文件
	sliceFile, err := fs.OpenFile(subDir, pieceInfo.Hash)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "打开文件失败")
	}

	// 加载 xref 表
	xref, err := LoadXref(sliceFile)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "加载xref失败")
	}

	// 读取加密数据
	encryptedData, err := ReadSegmentToFile(sliceFile, "CONTENT", xref)
	if err != nil {
		return handlePieceError(pool, task, pieceInfo, "读取加密数据失败")
	}
	return encryptedData, nil
	// // 读取数据签名
	// signatureBytes, err := ReadSegmentToFile(sliceFile, "SIGNATURE", xref)
	// if err != nil {
	// 	return handlePieceError(pool, task, pieceInfo, "读取数据签名失败")
	// }

	// // 生成 RSA 密钥对
	// _, publicKey, err := GenerateKeysFromSeed([]byte(task.FileHash), 2048)
	// if err != nil {
	// 	return handlePieceError(pool, task, pieceInfo, "生成RSA密钥对失败")
	// }

	// // 验证签名
	// if !verifySignature(publicKey, encryptedData, signatureBytes) {
	// 	return handlePieceError(pool, task, pieceInfo, "验证签名失败")
	// }

	// 解密数据
	// return decryptData(encryptedData, []byte(task.FileHash))
}

// handlePieceError 处理片段错误
func handlePieceError(pool *MemoryPool, task *DownloadTask, pieceInfo *DownloadPieceInfo, errMsg string) ([]byte, error) {
	log.Printf("%s: %v", errMsg, pieceInfo.Hash)
	pool.RevertDownloadPieceProgress(task.FileHash, pieceInfo.Hash)
	if !pool.IsDownloadComplete(task.FileHash) {
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
func combineAndDecode(path string, split [][]byte, dataShards, parityShards, size int) error {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		fmt.Println("创建文件失败:", err)
		return err
	}
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
