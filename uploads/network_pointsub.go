package uploads

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/pointsub"
	"go.uber.org/fx"
)

var version = "1.0.0"

// 定义协议名称
var (
	// 发送任务到网络的协议标识符
	SendingToNetworkProtocol = fmt.Sprintf("defs@stream/sending/network/%s", version)
	// 转发任务到网络的协议标识符
	ForwardToNetworkProtocol = fmt.Sprintf("defs@stream/forward/network/%s", version)
)

// NewPointSubParams 定义创建 PointSub 所需的参数
type NewPointSubParams struct {
	fx.In

	Host host.Host      // libp2p网络主机实例
	Opt  *fscfg.Options // 配置选项
}

// NewPointSubResult 定义 NewPointSub 的返回结果
type NewPointSubResult struct {
	fx.Out

	PS *pointsub.PointSub // PointSub 实例
}

// NewPointSub 创建并初始化 PointSub 实例
// 参数:
//   - params: NewPointSubParams 类型，包含创建所需的依赖项
//
// 返回值:
//   - NewPointSubResult: 包含创建的 PointSub 实例的结果结构
//   - error: 如果创建过程中出现错误，返回相应的错误信息
func NewPointSub(params NewPointSubParams) (out NewPointSubResult, err error) {
	// 客户端选项
	clientOpts := []pointsub.ClientOption{
		pointsub.WithReadTimeout(params.Opt.GetPointSubClientReadTimeout()),       // 客户端读取超时
		pointsub.WithWriteTimeout(params.Opt.GetPointSubClientWriteTimeout()),     // 客户端写入超时
		pointsub.WithConnectTimeout(params.Opt.GetPointSubClientConnectTimeout()), // 客户端连接超时
		pointsub.WithMaxRetries(params.Opt.GetPointSubClientMaxRetries()),         // 客户端最大重试次数
		pointsub.WithCompression(params.Opt.GetPointSubClientCompression()),       // 客户端压缩
	}

	// 服务端选项
	serverOpts := []pointsub.ServerOption{
		pointsub.WithMaxConcurrentConns(params.Opt.GetPointSubServerMaxConns()),           // 服务端最大并发连接数
		pointsub.WithServerReadTimeout(params.Opt.GetPointSubServerReadTimeout()),         // 服务端读取超时
		pointsub.WithServerWriteTimeout(params.Opt.GetPointSubServerWriteTimeout()),       // 服务端写入超时
		pointsub.WithServerBufferPoolSize(params.Opt.GetPointSubServerBufferSize()),       // 服务端缓冲池大小
		pointsub.WithServerCleanupInterval(params.Opt.GetPointSubServerCleanupInterval()), // 服务端清理间隔
	}

	// 创建 PointSub 实例
	ps, err := pointsub.NewPointSub(params.Host, clientOpts, serverOpts, params.Opt.GetPointSubEnableServer())
	if err != nil {
		logger.Errorf("创建 PointSub 失败: %v", err)
		return out, err
	}

	out.PS = ps
	return out, nil
}

// RegisterUploadPointSubProtocolParams 注册上传PointSub协议所需的参数
type RegisterUploadPointSubProtocolParams struct {
	fx.In

	Lifecycle fx.Lifecycle       // 生命周期管理
	Ctx       context.Context    // 全局上下文
	Opt       *fscfg.Options     // 配置选项
	Host      host.Host          // libp2p网络主机实例
	PS        *pointsub.PointSub // 发布订阅系统
	UM        *UploadManager     // 上传管理器
	DB        *database.DB       // 持久化存储
}

// RegisterUploadPointSubProtocol 注册上传的PointSub协议处理器
// 参数:
//   - params: RegisterUploadPointSubProtocolParams 类型，包含注册所需的所有依赖项
//
// 返回值:
//   - error: 如果注册过程中出现错误，返回相应的错误信息
func RegisterUploadPointSubProtocol(params RegisterUploadPointSubProtocolParams) error {
	if params.PS == nil {
		return fmt.Errorf("PointSub 实例不能为空")
	}
	// 如果没有启动服务端这里直接获取肯定报错
	// 获取服务端实例
	// server := params.PS.Server()
	// if server == nil {
	// 	return fmt.Errorf("PointSub 服务端未启动")
	// }

	params.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("正在注册上传 PointSub 协议...")
			// 如果配置了启动服务端再去获取服务端以及去操作其他操作
			if params.Opt.GetPointSubEnableServer() {
				server := params.PS.Server()
				if server == nil {
					return fmt.Errorf("PointSub 服务端未启动")
				}
				// 注册发送和转发处理函数
				server.Start(protocol.ID(SendingToNetworkProtocol), handleSendingToNetwork(params))
				server.Start(protocol.ID(ForwardToNetworkProtocol), handleForwardToNetwork(params))
			}

			logger.Info("上传 PointSub 协议注册成功")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("正在停止上传 PointSub 协议...")
			// 如果配置了启动服务端再去获取服务端以及去操作其他操作
			if params.Opt.GetPointSubEnableServer() {
				server := params.PS.Server()
				if server == nil {
					return fmt.Errorf("PointSub 服务端未启动")
				}
				server.Stop()
			}

			logger.Info("上传 PointSub 协议已停止")
			return nil
		},
	})

	return nil
}
