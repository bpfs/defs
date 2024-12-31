package network

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/bpfs/defs/utils/logger"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	protocols "github.com/libp2p/go-libp2p/core/protocol"
)

// StreamMutex 流互斥锁
var StreamMutex sync.Mutex

// SendStream 向指定的节点发送流消息
// 参数:
//   - protocol: 协议
//   - genre: 类型
//   - receiver: 接收方ID
//   - data: 发送的内容
//
// 返回:
//   - *streams.ResponseMessage: 响应消息
//   - error: 错误信息
func SendStream(ctx context.Context, h host.Host, protocol, genre string, receiver peer.ID, data []byte) (*streams.ResponseMessage, error) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	// 构建请求消息
	request := &streams.RequestMessage{
		Payload: data,
		Message: &streams.Message{
			Sender:   h.ID().String(),   // 发送方ID
			Receiver: receiver.String(), // 接收方ID
		},
	}
	if genre != "" {
		request.Message.Type = genre // 设置消息类型
	}

	// 序列化请求消息
	requestBytes, err := request.Marshal()
	if err != nil {
		logger.Errorf("序列化请求消息失败: %v", err)
		return nil, err
	}

	// 创建新的流
	stream, err := h.NewStream(ctx, receiver, protocols.ID(protocol))
	if err != nil {
		logger.Errorf("创建新流失败: %v", err)
		return nil, err
	}
	defer func() {
		stream.Close()       // 关闭流
		StreamMutex.Unlock() // 解锁
	}()

	// 设置流的截止时间
	_ = stream.SetDeadline(time.Now().UTC().Add(time.Second * 10))

	// 将消息写入流
	if err = streams.WriteStream(requestBytes, stream); err != nil {
		logger.Errorf("写入流失败: %v", err)
		return nil, err
	}

	// 从流中读取返回的消息
	responseByte, err := streams.ReadStream(stream)
	if err != nil {
		logger.Errorf("读取流失败: %v", err)
		return nil, err
	}

	// 如果返回的信息为空，直接退出
	if len(responseByte) == 0 {
		return nil, nil
	}

	// 解析响应消息
	response := new(streams.ResponseMessage)
	if err := response.Unmarshal(responseByte); err != nil {
		logger.Errorf("解析响应消息失败: %v", err)
		return nil, err
	}

	return response, nil
}

// SendStreamWithReader 使用流式读取器发送数据到指定节点
// 参数:
//   - protocol: protocols.ID 协议ID
//   - genre: string 消息类型
//   - receiver: peer.ID 目标节点ID
//   - reader: io.Reader 数据读取器
//
// 返回值:
//   - *streams.ResponseMessage: 响应消息
//   - error: 如果发送过程中发生错误，返回相应的错误信息
func SendStreamWithReader(ctx context.Context, h host.Host, protocol, genre string, receiver peer.ID, reader io.Reader) (*streams.ResponseMessage, error) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, time.Second*30) // 增加超时时间以适应大文件
	defer cancel()

	// 创建新的流
	stream, err := h.NewStream(ctx, receiver, protocols.ID(protocol))
	if err != nil {
		logger.Errorf("创建新流失败: %v", err)
		return nil, err
	}
	defer func() {
		stream.Close()       // 关闭流
		StreamMutex.Unlock() // 解锁
	}()

	// 设置流的截止时间
	_ = stream.SetDeadline(time.Now().UTC().Add(time.Second * 30)) // 增加截止时间

	// 构建请求消息头
	header := &streams.RequestMessage{
		Message: &streams.Message{
			Sender:   h.ID().String(),
			Receiver: receiver.String(),
			Type:     genre,
		},
	}

	// 序列化并发送请求消息头
	headerBytes, err := header.Marshal()
	if err != nil {
		logger.Errorf("序列化请求消息头失败: %v", err)
		return nil, err
	}
	if err = streams.WriteStream(headerBytes, stream); err != nil {
		logger.Errorf("写入请求消息头失败: %v", err)
		return nil, err
	}

	// 发送数据
	_, err = io.Copy(stream, reader)
	if err != nil {
		logger.Errorf("发送数据失败: %v", err)
		return nil, err
	}

	// 从流中读取返回的消息
	responseByte, err := streams.ReadStream(stream)
	if err != nil {
		logger.Errorf("读取响应失败: %v", err)
		return nil, err
	}

	// 如果返回的信息为空，直接退出
	if len(responseByte) == 0 {
		return nil, nil
	}

	// 解析响应消息
	response := new(streams.ResponseMessage)
	if err := response.Unmarshal(responseByte); err != nil {
		logger.Errorf("解析响应消息失败: %v", err)
		return nil, err
	}

	return response, nil
}

// SendStreamWithExistingStream 使用已存在的流发送消息
// 参数:
//   - stream: network.Stream 已经建立的流
//   - data: []byte 要发送的数据
//
// 返回:
//   - *streams.ResponseMessage: 响应消息
//   - error: 错误信息
func SendStreamWithExistingStream(stream network.Stream, data []byte) (*streams.ResponseMessage, error) {
	// 设置流的截止时间
	_ = stream.SetDeadline(time.Now().UTC().Add(time.Second * 10))

	// 构建请求消息
	request := &streams.RequestMessage{
		Payload: data,
		Message: &streams.Message{
			Sender:   stream.Conn().LocalPeer().String(),  // 发送方ID
			Receiver: stream.Conn().RemotePeer().String(), // 接收方ID
		},
	}

	// 序列化请求消息
	requestBytes, err := request.Marshal()
	if err != nil {
		logger.Errorf("序列化请求消息失败: %v", err)
		return nil, err
	}

	// 将消息写入流
	if err := streams.WriteStream(requestBytes, stream); err != nil {
		logger.Errorf("写入流失败: %v", err)
		return nil, err
	}

	// 从流中读取返回的消息
	responseByte, err := streams.ReadStream(stream)
	if err != nil {
		logger.Errorf("读取流失败: %v", err)
		return nil, err
	}

	// 如果返回的信息为空，直接退出
	if len(responseByte) == 0 {
		return nil, nil
	}

	// 解析响应消息
	response := new(streams.ResponseMessage)
	if err := response.Unmarshal(responseByte); err != nil {
		logger.Errorf("解析响应消息失败: %v", err)
		return nil, err
	}

	return response, nil
}
