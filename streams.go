package defs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/filestore"
	"github.com/bpfs/defs/sqlites"
	"github.com/dgraph-io/ristretto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"go.uber.org/fx"
)

// 流协议
const (
	// 文件片段上传协议
	StreamFileSliceUploadProtocol = "defs@stream:file/slice/upload/1.0.0"
	// 文件下载响应协议
	StreamFileDownloadResponseProtocol = "defs@stream:file/download/response/1.0.0"
)

type RegisterStreamProtocolInput struct {
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

// RegisterStreamProtocol 注册流
func RegisterStreamProtocol(lc fx.Lifecycle, input RegisterStreamProtocolInput) {
	// 流协议
	sp := &StreamProtocol{
		ctx:          input.Ctx,
		p2p:          input.P2P,
		pubsub:       input.PubSub,
		db:           input.DB,
		uploadChan:   input.UploadChan,
		downloadChan: input.DownloadChan,
		registry:     input.Registry,
		cache:        input.Cache,
		pool:         input.Pool,
	}
	// 注册文件片段上传流
	streams.RegisterStreamHandler(input.P2P.Host(), StreamFileSliceUploadProtocol, streams.HandlerWithRW(sp.HandleStreamFileSliceUploadStream))

	// 注册文件下载响应流
	streams.RegisterStreamHandler(input.P2P.Host(), StreamFileDownloadResponseProtocol, streams.HandlerWithRW(sp.HandleFileDownloadResponseStream))

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}

// 流协议
type StreamProtocol struct {
	ctx          context.Context         // 上下文
	p2p          *dep2p.DeP2P            // DeP2P网络
	pubsub       *pubsub.DeP2PPubSub     // DeP2P网络订阅
	db           *sqlites.SqliteDB       // sqlite数据库服务
	uploadChan   chan *uploadChan        // 用于刷新上传的通道
	downloadChan chan *downloadChan      // 用于刷新下载的通道
	registry     *eventbus.EventRegistry // 事件总线
	cache        *ristretto.Cache        // 缓存实例
	pool         *MemoryPool             // 内存池
}

func CreateTempFile(payload []byte) (file *os.File, err error) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "prefix-")
	if err != nil {
		logrus.Error("创建临时文件失败, error:", err)

		return nil, err
	}

	//  写入临时文件
	_, err = tmpFile.Write(payload)
	if err != nil {
		logrus.Error("写入临时文件失败, error:", err)
		return nil, err
	}

	return tmpFile, nil

}

// HandleStreamFileSliceUploadStream 处理文件片段上传的流消息
func (sp *StreamProtocol) HandleStreamFileSliceUploadStream(req *streams.RequestMessage, res *streams.ResponseMessage) error {
	/////////////////////// 以后要改进 ///////////////////////
	// TODO:先用临时文件的形式
	payload := new([]byte)
	if err := DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[HandleStreamFileSliceUploadStream] 解码失败:\t%v", err)
		return err
	}

	tmpFile, err := CreateTempFile(*payload)
	if err != nil {
		logrus.Errorf("创建临时文件失败:%v", err)
		return err
	}

	// 最后删除临时文件
	defer func() {
		os.Remove(tmpFile.Name())
		tmpFile.Close()
	}()
	/////////////////////// 以后要改进 ///////////////////////

	// LoadXref 从文件加载 xref 表
	xref, err := LoadXref(tmpFile)
	if err != nil {
		fmt.Println("从文件加载 xref 表失败:", err)
		return err
	}

	// ReadSegment 从文件读取段 ASSETID
	assetID, err := ReadSegmentToFile(tmpFile, "ASSETID", xref)
	if err != nil {
		logrus.Errorf("从文件加载 ASSETID 表失败:%v", err)

		return err
	}

	// ReadSegment 从文件读取段 SLICEID
	sliceId, err := ReadSegmentToFile(tmpFile, "SLICEHASH", xref)
	if err != nil {
		logrus.Errorf("从文件加载 SLICEHASH 表失败:%v", err)
		return err
	}

	// 新建文件存储
	fs, err := filestore.NewFileStore(SlicePath)
	if err != nil {
		logrus.Errorf("创建新建文件存储失败:%v ", err)
		return err
	}
	// 子目录当前主机+文件hash
	subDir := filepath.Join(sp.p2p.Host().ID().String(), string(assetID)) // 设置子目录

	// 写入本地文件
	if err := fs.Write(subDir, string(sliceId), *payload); err != nil {
		logrus.Error("存储接收内容失败, error:", err)
		return fmt.Errorf("请求无法处理")
	}
	// 组装响应数据
	res.Code = 200                                 // 响应代码
	res.Msg = "成功"                                 // 响应消息
	res.Data = []byte(sp.p2p.Host().ID().String()) // 响应数据(主机地址)

	return nil
}

