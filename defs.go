// Package defs 提供了DeFS去中心化存储系统的核心定义和接口
package defs

import (
	"context"
	"os"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/downloads"
	"github.com/bpfs/defs/files"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/kbucket"
	"github.com/bpfs/defs/net"

	"github.com/bpfs/defs/uploads"
	"github.com/bpfs/defs/utils/logger"
	"github.com/bpfs/defs/utils/paths"
	"github.com/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"
)

// NoOpThreshold 表示无操作的时间阈值，设定为 100 小时
var NoOpThreshold = 100 * time.Hour

// DeFS 是一个封装了DeFS去中心化(动态)存储的结构体
type DeFS struct {
	ctx  context.Context // 全局上下文，用于管理整个应用的生命周期和取消操作
	opt  *fscfg.Options  // 文件存储选项配置，包含各种系统设置和参数
	db   *database.DB    // 持久化存储，用于本地数据的存储和检索
	fs   afero.Afero     // 文件系统接口，提供跨平台的文件操作能力
	host host.Host       // libp2p网络主机实例
	// server       *pointsub.Server           // 处理请求的服务端
	// client       *pointsub.Client           // 用于发送请求的客户端
	routingTable *kbucket.RoutingTable      // 路由表，用于管理对等节点和路由
	nps          *pubsub.NodePubSub         // 发布订阅系统，用于节点之间的消息传递
	upload       *uploads.UploadManager     // 上传管理器，用于处理和管理文件上传任务
	download     *downloads.DownloadManager // 下载管理器，用于处理和管理文件下载任务

}

// Open 创建并返回一个新的DeFS实例
// 参数:
//   - h: libp2p网络主机实例
//   - options: 可选的配置选项列表
//
// 返回值:
//   - *DeFS: 新创建的DeFS实例
//   - error: 如果创建过程中发生错误则返回错误信息
//
// 示例:
//
// defs, err := Open(h,
//
//	fscfg.WithBucketSize(200),// 设置桶大小为 200
//	fscfg.WithMaxPeersPerCpl(10),// 每个 CPL 最多允许 10 个节点
//
//	// PubSub 相关配置
//	fscfg.WithPubSubOption(pubsub.WithSetFollowupTime(1 * time.Second)),
//	fscfg.WithPubSubOption(pubsub.WithSetGossipFactor(0.3)),
//	fscfg.WithPubSubOption(pubsub.WithSetMaxMessageSize(2 << 20)),
//	fscfg.WithPubSubOption(pubsub.WithNodeDiscovery(d)),
//
// )
//
//	if err != nil {
//		logger.Errorf("创建DeFS实例失败: %v", err)
//		return nil, err
//	}
func Open(h host.Host, options ...fscfg.Option) (*DeFS, error) {
	ctx := context.Background() // 创建全局上下文
	os.Setenv("TZ", "UTC")      // 设置时区为UTC

	// 创建默认配置选项
	opt := fscfg.DefaultOptions()

	// 应用所有提供的配置选项
	if err := opt.ApplyOptions(options...); err != nil {
		logger.Errorf("应用选项失败: %v", err)
		return nil, err
	}

	// 初始化所有必要的路径
	if err := paths.InitializePaths(
		paths.NewPathOptions(
			opt.GetRootPath(),     // 获取根路径
			opt.GetDownloadPath(), // 获取下载路径
		),
	); err != nil {
		logger.Errorf("初始化路径失败: %v", err)
		return nil, err
	}

	// 初始化服务端
	// server, err := pointsub.NewServer(h, pointsub.DefaultServerConfig())
	// if err != nil {
	// 	logger.Errorf("创建服务端失败: %v", err)
	// 	return nil, err
	// }

	// 初始化客户端
	// client, err := pointsub.NewClient(h, pointsub.DefaultClientConfig())
	// if err != nil {
	// 	logger.Errorf("创建客户端失败: %v", err)
	// 	return nil, err
	// }

	// 创建路由表
	rt, err := kbucket.CreateRoutingTable(h, opt, NoOpThreshold)
	if err != nil {
		logger.Errorf("创建路由表失败: %v", err)
		return nil, err
	}

	// 获取或创建发布订阅系统
	var nps *pubsub.NodePubSub
	if opt.GetNodePubSub() != nil {
		// 如果已配置则直接使用
		nps = opt.GetNodePubSub()
	} else {
		// 否则创建新的实例
		var err error
		nps, err = pubsub.NewNodePubSub(ctx, h, opt.GetPubSubOptions()...)
		if err != nil {
			logger.Errorf("创建节点发布订阅系统失败: %v", err)
			return nil, err
		}
	}

	// 创建DeFS实例
	defs := &DeFS{
		ctx:  ctx,
		opt:  opt,
		db:   nil,
		fs:   nil,
		host: h,
		// server:       server,
		// client:       client,
		routingTable: rt,
		nps:          nps,
		upload:       nil,
		download:     nil,
	}

	// 配置fx依赖注入选项
	opts := []fx.Option{
		globalInit(defs), // 全局初始化，提供基本依赖
		fx.Provide(
			database.NewDB,               // 创建数据库实例
			files.NewAferoFs,             // 创建文件系统实例
			uploads.NewUploadManager,     // 创建上传管理器实例
			downloads.NewDownloadManager, // 创建下载管理器实例

		),
		fx.Invoke(
			uploads.InitializeUploadManager,          // 初始化上传管理器
			uploads.RegisterUploadStreamProtocol,     // 注册上传流协议
			uploads.RegisterUploadPubsubProtocol,     // 注册上传PubSub协议
			downloads.InitializeDownloadManager,      // 初始化下载管理器
			downloads.RegisterDownloadPubsubProtocol, // 注册下载PubSub协议
			downloads.RegisterDownloadStreamProtocol, // 注册下载流协议
			database.InitDBTable,                     // 初始化数据库表
			net.RegisterHandshakeProtocol,            // 注册握手协议的处理函数
		),
	}

	// 填充DeFS实例的依赖项
	opts = append(opts, fx.Populate(
		&defs.ctx,
		&defs.opt,
		&defs.db,
		&defs.fs,
		&defs.host,
		// &defs.server,
		// &defs.client,
		&defs.routingTable,
		&defs.nps,
		&defs.upload,
		&defs.download,
	))

	// 创建并启动fx应用
	app := fx.New(opts...)
	return defs, app.Start(defs.ctx)
}

