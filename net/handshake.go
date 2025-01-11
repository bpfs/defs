// Package defs 提供了DeFS去中心化存储系统的网络握手协议实现
package net

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/libp2p/core/network"
	"github.com/dep2p/libp2p/core/peer"
	"github.com/dep2p/libp2p/core/peerstore"
	"github.com/dep2p/libp2p/core/protocol"
	logging "github.com/dep2p/log"
)

var logger = logging.Logger("net")

// 常量定义
const (
	HandshakeProtocol = "/defs/handshake/1.0.0" // 握手协议标识符
	maxMessageSize    = 1 << 20                 // 最大消息大小限制为1MB
	maxPeersToShare   = 100                     // 单次握手最多分享的节点数
	maxRetries        = 3                       // 最大重试次数
	initialRetryDelay = 10 * time.Second        // 初始重试延迟
	maxRetryDelay     = 50 * time.Second        // 最大重试延迟
	connectTimeout    = 30 * time.Second
)

// HandshakeMessage 定义握手消息的格式
type HandshakeMessage struct {
	Version    string          // 协议版本号,用于版本兼容性检查
	NodeID     peer.ID         // 发送消息节点的唯一标识符
	KnownPeers []peer.AddrInfo // 发送节点已知的其他节点的地址信息列表
}

// Handshake 执行与目标节点的握手过程
// 参数:
//   - ctx: 上下文对象,用于控制握手过程的生命周期
//   - h: libp2p主机实例,提供网络通信功能
//   - pi: 目标节点的地址信息,包含节点ID和多地址
//
// 返回值:
//   - []peer.AddrInfo: 从目标节点获取的其他节点的地址信息列表
//   - error: 如果握手过程中发生错误则返回错误信息
//
// 注意：
// 1. 连接池中累积了大量失效连接
// 2. 旧连接没有及时释放导致资源耗尽
// 3. 连接状态不一致导致新连接建立失败
//
// 建议：
// 1. 定期清理空闲连接
// 2. 实现心跳机制检测连接活性
// 3. 添加连接池大小限制
// 4. 监控并记录连接状态变化
func Handshake(ctx context.Context, h host.Host, pi peer.AddrInfo) ([]peer.AddrInfo, error) {
	var lastErr error

	// 创建带超时的上下文
	// connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	// defer cancel()

	// 实现指数退避重试逻辑
	for attempt := 0; attempt < maxRetries; attempt++ {
		// 计算本次重试的延迟时间
		if attempt > 0 {
			// 使用指数退避计算延迟时间: initialRetryDelay * 2^(attempt-1)
			delay := initialRetryDelay * time.Duration(1<<(attempt-1))
			// 确保延迟时间不超过最大值
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}

			// 添加随机抖动，避免多个节点同时重试
			jitter := time.Duration(rand.Int63n(int64(delay) / 2))
			delay = delay + jitter

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				logger.Debugf("上下文取消: %v", ctx.Err())
				return nil, fmt.Errorf("上下文取消")
			}
			logger.Errorf("正在进行第 %d 次重试连接节点 %s，延迟时间: %v",
				attempt+1, pi.ID, delay)
		}

		// 每次重试前，确保清理已有连接
		h.Network().ClosePeer(pi.ID)

		// 尝试建立连接
		// 问题点1: 当连接失败时，没有显式关闭已建立的连接
		// if err := h.Connect(connectCtx, pi); err != nil {
		// 	lastErr = err
		// 	logger.Errorf("连接节点 %s 失败 (尝试 %d/%d): %v", pi.ID, attempt+1, maxRetries, err)
		// 	continue
		// }

		// 后续的握手流程保持不变
		stream, err := h.NewStream(ctx, pi.ID, protocol.ID(HandshakeProtocol))
		if err != nil {
			lastErr = err
			logger.Errorf("打开流失败 (尝试 %d/%d): %v", attempt+1, maxRetries, err)
			h.Network().ClosePeer(pi.ID) // 显式关闭连接
			continue                     // 问题点2: stream创建失败时，底层连接可能仍然保持
		}
		defer func() {
			stream.Close()
			// 如果发生错误，确保关闭与该节点的连接
			if lastErr != nil {
				h.Network().ClosePeer(pi.ID)
			}
		}()

		// 3. 构造本地节点的握手消息
		msg := HandshakeMessage{
			Version:    "1.0.0",
			NodeID:     h.ID(),
			KnownPeers: getPeerAddrInfos(h, maxPeersToShare),
		}

		// 4. 向目标节点发送握手消息
		if err := WriteHandshakeMessage(stream, msg); err != nil {
			lastErr = err
			logger.Errorf("发送握手消息失败: %v", err)
			continue
		}

		// 5. 接收目标节点的响应消息
		response, err := ReadHandshakeMessage(stream)
		if err != nil {
			lastErr = err
			logger.Errorf("接收握手消息失败: %v", err)
			continue
		}

		// 6. 处理响应中包含的节点信息
		for _, peerInfo := range response.KnownPeers {
			if len(peerInfo.Addrs) > 0 {
				// 将新发现的节点添加到本地节点存储,设置永久有效期
				h.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, peerstore.ConnectedAddrTTL)
			}
		}

		// 如果执行到这里，说明握手成功，处理响应并返回
		logger.Infof("成功与节点 %s 完成握手 (尝试 %d/%d)", pi.ID, attempt+1, maxRetries)
		return response.KnownPeers, nil
	}

	// 所有重试都失败后返回最后一次的错误
	return nil, fmt.Errorf("握手失败，已重试 %d 次: %w", maxRetries, lastErr)
}