// HandleFileDownloadResponseStream 处理文件下载响应的流消息
// func (sp *StreamProtocol) HandleFileDownloadResponseStream(req *streams.RequestMessage, res *streams.ResponseMessage) error {

// 	receiver, err := peer.Decode(req.Message.Sender)
// 	if err != nil {
// 		logrus.Errorf("解析peerid失败: %v", err)
// 		return err
// 	}

// 	switch req.Message.Type {
// 	// 清单
// 	case "checklist":
// 		// 文件下载响应(清单)
// 		payload := new(FileDownloadResponseChecklistPayload)
// 		if err := DecodeFromBytes(req.Payload, payload); err != nil {
// 			logrus.Errorf("[HandleFileDownloadResponseStream] 解码失败:\t%v", err)
// 			return err
// 		}

// 		// 更新下载任务中特定片段的信息
// 		if len(payload.fileHash) > 0 {
// 			sp.pool.UpdateDownloadPieceInfo(receiver.String(), payload.assetID, payload.sliceTable, payload.availableSlices, payload.fileHash)
// 		} else {
// 			sp.pool.UpdateDownloadPieceInfo(receiver.String(), payload.assetID, payload.sliceTable, payload.availableSlices)
// 		}

// 		// 更新文件下载数据对象的状态
// 		if err := sp.db.UpdateFileDatabaseStatus(payload.assetID, 0, 3); err != nil { // 状态(3:进行中)
// 			return err
// 		}

// 		// 获取未完成的下载片段的哈希值
// 		pieceHashes := sp.pool.GetIncompleteDownloadPieces(payload.assetID)
// 		for _, hash := range pieceHashes {
// 			for k, v := range payload.availableSlices {
// 				if hash == v {
// 					// 文件下载请求(内容)
// 					responseContentPayload := FileDownloadRequestContentPayload{
// 						assetID:   payload.assetID,
// 						sliceHash: v,
// 					}

// 					// 尝试先通过流发送数据，失败后通过订阅发送
// 					_ = SendDataToPeer(sp.p2p, sp.pubsub, StreamFileDownloadResponseProtocol, PubsubFileDownloadResponseTopic, "content", receiver, responseContentPayload)

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
// 		if err := DecodeFromBytes(req.Payload, payload); err != nil {
// 			logrus.Errorf("[HandleFileDownloadResponseStream] 解码失败:\t%v", err)
// 			return err
// 		}

// 		fs, err := filestore.NewFileStore(DownloadPath)
// 		if err != nil {
// 			return err
// 		}
// 		// 子目录当前主机+文件hash
// 		subDir := filepath.Join(sp.p2p.Host().ID().String(), payload.assetID) // 设置子目录

// 		// 写入本地文件
// 		if err := fs.Write(subDir, payload.sliceHash, payload.sliceContent); err != nil {
// 			logrus.Error("存储接收内容失败, error:", err)
// 			return fmt.Errorf("请求无法处理")
// 		}

// 		// 根据文件片段的哈希值标记下载任务中的一个片段为完成
// 		if sp.pool.MarkDownloadPieceCompleteByHash(payload.assetID, payload.sliceHash) {
// 			// 获取文件下载检查事件总线
// 			bus := sp.registry.GetEventBus(EventFileDownloadCheck)
// 			if bus == nil {
// 				return fmt.Errorf("无法获取文件下载检查事件总线")
// 			}
// 			bus.Publish(EventFileDownloadCheck, payload.assetID)
// 		}
// 	}
// 	return nil
// }

// HandleFileDownloadResponseStream 处理文件下载响应的流消息
func (sp *StreamProtocol) HandleFileDownloadResponseStream(req *streams.RequestMessage, res *streams.ResponseMessage) error {

	receiver, err := peer.Decode(req.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return err
	}

	switch req.Message.Type {
	case "checklist":
		payload := new(FileDownloadResponseChecklistPayload)
		if err := DecodeFromBytes(req.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponseStream] 解码失败:\t%v", err)
			return err
		}

		// 处理文件下载响应清单
		// ProcessDownloadResponseChecklist(sp.pool, sp.db, sp.p2p, sp.pubsub, payload, receiver)
		go ProcessDownloadResponseChecklist(sp.pool, sp.db, sp.p2p, sp.pubsub, payload, receiver)

	case "content":
		payload := new(FileDownloadResponseContentPayload)
		if err := DecodeFromBytes(req.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponseStream] 解码失败:\t%v", err)
			return err
		}

		// 处理文件下载响应内容
		// ProcessDownloadResponseContent(sp.p2p, sp.db, sp.downloadChan, sp.registry, sp.pool, payload)
		go ProcessDownloadResponseContent(sp.p2p, sp.db, sp.downloadChan, sp.registry, sp.pool, payload)
	}
	// 组装响应数据
	res.Code = 200                                 // 响应代码
	res.Msg = "成功"                                 // 响应消息
	res.Data = []byte(sp.p2p.Host().ID().String()) // 响应数据(主机地址)

	return nil
}
