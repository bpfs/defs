package delete

import (
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/sirupsen/logrus"
)

// HandleFileDownloadResponsePubSub 处理文件删除响应的订阅消息
func HandleFileDeleteRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := afero.NewFileStore(filepath.Join(paths.SlicePath, p2p.Host().ID().String()))
	if err != nil {
		return
	}

	// 文件删除请求
	payload := new(FileDeleteRequestPayload)
	if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
		logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
		return
	}
	// 子目录当前主机+文件hash
	subDir := string(payload.FileID)

	// 列出指定子目录中的所有文件
	slices, err := fs.ListFiles(subDir)
	if err != nil {
		return
	}
	// 计数器
	counter := 0
	for _, sliceHash := range slices {
		sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
		if err != nil {
			logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
			continue
		}

		// 读取文件的 P2PKH 脚本
		p2pkhScriptData, _, err := segment.ReadFileSegment(sliceFile, "P2PKHSCRIPT")
		if err != nil {
			continue
		}

		// 验证脚本中的公钥哈希
		if script.VerifyScriptPubKeyHash(p2pkhScriptData, payload.PubKeyHash) {
			if err := fs.Delete(subDir, sliceHash); err != nil {
				logrus.Errorf("%s 删除文件失败:\t%v", sliceHash, err)
				continue
			}
		}
		counter++
	}
	if counter == len(slices) {
		// 把父级文件夹删除
		fs.DeleteAll(subDir)
	}
}
