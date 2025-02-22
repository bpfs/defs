package downloads

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/dep2p/pointsub"
	"github.com/dep2p/pubsub"

	"github.com/dep2p/go-dep2p/core/host"
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

// handleDownloadConnection 下载业务处理
// 参数:
//   - ctx: 上下文
//   - conn: 连接
//   - usp: 流协议实例
func handleDownloadConnection(ctx context.Context, conn net.Conn, usp *StreamProtocol) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 每次处理前重置超时
			conn.SetDeadline(time.Now().Add(ConnTimeout))

			if err := handleSegmentData(reader, writer, usp); err != nil {
				if err != io.EOF {
					logger.Errorf("处理分片数据失败: %v", err)
				}
				return
			}
		}
	}
}

// handleSegmentData 处理分片数据的通用函数
// 参数:
//   - reader: *bufio.Reader 读取器，用于读取数据
//   - writer: *bufio.Writer 写入器，用于写入数据
//   - usp: *StreamProtocol 流协议实例
//
// 返回值:
//   - error: 如果在处理过程中发生错误，返回相应的错误信息
func handleSegmentData(reader *bufio.Reader, writer *bufio.Writer, usp *StreamProtocol) error {
	// 创建一个可重用的长度缓冲区
	lenBuf := make([]byte, 4)

	// 读取请求长度
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		if err == io.EOF {
			return io.EOF
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			logger.Warnf("读取消息超时，将重试: %v", err)
			return nil
		}
		logger.Errorf("读取消息长度失败: %v", err)
		sendErrorResponse(writer, "读取消息失败")
		return err
	}

	// 解析请求长度
	msgLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])

	// 验证消息长度
	if msgLen <= 0 || msgLen > MaxBlockSize {
		logger.Errorf("无效的消息长度: %d", msgLen)
		sendErrorResponse(writer, fmt.Sprintf("无效的消息长度: %d", msgLen))
		return fmt.Errorf("无效的消息长度: %d", msgLen)
	}

	// 读取请求内容
	msgBuf := make([]byte, msgLen)
	if _, err := io.ReadFull(reader, msgBuf); err != nil {
		if err == io.EOF {
			return io.EOF
		}
		logger.Errorf("读取消息内容失败: %v", err)
		sendErrorResponse(writer, "读取消息失败")
		return err
	}

	// 解析载荷
	payload := new(pb.SegmentContentRequest)
	if err := payload.Unmarshal(msgBuf); err != nil {
		logger.Errorf("解码请求载荷失败: %v", err)
		sendErrorResponse(writer, "解码失败")
		return err
	}

	// 验证载荷
	if err := validatePayload(payload, usp); err != nil {
		logger.Errorf("验证载荷失败: %v", err)
		sendErrorResponse(writer, err.Error())
		return err
	}
	// TODO:这里应该获取判断内容组成一个结构体返回，然后发送方接收？？？

	// 获取片段存储数据
	segmentStorageData, err := GetSegmentStorageData(usp.db, usp.host.ID().String(),
		payload.TaskId, payload.FileId, payload.SegmentId)
	if err != nil {
		logger.Errorf("获取片段数据失败: %v", err)
		sendErrorResponse(writer, err.Error())
		return err
	}

	// 序列化响应数据
	responseData, err := segmentStorageData.Marshal()
	if err != nil {
		logger.Errorf("序列化响应数据失败: %v", err)
		sendErrorResponse(writer, err.Error())
		return err
	}

	// 重用 lenBuf 写入响应长度
	msgLen = len(responseData)
	lenBuf[0] = byte(msgLen >> 24)
	lenBuf[1] = byte(msgLen >> 16)
	lenBuf[2] = byte(msgLen >> 8)
	lenBuf[3] = byte(msgLen)

	// 写入长度和数据
	if _, err := writer.Write(lenBuf); err != nil {
		logger.Errorf("写入响应长度失败: %v", err)
		return err
	}

	if _, err := writer.Write(responseData); err != nil {
		logger.Errorf("写入响应数据失败: %v", err)
		return err
	}

	if err := writer.Flush(); err != nil {
		logger.Errorf("刷新响应缓冲区失败: %v", err)
		return err
	}

	return nil
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

// 辅助函数：发送错误响应
// 参数:
//   - writer: *bufio.Writer 写入器，用于写入错误响应
//   - msg: string 错误消息
//
// 返回值:
//   - error: 如果在发送过程中发生错误，返回相应的错误信息
func sendErrorResponse(writer *bufio.Writer, msg string) error {
	return sendResponse(writer, fmt.Sprintf("错误: %s\n", msg))
}

// 辅助函数：发送响应
// 参数:
//   - writer: *bufio.Writer 写入器，用于写入响应
//   - msg: string 响应消息
//
// 返回值:
//   - error: 如果在发送过程中发生错误，返回相应的错误信息
func sendResponse(writer *bufio.Writer, msg string) error {
	if _, err := writer.WriteString(msg); err != nil {
		logger.Errorf("发送响应失败: %v", err)
		return err
	}
	return writer.Flush()
}

// // handleRequestSegment 处理请求数据片段的请求
// // 参数:
// //   - req: 包含请求的详细信息
// //   - res: 用于设置响应内容
// //
// // 返回值:
// //   - int32 - 状态码，200表示成功，其他表示失败
// //   - string - 状态消息，成功或错误描述
// //
// // 功能:
// //   - 解析请求载荷
// //   - 获取片段存储数据
// //   - 序列化响应数据
// //   - 返回处理结果
// func handleRequestSegment(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
// 	// 创建一个新的 ContinuousDownloadRequest 对象用于解析请求载荷
// 	payload := new(pb.SegmentContentRequest)
// 	// 尝试解析请求载荷，如果失败则返回错误
// 	if err := payload.Unmarshal(req.Payload); err != nil {
// 		// 记录解码错误日志
// 		logger.Errorf("解码错误: %v", err)
// 		// 返回错误状态码和消息
// 		return 6603, "解码错误"
// 	}

// 	// 记录收到片段请求的日志
// 	logger.Infof("收到片段请求:  TaskID= %s, FileID= %s, SegmentIndex= %d, SegmentId= %s", payload.FileId, payload.TaskId, payload.SegmentIndex, payload.SegmentId)

// 	// 获取片段存储数据
// 	response, err := GetSegmentStorageData(sp.db, sp.host.ID().String(),
// 		payload.TaskId, payload.FileId, payload.SegmentId)
// 	if err != nil {
// 		logger.Errorf("获取片段数据失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 序列化响应载荷
// 	responseData, err := response.Marshal()
// 	// 如果序列化失败，记录错误日志并返回
// 	if err != nil {
// 		// 记录序列化响应数据失败的错误日志
// 		logger.Errorf("序列化响应数据失败: %v", err)
// 		// 返回错误状态码和消息
// 		return 500, "序列化响应数据失败"
// 	}

// 	// 设置响应内容
// 	res.Data = responseData

// 	// 记录成功发送片段的日志
// 	logger.Infof("成功发送片段: FileID=%s, SegmentIndex=%d, 响应大小=%d字节",
// 		payload.FileId, response.SegmentIndex, len(responseData))
// 	// 返回成功状态码和消息
// 	return 200, "成功"
// }
