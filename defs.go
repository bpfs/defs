// Package defs 提供了DeFS去中心化存储系统的核心定义和接口
package defs

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/downloads"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/net"
	"github.com/bpfs/defs/v2/operate/shared"
	"github.com/dep2p/go-dep2p/core/peerstore"

	"github.com/bpfs/defs/v2/uploads"
	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	logging "github.com/dep2p/log"
	"github.com/dep2p/pointsub"
	"github.com/dep2p/pubsub"
	"go.uber.org/fx"
)

var logger = logging.Logger("defs")

// NoOpThreshold 表示无操作的时间阈值，设定为 100 小时
var NoOpThreshold = 100 * time.Hour

// DeFS 是一个封装了DeFS去中心化(动态)存储的结构体
type DeFS struct {
	ctx      context.Context            // 全局上下文
	opt      *fscfg.Options             // 配置选项
	db       *database.DB               // 数据库实例
	fs       afero.Afero                // 文件系统实例
	host     host.Host                  // libp2p网络主机实例
	ps       *pointsub.PointSub         // 点对点传输实例
	nps      *pubsub.NodePubSub         // 发布订阅系统
	upload   *uploads.UploadManager     // 上传管理器
	download *downloads.DownloadManager // 下载管理器
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

	// 创建路由表
	// rt, err := kbucket.CreateRoutingTable(h, opt, NoOpThreshold)
	// if err != nil {
	// 	logger.Errorf("创建路由表失败: %v", err)
	// 	return nil, err
	// }

	// 创建DeFS实例
	defs := &DeFS{
		ctx:  ctx,
		opt:  opt,
		db:   nil,
		fs:   nil,
		host: h,
		ps:   nil,
		// routingTable: rt,
		nps:      nil,
		upload:   nil,
		download: nil,
	}

	// 配置fx依赖注入选项
	opts := []fx.Option{
		globalInit(defs), // 全局初始化，提供基本依赖
		fx.Provide(
			database.NewDB,               // 创建数据库实例
			files.NewAferoFs,             // 创建文件系统实例
			uploads.NewUploadManager,     // 创建上传管理器实例
			downloads.NewDownloadManager, // 创建下载管理器实例
			uploads.NewPointSub,          // 创建点对点传输实例
			uploads.NewNodePubSub,        // 创建发布订阅系统实例
		),
		fx.Invoke(
			uploads.InitializeUploadManager,            // 初始化上传管理器
			uploads.RegisterUploadPubsubProtocol,       // 注册上传PubSub协议
			uploads.RegisterUploadPointSubProtocol,     // 注册上传PointSub协议
			downloads.InitializeDownloadManager,        // 初始化下载管理器
			downloads.RegisterDownloadPubsubProtocol,   // 注册下载PubSub协议
			downloads.RegisterDownloadPointSubProtocol, // 注册下载PointSub协议
			//downloads.RegisterDownloadStreamProtocol,   // 注册下载流协议
			database.InitializeDatabase,         // 初始化数据库维护任务，包括定期GC和备份
			net.RegisterHandshakeProtocol,       // 注册握手协议的处理函数
			shared.RegisterSharedPubsubProtocol, // 注册文件共享相关的PubSub协议处理器
		),
	}

	// 填充DeFS实例的依赖项
	opts = append(opts, fx.Populate(
		&defs.ctx,
		&defs.opt,
		&defs.db,
		&defs.fs,
		&defs.host,
		&defs.ps,
		// &defs.routingTable,
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
		// 提供路由表
		// func() *kbucket.RoutingTable {
		// 	return defs.routingTable
		// },
		// 提供发布订阅
		// 发布订阅系统现在由 uploads.NewNodePubSub 提供
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

// PointSub 获取 PointSub 实例
// 返回值:
//   - *pointsub.PointSub: PointSub 实例
func (fs *DeFS) PointSub() *pointsub.PointSub {
	return fs.ps
}

// RoutingTable 获取客户端实例
// 返回值:
//   - *kbucket.RoutingTable : 路由表实例
// func (fs *DeFS) RoutingTable() *kbucket.RoutingTable {
// 	return fs.routingTable
// }

// NodePubSub 返回发布订阅系统
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统
func (fs *DeFS) NodePubSub() *pubsub.NodePubSub {
	if fs.nps == nil {
		logger.Warn("NodePubSub 实例尚未初始化")
	}
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
//   - 将节点添加到 PointSub 的服务节点列表中
//   - 同时通知发布订阅系统有新节点加入
/*
//   - 所有添加的节点都被视为非查询节点(queryPeer=false)
//   - 所有添加的节点都被设置为可替换(isReplaceable=true)
//   - 这样设计可以保证路由表的动态性和新鲜度
*/
func (fs *DeFS) AddRoutingPeer(pi peer.AddrInfo, mode int) (bool, error) {
	/**
	// 尝试将节点添加到路由表
	// 固定设置 queryPeer=false (非查询节点)
	// 固定设置 isReplaceable=true (可替换节点)
	success, err := fs.RoutingTable().AddPeer(fs.ctx, fs.host, pi, mode, false, true)
	if err != nil {
		logger.Errorf("添加节点到路由表失败: %v", err)
	}
	*/

	// 检查 PointSub 实例是否已初始化
	if fs.ps == nil {
		logger.Error("PointSub 实例尚未初始化")
		return false, fmt.Errorf("PointSub 实例尚未初始化")
	}

	// 获取 PointSub 客户端
	client := fs.ps.Client()
	if client == nil {
		logger.Error("PointSub 客户端未启动")
		return false, fmt.Errorf("PointSub 客户端未启动")
	}
	// 将节点信息永久保存到peerstore
	fs.host.Peerstore().AddAddrs(
		pi.ID,
		pi.Addrs,
		peerstore.PermanentAddrTTL,
	)
	// 尝试连接对方节点
	if err := fs.host.Connect(context.Background(), pi); err != nil {
		logger.Errorf("连接是比啊%v", err)
		return false, err
	}

	// 为两个协议添加服务节点
	if err := client.AddServerNode(protocol.ID(uploads.SendingToNetworkProtocol), pi.ID); err != nil {
		logger.Errorf("添加发送协议节点失败: %v", err)
		return false, err
	}
	if err := client.AddServerNode(protocol.ID(uploads.ForwardToNetworkProtocol), pi.ID); err != nil {
		logger.Errorf("添加转发协议节点失败: %v", err)
		return false, err
	}

	// 通知发布订阅系统有新节点加入
	if err := fs.NodePubSub().Pubsub().NotifyNewPeer(pi.ID); err != nil {
		logger.Errorf("通知发布订阅系统新节点失败: %v", err)
		return false, err
	}

	logger.Debugf("成功添加节点 %s", pi.ID)
	return true, nil
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
