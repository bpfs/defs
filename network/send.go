package network

import (
	"context"
	"sync"
	"time"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/util"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	protocols "github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sirupsen/logrus"
)

// 流互斥锁
var StreamMutex sync.Mutex

// SendStream 向指定的节点发流消息
// protocol		协议
// genre		类型
// receiver		接收方ID
// data			内容
func SendStream(p2p *dep2p.DeP2P, protocol, genre string, receiver peer.ID, data interface{}) (*streams.ResponseMessage, error) {
	ctx, cancel := context.WithTimeout(p2p.Context(), time.Second*10)
	defer cancel()

	// 编码
	payloadBytes, err := util.EncodeToBytes(data)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
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
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	//StreamMutex.Lock()
	stream, err := p2p.Host().NewStream(ctx, receiver, protocols.ID(protocol))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	defer func() {
		stream.Close()       // 执行完之后关闭流
		StreamMutex.Unlock() // 执行完之后解除锁
	}()

	_ = stream.SetDeadline(time.Now().UTC().Add(time.Second * 10))

	// 将消息写入流
	if err = streams.WriteStream(requestBytes, stream); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 从流中读取返回的消息
	responseByte, err := streams.ReadStream(stream)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 返回的信息为空，直接退出
	if len(responseByte) == 0 {
		return nil, nil
	}

	response := new(streams.ResponseMessage)
	if err := response.Unmarshal(responseByte); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	return response, nil
}

// SendPubSub 向指定的节点发送订阅消息
// topic		主题
// genre		类型
// receiver		接收方ID
// data			内容
func SendPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, topic, genre string, receiver peer.ID, data interface{}) error {
	// 编码
	payloadBytes, err := util.EncodeToBytes(data)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
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
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 发送请求
	if err := pubsub.BroadcastWithTopic(topic, requestBytes); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
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
// func SendDataToPeer(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, protocol, topic, genre string, receiver peer.ID, data interface{}) error {
// 	shouldSendPubSub := false
// 	StreamMutex.Lock()
// 	// 尝试通过流发送数据
// 	responseByte, err := SendStream(p2p, protocol, genre, receiver, data) // 协议
// 	if err != nil {
// 		// 流发送失败，标记为需要通过订阅发送
// 		shouldSendPubSub = true
// 	} else {
// 		var response streams.ResponseMessage
// 		if err := response.Unmarshal(responseByte); err != nil {
// 			// 解析响应失败，标记为需要通过订阅发送
// 			shouldSendPubSub = true
// 		} else if response.Code != 200 {
// 			// 响应状态码不是 200，标记为需要通过订阅发送
// 			shouldSendPubSub = true
// 		}
// 	}

// 	// 如果需要，通过订阅发送数据
// 	if shouldSendPubSub {
// 		return SendPubSub(p2p, pubsub, topic, genre, receiver, data) // 主题
// 	}

// 	// 数据通过流成功发送，无需进一步操作
// 	return nil
// }
