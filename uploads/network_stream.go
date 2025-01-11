// Package uploads 提供文件上传相关的功能实现
package uploads

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/segment"
	"github.com/bpfs/defs/v2/streams"
	"github.com/bpfs/defs/v2/utils/network"
	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dep2p/pubsub"

	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/libp2p/core/protocol"
	"go.uber.org/fx"
)

const (
	version = "1.0.0" // 协议版本号
)

var (
	// StreamSendingToNetworkProtocol 定义了发送任务到网络的协议标识符
	StreamSendingToNetworkProtocol = fmt.Sprintf("defs@stream/sending/network/%s", version)
	// StreamForwardToNetworkProtocol 定义了转发任务到网络的协议标识符
	StreamForwardToNetworkProtocol = fmt.Sprintf("defs@stream/forward/network/%s", version)
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

	// 添加生命周期钩子
	lc.Append(fx.Hook{
		// OnStart 钩子在应用启动时执行
		OnStart: func(ctx context.Context) error {
			// 注册发送任务到网络的请求处理器
			streams.RegisterStreamHandler(input.Host, protocol.ID(StreamSendingToNetworkProtocol), streams.HandlerWithRW(usp.handleSendingToNetwork))
			// 注册转发到网络的处理函数
			streams.RegisterStreamHandler(input.Host, protocol.ID(StreamForwardToNetworkProtocol), streams.HandlerWithRW(usp.handleForwardToNetwork))
			return nil
		},
		// OnStop 钩子在应用停止时执行
		OnStop: func(ctx context.Context) error {
			// 清理资源等停止逻辑
			return nil
		},
	})
}

// handleSendingToNetwork 处理发送任务到网络的请求
// 参数:
//   - req: *streams.RequestMessage 类型，包含请求的详细信息
//   - res: *streams.ResponseMessage 类型，用于设置响应内容
//
// 返回值:
//   - int32: 状态码，表示处理结果
//   - string: 状态消息，对处理结果的描述
func (sp *StreamProtocol) handleSendingToNetwork(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	// 解析请求载荷
	payload := new(pb.FileSegmentStorage)
	if err := payload.Unmarshal(req.Payload); err != nil {
		logger.Errorf("解码请求载荷失败: %v", err)
		return 6603, err.Error()
	}

	// 打印分片ID信息
	logger.Infof("=====> SegmentId: %v", payload.SegmentId)

	// 检查参数有效性
	if sp.opt == nil || sp.fs == nil || payload.SegmentContent == nil {
		logger.Error("文件写入的参数无效: 选项配置、文件系统或分片内容为空")
		return 500, "文件写入的参数无效"
	}

	// 构建文件片段存储map并将其存储为文件
	if err := buildAndStoreFileSegment(payload, sp.host.ID().String()); err != nil {
		logger.Errorf("存储接收内容失败: %v", err)
		return 500, err.Error()
	}

	// 创建 FileSegmentStorageStore 实例
	store := database.NewFileSegmentStorageSqlStore(sp.db.SqliteDB)
	payloadSql, err := database.ToFileSegmentStorageSql(payload)
	if err != nil {
		logger.Errorf("将 FileSegmentStorage 转换为 FileSegmentStorageSql失败: %v", err)
		return 500, err.Error()
	}

	// 将文件片段存储记录保存到数据库
	if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
		logger.Errorf("保存文件片段存储记录失败: %v", err)
		return 500, err.Error()
	}

	// 清空分片内容防止通道内容过大，在转发时重新查询
	payload.SegmentContent = nil

	// 将payload发送到转发通道
	//sp.upload.TriggerForward(payload)

	// 清空数据和请求载荷以释放内存
	payload = nil
	req.Payload = nil

	// 强制进行垃圾回收
	runtime.GC()

	return 200, "成功"
}

