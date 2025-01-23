package shared

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/script"
	"github.com/bpfs/defs/v2/segment"
	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dep2p/pubsub"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
)

// RequestSetFileSegmentPubSub 请求设置文件片段的共享状态
// 参数:
//   - ctx: 上下文对象,用于控制请求的生命周期
//   - host: libp2p网络主机实例
//   - nps: 节点发布订阅系统实例
//   - fileID: 要设置共享状态的文件ID
//   - pubkeyHash: 文件所有者的公钥哈希
//   - enableSharing: 是否启用共享
//
// 返回值:
//   - error: 如果请求发送成功返回nil,否则返回错误信息
func RequestSetFileSegmentPubSub(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	fileID string,
	pubkeyHash []byte,
	enableSharing bool,
) error {
	// 获取本地节点的地址信息
	addrInfo := peer.AddrInfo{
		ID:    host.ID(),    // 设置节点ID
		Addrs: host.Addrs(), // 设置节点地址列表
	}

	// 序列化地址信息为JSON格式
	addrInfoBytes, err := addrInfo.MarshalJSON()
	if err != nil {
		logger.Errorf("序列化AddrInfo失败: %v", err)
		return err
	}

	// 构造请求数据
	requestData := &pb.RequestSetFileSegmentPubSub{
		FileId:        fileID,        // 设置文件ID
		PubkeyHash:    pubkeyHash,    // 设置公钥哈希
		AddrInfo:      addrInfoBytes, // 设置地址信息
		EnableSharing: enableSharing, // 设置是否开启共享
	}

	// 序列化请求数据为二进制格式
	data, err := requestData.Marshal()
	if err != nil {
		logger.Errorf("序列化请求数据失败: %v", err)
		return err
	}

	// 获取设置共享文件请求的发布主题
	topic, err := nps.GetTopic(PubSubSetFileSegmentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return err
	}

	// 发布消息到网络
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发布消息失败: %v", err)
		return err
	}

	return nil
}

// HandleSetFileSegmentRequestPubSub 处理设置文件片段共享状态的请求
// 参数:
//   - ctx: 上下文对象,用于控制请求处理的生命周期
//   - opt: 文件系统配置选项
//   - db: 数据库实例
//   - fs: 文件系统接口
//   - nps: 节点发布订阅系统实例
//   - res: 接收到的消息
func HandleSetFileSegmentRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	res *pubsub.Message,
) {

	// 解析请求数据
	request := new(pb.RequestSetFileSegmentPubSub)
	if err := request.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析请求数据失败: %v", err)
		return
	}

	// 解析请求者的地址信息
	var addrInfo peer.AddrInfo
	if err := addrInfo.UnmarshalJSON(request.AddrInfo); err != nil {
		logger.Errorf("解析地址信息失败: %v", err)
		return
	}

	// 验证请求参数的有效性
	if request.FileId == "" || len(request.PubkeyHash) == 0 {
		logger.Error("文件ID或公钥哈希无效")
		return
	}

	// 获取文件片段存储实例
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 检查文件是否存在
	segments, err := store.GetFileSegmentStoragesByFileID(request.FileId, request.PubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段失败: %v", err)
		return
	}

	if len(segments) == 0 {
		logger.Error("文件不存在")
		return
	}
	// 遍历文件片段
	for _, segmentV := range segments {
		// 在这里处理每个 segment
		// 验证文件所有权
		pubKeyHash, err := script.ExtractPubKeyHashFromScript(segmentV.P2PkhScript)
		if err != nil {
			logger.Errorf("从P2PKH脚本提取公钥哈希失败: %v", err)
			return
		}

		if !bytes.Equal(pubKeyHash, request.PubkeyHash) {
			logger.Error("无权限设置文件共享状态")
			return
		}

		// 构建片段存储路径
		subDir := filepath.Join(paths.GetSlicePath(), nps.Host().ID().String(), request.FileId)

		// 打开片段文件
		file, err := os.OpenFile(filepath.Join(subDir, segmentV.SegmentId), os.O_RDWR, 0666)
		if err != nil {
			logger.Errorf("打开文件失败: %v", err)
			return
		}

		// 定义需要读取的片段类型
		segmentTypes := []string{"FILEID", "P2PKHSCRIPT"}

		// 读取文件片段
		segmentResults, _, err := segment.ReadFileSegments(file, segmentTypes)
		if err != nil {
			logger.Errorf("读取文件片段失败: %v", err)
			return
		}

		// 创建一个新的类型编解码器
		codec := segment.NewTypeCodec()

		// 获取并验证文件唯一标识
		fileId, exists := segmentResults["FILEID"]
		if !exists {
			logger.Error("文件ID不存在")
			return
		}

		// 解码文件唯一标识
		fileIdDecode, err := codec.Decode(fileId.Data)
		if err != nil {
			logger.Errorf("解码文件ID失败: %v", err)
			return
		}

		// 验证文件ID一致性
		if !reflect.DeepEqual(fileIdDecode, request.FileId) {
			logger.Error("文件ID不匹配")
			return
		}

		// 获取并验证P2PKH脚本
		p2pkhScript, exists := segmentResults["P2PKHSCRIPT"]
		if !exists {
			logger.Error("P2PKH脚本不存在")
			return
		}

		p2pkhScriptDecode, err := codec.Decode(p2pkhScript.Data)
		if err != nil {
			logger.Errorf("解码P2PKH脚本失败: %v", err)
			return
		}

		pubKeyHash, err = script.ExtractPubKeyHashFromScript(p2pkhScriptDecode.([]byte))
		if err != nil {
			logger.Errorf("从P2PKH脚本提取公钥哈希失败: %v", err)
			return
		}

		// 验证P2PKH脚本一致性
		if !reflect.DeepEqual(pubKeyHash, request.PubkeyHash) {
			logger.Error("P2PKH脚本不匹配")
			return
		}

		// 从文件加载 xref 表
		xref, err := segment.LoadXrefFromFile(file)
		if err != nil {
			logger.Errorf("加载xref表失败: %v", err)
			return
		}

		// 编码Shared
		shared, err := codec.Encode(request.EnableSharing)
		if err != nil {
			logger.Errorf("编码共享状态失败: %v", err)
			return
		}

		// 将段写入文件
		if err := segment.WriteSegmentToFile(file, "SHARED", shared, xref); err != nil {
			logger.Errorf("写入共享状态失败: %v", err)
			return
		}

		// 保存 xref 表并关闭文件
		if err := segment.SaveAndClose(file, xref); err != nil {
			logger.Errorf("保存xref表失败: %v", err)
			return
		}
		// 最后执行关闭
		file.Close()
	}

	// 更新文件的共享状态
	if err := store.UpdateFileSegmentShared(request.FileId, request.PubkeyHash, request.EnableSharing); err != nil {
		logger.Errorf("更新共享状态失败: %v", err)
		return
	}

	logger.Infof("设置文件共享状态成功, fileID: %s", request.FileId)
}
