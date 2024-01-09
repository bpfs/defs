package download

import (
	"context"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sirupsen/logrus"
)

// 流协议
type StreamProtocol struct {
	Ctx          context.Context         // 全局上下文
	Opt          *opts.Options           // 文件存储选项配置
	P2P          *dep2p.DeP2P            // DeP2P网络主机
	PubSub       *pubsub.DeP2PPubSub     // DeP2P网络订阅
	DB           *sqlites.SqliteDB       // sqlite数据库服务
	UploadChan   chan *core.UploadChan   // 用于刷新上传的通道
	DownloadChan chan *core.DownloadChan // 用于刷新下载的通道

	Registry *eventbus.EventRegistry // 事件总线
	Cache    *ristretto.Cache        // 缓存实例
	Pool     *pool.MemoryPool        // 内存池
}

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
		if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponseStream] 解码失败:\t%v", err)
			return err
		}

		// 处理文件下载响应清单
		// ProcessDownloadResponseChecklist(sp.pool, sp.db, sp.p2p, sp.pubsub, payload, receiver)
		go ProcessDownloadResponseChecklist(sp.Pool, sp.DB, sp.P2P, sp.PubSub, payload, receiver)

	case "content":
		payload := new(FileDownloadResponseContentPayload)
		if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponseStream] 解码失败:\t%v", err)
			return err
		}

		// 处理文件下载响应内容
		go ProcessDownloadResponseContent(sp.P2P, sp.DB, sp.DownloadChan, sp.Registry, sp.Pool, payload)
	}
	// 组装响应数据
	res.Code = 200                                 // 响应代码
	res.Msg = "成功"                                 // 响应消息
	res.Data = []byte(sp.P2P.Host().ID().String()) // 响应数据(主机地址)

	return nil
}

// SendDownloadInfo 向下载通道发送信息
func SendDownloadInfo(downloadChans chan *core.DownloadChan, fileID, sliceHash string, totalPieces, index int) {
	downloadInfo := &core.DownloadChan{
		FileID:      fileID,
		SliceHash:   sliceHash,
		TotalPieces: totalPieces,
		Index:       index,
	}
	downloadChans <- downloadInfo
}
