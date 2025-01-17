package shared

import (
	"context"

	"github.com/bpfs/defs/v2/database"
	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/pubsub"
)

// SetFileShared 设置文件的共享状态
// 参数:
//   - ctx: context.Context 上下文对象
//   - host: host.Host libp2p主机实例
//   - nps: *pubsub.NodePubSub 节点发布订阅系统实例
//   - fileStore: *database.FileAssetStore 文件资产存储实例
//   - fileID: string 文件唯一标识
//   - pubkeyHash: []byte 文件所有者的公钥哈希
//   - shareAmount: float64 共享金额
//
// 返回值:
//   - error: 如果设置成功返回nil，否则返回错误信息
func SetFileShared(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	fileStore *database.FileAssetStore,
	fileID string,
	pubkeyHash []byte,
	shareAmount float64,
) error {
	// 1. 先在本地数据库中设置文件共享状态
	if err := fileStore.SetFileShared(pubkeyHash, fileID, shareAmount); err != nil {
		logger.Errorf("设置文件共享状态失败: %v", err)
		return err
	}

	// 2. 通过P2P网络广播共享状态变更
	if err := RequestSetFileSegmentPubSub(
		ctx,
		host,
		nps,
		fileID,
		pubkeyHash,
		true, // enableSharing = true
	); err != nil {
		logger.Errorf("广播文件共享状态变更失败: %v", err)
		return err
	}

	return nil
}

// UnsetFileShared 取消文件的共享状态
// 参数:
//   - ctx: context.Context 上下文对象
//   - host: host.Host libp2p主机实例
//   - nps: *pubsub.NodePubSub 节点发布订阅系统实例
//   - fileStore: *database.FileAssetStore 文件资产存储实例
//   - fileID: string 文件唯一标识
//   - pubkeyHash: []byte 文件所有者的公钥哈希
//
// 返回值:
//   - error: 如果取消成功返回nil，否则返回错误信息
func UnsetFileShared(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	fileStore *database.FileAssetStore,
	fileID string,
	pubkeyHash []byte,
) error {
	// 1. 先在本地数据库中取消文件共享状态
	if err := fileStore.UnsetFileShared(pubkeyHash, fileID); err != nil {
		logger.Errorf("取消文件共享状态失败: %v", err)
		return err
	}

	// 2. 通过P2P网络广播共享状态变更
	if err := RequestSetFileSegmentPubSub(
		ctx,
		host,
		nps,
		fileID,
		pubkeyHash,
		false, // enableSharing = false
	); err != nil {
		logger.Errorf("广播文件共享状态变更失败: %v", err)
		return err
	}

	return nil
}
