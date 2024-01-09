package upload

import (
	"context"
	"fmt"
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

	Registry *eventbus.EventRegistry // 事件总线
	Cache    *ristretto.Cache        // 缓存实例
	Pool     *pool.MemoryPool        // 内存池
}

// HandleStreamFileSliceUploadStream 处理文件片段上传的流消息
func (sp *StreamProtocol) HandleStreamFileSliceUploadStream(req *streams.RequestMessage, res *streams.ResponseMessage) error {
	/////////////////////// 以后要改进 ///////////////////////
	// TODO:先用临时文件的形式
	payload := new([]byte)
	if err := util.DecodeFromBytes(req.Payload, payload); err != nil {
		logrus.Errorf("[HandleStreamFileSliceUploadStream] 解码失败:\t%v", err)
		return err
	}

	tmpFile, err := CreateTempFile(*payload)
	if err != nil {
		logrus.Errorf("创建临时文件失败:%v", err)
		return err
	}

	// 最后删除临时文件
	defer func() {
		os.Remove(tmpFile.Name())
		tmpFile.Close()
	}()

	segmentTypes := []string{
		"FILEID",
		"SLICEHASH",
	}
	segmentResults, _, err := segment.ReadFileSegments(tmpFile, segmentTypes)
	if err != nil {
		return err
	}

	// 检查并提取每个段的数据
	for _, result := range segmentResults {
		if result.Error != nil {
			return result.Error
		}
	}

	fileID := segmentResults["FILEID"].Data     // 读取文件的唯一标识
	sliceId := segmentResults["SLICEHASH"].Data // 读取文件片段的哈希值

	// ReadSegment 从文件读取段 FILEID
	// fileID, err := segment.ReadSegmentToFile(tmpFile, "FILEID", xref)
	// if err != nil {
	// 	logrus.Errorf("从文件加载 FILEID 表失败:%v", err)

	// 	return err
	// }

	// ReadSegment 从文件读取段 SLICEID
	// sliceId, err := segment.ReadSegmentToFile(tmpFile, "SLICEHASH", xref)
	// if err != nil {
	// 	logrus.Errorf("从文件加载 SLICEHASH 表失败:%v", err)
	// 	return err
	// }

	// 新建文件存储
	fs, err := afero.NewFileStore(paths.SlicePath)
	if err != nil {
		logrus.Errorf("创建新建文件存储失败:%v ", err)
		return err
	}
	// 子目录当前主机+文件hash
	subDir := filepath.Join(sp.P2P.Host().ID().String(), string(fileID)) // 设置子目录

	// 写入本地文件
	if err := fs.Write(subDir, string(sliceId), *payload); err != nil {
		logrus.Error("存储接收内容失败, error:", err)
		return fmt.Errorf("请求无法处理")
	}
	// 组装响应数据
	res.Code = 200                                 // 响应代码
	res.Msg = "成功"                                 // 响应消息
	res.Data = []byte(sp.P2P.Host().ID().String()) // 响应数据(主机地址)

	return nil
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
