// 功能：P2P模块的核心文件，协调和管理P2P通信的各个部分。
// 主要职责：
// 	初始化P2P模块，加载配置和依赖项。
// 	提供P2P通信的基本功能和接口，如创建连接、发送和接收数据等。
// 	整合其他P2P相关文件（如listener.go、local.go、remote.go、stream.go）的功能。
// 典型操作：初始化P2P模块，提供P2P通信的基本接口和功能，协调其他P2P模块。

package p2p

import (
	p2phost "github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("p2p-mount") // 定义日志记录器

// P2P 结构体包含当前运行的流和监听器的信息
type P2P struct {
	ListenersLocal *Listeners      // 本地监听器管理
	ListenersP2P   *Listeners      // p2p监听器管理
	Streams        *StreamRegistry // 流注册表

	identity  peer.ID          // 对等节点ID
	peerHost  p2phost.Host     // p2p主机实例
	peerstore pstore.Peerstore // 对等节点存储
}

// New 创建一个新的 P2P 结构体实例
// 参数：
//   - identity: peer.ID，对等节点ID
//   - peerHost: p2phost.Host，libp2p主机实例
//   - peerstore: pstore.Peerstore，对等节点存储实例
//
// 返回：*P2P，指向P2P实例的指针
func New(identity peer.ID, peerHost p2phost.Host, peerstore pstore.Peerstore) *P2P {
	return &P2P{
		identity:  identity,  // 初始化对等节点ID
		peerHost:  peerHost,  // 初始化p2p主机
		peerstore: peerstore, // 初始化对等节点存储

		ListenersLocal: newListenersLocal(),       // 创建本地监听器管理
		ListenersP2P:   newListenersP2P(peerHost), // 创建p2p监听器管理

		Streams: &StreamRegistry{
			Streams:     map[uint64]*Stream{},   // 初始化流字典
			ConnManager: peerHost.ConnManager(), // 设置连接管理器
			conns:       map[peer.ID]int{},      // 初始化连接字典
		},
	}
}

// CheckProtoExists 检查是否已注册协议处理程序
// 参数：
//   - proto: protocol.ID，协议ID
//
// 返回：bool，返回是否存在协议处理程序
func (p2p *P2P) CheckProtoExists(proto protocol.ID) bool {
	protos := p2p.peerHost.Mux().Protocols() // 获取已注册的协议列表

	for _, p := range protos {
		if p != proto { // 检查协议ID是否匹配
			continue
		}
		return true // 存在匹配协议，返回true
	}
	return false // 不存在匹配协议，返回false
}
