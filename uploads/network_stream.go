// Package uploads 提供文件上传相关的功能实现
package uploads

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/segment"
	"github.com/bpfs/defs/v2/utils/paths"
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
	// StreamSendingToNetworkProtocol 定义了发送任务到网络的协议标识符
	StreamSendingToNetworkProtocol = fmt.Sprintf("defs@stream/sending/network/%s", version)
	// StreamForwardToNetworkProtocol 定义了转发任务到网络的协议标识符
	StreamForwardToNetworkProtocol = fmt.Sprintf("defs@stream/forward/network/%s", version)
	// 工作池大小
	maxWorkers = runtime.NumCPU() * 2
	// 工作通道
	workChan = make(chan *processTask, maxWorkers*10)
)

type processTask struct {
	payload *pb.FileSegmentStorage
	usp     *StreamProtocol
}

// init 初始化
func init() {
	// 启动工作池
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}
}

// worker 工作协程
// 主要步骤：
// 1. 从工作通道接收任务
// 2. 处理数据
// 3. 如果处理失败，记录错误
func worker() {
	for task := range workChan {
		// 处理数据
		if err := processPayload(task.payload, task.usp); err != nil {
			logger.Errorf("处理数据失败: %v", err)
		}
	}
}

// StreamProtocol 定义了流协议的结构体
type StreamProtocol struct {
	ctx          context.Context       // 全局上下文，用于管理整个应用的生命周期和取消操作
	opt          *fscfg.Options        // 文件存储选项配置，包含各种系统设置和参数
	db           *database.DB          // 持久化存储，用于本地数据的存储和检索
	fs           afero.Afero           // 文件系统接口，提供跨平台的文件操作能力
	host         host.Host             // libp2p网络主机实例
	routingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	nps          *pubsub.NodePubSub    // 发布订阅系统，用于节点之间的消息传递
	upload       *UploadManager        // 上传管理器，用于处理和管理文件上传任务，包括任务调度、状态跟踪等
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
	Upload       *UploadManager        // 上传管理器，用于处理和管理文件上传任务，包括任务调度、状态跟踪等
}

