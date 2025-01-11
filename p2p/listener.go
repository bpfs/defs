// 功能：管理P2P网络中的监听器。
// 主要职责：
// 	创建和管理监听器，用于接收来自其他节点的连接。
// 	处理新的传入连接，建立通信通道。
// 	监听特定的地址和协议，以确保能够响应其他节点的连接请求。
// 典型操作：启动监听器，处理新的连接请求，关闭监听器。

package p2p

import (
	"errors"
	"sync"

	p2phost "github.com/dep2p/libp2p/core/host"
	net "github.com/dep2p/libp2p/core/network"
	"github.com/dep2p/libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

// Listener 接口定义了监听连接并将其代理到目标的功能
type Listener interface {
	Protocol() protocol.ID       // 获取协议ID的方法
	ListenAddress() ma.Multiaddr // 获取监听地址的方法
	TargetAddress() ma.Multiaddr // 获取目标地址的方法

	key() protocol.ID // 获取协议ID作为键的方法

	// close 关闭监听器的方法。不影响子流
	close()
}

// Listeners 管理一组 Listener 实现，检查冲突并可选择性地分派连接
type Listeners struct {
	sync.RWMutex // 读写锁，用于并发安全

	Listeners map[protocol.ID]Listener // 存储协议ID和Listener映射关系的字典
}

// newListenersLocal 创建一个本地的 Listeners 实例
// 参数：无
// 返回：*Listeners，指向Listeners实例的指针
func newListenersLocal() *Listeners {
	return &Listeners{
		Listeners: map[protocol.ID]Listener{}, // 初始化Listeners字典
	}
}

// newListenersP2P 创建一个基于p2p主机的 Listeners 实例
// 参数：
//   - host: p2phost.Host，libp2p主机实例
//
// 返回：*Listeners，指向Listeners实例的指针
func newListenersP2P(host p2phost.Host) *Listeners {
	reg := &Listeners{
		Listeners: map[protocol.ID]Listener{}, // 初始化Listeners字典
	}

	// 设置流处理程序匹配逻辑
	host.SetStreamHandlerMatch("/x/", func(p protocol.ID) bool {
		reg.RLock()         // 读锁定
		defer reg.RUnlock() // 方法结束时解锁

		_, ok := reg.Listeners[p] // 检查协议ID是否存在
		return ok
	}, func(stream net.Stream) { // 定义流处理逻辑
		reg.RLock()         // 读锁定
		defer reg.RUnlock() // 方法结束时解锁

		l := reg.Listeners[stream.Protocol()] // 获取对应的Listener
		if l != nil {
			go l.(*remoteListener).handleStream(stream) // 异步处理流
		}
	})

	return reg // 返回初始化的Listeners实例
}

// Register 在注册表中注册listenerInfo并启动它
// 参数：
//   - l: Listener，待注册的监听器
//
// 返回：error，表示注册过程中的错误信息，如果成功则返回nil
func (r *Listeners) Register(l Listener) error {
	r.Lock()         // 写锁定
	defer r.Unlock() // 方法结束时解锁

	if _, ok := r.Listeners[l.key()]; ok { // 检查监听器是否已注册
		return errors.New("监听器已注册") // 返回错误
	}

	r.Listeners[l.key()] = l // 将监听器添加到字典中
	return nil               // 返回空值表示成功
}

// Close 关闭匹配条件的监听器
// 参数：
//   - matchFunc: func(listener Listener) bool，用于匹配监听器的函数
//
// 返回：int，表示关闭的监听器数量
func (r *Listeners) Close(matchFunc func(listener Listener) bool) int {
	todo := make([]Listener, 0)     // 初始化待处理的监听器列表
	r.Lock()                        // 写锁定
	for _, l := range r.Listeners { // 遍历Listeners字典
		if !matchFunc(l) { // 检查监听器是否匹配
			continue
		}

		if _, ok := r.Listeners[l.key()]; ok { // 再次检查监听器是否存在
			delete(r.Listeners, l.key()) // 从字典中删除监听器
			todo = append(todo, l)       // 将监听器添加到待处理列表
		}
	}
	r.Unlock() // 解锁

	for _, l := range todo { // 遍历待处理的监听器列表
		l.close() // 关闭监听器
	}

	return len(todo) // 返回关闭的监听器数量
}
