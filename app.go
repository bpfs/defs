package defs

import (
	"context"
	"os"

	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/sqlites"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/dgraph-io/ristretto"
	"go.uber.org/fx"
)

// FS提供了与DeFS交互所需的各种函数
type FS struct {
	ctx          context.Context     // 全局上下文
	opt          *Options            // 文件存储选项配置
	p2p          *dep2p.DeP2P        // DeP2P网络主机
	pubsub       *pubsub.DeP2PPubSub // DeP2P网络订阅
	db           *sqlites.SqliteDB   // sqlite数据库服务
	uploadChan   chan *uploadChan    // 用于刷新上传的通道
	downloadChan chan *downloadChan  // 用于刷新下载的通道

	registry *eventbus.EventRegistry // 事件总线
	cache    *ristretto.Cache        // 缓存实例
	pool     *MemoryPool             // 内存池
}

// 检查并设置选项
func checkAndSetOptions(opt *Options) error {
	return nil
}

// 全局初始化
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
		func() *Options {
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
		func() chan *uploadChan {
			return make(chan *uploadChan)
		},
		// 初始化下载通道
		func() chan *downloadChan {
			return make(chan *downloadChan)
		},
	)
}

// Open 返回一个新的文件存储对象
func Open(opt *Options, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub) (*FS, error) {
	// 1. 检查并设置选项
	if err := checkAndSetOptions(opt); err != nil {
		return nil, err
	}
	// 2. 本地文件夹
	if err := initDirectories(); err != nil {
		return nil, err
	}
	// 3. 本地数据库
	db, err := sqlites.NewSqliteDB(BusinessDbPath, sqlites.DbFile)
	if err != nil {
		return nil, err
	}
	// 3.1 数据库表
	if err := db.InitDBTable(); err != nil {
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
			NewEventRegistry,  // 新的事件总线
			NewRistrettoCache, // 新的缓存实例
			NewMemoryPool,     // 新的内存池
		),
		fx.Invoke(
			RegisterEventProtocol,  // 注册事件
			RegisterStreamProtocol, // 注册流
			RegisterPubsubProtocol, // 注册订阅
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
		&fs.registry,
		&fs.cache,
		&fs.pool,
	))
	app := fx.New(opts...)

	// 启动所有长时间运行的 goroutine，例如网络服务器或消息队列消费者。
	return fs, app.Start(fs.ctx)
}

// initDirectories 确保所有预定义的文件夹都存在
func initDirectories() error {
	// 所有需要检查的目录
	directories := []string{
		Files,        // 文件目录
		Logs,         // 日志目录
		UploadPath,   // 上传目录
		SlicePath,    // 切片目录
		DownloadPath, // 下载目录
	}

	// 遍历每个目录并确保它存在
	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
