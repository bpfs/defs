package download

import (
	"fmt"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/sqlites"
)

// 文件下载请求(清单)
type FileDownloadRequestChecklistPayload struct {
	FileID      string // 文件的唯一标识(外部标识)
	UserPubHash []byte // 用户的公钥哈希
}

// 文件下载请求(内容)
type FileDownloadRequestContentPayload struct {
	FileID    string // 文件的唯一标识(外部标识)
	SliceHash string // 待下载的切片哈希
	Index     int    // 文件片段的索引(该片段在文件中的顺序位置)
}

// 文件下载响应(清单)
type FileDownloadResponseChecklistPayload struct {
	FileID          string                 // 文件的唯一标识
	FileKey         string                 // 文件的密钥
	Name            string                 // 文件的名称
	Size            int64                  // 文件的长度(以字节为单位)
	SliceTable      map[int]core.HashTable // 文件片段的哈希表
	AvailableSlices map[int]string         // 本地存储的文件片段信息
}

// 文件下载响应(内容)
type FileDownloadResponseContentPayload struct {
	FileID       string // 文件的唯一标识(外部标识)
	SliceHash    string // 下载的切片哈希
	Index        int    // 文件片段的索引(该片段在文件中的顺序位置)
	SliceContent []byte // 切片内容
}

func Download(
	// ctx context.Context, // 全局上下文
	// opt *opts.Options, // 文件存储选项配置
	// p2p *dep2p.DeP2P, // DeP2P网络主机
	// pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	db *sqlites.SqliteDB, // sqlite数据库服务
	// uploadChan chan *core.UploadChan, // 用于刷新上传的通道
	// downloadChan chan *core.DownloadChan, // 用于刷新下载的通道
	registry *eventbus.EventRegistry, // 事件总线
	// cache *ristretto.Cache, // 缓存实例
	pool *pool.MemoryPool, // 内存池

	fileID string, // 文件的唯一标识(外部标识)
	fileKey string, // 文件的密钥
	userPubHash []byte, // 用户的公钥哈希
) error {

	// 查询下载的文件是否存在
	if !sqlite.SelectOneFileID(db, fileID) {
		return fmt.Errorf("文件不存在")
	}

	// 添加一个新的下载任务
	if err := pool.AddDownloadTask(fileID, fileKey); err != nil {
		return err
	}

	// 无法获取文件下载开始事件总线
	bus := registry.GetEventBus(config.EventFileDownloadStart)
	if bus == nil {
		return fmt.Errorf("无法获取文件下载开始事件总线")
	}
	bus.Publish(config.EventFileDownloadStart, fileID, userPubHash)

	return nil
}
