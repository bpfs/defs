package defs

import (
	"context"
	"path/filepath"
	"time"

	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/filestore"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/dgraph-io/ristretto"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"

	"github.com/sirupsen/logrus"
)

// 订阅主题
const (
	// 文件上传请求主题
	PubsubFileUploadRequestTopic = "defs@pubsub:file/upload/request/1.0.0"
	// 文件上传响应主题
	PubsubFileUploadResponseTopic = "defs@pubsub:file/upload/response/1.0.0"
	// 文件下载请求主题
	PubsubFileDownloadRequestTopic = "defs@pubsub:file/download/request/1.0.0"
	// 文件下载响应主题
	PubsubFileDownloadResponseTopic = "defs@pubsub:file/download/response/1.0.0"
)

type RegisterPubsubProtocolInput struct {
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

// RegisterPubsubProtocol 注册订阅
func RegisterPubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 文件上传请求主题
	if err := input.PubSub.SubscribeWithTopic(PubsubFileUploadRequestTopic, func(res *streams.RequestMessage) {
		HandleFileUploadRequestPubSub(input.P2P, input.PubSub, res)
	}, true); err != nil {
		logrus.Errorf("注册文件上传请求失败：%v \n", err)
	}

	// 文件上传响应主题
	if err := input.PubSub.SubscribeWithTopic(PubsubFileUploadResponseTopic, func(res *streams.RequestMessage) {
		HandleFileUploadResponsePubSub(input.P2P, input.PubSub, input.Pool, res)
	}, true); err != nil {
		logrus.Errorf("注册文件上传响应失败：%v \n", err)
	}

	// 文件下载请求主题
	if err := input.PubSub.SubscribeWithTopic(PubsubFileDownloadRequestTopic, func(res *streams.RequestMessage) {
		HandleFileDownloadRequestPubSub(input.P2P, input.PubSub, res)
	}, true); err != nil {
		logrus.Errorf("注册文件下载请求失败：%v \n", err)
	}

	// 文件下载响应主题
	if err := input.PubSub.SubscribeWithTopic(PubsubFileDownloadResponseTopic, func(res *streams.RequestMessage) {
		HandleFileDownloadResponsePubSub(input.P2P, input.PubSub, input.DB, input.DownloadChan, input.Registry, input.Pool, res)
	}, true); err != nil {
		logrus.Errorf("注册文件下载响应失败：%v \n", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}

// HandleFileUploadRequestPubSub 处理文件上传请求的订阅消息
func HandleFileUploadRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := filestore.NewFileStore(SlicePath)
	if err != nil {
		return
	}

	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	// 文件上传请求(检查)
	payload := new(FileUploadRequestCheckPayload)
	if err := DecodeFromBytes(res.Payload, payload); err != nil {
		logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
		return
	}

	// 子目录当前主机+文件hash
	subDir := filepath.Join(p2p.Host().ID().String(), string(payload.AssetID)) // 设置子目录

	switch res.Message.Type {
	// 检查
	case "check":
		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)
		if err != nil {
			logrus.Errorf("[check] 获取切片失败:\t%v", err)
			return
		}

		// 文件资产不存在
		if len(slices) == 0 {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.AssetID, sliceHash)
			if err != nil {
				logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
				continue
			}

			// LoadXref 从文件加载 xref 表
			xref, err := LoadXref(sliceFile)
			if err != nil {
				logrus.Errorf("从文件加载 xref 表失败:\t%v", err)
				continue
			}

			// 从文件读取上传时间
			uploadTimeBytes, err := ReadSegmentToFile(sliceFile, "UPLOADTIME", xref)
			if err != nil {
				continue
			}
			uploadTimeUnix, err := FromBytes[int64](uploadTimeBytes)
			if err != nil {
				continue
			}
			uploadTime := time.Unix(uploadTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time
			// 检查接收到的上传时间与文件存储的上传时间是否一致
			// 一致则表示为本次存储内容
			if payload.UploadTime.Equal(uploadTime) {
				continue
			}
			// 否则，回复消息告知已存在
			// 向指定的指定节点发送文件上传响应的订阅消息
			if err := SendPubSub(p2p, pubsub, PubsubFileUploadResponseTopic, "exist", peer.ID(receiver.String()), payload); err != nil {
				return
			}

		}

	// 撤销
	case "cancel":
		// TODO: 删除本地切片
		logrus.Printf("需删除本地:\t%s\t文件片段\n", payload.AssetID)

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)
		if err != nil {
			logrus.Errorf("[cancel ]获取切片失败:\t%v", err)
			return
		}

		// 文件资产不存在
		if len(slices) == 0 {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.AssetID, sliceHash)
			if err != nil {
				logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
				continue
			}

			// LoadXref 从文件加载 xref 表
			xref, err := LoadXref(sliceFile)
			if err != nil {
				logrus.Errorf("从文件加载 xref 表失败:\t%v", err)
				continue
			}

			// 从文件读取上传时间
			uploadTimeBytes, err := ReadSegmentToFile(sliceFile, "UPLOADTIME", xref)
			if err != nil {
				continue
			}
			uploadTimeUnix, err := FromBytes[int64](uploadTimeBytes)
			if err != nil {
				continue
			}
			uploadTime := time.Unix(uploadTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time
			// 检查接收到的上传时间与文件存储的上传时间是否一致
			// 一致则表示为本次存储内容
			if payload.UploadTime.Equal(uploadTime) {
				// 删除该文件片段
				_ = fs.Delete(payload.AssetID, sliceHash)
			}
		}
	}
}

