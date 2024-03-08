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
	"go.uber.org/fx"
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
	registry.RegisterEvent(config.EventFileUploadCheck, eventbus.New())

	// 注册文件片段上传事件总线
	registry.RegisterEvent(config.EventFileSliceUpload, eventbus.New())

	// 注册文件下载开始事件总线
	registry.RegisterEvent(config.EventFileDownloadStart, eventbus.New())

	// 注册文件下载检查事件总线
	registry.RegisterEvent(config.EventFileDownloadCheck, eventbus.New())

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

// RegisterEvents 注册事件
func RegisterEventProtocol(lc fx.Lifecycle, input RegisterEventProtocolInput) error {
	// 注册文件上传检查事件
	if err := upload.RegisterFileUploadCheckEvent(
		input.P2P,
		input.PubSub,
		input.DB,
		input.Registry,
		input.Pool,
	); err != nil {
		return err
	}

	// 注册文件片段上传事件
	if err := upload.RegisterFileSliceUploadEvent(
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
	if err := download.RegisterFileDownloadStartEvent(
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
	if err := download.RegisterFileDownloadCheckEvent(
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
