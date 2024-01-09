package delete

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
)

// 文件删除请求
type FileDeleteRequestPayload struct {
	FileID     string // 文件的唯一标识
	PubKeyHash []byte // 所有者的私钥
}

// Delete 删除文件
func Delete(
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	db *sqlites.SqliteDB, // sqlite数据库服务

	fileID string, // 文件的唯一标识
	ownerPriv *ecdsa.PrivateKey, // 所有者的私钥
) error {

	// 查询删除的文件是否存在
	if !sqlite.SelectOneFileID(db, fileID) {
		return fmt.Errorf("文件不存在")
	}

	pubKeyHash, err := script.GetPubKeyHashFromPrivKey(ownerPriv) // 从ECDSA私钥中提取公钥哈希
	if err != nil {
		return err
	}

	requestDeletePayload := &FileDeleteRequestPayload{
		FileID:     fileID,
		PubKeyHash: pubKeyHash,
	}

	// 向指定的全网节点发送文件删除请求订阅消息
	return network.SendPubSub(p2p, pubsub, config.PubsubFileDeleteRequestTopic, "", "", requestDeletePayload)
}
