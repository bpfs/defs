package search

import (
	"path/filepath"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// HandleAddSearchRequestPubSub 处理新增搜索请求的订阅消息
func HandleAddSearchRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := afero.NewFileStore(filepath.Join(paths.SlicePath, p2p.Host().ID().String()))
	if err != nil {
		return
	}

	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	switch res.Message.Type {
	// 文件的唯一标识
	case "fileID":
		// 文件名称修改请求
		payload := new(FileAddSearchRequestPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(payload.Value)
		if err != nil {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.Value, sliceHash)
			if err != nil {
				continue
			}

			// TODO: 需要的字段
			segmentTypes := []string{
				"FILEID",      // 文件的唯一标识
				"NAME",        // 文件的名称
				"SIZE",        // 文件的长度
				"MODTIME",     // 文件的修改时间
				"UPLOADTIME",  // 文件的上传时间
				"P2PKHSCRIPT", // 文件的 P2PKH 脚本
			}
			segmentResults, _, err := segment.ReadFileSegments(sliceFile, segmentTypes)
			if err != nil {
				continue
			}

			// 检查并提取每个段的数据
			for _, result := range segmentResults {
				if result.Error != nil {
					continue
				}
			}

			fileID := string(segmentResults["FILEID"].Data)
			if fileID != payload.Value {
				continue
			}

			size, err := util.FromBytes[int64](segmentResults["SIZE"].Data)
			if err != nil {
				continue
			}

			modTimeUnix, err := util.FromBytes[int64](segmentResults["MODTIME"].Data)
			if err != nil {
				continue
			}
			modTime := time.Unix(modTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time

			uploadTimeUnix, err := util.FromBytes[int64](segmentResults["UPLOADTIME"].Data)
			if err != nil {
				continue
			}
			uploadTime := time.Unix(uploadTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time

			// 新增搜索响应
			responsePayload := SearchChan{
				MD5:        payload.MD5,                         // 请求值的MD5哈希
				FileID:     fileID,                              // 文件的唯一标识
				Name:       string(segmentResults["NAME"].Data), // 文件的名称
				Size:       size,                                // 文件的长度(以字节为单位)
				UploadTime: uploadTime,                          // 上传时间
				ModTime:    modTime,                             // 修改时间(非文件修改时间)
			}

			if err := network.SendPubSub(p2p, pubsub, config.PubsubAddSearchResponseTopic, "fileID", receiver, responsePayload); err == nil {
				return // 成功后直接退出
			}
		}

	// 文件的名称
	case "name":
		// 文件共享修改请求
		payload := new(FileAddSearchRequestPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}

		rows, err := sqlite.SelectSharedDatabaseStatus(db, payload.Value)
		if err != nil {
			return
		}

		// 新增搜索响应
		var responsePayload []SearchChan
		for rows.Next() {
			rp := new(SearchChan)
			if err := rows.Scan(
				&rp.FileID,     // 文件的唯一标识
				&rp.Name,       // 文件的名称
				&rp.Size,       // 文件的长度(以字节为单位)
				&rp.UploadTime, // 上传时间
				&rp.ModTime,    // 修改时间
				&rp.Xref,       // Xref表中段的数量
			); err != nil {
				continue
			}
			rp.MD5 = payload.MD5 // 请求值的MD5哈希

			responsePayload = append(responsePayload, *rp)
		}

		if err := network.SendPubSub(p2p, pubsub, config.PubsubAddSearchResponseTopic, "name", receiver, responsePayload); err == nil {
			return // 成功后直接退出
		}
	}
}

// HandleAddSearchResponsePubSub 处理新增搜索响应的订阅消息
func HandleAddSearchResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, searchChan chan *SearchChan, res *streams.RequestMessage) {
	switch res.Message.Type {
	// 文件的唯一标识
	case "fileID":
		payload := new(SearchChan)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}
		searchChan <- payload // 将结果发送到通道

	// 文件的名称
	case "name":
		var payloads []SearchChan
		if err := util.DecodeFromBytes(res.Payload, &payloads); err != nil {
			return
		}
		for _, payload := range payloads {
			tempPayload := payload     // 创建 payload 的副本以避免循环变量共享问题
			searchChan <- &tempPayload // 将每个结果发送到通道
		}
	}
}