// HandleFileUploadResponsePubSub 处理文件上传响应的订阅消息
func HandleFileUploadResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, pool *MemoryPool, res *streams.RequestMessage) {
	// receiver, err := peer.Decode(res.Message.Sender)
	// if err != nil {
	// 	logrus.Errorf("解析peerid失败:\t%v", err)
	// 	return
	// }

	switch res.Message.Type {
	// 存在
	case "exist":
		// 文件上传请求(检查)
		payload := new(FileUploadRequestCheckPayload)
		if err := DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
			return
		}

		// 暂停指定的上传任务
		if err := pool.PauseUploadTask(payload.AssetID); err != nil {
			return
		}

		// 向指定的指定节点发送文件上传响应的订阅消息
		if err := SendPubSub(p2p, pubsub, PubsubFileUploadResponseTopic, "cancel", "", payload); err != nil {
			return
		}
	}
}

// HandleFileDownloadRequestPubSub 处理文件下载请求的订阅消息
func HandleFileDownloadRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := filestore.NewFileStore(filepath.Join(SlicePath, p2p.Host().ID().String()))
	if err != nil {
		return
	}

	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	switch res.Message.Type {
	// 清单
	case "checklist":
		// 文件下载请求(清单)
		payload := new(FileDownloadRequestChecklistPayload)
		if err := DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
			return
		}
		// 子目录当前主机+文件hash
		//subDir := filepath.Join(p2p.Host().ID().String(), string(payload.AssetID)) // 设置子目录
		subDir := string(payload.AssetID)

		// 文件下载响应(清单)
		responseChecklistPayload := FileDownloadResponseChecklistPayload{
			AssetID:         payload.AssetID,     // 文件资产的唯一标识
			FileHash:        "",                  // 文件内容的哈希值
			Name:            "",                  // 文件的基本名称
			Size:            0,                   // 常规文件的长度(以字节为单位)
			SliceTable:      map[int]HashTable{}, // 文件片段的哈希表
			AvailableSlices: map[int]string{},    // 本地存储的文件片段信息
		}

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)

		if err != nil {
			logrus.Errorf("[checklist]获取切片失败:\t%v", err)
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.AssetID, sliceHash)
			if err != nil {
				logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
				continue
			}

			// LoadXref 从文件加载 xref 表
			xref, err := LoadXref(sliceFile)
			if err != nil {
				logrus.Errorf("从文件加载 xref 表失败:\t%v", err)
				continue
			}

			// 只从第一个文件片段中读取整个文件的切片哈希表
			if len(responseChecklistPayload.SliceTable) == 0 {
				sliceTableBytes, err := ReadSegmentToFile(sliceFile, "SLICETABLE", xref)
				if err != nil {
					logrus.Errorf("从文件加载 SLICETABLE 表失败:%v", err)
					continue
				}
				var sliceTable map[int]HashTable
				if err := DecodeFromBytes(sliceTableBytes, &sliceTable); err != nil {
					logrus.Errorf("[HandleDownload] 解码失败:\t%v", err)
					continue
				}

				responseChecklistPayload.SliceTable = sliceTable
			}

			// 从文件读取文件资产的唯一标识
			assetIDBytes, err := ReadSegmentToFile(sliceFile, "ASSETID", xref)
			if err != nil {
				logrus.Errorf("从文件加载 ASSETID 表失败:%v", err)
				continue
			}
			// 文件资产唯一标识不一致
			if payload.AssetID != string(assetIDBytes) {
				continue
			}

			// 从文件读取文件的基本名称
			nameBytes, err := ReadSegmentToFile(sliceFile, "NAME", xref)
			if err != nil {
				logrus.Errorf("从文件加载 NAME 表失败:%v", err)
				continue
			}

			responseChecklistPayload.Name = string(nameBytes)
			// if responseChecklistPayload.Name, err = FromBytes[string](nameBytes); err != nil {
			// 	continue
			// }

			// 从文件读取文件的长度
			sizeBytes, err := ReadSegmentToFile(sliceFile, "SIZE", xref)
			if err != nil {
				continue
			}
			if responseChecklistPayload.Size, err = FromBytes[int64](sizeBytes); err != nil {
				continue
			}

			// 从文件读取切片的哈希和索引
			sliceHashBytes, err := ReadSegmentToFile(sliceFile, "SLICEHASH", xref)
			if err != nil {
				logrus.Errorf("从文件加载 SLICEHASH 表失败:%v", err)
				continue
			}
			// 文件片段存储命名哈希与内容读取的不一致
			if sliceHash != string(sliceHashBytes) {
				continue
			}
			indexBytes, err := ReadSegmentToFile(sliceFile, "INDEX", xref)
			if err != nil {
				logrus.Errorf("从文件加载 INDEX 表失败:%v", err)
				continue
			}
			index, err := FromBytes[int32](indexBytes)
			if err != nil {
				logrus.Errorf("[index]FromBytes 转化 失败:%v", err)
				continue
			}

			// 从文件读取切片的共享状态
			sharedBytes, err := ReadSegmentToFile(sliceFile, "SHARED", xref)
			if err == nil && sharedBytes != nil {
				shared, err := FromBytes[bool](sharedBytes)
				if err != nil {
					logrus.Errorf("[shared]FromBytes 转化 失败:%v", err)
					continue
				}
				if shared {
					// 从文件读取文件内容的哈希值
					fileHashBytes, err := ReadSegmentToFile(sliceFile, "FILEHASH", xref)
					if err == nil {
						responseChecklistPayload.FileHash = string(fileHashBytes)
					}
				}
			}

			responseChecklistPayload.AvailableSlices[int(index)] = string(sliceHashBytes)
		}

		// 发送响应清单
		_ = SendDataToPeer(p2p, pubsub, StreamFileDownloadResponseProtocol, PubsubFileDownloadResponseTopic, "checklist", receiver, responseChecklistPayload)

	// 内容
	case "content":
		// 文件下载请求(内容)
		payload := new(FileDownloadRequestContentPayload)
		if err := DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
			return
		}

		// 读取文件
		sliceContent, err := fs.Read(payload.AssetID, payload.SliceHash)
		if err != nil {
			return
		}

		// 文件下载响应(内容)
		responseContentPayload := FileDownloadResponseContentPayload{
			AssetID:      payload.AssetID,
			SliceHash:    payload.SliceHash,
			Index:        payload.Index,
			SliceContent: sliceContent,
		}

		// 尝试先通过流发送数据，失败后通过订阅发送
		_ = SendDataToPeer(p2p, pubsub, StreamFileDownloadResponseProtocol, PubsubFileDownloadResponseTopic, "content", receiver, responseContentPayload)
	}
}