// globalInit 执行全局初始化配置
// 参数:
//   - defs: DeFS实例指针
//
// 返回值:
//   - fx.Option: fx依赖注入选项
func globalInit(defs *DeFS) fx.Option {
	return fx.Provide(
		// 提供上下文
		func(lc fx.Lifecycle) context.Context {
			lc.Append(fx.Hook{
				OnStop: func(_ context.Context) error {
					return nil
				},
			})
			return defs.ctx
		},
		// 提供配置选项
		func() *fscfg.Options {
			return defs.opt
		},
		// 提供网络主机
		func() host.Host {
			return defs.host
		},
		// 提供服务端
		// func() *pointsub.Server {
		// 	return defs.server
		// },
		// 提供客户端
		// func() *pointsub.Client {
		// 	return defs.client
		// },
		// 提供路由表
		func() *kbucket.RoutingTable {
			return defs.routingTable
		},
		// 提供发布订阅
		func() *pubsub.NodePubSub {
			return defs.nps
		},
	)
}

// Ctx 获取全局上下文
// 返回值:
//   - context.Context: 全局上下文实例
func (fs *DeFS) Ctx() context.Context {
	return fs.ctx
}

// Opt 获取配置选项
// 返回值:
//   - *fscfg.Options: 配置选项实例
func (fs *DeFS) Opt() *fscfg.Options {
	return fs.opt
}

// DB 获取数据库实例
// 返回值:
//   - *database.DB: 数据库实例
func (fs *DeFS) DB() *database.DB {
	return fs.db
}

// Afero 获取文件系统实例
// 返回值:
//   - afero.Afero: 文件系统实例
func (fs *DeFS) Afero() afero.Afero {
	return fs.fs
}

// Host 获取网络主机实例
// 返回值:
//   - host.Host: 网络主机实例
func (fs *DeFS) Host() host.Host {
	return fs.host
}

// Server 获取服务端实例
// 返回值:
//   - *pointsub.Server: 服务端实例
// func (fs *DeFS) Server() *pointsub.Server {
// 	return fs.server
// }

// Client 获取客户端实例
// 返回值:
//   - *pointsub.Client: 客户端实例
// func (fs *DeFS) Client() *pointsub.Client {
// 	return fs.client
// }

// RoutingTable 获取客户端实例
// 返回值:
//   - *kbucket.RoutingTable : 路由表实例
func (fs *DeFS) RoutingTable() *kbucket.RoutingTable {
	return fs.routingTable
}

// NodePubSub 返回发布订阅系统
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统
func (fs *DeFS) NodePubSub() *pubsub.NodePubSub {
	return fs.nps
}

// Upload 获取上传管理器实例
// 返回值:
//   - *uploads.UploadManager: 上传管理器实例
func (fs *DeFS) Upload() *uploads.UploadManager {
	return fs.upload
}

// Download 获取下载管理器实例
// 返回值:
//   - *downloads.DownloadManager: 下载管理器实例
func (fs *DeFS) Download() *downloads.DownloadManager {
	return fs.download
}

// AddRoutingPeer 尝试将对等节点添加到路由表
// 参数:
//   - pi: 要添加的对等节点地址信息
//   - mode: 运行模式 (建议:0:客户端 1:服务端,可根据实际需求设置)
//
// 返回值:
//   - bool: 如果对等节点成功添加则返回 true
//   - error: 如果添加失败则返回错误
//
// 说明:
//   - 所有添加的节点都被视为非查询节点(queryPeer=false)
//   - 所有添加的节点都被设置为可替换(isReplaceable=true)
//   - 这样设计可以保证路由表的动态性和新鲜度
func (fs *DeFS) AddRoutingPeer(pi peer.AddrInfo, mode int) (bool, error) {
	// 尝试将节点添加到路由表
	// 固定设置 queryPeer=false (非查询节点)
	// 固定设置 isReplaceable=true (可替换节点)
	success, err := fs.RoutingTable().AddPeer(fs.ctx, fs.host, pi, mode, false, true)
	if err != nil {
		logger.Errorf("添加节点到路由表失败: %v", err)
		return false, err
	}

	// 通知发布订阅系统有新节点加入
	if err = fs.NodePubSub().Pubsub().NotifyNewPeer(pi.ID); err != nil {
		logger.Errorf("通知发布订阅系统新节点失败: %v", err)
		return false, err
	}

	if success {
		logger.Debugf("成功添加节点 %s", pi.ID)
	}
	return success, nil
}

// AddPubSubPeer 添加节点到发布订阅系统
// 参数:
//   - peer: 要添加的对等节点ID
//
// 返回值:
//   - error: 如果添加失败则返回错误
func (fs *DeFS) AddPubSubPeer(peer peer.ID) error {
	// 通知发布订阅系统有新节点加入
	if err := fs.NodePubSub().Pubsub().NotifyNewPeer(peer); err != nil {
		logger.Errorf("通知发布订阅系统新节点失败: %v", err)
		return err
	}

	logger.Debugf("成功添加发布订阅节点 %s", peer)
	return nil
}
