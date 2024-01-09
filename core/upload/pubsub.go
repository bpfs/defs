package upload

import (
	"path/filepath"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sirupsen/logrus"
)

// HandleFileUploadRequestPubSub 处理文件上传请求的订阅消息
func HandleFileUploadRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := afero.NewFileStore(paths.SlicePath)
	if err != nil {
		return
	}

	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	// 文件上传请求(检查)
	payload := new(FileUploadRequestCheckPayload)
	if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
		logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
		return
	}

	// 子目录当前主机+文件hash
	subDir := filepath.Join(p2p.Host().ID().String(), string(payload.FileID)) // 设置子目录

	switch res.Message.Type {
	// 检查
	case "check":
		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)
		if err != nil {
			logrus.Errorf("[check] 获取切片失败:\t%v", err)
			return
		}

		// 文件不存在
		if len(slices) == 0 {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
			if err != nil {
				logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
				continue
			}

			// 从文件读取上传时间
			uploadTimeBytes, _, err := segment.ReadFileSegment(sliceFile, "UPLOADTIME")
			if err != nil {
				continue
			}
			uploadTimeUnix, err := util.FromBytes[int64](uploadTimeBytes)
			if err != nil {
				continue
			}
			uploadTime := time.Unix(uploadTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time
			// 检查接收到的上传时间与文件存储的上传时间是否一致
			// 一致则表示为本次存储内容
			if payload.UploadTime.Equal(uploadTime) {
				continue
			}
			// 否则，回复消息告知已存在
			// 向指定的指定节点发送文件上传响应的订阅消息
			if err := network.SendPubSub(p2p, pubsub, config.PubsubFileUploadResponseTopic, "exist", receiver, payload); err != nil {
				return
			}

		}

	// 撤销
	case "cancel":
		// TODO: 删除本地切片
		logrus.Printf("需删除本地:\t%s\t文件片段\n", payload.FileID)

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(subDir)
		if err != nil {
			logrus.Errorf("[cancel ]获取切片失败:\t%v", err)
			return
		}

		// 文件不存在
		if len(slices) == 0 {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
			if err != nil {
				logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
				continue
			}

			// 从文件读取上传时间
			uploadTimeBytes, _, err := segment.ReadFileSegment(sliceFile, "UPLOADTIME")
			if err != nil {
				continue
			}
			uploadTimeUnix, err := util.FromBytes[int64](uploadTimeBytes)
			if err != nil {
				continue
			}
			uploadTime := time.Unix(uploadTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time
			// 检查接收到的上传时间与文件存储的上传时间是否一致
			// 一致则表示为本次存储内容
			if payload.UploadTime.Equal(uploadTime) {
				// 删除该文件片段
				_ = fs.Delete(payload.FileID, sliceHash)
			}
		}
	}
}

// HandleFileUploadResponsePubSub 处理文件上传响应的订阅消息
func HandleFileUploadResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, pool *pool.MemoryPool, res *streams.RequestMessage) {
	// receiver, err := peer.Decode(res.Message.Sender)
	// if err != nil {
	// 	logrus.Errorf("解析peerid失败:\t%v", err)
	// 	return
	// }

	switch res.Message.Type {
	// 存在
	case "exist":
		// 文件上传请求(检查)
		payload := new(FileUploadRequestCheckPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
			return
		}

		// 暂停指定的上传任务
		if err := pool.PauseUploadTask(payload.FileID); err != nil {
			return
		}

		// 向指定的指定节点发送文件上传响应的订阅消息
		if err := network.SendPubSub(p2p, pubsub, config.PubsubFileUploadResponseTopic, "cancel", "", payload); err != nil {
			return
		}
	}
}
