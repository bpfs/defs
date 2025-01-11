// 功能：管理P2P网络中的数据流。
// 主要职责：
// 	创建和管理数据流，用于在节点之间传输数据。
// 	处理数据流的建立、维护和终止。
// 	提供数据流的状态信息，如流的方向、状态、传输的数据等。
// 典型操作：创建数据流，管理数据传输，关闭数据流。

package p2p

import (
	"io"
	"sync"

	ifconnmgr "github.com/dep2p/libp2p/core/connmgr"
	net "github.com/dep2p/libp2p/core/network"
	peer "github.com/dep2p/libp2p/core/peer"
	protocol "github.com/dep2p/libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

const cmgrTag = "stream-fwd" // 定义连接管理器标签

// Stream 结构体包含活动的传入和传出p2p流的信息
type Stream struct {
	id uint64 // 流的唯一标识符

	Protocol protocol.ID // 协议ID

	OriginAddr ma.Multiaddr // 原地址
	TargetAddr ma.Multiaddr // 目标地址
	peer       peer.ID      // 对等节点ID

	Local  manet.Conn // 本地连接
	Remote net.Stream // 远程流

	Registry *StreamRegistry // 流注册表
}

// close 关闭流的端点并取消注册
func (s *Stream) close() {
	s.Registry.Close(s)
}

// reset 关闭流的端点并取消注册
func (s *Stream) reset() {
	s.Registry.Reset(s)
}

// startStreaming 开始流数据传输
func (s *Stream) startStreaming() {
	go func() {
		_, err := io.Copy(s.Local, s.Remote) // 将数据从远程流复制到本地连接
		if err != nil {
			log.Debugf("复制数据失败 %s/%s", s.peer, s.Protocol) // 记录错误
			s.reset()                                      // 如果出错，重置流
		} else {
			s.close() // 否则，关闭流
		}
	}()

	go func() {
		_, err := io.Copy(s.Remote, s.Local) // 将数据从本地连接复制到远程流
		if err != nil {
			s.reset() // 如果出错，重置流
		} else {
			s.close() // 否则，关闭流
		}
	}()
}

// StreamRegistry 是一组活跃的传入和传出的协议应用流
type StreamRegistry struct {
	sync.Mutex // 互斥锁，用于并发控制

	Streams map[uint64]*Stream // 存储流ID和Stream结构体的映射
	conns   map[peer.ID]int    // 存储对等节点ID和连接数量的映射
	nextID  uint64             // 下一个流ID

	ifconnmgr.ConnManager // 连接管理器接口
}

// Register 在注册表中注册一个流
// 参数：
//   - streamInfo: *Stream，待注册的流信息
func (r *StreamRegistry) Register(streamInfo *Stream) {
	r.Lock()
	defer r.Unlock()

	r.ConnManager.TagPeer(streamInfo.peer, cmgrTag, 20) // 给对等节点打标签
	r.conns[streamInfo.peer]++                          // 增加对等节点的连接数量

	streamInfo.id = r.nextID         // 设置流的唯一标识符
	r.Streams[r.nextID] = streamInfo // 将流信息添加到注册表
	r.nextID++                       // 更新下一个流ID

	streamInfo.startStreaming() // 开始数据传输
}

// Deregister 从注册表中注销一个流
// 参数：
//   - streamID: uint64，待注销的流ID
func (r *StreamRegistry) Deregister(streamID uint64) {
	r.Lock()
	defer r.Unlock()

	s, ok := r.Streams[streamID] // 获取流信息
	if !ok {
		return // 如果流不存在，直接返回
	}
	p := s.peer
	r.conns[p]-- // 减少对等节点的连接数量
	if r.conns[p] < 1 {
		delete(r.conns, p)                  // 如果连接数量少于1，删除对等节点映射
		r.ConnManager.UntagPeer(p, cmgrTag) // 取消对等节点的标签
	}

	delete(r.Streams, streamID) // 从注册表中删除流
}

// Close 关闭流的端点并注销它
// 参数：
//   - s: *Stream，待关闭的流
func (r *StreamRegistry) Close(s *Stream) {
	_ = s.Local.Close()         // 关闭本地连接
	_ = s.Remote.Close()        // 关闭远程流
	s.Registry.Deregister(s.id) // 从注册表中注销流
}

// Reset 关闭流的端点并注销它
// 参数：
//   - s: *Stream，待重置的流
func (r *StreamRegistry) Reset(s *Stream) {
	_ = s.Local.Close()         // 关闭本地连接
	_ = s.Remote.Reset()        // 重置远程流
	s.Registry.Deregister(s.id) // 从注册表中注销流
}
