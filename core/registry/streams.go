package registry

import (
	"context"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/download"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/upload"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"go.uber.org/fx"
)

type RegisterStreamProtocolInput struct {
	fx.In
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

// RegisterStreamProtocol 注册流
func RegisterStreamProtocol(lc fx.Lifecycle, input RegisterStreamProtocolInput) {
	// 流协议
	usp := &upload.StreamProtocol{
		Ctx:          input.Ctx,
		Opt:          input.Opt,
		P2P:          input.P2P,
		PubSub:       input.PubSub,
		DB:           input.DB,
		UploadChan:   input.UploadChan,
		DownloadChan: input.DownloadChan,
		Registry:     input.Registry,
		Cache:        input.Cache,
		Pool:         input.Pool,
	}
	// 注册文件片段上传流
	streams.RegisterStreamHandler(input.P2P.Host(), config.StreamFileSliceUploadProtocol, streams.HandlerWithRW(usp.HandleStreamFileSliceUploadStream))

	// 流协议
	dsp := &download.StreamProtocol{
		Ctx:          input.Ctx,
		Opt:          input.Opt,
		P2P:          input.P2P,
		PubSub:       input.PubSub,
		DB:           input.DB,
		UploadChan:   input.UploadChan,
		DownloadChan: input.DownloadChan,
		Registry:     input.Registry,
		Cache:        input.Cache,
		Pool:         input.Pool,
	}

	// 注册文件下载响应流
	streams.RegisterStreamHandler(input.P2P.Host(), config.StreamFileDownloadResponseProtocol, streams.HandlerWithRW(dsp.HandleFileDownloadResponseStream))

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}
