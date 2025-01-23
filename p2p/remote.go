// 功能：处理远程节点的P2P通信设置和操作。
// 主要职责：
// 	管理与远程节点的连接和通信。
// 	处理与远程节点相关的连接请求和数据传输。
// 	提供远程节点的状态信息，如已建立的连接、正在进行的流等。
// 典型操作：配置远程节点连接，建立和管理与远程节点的通信，提供远程节点状态。

package p2p

import (
	"context"
	"fmt"

	net "github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/protocol"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

var maPrefix = "/" + ma.ProtocolWithCode(ma.P_IPFS).Name + "/" // 定义多地址前缀

// remoteListener 接受libp2p流并将其代理到manet主机
type remoteListener struct {
	p2p *P2P // P2P 实例

	// 应用协议标识符
	proto protocol.ID

	// 代理传入连接的地址
	addr ma.Multiaddr

	// 如果设置为true，则处理程序在转发任何数据之前发送'<base58 remote peerid>\n'到目标
	reportRemote bool
}

// ForwardRemote 创建一个新的p2p监听器
// 参数：
//   - ctx: context.Context，上下文用于控制生命周期
//   - proto: protocol.ID，协议ID
//   - addr: ma.Multiaddr，目标地址
//   - reportRemote: bool，是否报告远程对等节点
//
// 返回：
//   - Listener，返回监听器接口
//   - error，返回可能发生的错误
func (p2p *P2P) ForwardRemote(ctx context.Context, proto protocol.ID, addr ma.Multiaddr, reportRemote bool) (Listener, error) {
	listener := &remoteListener{
		p2p: p2p, // 绑定P2P实例

		proto: proto, // 设置协议ID
		addr:  addr,  // 设置目标地址

		reportRemote: reportRemote, // 设置是否报告远程对等节点
	}

	if err := p2p.ListenersP2P.Register(listener); err != nil { // 注册监听器
		log.Debugf("注册监听器失败 %s/%s", listener.addr, listener.proto) // 记录错误
		return nil, err                                            // 如果出错，返回错误
	}

	return listener, nil // 返回监听器
}

// handleStream 处理传入的libp2p流
// 参数：
//   - remote: net.Stream，远程libp2p流
func (l *remoteListener) handleStream(remote net.Stream) {
	local, err := manet.Dial(l.addr) // 拨号连接到目标地址
	if err != nil {
		log.Debugf("拨号连接到目标地址失败 %s/%s", l.addr, l.proto) // 记录错误
		_ = remote.Reset()                               // 如果出错，重置远程流
		return
	}

	peer := remote.Conn().RemotePeer() // 获取远程对等节点

	if l.reportRemote {
		if _, err := fmt.Fprintf(local, "%s\n", peer); err != nil { // 发送远程对等节点ID
			log.Debugf("发送远程对等节点ID失败 %s/%s", peer, l.proto) // 记录错误
			_ = remote.Reset()                              // 如果出错，重置远程流
			return
		}
	}

	peerMa, err := ma.NewMultiaddr(maPrefix + peer.String()) // 创建对等节点多地址
	if err != nil {
		log.Debugf("创建对等节点多地址失败 %s/%s", peer, l.proto) // 记录错误
		_ = remote.Reset()                             // 如果出错，重置远程流
		return
	}

	// 创建新的Stream结构体
	stream := &Stream{
		Protocol: l.proto, // 设置协议ID

		OriginAddr: peerMa, // 设置原地址
		TargetAddr: l.addr, // 设置目标地址
		peer:       peer,   // 设置对等节点ID

		Local:  local,  // 本地连接
		Remote: remote, // 远程流

		Registry: l.p2p.Streams, // 流注册表
	}

	l.p2p.Streams.Register(stream) // 注册新流
}

// Protocol 获取协议ID
// 返回：
//   - protocol.ID，返回协议ID
func (l *remoteListener) Protocol() protocol.ID {
	return l.proto // 返回协议ID
}

// ListenAddress 获取监听地址
// 返回：
//   - ma.Multiaddr，返回监听地址
func (l *remoteListener) ListenAddress() ma.Multiaddr {
	addr, err := ma.NewMultiaddr(maPrefix + l.p2p.identity.String()) // 创建监听地址
	if err != nil {
		panic(err) // 如果出错，触发恐慌
	}
	return addr // 返回监听地址
}

// TargetAddress 获取目标地址
// 返回：
//   - ma.Multiaddr，返回目标地址
func (l *remoteListener) TargetAddress() ma.Multiaddr {
	return l.addr // 返回目标地址
}

// close 关闭监听器
func (l *remoteListener) close() {}

// key 获取监听器的键（协议ID）
// 返回：
//   - protocol.ID，返回协议ID
func (l *remoteListener) key() protocol.ID {
	return l.proto // 返回协议ID
}