// handleForwardToNetwork 处理转发任务到网络的请求
// 参数:
//   - req: *streams.RequestMessage 类型，包含请求的详细信息
//   - res: *streams.ResponseMessage 类型，用于设置响应内容
//
// 返回值:
//   - int32: 状态码，表示处理结果
//   - string: 状态消息，对处理结果的描述
func (sp *StreamProtocol) handleForwardToNetwork(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	// 解析请求载荷
	payload := new(pb.FileSegmentStorage)
	if err := payload.Unmarshal(req.Payload); err != nil {
		logger.Errorf("解码请求载荷失败: %v", err)
		return 6603, err.Error()
	}

	logger.Infof("转发=====> SegmentId: %v,内容%d", payload.SegmentId, len(payload.SegmentContent))

	// 检查参数有效性
	if sp.opt == nil || sp.fs == nil || payload.SegmentContent == nil {
		logger.Error("文件写入的参数无效: 选项配置、文件系统或分片内容为空")
		return 500, "文件写入的参数无效"
	}

	// 构建文件片段存储map并将其存储为文件
	if err := buildAndStoreFileSegment(payload, sp.host.ID().String()); err != nil {
		logger.Errorf("存储接收内容失败: %v", err)
		return 500, err.Error()
	}

	// 创建 FileSegmentStorageStore 实例
	store := database.NewFileSegmentStorageSqlStore(sp.db.SqliteDB)
	payloadSql, err := database.ToFileSegmentStorageSql(payload)
	if err != nil {
		logger.Errorf("将 FileSegmentStorage 转换为 FileSegmentStorageSql失败: %v", err)
		return 500, err.Error()
	}

	// 将文件片段存储记录保存到数据库
	if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
		logger.Errorf("保存文件片段存储记录失败: %v", err)
		return 500, err.Error()
	}

	// 清空数据和请求载荷以释放内存
	payload = nil
	req.Payload = nil

	// 强制进行垃圾回收
	runtime.GC()

	return 200, "成功"
}

// sendForwardRequest 发送转发请求到网络中的其他节点
// 参数:
//   - ctx: context.Context 类型，用于控制请求的上下文
//   - h: host.Host 类型，libp2p主机实例
//   - routingTable: *kbucket.RoutingTable 类型，用于查找目标节点
//   - payload: *pb.FileSegmentStorage 类型，要转发的文件片段存储对象
//
// 返回值:
//   - error: 如果转发过程中发生错误，返回相应的错误信息
func sendForwardRequest(ctx context.Context, h host.Host, routingTable *kbucket.RoutingTable, payload *pb.FileSegmentStorage) error {
	maxRetries := 3 // 最大重试次数
	for retry := 0; retry < maxRetries; retry++ {
		// 检查节点数量是否足够
		if routingTable.Size() < 1 {
			logger.Warnf("转发时所需节点不足: %d", routingTable.Size())
			time.Sleep(1 * time.Second)
			continue
		}

		// 获取最近的节点
		receiverPeers := routingTable.NearestPeers(kbucket.ConvertKey(payload.SegmentId), 5, 2)
		if len(receiverPeers) == 0 {
			logger.Warn("没有找到合适的节点，重试中...")
			continue
		}

		// 随机选择一个节点
		node := receiverPeers[rand.Intn(len(receiverPeers))]
		network.StreamMutex.Lock()

		// 序列化存储对象
		data, err := payload.Marshal()
		if err != nil {
			logger.Errorf("序列化payload失败: %v", err)
			return err
		}

		// 发送文件片段到目标节点
		res, err := network.SendStream(ctx, h, StreamForwardToNetworkProtocol, "", node, data)
		if err != nil || res == nil {
			logger.Warnf("向节点 %s 发送数据失败: %v，重试中...", node.String(), err)
			continue
		}
		if res.Code != 200 {
			logger.Warnf("向节点 %s 发送数据失败，响应码: %d，重试中...", node.String(), res.Code)
			continue
		}

		logger.Infof("=====> 成功转发 SegmentId: %v", payload.SegmentId)
		return nil
	}

	return fmt.Errorf("转发文件片段失败，已达到最大重试次数")
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