// HandleileFileDownloadResponsePubSub 处理文件下载响应的订阅消息
// func HandleileFileDownloadResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, registry *eventbus.EventRegistry, pool *MemoryPool, res *streams.RequestMessage) {
// 	receiver, err := peer.Decode(res.Message.Sender)
// 	if err != nil {
// 		logrus.Errorf("解析peerid失败: %v", err)
// 		return
// 	}

// 	switch res.Message.Type {
// 	// 清单
// 	case "checklist":
// 		// 文件下载响应(清单)
// 		payload := new(FileDownloadResponseChecklistPayload)
// 		if err := DecodeFromBytes(res.Payload, payload); err != nil {
// 			logrus.Errorf("[HandleileFileDownloadResponsePubSub] 解码失败:\t%v", err)
// 			return
// 		}

// 		// 更新下载任务中特定片段的信息
// 		if len(payload.fileHash) > 0 {
// 			pool.UpdateDownloadPieceInfo(receiver.String(), payload.assetID, payload.sliceTable, payload.availableSlices, payload.fileHash)
// 		} else {
// 			pool.UpdateDownloadPieceInfo(receiver.String(), payload.assetID, payload.sliceTable, payload.availableSlices)
// 		}

// 		// 更新文件下载数据对象的状态
// 		if err := db.UpdateFileDatabaseStatus(payload.assetID, 0, 3); err != nil { // 状态(3:进行中)
// 			return
// 		}

