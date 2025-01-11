package downloads

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/streams"
	"github.com/dep2p/pubsub"

	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/libp2p/core/protocol"
	"go.uber.org/fx"
)

const (
	version = "1.0.0" // 协议版本号
)

var (
	// StreamRequestSegmentProtocol 定义了请求数据片段的协议标识符
	StreamRequestSegmentProtocol = fmt.Sprintf("defs@stream/request/segment/%s", version)
	// StreamSendSegmentProtocol 定义了发送数据片段的协议标识符
	// StreamSendSegmentProtocol = fmt.Sprintf("defs@stream/send/segment/%s", version)
)

// StreamProtocol 定义了流协议的结构体
type StreamProtocol struct {
	ctx          context.Context       // 全局上下文，用于管理整个应用的生命周期和取消操作
	opt          *fscfg.Options        // 文件存储选项配置，包含各种系统设置和参数
	db           *database.DB          // 持久化存储，用于本地数据的存储和检索
	fs           afero.Afero           // 文件系统接口，提供跨平台的文件操作能力
	host         host.Host             // libp2p网络主机实例
	routingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	nps          *pubsub.NodePubSub    // 发布订阅系统，用于节点之间的消息传递
	dm           *DownloadManager      // 下载管理器，用于处理和管理文件下载任务
}

// RegisterStreamProtocolInput 定义了注册流协议所需的输入参数
type RegisterStreamProtocolInput struct {
	fx.In

	Ctx          context.Context       // 全局上下文，用于管理整个应用的生命周期和取消操作
	Opt          *fscfg.Options        // 文件存储选项配置，包含各种系统设置和参数
	DB           *database.DB          // 持久化存储，用于本地数据的存储和检索
	FS           afero.Afero           // 文件系统接口，提供跨平台的文件操作能力
	Host         host.Host             // libp2p网络主机实例
	RoutingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	NPS          *pubsub.NodePubSub    // 发布订阅系统，用于节点之间的消息传递
	DM           *DownloadManager      // 下载管理器，用于处理和管理文件下载任务
}

// RegisterDownloadStreamProtocol 注册下载流协议
// 参数:
//   - lc: 生命周期管理器
//   - input: 注册所需的依赖项
//
// 返回值: error - 注册过程中的错误信息
//
// 功能:
//   - 创建流协议实例
//   - 注册请求和发送数据片段的处理器
//   - 管理协议的生命周期
func RegisterDownloadStreamProtocol(lc fx.Lifecycle, input RegisterStreamProtocolInput) {
	// 创建流协议实例
	dsp := &StreamProtocol{
		ctx:          input.Ctx,
		opt:          input.Opt,
		db:           input.DB,
		fs:           input.FS,
		host:         input.Host,
		routingTable: input.RoutingTable,
		nps:          input.NPS,
		dm:           input.DM,
	}

	// 添加生命周期钩子
	lc.Append(fx.Hook{
		// OnStart 钩子在应用启动时执行
		OnStart: func(ctx context.Context) error {
			// 注册请求数据片段的处理器
			streams.RegisterStreamHandler(input.Host, protocol.ID(StreamRequestSegmentProtocol), streams.HandlerWithRW(dsp.handleRequestSegment))
			// 注册发送数据片段的处理器
			// streams.RegisterStreamHandler(input.P2P.Host(), protocol.ID(StreamSendSegmentProtocol), streams.HandlerWithRW(dsp.handleSendSegment))
			return nil
		},
		// OnStop 钩子在应用停止时执行
		OnStop: func(ctx context.Context) error {
			// 清理资源等停止逻辑
			return nil
		},
	})
}

// handleRequestSegment 处理请求数据片段的请求
// 参数:
//   - req: 包含请求的详细信息
//   - res: 用于设置响应内容
//
// 返回值:
//   - int32 - 状态码，200表示成功，其他表示失败
//   - string - 状态消息，成功或错误描述
//
// 功能:
//   - 解析请求载荷
//   - 获取片段存储数据
//   - 序列化响应数据
//   - 返回处理结果
func (sp *StreamProtocol) handleRequestSegment(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	// 创建一个新的 ContinuousDownloadRequest 对象用于解析请求载荷
	payload := new(pb.SegmentContentRequest)
	// 尝试解析请求载荷，如果失败则返回错误
	if err := payload.Unmarshal(req.Payload); err != nil {
		// 记录解码错误日志
		logger.Errorf("解码错误: %v", err)
		// 返回错误状态码和消息
		return 6603, "解码错误"
	}

	// 记录收到片段请求的日志
	logger.Infof("收到片段请求:  TaskID= %s, FileID= %s, SegmentIndex= %d, SegmentId= %s", payload.FileId, payload.TaskId, payload.SegmentIndex, payload.SegmentId)

	// 获取片段存储数据
	response, err := GetSegmentStorageData(sp.db, sp.host.ID().String(),
		payload.TaskId, payload.FileId, payload.SegmentId)
	if err != nil {
		logger.Errorf("获取片段数据失败: %v", err)
		return 500, err.Error()
	}

	// 序列化响应载荷
	responseData, err := response.Marshal()
	// 如果序列化失败，记录错误日志并返回
	if err != nil {
		// 记录序列化响应数据失败的错误日志
		logger.Errorf("序列化响应数据失败: %v", err)
		// 返回错误状态码和消息
		return 500, "序列化响应数据失败"
	}

	// 设置响应内容
	res.Data = responseData

	// 记录成功发送片段的日志
	logger.Infof("成功发送片段: FileID=%s, SegmentIndex=%d, 响应大小=%d字节",
		payload.FileId, response.SegmentIndex, len(responseData))
	// 返回成功状态码和消息
	return 200, "成功"
}

// handleSendSegment 处理发送数据片段的请求
// 参数:
//   - req: 包含请求的详细信息
//   - res: 用于设置响应内容
//
// 返回值:
//   - int32 - 状态码，200表示成功，其他表示失败
//   - string - 状态消息，成功或错误描述
//
// 功能:
//   - 解析请求载荷
//   - 保存片段数据
//   - 更新下载进度
//   - 返回处理结果
// func (sp *StreamProtocol) handleSendSegment(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
// // 解析请求载荷
// payload := new(pb.SegmentResponse)
// if err := payload.Unmarshal(req.Payload); err != nil {
// 	logger.Errorf("解码错误: %v", err)
// 	return 6603, "解码错误"
// }

// // 打印接收信息
// logger.Printf("接收到片段数据: FileID=%s, SegmentID=%s", payload.FileId, payload.SegmentId)

// // 将接收到的片段数据保存到本地
// err := sp.dm.SaveSegmentData(payload.FileId, payload.SegmentId, payload.Data)
// if err != nil {
// 	logger.Errorf("保存片段数据失败: %v", err)
// 	return 500, "保存片段数据失败"
// }

// // 更新下载任务状态
// if err := sp.dm.UpdateDownloadProgress(payload.FileId, payload.SegmentId); err != nil {
// 	logger.Errorf("更新下载进度失败: %v", err)
// 	return 500, "更新下载进度失败"
// }

// 	return 200, "成功"
// }