// RegisterHandshakeProtocol 注册握手协议的处理函数
// 参数:
//   - h: libp2p主机实例,用于注册协议处理器
func RegisterHandshakeProtocol(h host.Host) {
	// 为握手协议设置流处理器
	h.SetStreamHandler(protocol.ID(HandshakeProtocol), func(s network.Stream) {
		defer s.Close() // 确保流在处理完成后关闭

		// 1. 读取对方发送的握手消息
		msg, err := ReadHandshakeMessage(s)
		if err != nil {
			logger.Errorf("读取握手消息失败: %v", err)
			return
		}

		// 2. 获取远程节点的网络信息
		remoteAddr := s.Conn().RemoteMultiaddr() // 获取远程节点的多地址
		remotePeer := s.Conn().RemotePeer()      // 获取远程节点的ID

		// 3. 将远程节点信息添加到本地节点存储
		h.Peerstore().AddAddr(remotePeer, remoteAddr, peerstore.ConnectedAddrTTL)

		// 4. 处理收到的其他节点信息
		processedCount := 0
		for _, peerInfo := range msg.KnownPeers {
			if processedCount >= maxPeersToShare {
				break
			}
			if len(peerInfo.Addrs) > 0 {
				// 将新发现的节点添加到本地节点存储
				h.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, peerstore.ConnectedAddrTTL)
				processedCount++
			}
		}

		// 5. 构造响应消息
		response := HandshakeMessage{
			Version:    "1.0.0",
			NodeID:     h.ID(),
			KnownPeers: getPeerAddrInfos(h, maxPeersToShare),
		}

		// 6. 发送响应消息
		if err := WriteHandshakeMessage(s, response); err != nil {
			logger.Errorf("发送响应失败: %v", err)
			return
		}

		// 记录握手完成的日志
		logger.Debugf("完成与节点 %s 的握手，处理了 %d 个已知节点", remotePeer.String(), processedCount)
	})
}

// getPeerAddrInfos 获取主机已知的所有节点的地址信息
// 参数:
//   - h: libp2p主机实例,用于访问节点存储
//   - limit: 返回的最大节点数量
//
// 返回值:
//   - []peer.AddrInfo: 包含节点ID和多地址的节点信息列表
func getPeerAddrInfos(h host.Host, limit int) []peer.AddrInfo {
	var (
		peerInfos []peer.AddrInfo // 存储节点信息的切片
		count     = 0             // 已处理的节点计数器
	)

	// 遍历所有已知节点
	for _, peerID := range h.Peerstore().Peers() {
		// 跳过自身节点
		if peerID == h.ID() {
			continue
		}

		// 获取节点地址
		addrs := h.Peerstore().Addrs(peerID)
		if len(addrs) > 0 {
			// 将节点信息添加到结果列表
			peerInfos = append(peerInfos, peer.AddrInfo{
				ID:    peerID,
				Addrs: addrs,
			})

			// 增加计数器并检查是否达到限制
			count++
			if count >= limit {
				break
			}
		}
	}
	return peerInfos
}

// WriteHandshakeMessage 将握手消息写入网络流
// 参数:
//   - s: 网络流,用于发送数据
//   - msg: 要发送的握手消息
//
// 返回值:
//   - error: 如果写入过程中发生错误则返回错误信息
func WriteHandshakeMessage(s network.Stream, msg HandshakeMessage) error {
	// 1. 将消息对象序列化为JSON格式的字节数组
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Errorf("序列化消息失败: %v", err)
		return err
	}

	// 2. 写入消息长度(4字节的无符号整数)
	length := uint32(len(data))
	if err := binary.Write(s, binary.BigEndian, length); err != nil {
		logger.Errorf("写入消息长度失败: %v", err)
		return err
	}

	// 3. 写入消息内容
	if _, err := s.Write(data); err != nil {
		logger.Errorf("写入消息内容失败: %v", err)
		return err
	}

	return nil
}

// ReadHandshakeMessage 从网络流中读取握手消息
// 参数:
//   - s: 网络流,用于接收数据
//
// 返回值:
//   - HandshakeMessage: 读取到的握手消息
//   - error: 如果读取过程中发生错误则返回错误信息
func ReadHandshakeMessage(s network.Stream) (HandshakeMessage, error) {
	// 1. 读取消息长度(4字节的无符号整数)
	var length uint32
	if err := binary.Read(s, binary.BigEndian, &length); err != nil {
		logger.Errorf("读取消息长度失败: %v", err)
		return HandshakeMessage{}, err
	}

	// 2. 检查消息长度是否超出限制
	if length > maxMessageSize {
		logger.Errorf("消息长度超出限制: %d > %d", length, maxMessageSize)
		return HandshakeMessage{}, fmt.Errorf("消息长度超出限制: %d > %d", length, maxMessageSize)
	}

	// 3. 读取指定长度的消息内容
	data := make([]byte, length)
	if _, err := io.ReadFull(s, data); err != nil {
		logger.Errorf("读取消息内容失败: %v", err)
		return HandshakeMessage{}, err
	}

	// 4. 将JSON格式的消息反序列化为消息对象
	var msg HandshakeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		logger.Errorf("反序列化消息失败: %v", err)
		return HandshakeMessage{}, err
	}

	return msg, nil
}
