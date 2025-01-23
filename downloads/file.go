package downloads

import (
	"context"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"

	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/pubsub"
)

// NewDownloadFile 创建并初始化一个新的下载文件实例
// 参数:
//   - ctx: 上下文对象,用于控制请求的生命周期
//   - db: 数据库实例,用于存储下载记录
//   - h: libp2p主机实例,用于网络通信
//   - nps: 发布订阅系统实例,用于网络寻址
//   - fileID: 要下载的文件ID
//   - ownerPriv: 文件所有者的私钥,用于身份验证和加密
//
// 返回值:
//   - *pb.DownloadOperationInfo: 下载操作信息,包含任务ID、文件路径等
//   - error: 错误信息
//
// 功能:
//   - 生成下载任务ID和密钥分片
//   - 通过P2P网络获取文件信息
//   - 创建下载记录并保存到数据库
func NewDownloadFile(ctx context.Context, db *database.DB, h host.Host, nps *pubsub.NodePubSub,
	taskID string,
	fileID string,
	pubkeyHash []byte,
	firstKeyShare []byte,
) (*pb.DownloadOperationInfo, error) {
	// 通过P2P网络发送文件信息请求并等待响应
	response, err := RequestFileInfoPubSub(ctx, h, nps, taskID, fileID, pubkeyHash)
	if err != nil {
		logger.Errorf("发送文件信息请求并等待响应失败: %v", err)
		return nil, err
	}

	// 设置下载任务的初始状态为待下载(PENDING)
	status := pb.DownloadStatus_DOWNLOAD_STATUS_PENDING
	// 获取系统默认的下载文件保存路径
	filePath := paths.DefaultDownloadPath()

	// 创建下载文件记录并保存到数据库中
	_, err = CreateDownloadFileRecord(
		db.BadgerDB,         // 数据库实例
		taskID,              // 任务ID
		fileID,              // 文件ID
		pubkeyHash,          // 公钥哈希
		firstKeyShare,       // 第一个密钥分片
		filePath,            // 文件保存路径
		response.FileMeta,   // 文件元数据
		response.SliceTable, // 文件分片表
		status,              // 下载状态
	)
	if err != nil {
		logger.Errorf("创建下载文件记录失败: %v", err)
		return nil, err
	}

	// 构造并返回下载操作信息对象
	downloadInfo := &pb.DownloadOperationInfo{
		TaskId:   taskID,            // 任务ID
		FilePath: filePath,          // 文件保存路径
		FileId:   fileID,            // 文件ID
		FileMeta: response.FileMeta, // 文件元数据
	}

	return downloadInfo, nil
}
