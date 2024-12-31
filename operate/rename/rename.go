package rename

import (
	"bytes"
	"context"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
	"github.com/dep2p/pubsub"
)

// RequestRenameFileSegmentPubSub 请求重命名文件
// 参数:
//   - ctx: 上下文对象,用于控制请求的生命周期
//   - nps: 节点发布订阅系统实例,用于发布重命名请求
//   - fileID: 要重命名的文件ID,用于标识目标文件
//   - pubkeyHash: 文件所有者的公钥哈希,用于验证权限
//   - newName: 新的文件名,文件将被重命名为此名称
//
// 返回值:
//   - error: 如果请求发送成功返回nil,否则返回错误信息
func RequestRenameFileSegmentPubSub(
	ctx context.Context,
	nps *pubsub.NodePubSub,
	fileID string,
	pubkeyHash []byte,
	newName string,
) error {
	// 构建重命名请求消息,包含文件ID、公钥哈希和新文件名
	request := &pb.RequestRenameFileSegmentPubSub{
		FileId:     fileID,     // 设置文件ID
		PubkeyHash: pubkeyHash, // 设置公钥哈希
		NewName:    newName,    // 设置新文件名
	}

	// 将请求消息序列化为二进制数据
	data, err := request.Marshal()
	if err != nil {
		logger.Errorf("序列化重命名文件请求数据失败: %v", err)
		return err
	}

	// 获取用于发布重命名请求的主题
	topic, err := nps.GetTopic(PubSubRenameFileSegmentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		return err
	}

	// 将序列化后的请求数据发布到网络
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发送消息失败: %v", err)
		return err
	}

	return nil
}

// HandleRenameFileSegmentRequestPubSub 处理重命名文件请求
// 参数:
//   - ctx: 上下文对象,用于控制请求处理的生命周期
//   - opt: 文件系统配置选项,包含系统运行所需的配置信息
//   - db: 数据库实例,用于访问文件存储数据
//   - fs: 文件系统接口,用于文件操作
//   - nps: 节点发布订阅系统实例,用于网络通信
//   - res: 接收到的消息,包含重命名请求数据
func HandleRenameFileSegmentRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	res *pubsub.Message,
) {
	// 从消息数据中解析出重命名请求
	request := new(pb.RequestRenameFileSegmentPubSub)
	if err := request.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析重命名文件请求数据失败: %v", err)
		return
	}

	// 创建文件片段存储访问实例
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 根据文件ID和公钥哈希获取文件片段信息
	segments, err := store.GetFileSegmentStoragesByFileID(request.FileId, request.PubkeyHash)
	if err != nil {
		logger.Errorf("获取文件片段失败: %v", err)
		return
	}
	// 检查文件是否存在
	if len(segments) == 0 {
		logger.Error("文件不存在")
		return
	}

	// 验证请求者是否为文件所有者
	if !bytes.Equal(segments[0].P2PkhScript, request.PubkeyHash) {
		logger.Error("无权限重命名该文件")
		return
	}

	// 执行文件重命名操作
	if err := store.UpdateFileSegmentName(request.FileId, request.PubkeyHash, request.NewName); err != nil {
		logger.Errorf("更新文件名失败: %v", err)
		return
	}

	// 记录重命名成功的日志
	logger.Infof("成功重命名文件: fileID=%s, newName=%s", request.FileId, request.NewName)
}
