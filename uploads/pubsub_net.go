package uploads

import (
	"context"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
	"github.com/dep2p/pubsub"
)

// HandleDeleteFileSegmentRequestPubSub 处理删除文件切片请求主题
// 参数:
//   - ctx: context.Context 上下文对象,用于控制请求的生命周期
//   - opt: *fscfg.Options 配置选项,包含系统配置信息
//   - db: *database.DB 数据库实例,用于数据持久化
//   - fs: afero.Afero 文件系统实例,用于文件操作
//   - nps: *pubsub.NodePubSub 发布订阅系统,用于消息通信
//   - upload: *UploadManager 上传管理器实例,处理上传相关逻辑
//   - res: *pubsub.Message 接收到的消息,包含请求数据
//
// 返回值:
//   - error 返回处理过程中的错误信息
//
// 功能:
//   - 处理删除文件切片信息的请求
//   - 解析请求数据并删除对应的文件切片存储记录
func HandleDeleteFileSegmentRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	upload *UploadManager,
	res *pubsub.Message,
) error {
	// 解析请求消息数据到UploadPubSubDeleteFileSegmentRequest结构
	payload := new(pb.UploadPubSubDeleteFileSegmentRequest)
	if err := payload.Unmarshal(res.Data); err != nil {
		logger.Error("解析删除文件切片请求数据失败", err)
		return err
	}

	// 创建文件切片存储SQL操作实例
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 根据文件ID和公钥哈希删除相关的文件切片存储记录
	if err := store.DeleteFileSegmentStoragesByFileID(payload.FileId, payload.PubkeyHash); err != nil {
		logger.Error("删除文件切片存储记录失败", err)
		return err
	}

	return nil
}
