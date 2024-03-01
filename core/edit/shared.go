package edit

import (
	"fmt"
	"time"

	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
)

// 文件新增共享请求
type FileAddSharedRequestPayload struct {
	FileID      string    // 文件的唯一标识
	FileKey     string    // 文件的密钥
	UserPubHash []byte    // 用户的公钥哈希
	Expiry      time.Time // 有效期
}

// AddShared 新增共享
func AddShared(
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	db *sqlites.SqliteDB, // sqlite数据库服务
	fileID string, // 文件的唯一标识
	fileKey string, // 文件的密钥
	userPubHash []byte, // 用户的公钥哈希
	expiry time.Time, // 有效期
) error {

	// 查询共享的文件是否存在
	if !sqlite.SelectOneFileID(db, fileID) {
		return fmt.Errorf("文件不存在")
	}

	requestEditPayload := &FileAddSharedRequestPayload{
		FileID:      fileID,
		FileKey:     fileKey,
		UserPubHash: userPubHash,
		Expiry:      expiry,
	}

	// 向指定的全网节点发送文件删除请求订阅消息
	return network.SendPubSub(p2p, pubsub, config.PubsubAddSharedRequestTopic, "", "", requestEditPayload)
}
