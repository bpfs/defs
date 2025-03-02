package downloads

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	defsproto "github.com/bpfs/defs/v2/utils/protocol"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/pointsub"
	"github.com/dep2p/pubsub"

	"github.com/dep2p/go-dep2p/core/protocol"
	"go.uber.org/fx"
)

const (
	version      = "1.0.0"           // 协议版本号
	MaxBlockSize = 1024 * 1024 * 100 // 最大块大小，100MB
	ConnTimeout  = 60 * time.Second  // 连接超时时间
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
	usp := &StreamProtocol{
		ctx:          input.Ctx,
		opt:          input.Opt,
		db:           input.DB,
		fs:           input.FS,
		host:         input.Host,
		routingTable: input.RoutingTable,
		nps:          input.NPS,
		dm:           input.DM,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {

			// 创建下载到网络的监听器
			downloadListener, err := pointsub.Listen(input.Host, protocol.ID(StreamRequestSegmentProtocol))
			if err != nil {
				logger.Errorf("创建转发任务监听器失败: %v", err)
				return err
			}

			// 使用 WaitGroup 追踪所有连接处理
			var wg sync.WaitGroup

			// 启动下载任务处理协程
			go func() {
				for {
					conn, err := downloadListener.Accept()
					if err != nil {
						if ctx.Err() != nil {
							wg.Wait() // 等待所有连接处理完成
							return
						}
						logger.Errorf("接受下载任务连接失败: %v", err)
						continue
					}

					wg.Add(1)
					go func(c net.Conn) {
						defer wg.Done()
						handleDownloadConnection(ctx, c, usp)
					}(conn)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 等待一定时间让连接优雅关闭
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	})

}

// SegmentMessage 定义片段消息接口
type SegmentMessage interface {
	defsproto.Message
	GetSegmentId() string
}

// SegmentRequestMessage 请求消息封装
type SegmentRequestMessage struct {
	Request *pb.SegmentContentRequest
}

// 实现 Message 接口
func (m *SegmentRequestMessage) Marshal() ([]byte, error) {
	// 添加日志记录序列化前的数据
	logger.Infof("序列化请求消息: taskID=%s, fileID=%s, segmentID=%s",
		m.Request.TaskId,
		m.Request.FileId,
		m.Request.SegmentId)

	data, err := m.Request.Marshal()
	if err != nil {
		logger.Errorf("序列化请求消息失败: %v", err)
		return nil, err
	}
	logger.Infof("序列化后的数据(hex): %x", data)
	return data, nil
}

func (m *SegmentRequestMessage) Unmarshal(data []byte) error {
	if m.Request == nil {
		m.Request = &pb.SegmentContentRequest{}
	}

	// 添加反序列化前数据的十六进制打印
	logger.Infof("准备反序列化的数据(hex): %x", data)

	// 解析消息包装器
	wrapper := &defsproto.MessageWrapper{}
	if err := wrapper.Unmarshal(data); err != nil {
		logger.Errorf("解析消息包装器失败: %v, 数据(hex): %x", err, data)
		return err
	}

	// 使用包装器中的实际负载
	// logger.Infof("解析后的负载数据(hex): %x", wrapper.Payload)

	// 反序列化实际的消息内容
	err := m.Request.Unmarshal(wrapper.Payload)
	if err != nil {
		logger.Errorf("反序列化请求消息失败: %v, 数据(hex): %x", err, wrapper.Payload)
		return err
	}

	// 添加日志记录反序列化后的数据
	logger.Infof("反序列化请求消息: taskID=%s, fileID=%s, segmentID=%s",
		m.Request.TaskId,
		m.Request.FileId,
		m.Request.SegmentId)

	return nil
}

func (m *SegmentRequestMessage) GetSegmentId() string {
	return m.Request.SegmentId
}

// SegmentResponseMessage 响应消息封装
type SegmentResponseMessage struct {
	Response *pb.SegmentContentResponse
}

// Marshal 实现 Message 接口
func (m *SegmentResponseMessage) Marshal() ([]byte, error) {
	if m.Response == nil {
		return nil, fmt.Errorf("response is nil")
	}
	return m.Response.Marshal()
}

func (m *SegmentResponseMessage) Unmarshal(data []byte) error {
	if m.Response == nil {
		m.Response = &pb.SegmentContentResponse{}
	}

	// 添加调试日志
	logger.Infof("开始反序列化响应消息, 数据长度=%d", len(data))

	// 解析消息包装器
	wrapper := &defsproto.MessageWrapper{}
	if err := wrapper.Unmarshal(data); err != nil {
		logger.Errorf("解析消息包装器失败: %v, 数据(hex): %x", err, data)
		return err
	}

	// 尝试反序列化实际的消息内容
	err := m.Response.Unmarshal(wrapper.Payload)
	if err != nil {
		logger.Errorf("响应消息反序列化失败: %v, 数据前50字节(hex): %x",
			err, wrapper.Payload[:min(50, len(wrapper.Payload))])
		return err
	}

	// 只有在非错误响应时才验证必要字段
	if !m.Response.HasError {
		if m.Response.SegmentId == "" {
			logger.Errorf("响应消息中缺少分片ID")
			return fmt.Errorf("响应消息中缺少分片ID")
		}
	}

	// 添加成功日志
	if m.Response.HasError {
		logger.Infof("响应消息反序列化成功: 错误信息=%s, 错误码=%v",
			m.Response.ErrorMessage,
			m.Response.ErrorCode)
	} else {
		logger.Infof("响应消息反序列化成功: segmentID=%s, dataSize=%d",
			m.Response.SegmentId,
			len(m.Response.SegmentContent))
	}

	return nil
}

func (m *SegmentResponseMessage) GetSegmentId() string {
	if m.Response == nil {
		return ""
	}
	return m.Response.SegmentId
}

// handleDownloadConnection 修改连接处理
func handleDownloadConnection(ctx context.Context, conn net.Conn, usp *StreamProtocol) {
	// 创建协议处理器
	handler := defsproto.NewHandler(conn, &defsproto.Options{
		MaxRetries:     3,
		RetryDelay:     time.Second,
		HeartBeat:      30 * time.Second,
		DialTimeout:    ConnTimeout,
		WriteTimeout:   ConnTimeout * 2, // 增加写超时
		ReadTimeout:    ConnTimeout * 2, // 增加读超时
		ProcessTimeout: ConnTimeout * 2, // 增加处理超时
		MaxConns:       100,
		Rate:           50 * 1024 * 1024, // 50MB/s
		Window:         20 * 1024 * 1024, // 20MB window
		Threshold:      10 * 1024 * 1024, // 10MB threshold
		QueueSize:      1000,
		QueuePolicy:    defsproto.PolicyBlock,
	})
	defer func() {
		handler.StopHeartbeat() // 确保心跳停止
		handler.Close()
	}()

	// 启动心跳
	handler.StartHeartbeat()

	// 启动消息处理
	handler.StartQueueProcessor(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := handleSegmentData(handler, usp); err != nil {
				if err != io.EOF {
					logger.Errorf("处理分片数据失败: %v", err)
				}
				return
			}
		}
	}
}

// handleSegmentData 使用协议处理器处理数据
func handleSegmentData(handler *defsproto.Handler, usp *StreamProtocol) error {
	// 接收请求消息
	reqMsg := &SegmentRequestMessage{
		Request: &pb.SegmentContentRequest{},
	}

	if err := handler.ReceiveMessage(reqMsg); err != nil {
		// 如果是EOF，属于正常连接关闭
		if err == io.EOF {
			return err
		}
		logger.Errorf("接收请求消息失败: %v", err)
		return sendErrorResponse(handler, pb.SegmentError_SEGMENT_ERROR_NETWORK, "接收请求消息失败")
	}

	// 验证接收到的消息
	if reqMsg.Request == nil {
		logger.Errorf("请求消息为空")
		return sendErrorResponse(handler, pb.SegmentError_SEGMENT_ERROR_INVALID_REQUEST, "请求消息为空")
	}

	// 验证载荷
	if err := validatePayload(reqMsg.Request, usp); err != nil {
		logger.Errorf("验证载荷失败: taskID=%s, fileID=%s, segmentID=%s, error=%v",
			reqMsg.Request.TaskId,
			reqMsg.Request.FileId,
			reqMsg.Request.SegmentId,
			err)

		// 根据具体错误类型返回对应的错误码
		var errCode pb.SegmentError
		switch {
		case reqMsg.Request.FileId == "":
			errCode = pb.SegmentError_SEGMENT_ERROR_INVALID_FILEID
		case reqMsg.Request.SegmentId == "":
			errCode = pb.SegmentError_SEGMENT_ERROR_INVALID_SEGMENTID
		case reqMsg.Request.TaskId == "":
			errCode = pb.SegmentError_SEGMENT_ERROR_INVALID_TASKID
		case reqMsg.Request.PubkeyHash == nil:
			errCode = pb.SegmentError_SEGMENT_ERROR_INVALID_REQUEST
			err = fmt.Errorf("公钥哈希为空")
		default:
			errCode = pb.SegmentError_SEGMENT_ERROR_INVALID_REQUEST
		}
		return sendErrorResponse(handler, errCode, err.Error())
	}

	// 获取片段数据
	segmentData, err := GetSegmentStorageData(usp.db, usp.host.ID().String(),
		reqMsg.Request.TaskId, reqMsg.Request.FileId, reqMsg.Request.SegmentId)
	if err != nil {
		logger.Errorf("获取片段数据失败: taskID=%s, fileID=%s, segmentID=%s, error=%v",
			reqMsg.Request.TaskId,
			reqMsg.Request.FileId,
			reqMsg.Request.SegmentId,
			err)

		var errCode pb.SegmentError
		if os.IsNotExist(err) {
			errCode = pb.SegmentError_SEGMENT_ERROR_SEGMENT_NOT_FOUND
		} else if os.IsPermission(err) {
			errCode = pb.SegmentError_SEGMENT_ERROR_FILE_PERMISSION
		} else {
			errCode = pb.SegmentError_SEGMENT_ERROR_SYSTEM
			// 错误信息中包含具体原因，接收方会根据消息内容处理
		}
		return sendErrorResponse(handler, errCode, err.Error())
	}

	// 验证返回的数据完整性
	if segmentData == nil {
		logger.Errorf("获取到的片段数据为空")
		return sendErrorResponse(handler, pb.SegmentError_SEGMENT_ERROR_SEGMENT_CORRUPTED, "片段数据为空")
	}

	// 发送响应
	respMsg := &SegmentResponseMessage{
		Response: segmentData,
	}

	if err := handler.SendMessage(respMsg); err != nil {
		logger.Errorf("发送响应失败: %v", err)
		return sendErrorResponse(handler, pb.SegmentError_SEGMENT_ERROR_NETWORK, "发送响应失败")
	}
	return nil
}

// sendErrorResponse 使用协议发送错误响应
func sendErrorResponse(handler *defsproto.Handler, errCode pb.SegmentError, errMsg string) error {
	resp := &SegmentResponseMessage{
		Response: &pb.SegmentContentResponse{
			HasError:     true,
			ErrorMessage: errMsg,
			ErrorCode:    errCode,
		},
	}
	return handler.SendMessage(resp)
}

// validatePayload 验证载荷数据
// 参数:
//   - payload: *pb.SegmentContentRequest 载荷
//   - usp: *StreamProtocol 流协议实例
//
// 返回值:
//   - error: 如果在验证过程中发生错误，返回相应的错误信息
func validatePayload(payload *pb.SegmentContentRequest, usp *StreamProtocol) error {
	if usp.opt == nil || usp.fs == nil {
		logger.Errorf("系统配置无效")
		return fmt.Errorf("系统配置无效")
	}

	if payload == nil {
		logger.Errorf("载荷为空")
		return fmt.Errorf("载荷为空")
	}

	if payload.FileId == "" {
		logger.Errorf("文件ID为空")
		return fmt.Errorf("文件ID为空")
	}

	if payload.SegmentId == "" {
		logger.Errorf("分片ID为空")
		return fmt.Errorf("分片ID为空")
	}

	return nil
}
