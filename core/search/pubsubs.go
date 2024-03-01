package search

import (
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/core/util"
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

	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	switch res.Message.Type {
	// 文件的唯一标识
	case "fileID":
		// 搜索修改请求
		payload := new(FileAddSearchRequestPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}

		rows, err := sqlite.SelectSharedByFileIDDatabaseStatus(db, payload.Value)
		if err != nil {
			return
		}

		// 新增搜索响应
		var responsePayload []core.SearchChan
		for rows.Next() {
			rp := new(core.SearchChan)
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
		logrus.Printf("搜到到%v", responsePayload)
		for _, value := range responsePayload {
			if err := network.SendPubSub(p2p, pubsub, config.PubsubAddSearchResponseTopic, "fileID", receiver, value); err == nil {
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

		rows, err := sqlite.SelectSharedByNameDatabaseStatus(db, payload.Value)
		if err != nil {
			return
		}

		// 新增搜索响应
		var responsePayload []core.SearchChan
		for rows.Next() {
			rp := new(core.SearchChan)
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
		logrus.Printf("搜到到%v", responsePayload)
		if err := network.SendPubSub(p2p, pubsub, config.PubsubAddSearchResponseTopic, "name", receiver, responsePayload); err == nil {
			return // 成功后直接退出
		}
	}
}

// HandleAddSearchResponsePubSub 处理新增搜索响应的订阅消息
func HandleAddSearchResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, searchChan chan *core.SearchChan, res *streams.RequestMessage) {
	switch res.Message.Type {
	// 文件的唯一标识
	case "fileID":
		payload := new(core.SearchChan)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}
		searchChan <- payload // 将结果发送到通道

	// 文件的名称
	case "name":
		var payloads []core.SearchChan
		if err := util.DecodeFromBytes(res.Payload, &payloads); err != nil {
			return
		}
		for _, payload := range payloads {
			tempPayload := payload     // 创建 payload 的副本以避免循环变量共享问题
			searchChan <- &tempPayload // 将每个结果发送到通道
		}
	}
}
