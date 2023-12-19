package defs

import (
	"context"
	"time"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/kbucket"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	protocols "github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sirupsen/logrus"
)

// sendStream 向指定的节点发流消息
// protocol		协议
// genre		类型
// receiver		接收方ID
// data			内容
func sendStream(p2p *dep2p.DeP2P, protocol, genre string, receiver peer.ID, data interface{}) ([]byte, error) {
	ctx, cancel := context.WithTimeout(p2p.Context(), time.Second*10)
	defer cancel()

	// 编码
	payloadBytes, err := EncodeToBytes(data)
	//payloadBytes, err := toBytes(data)
	if err != nil {
		logrus.Errorf("[sendStream] 编码失败:\t%v", err)
		return nil, err
	}

	// 请求消息
	request := &streams.RequestMessage{
		Payload: payloadBytes,
		Message: &streams.Message{
			Sender:   p2p.Host().ID().String(), // 发送方ID
			Receiver: receiver.String(),        // 接收方ID
		},
	}
	if genre != "" {
		request.Message.Type = genre // 消息类型
	}

	// 序列化
	requestBytes, err := request.Marshal()
	if err != nil {
		return nil, err
	}

	stream, err := p2p.Host().NewStream(ctx, receiver, protocols.ID(protocol))
	if err != nil {
		return nil, err
	}
	defer func() {
		stream.Close()       // 执行完之后关闭流
		streamMutex.Unlock() // 执行完之后解除锁
	}()
	_ = stream.SetDeadline(time.Now().Add(time.Second * 10))

	// 将消息写入流
	if err = streams.WriteStream(requestBytes, stream); err != nil {
		return nil, err
	}

	// 从流中读取消息
	return streams.ReadStream(stream)
}

// SendPubSub 向指定的节点发送订阅消息
// topic		主题
// genre		类型
// receiver		接收方ID
// data			内容
func SendPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, topic, genre string, receiver peer.ID, data interface{}) error {
	// 编码
	payloadBytes, err := EncodeToBytes(data)
	if err != nil {
		logrus.Errorf("[SendPubSub] 编码失败:\t%v", err)
		return err
	}

	// 请求消息
	request := &streams.RequestMessage{
		Payload: payloadBytes,
		Message: &streams.Message{
			Sender: p2p.Host().ID().String(), // 发送方ID
		},
	}
	if genre != "" {
		request.Message.Type = genre // 消息类型
	}
	if receiver != "" {
		request.Message.Receiver = receiver.String() // 接收方ID
	}

	// 序列化
	requestBytes, err := request.Marshal()
	if err != nil {
		logrus.Errorf("[SendPubSub] 序列化失败:\t%v", err)
		return err
	}

	// 发送请求
	if err := pubsub.BroadcastWithTopic(topic, requestBytes); err != nil {
		logrus.Errorf("[SendPubSub] 发送失败:\t%v", err)
		return err
	}

	return nil
}

// SendDataToPeer 尝试先通过流发送数据，失败后通过订阅发送
// protocol		协议
// topic		主题
// genre		类型
// receiver		接收方ID
// data			内容
func SendDataToPeer(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, protocol, topic, genre string, receiver peer.ID, data interface{}) error {
	shouldSendPubSub := false
	streamMutex.Lock()
	// 尝试通过流发送数据
	responseByte, err := sendStream(p2p, protocol, genre, receiver, data) // 协议
	if err != nil {
		// 流发送失败，标记为需要通过订阅发送
		shouldSendPubSub = true
	} else {
		var response streams.ResponseMessage
		if err := response.Unmarshal(responseByte); err != nil {
			// 解析响应失败，标记为需要通过订阅发送
			shouldSendPubSub = true
		} else if response.Code != 200 {
			// 响应状态码不是 200，标记为需要通过订阅发送
			shouldSendPubSub = true
		}
	}

	// 如果需要，通过订阅发送数据
	if shouldSendPubSub {
		return SendPubSub(p2p, pubsub, topic, genre, receiver, data) // 主题
	}

	// 数据通过流成功发送，无需进一步操作
	return nil
}

// sendSlice 向指定的 peer 发送文件片段
func sendSlice(opt *Options, p2p *dep2p.DeP2P, sliceHash string, sliceByte []byte) (*streams.ResponseMessage, peer.ID, error) {
	for i := 0; ; {
		// 返回与给定 ID 最接近的 "count" 对等点的列表
		receiverPeers := p2p.RoutingTable(2).NearestPeers(kbucket.ConvertKey(sliceHash), i+1)

		// TODO:只是为了测试后面删除,因为没有失败在处理的逻辑
		if p2p.RoutingTable(2).Size() < int(opt.routingTableLow) { // 路由表中连接的最小节点数量
			// 延时1秒后退出
			time.Sleep(1 * time.Second)
			continue
		}

		if len(receiverPeers) == i {
			// 应该放入本地后面再处理
			return nil, "", nil
		}
		if len(receiverPeers) == 0 {
			i++
			continue
		}
		if i >= len(receiverPeers) {
			i = len(receiverPeers) - 1
		}
		if i < 0 {
			i = 0
		}

		// 向指定的节点发流消息
		responseByte, err := sendStream(p2p, StreamFileSliceUploadProtocol, "", receiverPeers[i], sliceByte)
		if err != nil {
			i++
			continue
		}
		var response streams.ResponseMessage
		if err := response.Unmarshal(responseByte); err != nil {
			return nil, "", err
		}

		return &response, receiverPeers[i], nil
	}

}
