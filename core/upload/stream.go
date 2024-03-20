package upload

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"

	"github.com/sirupsen/logrus"
)

// 流协议
type StreamProtocol struct {
	Ctx          context.Context         // 全局上下文
	Opt          *opts.Options           // 文件存储选项配置
	P2P          *dep2p.DeP2P            // DeP2P网络主机
	PubSub       *pubsub.DeP2PPubSub     // DeP2P网络订阅
	DB           *sqlites.SqliteDB       // sqlite数据库服务
	UploadChan   chan *core.UploadChan   // 用于刷新上传的通道
	DownloadChan chan *core.DownloadChan // 用于刷新下载的通道
	StorageChan  chan *core.StorageChan  // 用于存储奖励的通知

	Registry *eventbus.EventRegistry // 事件总线
	Cache    *ristretto.Cache        // 缓存实例
	Pool     *pool.MemoryPool        // 内存池
}

// HandleStreamFileSliceUploadStream 处理文件片段上传的流消息
func (sp *StreamProtocol) HandleStreamFileSliceUploadStream(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
	// 尝试从请求负载中解码出文件片段的内容
	payload := new([]byte)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[HandleStreamFileSliceUploadStream] 解码失败:\t%v", err)
		// 返回400状态码和错误描述信息，表示请求负载解码失败
		return 400, "解码请求负载失败"
	}

	// 创建一个临时文件来存储接收到的文件片段内容
	tmpFile, err := CreateTempFile(*payload)
	if err != nil {
		logrus.Errorf("创建临时文件失败:%v", err)
		// 返回500状态码和错误描述信息，表示创建临时文件失败
		return 500, "创建临时文件失败"
	}

	// 确保在函数返回前删除临时文件
	defer func() {
		os.Remove(tmpFile.Name())
		tmpFile.Close()
	}()

	// 定义需要从文件中读取的段类型
	segmentTypes := []string{"FILEID", "SLICEHASH", "SLICETABLE", "INDEX"}
	// 读取这些段的内容
	segmentResults, _, err := segment.ReadFileSegments(tmpFile, segmentTypes)
	if err != nil {
		return 500, "读取文件段失败"
	}

	// 检查每个读取段的结果是否有错误
	for _, result := range segmentResults {
		if result.Error != nil {
			return 400, "读取文件段中存在错误"
		}
	}

	// 解析出各个段的具体数据
	fileID := segmentResults["FILEID"].Data
	sliceId := segmentResults["SLICEHASH"].Data
	sliceTableData := segmentResults["SLICETABLE"].Data
	indexData := segmentResults["INDEX"].Data

	// 解码片段哈希表
	var sliceTable map[int]core.HashTable
	if err := util.DecodeFromBytes(sliceTableData, &sliceTable); err != nil {
		return 400, "解码片段哈希表失败"
	}

	// 计算总片段数
	var totalPieces int
	for _, v := range sliceTable {
		if !v.IsRsCodes {
			totalPieces++
		}
	}

	// 解码索引
	index32, err := util.FromBytes[int32](indexData)
	if err != nil {
		return 400, "解码索引失败"
	}
	index := int(index32)

	// 创建文件存储服务
	fs, err := afero.NewFileStore(paths.GetSlicePath())
	if err != nil {
		logrus.Errorf("创建文件存储服务失败:%v ", err)
		return 500, "创建文件存储服务失败"
	}

	// 设置文件存储的子目录
	subDir := filepath.Join(sp.P2P.Host().ID().String(), string(fileID))

	// 将文件片段内容写入本地存储
	if err := fs.Write(subDir, string(sliceId), *payload); err != nil {
		logrus.Error("存储接收内容失败, error:", err)
		return 500, "存储接收内容失败"
	}

	// 异步向存储奖励通道发送信息
	go SendStorageInfo(sp.StorageChan, string(fileID), string(sliceId), totalPieces, index, sp.P2P.Host().ID().String())

	// 设置成功的响应消息
	res.Data = []byte(sp.P2P.Host().ID().String()) // 响应数据（主机地址）

	// 返回200状态码和成功消息
	return 200, "成功"
}

// SendStorageInfo 向存储奖励通道发送信息
func SendStorageInfo(storageChans chan *core.StorageChan, fileID, sliceHash string, totalPieces, index int, peerIDs string) {
	storageInfo := &core.StorageChan{
		FileID:      fileID,      // 文件的唯一标识(外部标识)
		SliceHash:   sliceHash,   // 文件片段的哈希值(外部标识)
		TotalPieces: totalPieces, // 文件总片数
		Index:       index,       // 文件片段的索引(该片段在文件中的顺序位置)
		Pid:         peerIDs,     // 节点ID
	}
	storageChans <- storageInfo
}

func CreateTempFile(payload []byte) (file *os.File, err error) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "prefix-")
	if err != nil {
		logrus.Error("创建临时文件失败, error:", err)

		return nil, err
	}

	//  写入临时文件
	_, err = tmpFile.Write(payload)
	if err != nil {
		logrus.Error("写入临时文件失败, error:", err)
		return nil, err
	}

	return tmpFile, nil

}
