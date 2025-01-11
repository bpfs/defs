// 功能：处理本地节点的P2P通信设置和操作。
// 主要职责：
//  管理本地节点的P2P配置，包括连接管理和资源分配。
//  处理本地节点与其他节点之间的连接和通信。
//  提供本地节点的状态信息，如已建立的连接、正在进行的流等。
// 典型操作：配置本地节点，建立和管理连接，提供节点状态。

package p2p

import (
	"context"
	"time"

	net "github.com/dep2p/libp2p/core/network"
	"github.com/dep2p/libp2p/core/peer"
	"github.com/dep2p/libp2p/core/protocol"
	tec "github.com/jbenet/go-temp-err-catcher"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// localListener 结构体定义了本地监听器，它将manet流代理到libp2p服务
type localListener struct {
	ctx context.Context // 上下文，用于控制生命周期

	p2p *P2P // P2P 实例

	proto protocol.ID  // 协议ID
	laddr ma.Multiaddr // 监听地址
	peer  peer.ID      // 对等节点ID

	listener manet.Listener // manet监听器
}

// ForwardLocal 创建一个新的P2P流到远程监听器
// 参数：
//   - ctx: context.Context，上下文用于控制生命周期
//   - peer: peer.ID，对等节点ID
//   - proto: protocol.ID，协议ID
//   - bindAddr: ma.Multiaddr，绑定地址
//
// 返回：
//   - Listener，返回监听器接口
//   - error，返回可能发生的错误
func (p2p *P2P) ForwardLocal(ctx context.Context, peer peer.ID, proto protocol.ID, bindAddr ma.Multiaddr) (Listener, error) {
	listener := &localListener{
		ctx:   ctx,   // 初始化上下文
		p2p:   p2p,   // 绑定P2P实例
		proto: proto, // 设置协议ID
		peer:  peer,  // 设置对等节点ID
	}

	maListener, err := manet.Listen(bindAddr) // 尝试在绑定地址上监听
	if err != nil {
		return nil, err // 如果出错，返回错误
	}

	listener.listener = maListener          // 设置manet监听器
	listener.laddr = maListener.Multiaddr() // 获取并设置监听地址

	if err := p2p.ListenersLocal.Register(listener); err != nil {
		log.Debugf("监听器已注册 %s/%s", listener.peer, listener.proto) // 记录错误
		return nil, err                                           // 注册监听器，如果出错，返回错误
	}

	go listener.acceptConns() // 异步接受连接

	return listener, nil // 返回监听器
}

// dial 创建一个新的libp2p流连接到远程节点
// 参数：
//   - ctx: context.Context，上下文用于控制生命周期
//
// 返回：
//   - net.Stream，返回libp2p流
//   - error，返回可能发生的错误
func (l *localListener) dial(ctx context.Context) (net.Stream, error) {
	cctx, cancel := context.WithTimeout(ctx, time.Second*30) // 创建一个带超时的上下文
	defer cancel()                                           // 方法结束时取消上下文

	return l.p2p.peerHost.NewStream(cctx, l.peer, l.proto) // 使用libp2p主机创建新流
}

// acceptConns 接受传入的连接
func (l *localListener) acceptConns() {
	for {
		local, err := l.listener.Accept() // 接受新的连接
		if err != nil {
			if tec.ErrIsTemporary(err) { // 如果是临时错误，继续接受
				continue
			}
			return // 非临时错误，退出循环
		}

		go l.setupStream(local) // 异步设置流
	}
}

// setupStream 设置新的本地和远程流
// 参数：
//   - local: manet.Conn，本地连接
func (l *localListener) setupStream(local manet.Conn) {
	remote, err := l.dial(l.ctx) // 拨号创建远程流
	if err != nil {
		local.Close()                                 // 如果出错，关闭本地连接
		log.Warnf("连接远程节点 %s/%s 失败", l.peer, l.proto) // 记录错误
		return
	}

	// 创建新的Stream结构体
	stream := &Stream{
		Protocol: l.proto, // 设置协议ID

		OriginAddr: local.RemoteMultiaddr(), // 设置原地址
		TargetAddr: l.TargetAddress(),       // 设置目标地址
		peer:       l.peer,                  // 设置对等节点ID

		Local:  local,  // 本地连接
		Remote: remote, // 远程流

		Registry: l.p2p.Streams, // 流注册表
	}

	l.p2p.Streams.Register(stream) // 注册新流
}

// close 关闭监听器
func (l *localListener) close() {
	l.listener.Close() // 关闭manet监听器
}

// Protocol 获取协议ID
// 返回：
//   - protocol.ID，返回协议ID
func (l *localListener) Protocol() protocol.ID {
	return l.proto // 返回协议ID
}

// ListenAddress 获取监听地址
// 返回：
//   - ma.Multiaddr，返回监听地址
func (l *localListener) ListenAddress() ma.Multiaddr {
	return l.laddr // 返回监听地址
}

// TargetAddress 获取目标地址
// 返回：
//   - ma.Multiaddr，返回目标地址
func (l *localListener) TargetAddress() ma.Multiaddr {
	addr, err := ma.NewMultiaddr(maPrefix + l.peer.String()) // 创建新的多地址
	if err != nil {
		log.Debugf("创建目标地址失败 %s/%s", l.peer, l.proto) // 记录错误
		panic(err)                                    // 如果出错，触发恐慌
	}
	return addr // 返回目标地址
}

// key 获取监听器的键（协议ID）
// 返回：
//   - protocol.ID，返回协议ID
func (l *localListener) key() protocol.ID {
	return protocol.ID(l.ListenAddress().String()) // 返回监听地址的字符串形式作为协议ID
}