// 		// 获取未完成的下载片段的哈希值
// 		pieceHashes := pool.GetIncompleteDownloadPieces(payload.assetID)
// 		for _, hash := range pieceHashes {
// 			for k, v := range payload.availableSlices {
// 				if hash == v {
// 					// 文件下载请求(内容)
// 					responseContentPayload := FileDownloadRequestContentPayload{
// 						assetID:   payload.assetID,
// 						sliceHash: v,
// 					}

// 					// 尝试先通过流发送数据，失败后通过订阅发送
// 					_ = SendDataToPeer(p2p, pubsub, StreamFileDownloadResponseProtocol, PubsubFileDownloadResponseTopic, "content", receiver, responseContentPayload)

// 					if (k + 1) != len(payload.availableSlices) {
// 						// 延时1秒循环，释放网络压力
// 						time.Sleep(1 * time.Second)
// 					}
// 				}
// 			}
// 		}

// 	// 内容
// 	case "content":
// 		// 文件下载响应(内容)
// 		payload := new(FileDownloadResponseContentPayload)
// 		if err := DecodeFromBytes(res.Payload, payload); err != nil {
// 			logrus.Errorf("[HandleileFileDownloadResponsePubSub] 解码失败:\t%v", err)
// 			return
// 		}

// 		fs, err := filestore.NewFileStore(DownloadPath)
// 		if err != nil {
// 			return
// 		}
// 		// 子目录当前主机+文件hash
// 		subDir := filepath.Join(p2p.Host().ID().String(), payload.assetID) // 设置子目录

// 		// 写入本地文件
// 		if err := fs.Write(subDir, payload.sliceHash, payload.sliceContent); err != nil {
// 			logrus.Error("存储接收内容失败, error:", err)
// 			return
// 		}

// 		// 根据文件片段的哈希值标记下载任务中的一个片段为完成
// 		if pool.MarkDownloadPieceCompleteByHash(payload.assetID, payload.sliceHash) {
// 			// 获取文件下载检查事件总线
// 			bus := registry.GetEventBus(EventFileDownloadCheck)
// 			if bus == nil {
// 				return
// 			}
// 			bus.Publish(EventFileDownloadCheck, payload.assetID)
// 		}
// 	}
// }

// HandleFileDownloadResponsePubSub 处理文件下载响应的订阅消息
func HandleFileDownloadResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, downloadChan chan *downloadChan, registry *eventbus.EventRegistry, pool *MemoryPool, res *streams.RequestMessage) {
	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	switch res.Message.Type {
	case "checklist":
		payload := new(FileDownloadResponseChecklistPayload)
		if err := DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponsePubSub] 解码失败:\t%v", err)
			return
		}

		// 处理文件下载响应清单
		ProcessDownloadResponseChecklist(pool, db, p2p, pubsub, payload, receiver)

	case "content":
		payload := new(FileDownloadResponseContentPayload)
		if err := DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponsePubSub] 解码失败:\t%v", err)
			return
		}

		// 处理文件下载响应内容
		ProcessDownloadResponseContent(p2p, db, downloadChan, registry, pool, payload)
	}
}

// ProcessDownloadResponseChecklist 处理文件下载响应清单
func ProcessDownloadResponseChecklist(pool *MemoryPool, db *sqlites.SqliteDB, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, payload *FileDownloadResponseChecklistPayload, receiver peer.ID) {
	// 更新下载任务中特定片段的信息
	if len(payload.FileHash) > 0 {
		pool.UpdateDownloadPieceInfo(receiver.String(), payload.AssetID, payload.Name, payload.Size, payload.SliceTable, payload.AvailableSlices, payload.FileHash)
	} else {
		pool.UpdateDownloadPieceInfo(receiver.String(), payload.AssetID, payload.Name, payload.Size, payload.SliceTable, payload.AvailableSlices)
	}

	// 更新文件下载数据对象的状态
	if err := db.UpdateFileDatabaseStatus(payload.AssetID, 0, 3); err != nil { // 状态(3:进行中)
		logrus.Errorf("更新数据库状态失败:\t%v", err)
		return
	}
	// 发送文件下载请求（内容）
	SendDownloadRequestContents(pool, p2p, pubsub, payload, receiver)
}

