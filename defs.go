package defs

import (
	"context"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/opts"

	"github.com/bpfs/defs/core/cache"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/registry"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/ristretto"
	"go.uber.org/fx"

	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
)

// FS 提供了与DeFS交互所需的各种函数
type FS struct {
	ctx          context.Context         // 全局上下文
	opt          *opts.Options           // 文件存储选项配置
	p2p          *dep2p.DeP2P            // DeP2P网络主机
	pubsub       *pubsub.DeP2PPubSub     // DeP2P网络订阅
	db           *sqlites.SqliteDB       // sqlite数据库服务
	uploadChan   chan *core.UploadChan   // 用于刷新上传的通道
	downloadChan chan *core.DownloadChan // 用于刷新下载的通道
	searchChan   chan *core.SearchChan   // 用于刷新搜索的通道

	registry *eventbus.EventRegistry // 事件总线
	cache    *ristretto.Cache        // 缓存实例
	pool     *pool.MemoryPool        // 内存池
}

// Open 返回一个新的文件存储对象
func Open(opt *opts.Options, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub) (*FS, error) {
	// 1. 检查并设置选项
	if err := checkAndSetOptions(opt); err != nil {
		return nil, err
	}
	// 2. 本地文件夹
	if err := paths.InitDirectories(opt.GetRootPath()); err != nil {
		return nil, err
	}
	// 3. 本地数据库
	db, err := sqlites.NewSqliteDB(paths.GetBusinessDbPath(), opts.DbFile)
	if err != nil {
		return nil, err
	}
	// 3.1 数据库表
	if err := sqlite.InitDBTable(db); err != nil {
		return nil, err
	}

	ctx := context.Background()
	fs := &FS{
		ctx:    ctx,
		opt:    opt,
		p2p:    p2p,
		pubsub: pubsub,
		db:     db,
	}

	// fx 配置项
	opts := []fx.Option{
		globalInit(fs),
		fx.Provide(
			registry.NewEventRegistry, // 新的事件总线
			cache.NewRistrettoCache,   // 新的缓存实例
			pool.NewMemoryPool,        // 新的内存池
		),
		fx.Invoke(
			registry.RegisterEventProtocol,  // 注册事件
			registry.RegisterStreamProtocol, // 注册流
			registry.RegisterPubsubProtocol, // 注册订阅
		),
	}
	opts = append(opts, fx.Populate(
		&fs.ctx,
		&fs.opt,
		&fs.p2p,
		&fs.pubsub,
		&fs.db,
		&fs.uploadChan,
		&fs.downloadChan,
		&fs.searchChan,
		&fs.registry,
		&fs.cache,
		&fs.pool,
	))
	app := fx.New(opts...)

	// 启动所有长时间运行的 goroutine，例如网络服务器或消息队列消费者。
	return fs, app.Start(fs.ctx)
}

// checkAndSetOptions 检查并设置选项
func checkAndSetOptions(opt *opts.Options) error {
	return nil
}

// globalInit 全局初始化
func globalInit(fs *FS) fx.Option {
	return fx.Provide(
		// 获取上下文
		func(lc fx.Lifecycle) context.Context {
			lc.Append(fx.Hook{
				OnStop: func(_ context.Context) error {
					return nil
				},
			})
			return fs.ctx
		},
		func() *opts.Options {
			return fs.opt
		},
		func() *dep2p.DeP2P {
			return fs.p2p
		},
		func() *pubsub.DeP2PPubSub {
			return fs.pubsub
		},
		func() *sqlites.SqliteDB {
			return fs.db
		},
		// 初始化上传通道
		func() chan *core.UploadChan {
			return make(chan *core.UploadChan)
		},
		// 初始化下载通道
		func() chan *core.DownloadChan {
			return make(chan *core.DownloadChan)
		},
		// 初始化搜索通道
		func() chan *core.SearchChan {
			return make(chan *core.SearchChan)
		},
	)
}

// initDirectories 确保所有预定义的文件夹都存在
// func initDirectories(opt *opts.Options) error {
// 	// 所有需要检查的目录
// 	directories := []string{
// 		paths.Files,        // 文件目录
// 		paths.Logs,         // 日志目录
// 		paths.UploadPath,   // 上传目录
// 		paths.SlicePath,    // 切片目录
// 		paths.DownloadPath, // 下载目录
// 	}

// 	// 遍历每个目录并确保它存在
// 	for _, dir := range directories {
// 		if err := os.MkdirAll(dir, 0755); err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// DownloadChan 获取用于刷新下载的通道
func (fs *FS) DownloadChan() chan *core.DownloadChan {
	return fs.downloadChan
}

// SearchChan 用于刷新搜索的通道
func (fs *FS) SearchChan() chan *core.SearchChan {
	return fs.searchChan
}

// UploadChan 获取用于刷新上传的通道
func (fs *FS) UploadChan() chan *core.UploadChan {
	return fs.uploadChan
}

// Ctx 获取全局上下文
func (fs *FS) Ctx() context.Context {
	return fs.ctx
}

// Opt 获取文件存储选项配置
func (fs *FS) Opt() *opts.Options {
	return fs.opt
}

// P2P 获取DeP2P网络主机
func (fs *FS) P2P() *dep2p.DeP2P {
	return fs.p2p
}

// Pubsub 获取DeP2P网络订阅
func (fs *FS) Pubsub() *pubsub.DeP2PPubSub {
	return fs.pubsub
}

// DB 获取sqlite数据库服务
func (fs *FS) DB() *sqlites.SqliteDB {
	return fs.db
}

// Registry 获取事件总线
func (fs *FS) Registry() *eventbus.EventRegistry {
	return fs.registry
}

// Cache 获取缓存实例
func (fs *FS) Cache() *ristretto.Cache {
	return fs.cache
}

// Pool 获取内存池
func (fs *FS) Pool() *pool.MemoryPool {
	return fs.pool
}
