package search

import (
	"crypto/md5"
	"fmt"
	"time"

	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
)

// 文件新增搜索请求
type FileAddSearchRequestPayload struct {
	Value       string // 搜索值
	MD5         string // 请求值的MD5哈希
	UserPubHash []byte // 用户的公钥哈希
}

// 搜索响应
type SearchChan struct {
	MD5        string    // 请求值的MD5哈希
	FileID     string    // 文件的唯一标识
	Name       string    // 文件的名称
	Size       int64     // 文件的长度(以字节为单位)
	UploadTime time.Time // 上传时间
	ModTime    time.Time // 修改时间(非文件修改时间)
	Xref       int64     // Xref表中段的数量
}

// AddSearch 新增搜索
func AddSearch(
	p2p *dep2p.DeP2P, // DeP2P网络主机
	pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	cache *ristretto.Cache, // 缓存实例

	key string, // 搜索类("fileID"、"name"")
	value string, // 搜索值
	userPubHash []byte, // 用户的公钥哈希
) (string, error) {
	// 创建 MD5 哈希
	hash := md5.New()
	hash.Write([]byte(key + value))
	md5Value := fmt.Sprintf("%x", hash.Sum(nil))

	requestSearchPayload := &FileAddSearchRequestPayload{
		Value:       value,
		MD5:         md5Value,
		UserPubHash: userPubHash,
	}

	// 检查并设置缓存项
	if !checkAndSet(cache, md5Value) {
		return "", fmt.Errorf("请求过于频繁")
	}

	var err error
	switch key {
	case "fileID":
		err = network.SendPubSub(p2p, pubsub, config.PubsubAddSearchRequestTopic, "fileID", "", requestSearchPayload)
	case "name":
		err = network.SendPubSub(p2p, pubsub, config.PubsubAddSearchRequestTopic, "name", "", requestSearchPayload)
	default:
		err = fmt.Errorf("非法请求")
	}

	return md5Value, err
}

// checkAndSet 检查并设置缓存项
func checkAndSet(cache *ristretto.Cache, key string) bool {
	if content, found := cache.Get(key); found {
		if time.Now().Before(content.(time.Time)) {
			return false
		}
	}

	// 更新缓存，设置过期时间为1分钟后
	cache.Set(key, time.Now().Add(time.Minute), 1)
	return true
}