// SendDownloadRequestContents 发送文件下载请求（内容）
func SendDownloadRequestContents(pool *MemoryPool, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, payload *FileDownloadResponseChecklistPayload, receiver peer.ID) {
	// 获取未完成的下载片段的哈希值

	pieceHashes := pool.GetIncompleteDownloadPieces(payload.AssetID)
	for _, hash := range pieceHashes {
		for availableIndex, availableHash := range payload.AvailableSlices { // 本地存储的文件片段信息
			if hash == availableHash {
				responseContentPayload := FileDownloadRequestContentPayload{
					AssetID:   payload.AssetID,
					SliceHash: availableHash,
					Index:     availableIndex,
				}
				// 尝试先通过流发送数据，失败后通过订阅发送
				//_ = SendDataToPeer(p2p, pubsub, StreamFileDownloadResponseProtocol, PubsubFileDownloadResponseTopic, "content", receiver, responseContentPayload)
				if err := SendPubSub(p2p, pubsub, PubsubFileDownloadRequestTopic, "content", receiver, responseContentPayload); err != nil {
					logrus.Errorf("[PubsubFileDownloadRequestTopic]发送订阅失败%v", err)
					return
				}

				time.Sleep(1 * time.Second) // 延时以释放网络压力
			}
		}
	}
}

// ProcessDownloadResponseContent 处理文件下载响应内容
func ProcessDownloadResponseContent(p2p *dep2p.DeP2P, db *sqlites.SqliteDB, downloadChan chan *downloadChan, registry *eventbus.EventRegistry, pool *MemoryPool, payload *FileDownloadResponseContentPayload) {
	fs, err := filestore.NewFileStore(DownloadPath)
	if err != nil {
		logrus.Errorf("创建文件存储失败:\t%v", err)
		return
	}
	// 子目录当前主机+文件hash
	subDir := filepath.Join(p2p.Host().ID().String(), string(payload.AssetID)) // 设置子目录

	// 写入本地文件
	if err := fs.Write(subDir, payload.SliceHash, payload.SliceContent); err != nil {
		logrus.Errorf("写入本地文件失败:\t%v", err)
		return
	}

	// 根据文件片段的哈希值标记下载任务中的一个片段为完成
	if pool.MarkDownloadPieceCompleteByHash(payload.AssetID, payload.SliceHash) {
		// 触发文件下载检查事件
		bus := registry.GetEventBus(EventFileDownloadCheck)
		if bus != nil {
			bus.Publish(EventFileDownloadCheck, payload.AssetID)
		}
	}

	go func() {
		// 获取下载任务
		task, exists := pool.DownloadTasks[string(payload.AssetID)]
		if !exists {
			logrus.Errorf("下载任务不存在: %s", string(payload.AssetID))
		}
		// 向下载通道发送信息
		SendDownloadInfo(downloadChan, payload.AssetID, payload.SliceHash, task.TotalPieces, payload.Index)
	}()

}

// ProcessDownloadResponseGetContent 处理文件下载响应内容
func ProcessDownloadResponseGetContent(p2p *dep2p.DeP2P, db *sqlites.SqliteDB, downloadChan chan *downloadChan, registry *eventbus.EventRegistry, pool *MemoryPool, payload *FileDownloadResponseContentPayload) {
	fs, err := filestore.NewFileStore(DownloadPath)
	if err != nil {
		logrus.Errorf("创建文件存储失败:\t%v", err)
		return
	}
	// 子目录当前主机+文件hash
	subDir := filepath.Join(p2p.Host().ID().String(), string(payload.AssetID)) // 设置子目录

	// 写入本地文件
	if err := fs.Write(subDir, payload.SliceHash, payload.SliceContent); err != nil {
		logrus.Errorf("写入本地文件失败:\t%v", err)
		return
	}

	// 根据文件片段的哈希值标记下载任务中的一个片段为完成
	if pool.MarkDownloadPieceCompleteByHash(payload.AssetID, payload.SliceHash) {
		// 触发文件下载检查事件
		bus := registry.GetEventBus(EventFileDownloadCheck)
		if bus != nil {
			bus.Publish(EventFileDownloadCheck, payload.AssetID)
		}
	}

	go func() {
		// 获取下载任务
		task, exists := pool.DownloadTasks[string(payload.AssetID)]
		if !exists {
			logrus.Errorf("下载任务不存在: %s", string(payload.AssetID))
		}
		// 向下载通道发送信息
		SendDownloadInfo(downloadChan, payload.AssetID, payload.SliceHash, task.TotalPieces, payload.Index)
	}()

}
