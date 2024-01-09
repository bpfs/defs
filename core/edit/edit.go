package edit

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

// 文件名称修改请求
type FileEditNameRequestPayload struct {
	FileID     string // 文件的唯一标识
	NewName    string // 文件的名称
	PubKeyHash []byte // 所有者的私钥
}

// EditName 修改文件名称
func EditName(
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	db *sqlites.SqliteDB, // sqlite数据库服务
	fileID string, // 文件的唯一标识
	ownerPriv *ecdsa.PrivateKey, // 所有者的私钥
	newName string, // 文件的新名称
) error {

	// 查询修改的文件是否存在
	if !sqlite.SelectOneFileID(db, fileID) {
		return fmt.Errorf("文件不存在")
	}

	pubKeyHash, err := script.GetPubKeyHashFromPrivKey(ownerPriv) // 从ECDSA私钥中提取公钥哈希
	if err != nil {
		return err
	}

	requestEditPayload := &FileEditNameRequestPayload{
		FileID:     fileID,
		NewName:    newName,
		PubKeyHash: pubKeyHash,
	}

	// 向指定的全网节点发送文件删除请求订阅消息
	return network.SendPubSub(p2p, pubsub, config.PubsubFileEditRequestTopic, "name", "", requestEditPayload)
}

// 文件共享修改请求
type FileEditSharedRequestPayload struct {
	FileID     string // 文件的唯一标识
	FileKey    string // 文件的密钥
	Shared     bool   // 文件共享状态
	PubKeyHash []byte // 所有者的私钥
}

// EditShared 修改文件共享
func EditShared(
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	db *sqlites.SqliteDB, // sqlite数据库服务
	//address string, // 文件所有者的地址

	fileID string, // 文件的唯一标识
	fileKey string, // 文件的密钥
	shared bool, // 文件共享状态
	ownerPriv *ecdsa.PrivateKey, // 所有者的私钥
) error {

	// 查询修改的文件是否存在
	if !sqlite.SelectOneFileID(db, fileID) {
		return fmt.Errorf("文件不存在")
	}
	pubKeyHash, err := script.GetPubKeyHashFromPrivKey(ownerPriv) // 从ECDSA私钥中提取公钥哈希
	if err != nil {
		return err
	}

	requestEditPayload := &FileEditSharedRequestPayload{
		FileID:     fileID,
		FileKey:    fileKey,
		Shared:     shared,
		PubKeyHash: pubKeyHash,
	}

	// 向指定的全网节点发送文件删除请求订阅消息
	return network.SendPubSub(p2p, pubsub, config.PubsubFileEditRequestTopic, "shared", "", requestEditPayload)
}