// RegisterUploadStreamProtocol 注册上传流协议
// 参数:
//   - lc: fx.Lifecycle 类型，用于管理组件的生命周期
//   - input: RegisterStreamProtocolInput 类型，包含注册所需的所有依赖项
//
// 返回值: 无
func RegisterUploadStreamProtocol(lc fx.Lifecycle, input RegisterStreamProtocolInput) {
	// 创建流协议实例
	usp := &StreamProtocol{
		ctx:          input.Ctx,
		opt:          input.Opt,
		db:           input.DB,
		fs:           input.FS,
		host:         input.Host,
		routingTable: input.RoutingTable,
		nps:          input.NPS,
		upload:       input.Upload,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 创建发送任务到网络的监听器
			sendingListener, err := pointsub.Listen(input.Host, protocol.ID(StreamSendingToNetworkProtocol))
			if err != nil {
				logger.Errorf("创建发送任务监听器失败: %v", err)
				return err
			}

			// 创建转发到网络的监听器
			forwardListener, err := pointsub.Listen(input.Host, protocol.ID(StreamForwardToNetworkProtocol))
			if err != nil {
				logger.Errorf("创建转发任务监听器失败: %v", err)
				return err
			}

			// 使用 WaitGroup 追踪所有连接处理
			var wg sync.WaitGroup

			// 启动发送任务处理协程
			go func() {
				for {
					conn, err := sendingListener.Accept()
					if err != nil {
						if ctx.Err() != nil {
							wg.Wait() // 等待所有连接处理完成
							return
						}
						logger.Errorf("接受发送任务连接失败: %v", err)
						continue
					}

					wg.Add(1)
					go func(c net.Conn) {
						defer wg.Done()
						handleSendingConnection(ctx, c, usp)
					}(conn)
				}
			}()

			// 启动转发任务处理协程
			go func() {
				for {
					conn, err := forwardListener.Accept()
					if err != nil {
						if ctx.Err() != nil {
							wg.Wait() // 等待所有连接处理完成
							return
						}
						logger.Errorf("接受转发任务连接失败: %v", err)
						continue
					}

					wg.Add(1)
					go func(c net.Conn) {
						defer wg.Done()
						handleForwardConnection(ctx, c, usp)
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

// handleSegmentData 处理分片数据的通用函数
// 参数:
//   - reader: *bufio.Reader 读取器，用于读取数据
//   - writer: *bufio.Writer 写入器，用于写入数据
//   - usp: *StreamProtocol 流协议实例
//
// 返回值:
//   - error: 如果在处理过程中发生错误，返回相应的错误信息
func handleSegmentData(reader *bufio.Reader, writer *bufio.Writer, usp *StreamProtocol) error {
	// 读取4字节的长度前缀
	lenBuf := make([]byte, 4)
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

	// 解析消息长度
	msgLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])

	// 验证消息长度
	if msgLen <= 0 || msgLen > MaxBlockSize {
		logger.Errorf("无效的消息长度: %d", msgLen)
		sendErrorResponse(writer, fmt.Sprintf("无效的消息长度: %d", msgLen))
		return fmt.Errorf("无效的消息长度: %d", msgLen)
	}

	// 读取消息内容
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
	payload := new(pb.FileSegmentStorage)
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

	// 立即发送成功响应，不等待处理完成
	response := "success\n"
	if _, err := writer.WriteString(response); err != nil {
		logger.Errorf("发送成功响应失败: %v", err)
		return err
	}
	if err := writer.Flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	// 异步处理数据
	select {
	case workChan <- &processTask{payload: payload, usp: usp}:
		// 成功加入工作队列
	default:
		logger.Warnf("工作队列已满，直接处理")
		// 队列满时直接处理
		if err := processPayload(payload, usp); err != nil {
			logger.Errorf("处理数据失败: %v", err)
		}
	}

	return nil
}

// validatePayload 验证载荷数据
func validatePayload(payload *pb.FileSegmentStorage, usp *StreamProtocol) error {
	if usp.opt == nil || usp.fs == nil {
		logger.Errorf("系统配置无效")
		return fmt.Errorf("系统配置无效")
	}

	if payload == nil {
		logger.Errorf("载荷为空")
		return fmt.Errorf("载荷为空")
	}

	if payload.SegmentContent == nil {
		logger.Errorf("分片内容为空")
		return fmt.Errorf("分片内容为空")
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

// processPayload 处理载荷数据
// 参数:
//   - payload: *pb.FileSegmentStorage 文件片段存储对象
//   - usp: *StreamProtocol 流协议实例
//
// 返回值:
//   - error: 如果在处理过程中发生错误，返回相应的错误信息
func processPayload(payload *pb.FileSegmentStorage, usp *StreamProtocol) error {
	// 构建并存储文件片段
	if err := buildAndStoreFileSegment(payload, usp.host.ID().String()); err != nil {
		logger.Errorf("存储文件片段失败: %v", err)
		return err
	}

	// 保存到数据库
	store := database.NewFileSegmentStorageSqlStore(usp.db.SqliteDB)
	payloadSql, err := database.ToFileSegmentStorageSql(payload)
	if err != nil {
		logger.Errorf("转换数据失败: %v", err)
		return err
	}

	// 保存到数据库
	if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
		logger.Errorf("保存到数据库失败: %v", err)
		return err
	}

	// 清空分片内容防止通道内容过大，在转发时重新查询
	payload.SegmentContent = nil

	// 将payload发送到转发通道
	usp.upload.TriggerForward(payload)

	// 清空数据和请求载荷以释放内存
	payload = nil
	runtime.GC()

	return nil
}

// handleSendingConnection 和 handleForwardConnection 现在可以简化为：
func handleSendingConnection(ctx context.Context, conn net.Conn, usp *StreamProtocol) {
	handleConnection(ctx, conn, usp)
}

func handleForwardConnection(ctx context.Context, conn net.Conn, usp *StreamProtocol) {
	handleConnection(ctx, conn, usp)
}

// handleConnection 统一的连接处理函数
// 参数:
//   - ctx: context.Context 上下文，用于管理连接的生命周期
//   - conn: net.Conn 连接实例
//   - usp: *StreamProtocol 流协议实例
func handleConnection(ctx context.Context, conn net.Conn, usp *StreamProtocol) {
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

// buildAndStoreFileSegment 构建文件片段存储map并将其存储为文件
// 参数:
//   - payload: *pb.FileSegmentStorage 文件片段存储对象
//   - hostID: string 主机ID，用于构建文件路径
//
// 返回值:
//   - error: 如果在处理过程中发生错误，返回相应的错误信息
func buildAndStoreFileSegment(payload *pb.FileSegmentStorage, hostID string) error {
	// 构建文件片段存储map
	segmentMap, err := buildFileSegmentStorageMap(payload)
	if err != nil {
		logger.Errorf("构建文件片段存储map失败: %v", err)
		return err
	}

	// 设置文件存储的路径
	filePath := filepath.Join(paths.GetSlicePath(), hostID, payload.FileId, payload.SegmentId)

	// 使用segment.WriteFileSegment存储文件片段
	if err := segment.WriteFileSegment(filePath, segmentMap); err != nil {
		logger.Errorf("存储文件片段失败: %v", err)
		return err
	}

	return nil
}

// buildFileSegmentStorageMap 构建文件片段存储map
// 参数:
//   - payload: *pb.FileSegmentStorage 文件片段存储对象
//
// 返回值:
//   - map[string][]byte: 构建的map，key为字段名称的大写，值为对应内容的[]byte
//   - error: 如果在构建过程中发生错误，返回相应的错误信息
func buildFileSegmentStorageMap(payload *pb.FileSegmentStorage) (map[string][]byte, error) {
	result := make(map[string][]byte)
	codec := segment.NewTypeCodec()

	//////////////////// 基本文件信息 ////////////////////

	// 编码FileId
	fileId, err := codec.Encode(payload.FileId)
	if err != nil {
		logger.Errorf("编码 FileId 失败: %v", err)
		return nil, err
	}
	result["FILEID"] = fileId

	// 编码Name
	name, err := codec.Encode(payload.Name)
	if err != nil {
		logger.Errorf("编码 Name 失败: %v", err)
		return nil, err
	}
	result["NAME"] = name

	// 编码Extension
	extension, err := codec.Encode(payload.Extension)
	if err != nil {
		logger.Errorf("编码 Extension 失败: %v", err)
		return nil, err
	}
	result["EXTENSION"] = extension

	// 编码Size
	size, err := codec.Encode(payload.Size_)
	if err != nil {
		logger.Errorf("编码 Size 失败: %v", err)
		return nil, err
	}
	result["SIZE"] = size

	// 编码ContentType
	contentType, err := codec.Encode(payload.ContentType)
	if err != nil {
		logger.Errorf("编码 ContentType 失败: %v", err)
		return nil, err
	}
	result["CONTENTTYPE"] = contentType

	// 编码Sha256Hash
	sha256Hash, err := codec.Encode(payload.Sha256Hash)
	if err != nil {
		logger.Errorf("编码 Sha256Hash 失败: %v", err)
		return nil, err
	}
	result["SHA256HASH"] = sha256Hash

	// 编码UploadTime
	uploadTime, err := codec.Encode(payload.UploadTime)
	if err != nil {
		logger.Errorf("编码 UploadTime 失败: %v", err)
		return nil, err
	}
	result["UPLOADTIME"] = uploadTime

	//////////////////// 身份验证和安全相关 ////////////////////

	// 编码P2PkhScript
	p2pkhScript, err := codec.Encode(payload.P2PkhScript)
	if err != nil {
		logger.Errorf("编码 P2PkhScript 失败: %v", err)
		return nil, err
	}
	result["P2PKHSCRIPT"] = p2pkhScript

	// 编码P2PkScript
	p2pkScript, err := codec.Encode(payload.P2PkScript)
	if err != nil {
		logger.Errorf("编码 P2PkScript 失败: %v", err)
		return nil, err
	}
	result["P2PKSCRIPT"] = p2pkScript

	//////////////////// 分片信息 ////////////////////

	// 编码SliceTable
	if payload.SliceTable != nil {
		sliceTableBytes, err := files.SerializeSliceTable(payload.SliceTable)
		if err != nil {
			logger.Errorf("序列化 SliceTable 失败: %v", err)
			return nil, err
		}
		sliceTable, err := codec.Encode(sliceTableBytes)
		if err != nil {
			logger.Errorf("编码 SliceTable 失败: %v", err)
			return nil, err
		}
		result["SLICETABLE"] = sliceTable
	} else {
		logger.Error("文件哈希表为空")
		return nil, fmt.Errorf("文件哈希表为空")
	}

	//////////////////// 分片元数据 ////////////////////

	// 编码SegmentId
	segmentId, err := codec.Encode(payload.SegmentId)
	if err != nil {
		logger.Errorf("编码 SegmentId 失败: %v", err)
		return nil, err
	}
	result["SEGMENTID"] = segmentId

	// 编码SegmentIndex
	segmentIndex, err := codec.Encode(payload.SegmentIndex)
	if err != nil {
		logger.Errorf("编码 SegmentIndex 失败: %v", err)
		return nil, err
	}
	result["SEGMENTINDEX"] = segmentIndex

	// 编码Crc32Checksum
	crc32Checksum, err := codec.Encode(payload.Crc32Checksum)
	if err != nil {
		logger.Errorf("编码 Crc32Checksum 失败: %v", err)
		return nil, err
	}
	result["CRC32CHECKSUM"] = crc32Checksum

	//////////////////// 分片内容和加密 ////////////////////

	// 编码SegmentContent
	segmentContent, err := codec.Encode(payload.SegmentContent)
	if err != nil {
		logger.Errorf("编码 SegmentContent 失败: %v", err)
		return nil, err
	}
	result["SEGMENTCONTENT"] = segmentContent

	// 编码EncryptionKey
	encryptionKey, err := codec.Encode(payload.EncryptionKey)
	if err != nil {
		logger.Errorf("编码 EncryptionKey 失败: %v", err)
		return nil, err
	}
	result["ENCRYPTIONKEY"] = encryptionKey

	// 编码Signature
	signature, err := codec.Encode(payload.Signature)
	if err != nil {
		logger.Errorf("编码 Signature 失败: %v", err)
		return nil, err
	}
	result["SIGNATURE"] = signature

	//////////////////// 其他属性 ////////////////////

	// 编码Shared
	shared, err := codec.Encode(payload.Shared)
	if err != nil {
		logger.Errorf("编码 Shared 失败: %v", err)
		return nil, err
	}
	result["SHARED"] = shared

	// 编码Version
	version, err := codec.Encode(payload.Version)
	if err != nil {
		logger.Errorf("编码 Version 失败: %v", err)
		return nil, err
	}
	result["VERSION"] = version

	return result, nil
}
