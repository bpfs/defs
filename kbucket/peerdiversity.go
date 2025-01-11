// Package kbucket 提供了基于Kademlia DHT的路由表实现
package kbucket

import (
	"context"
	"time"

	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/kbucket/peerdiversity"
	"github.com/bpfs/defs/v2/net"

	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/libp2p/core/peer"
	"github.com/dep2p/libp2p/p2p/host/peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

// mockPeerGroupFilter 是一个模拟的 PeerGroupFilter 结构体,用于测试和开发
type mockPeerGroupFilter struct {
	// peerAddressFunc 用于返回指定 Peer 的地址列表的函数
	peerAddressFunc func(p peer.ID) []ma.Multiaddr
	// allowFnc 用于判断是否允许指定的 PeerGroupInfo 的函数
	allowFnc func(g peerdiversity.PeerGroupInfo) bool
	// incrementFnc 增加指定 PeerGroupInfo 计数的函数
	incrementFnc func(g peerdiversity.PeerGroupInfo)
	// decrementFnc 减少指定 PeerGroupInfo 计数的函数
	decrementFnc func(p peerdiversity.PeerGroupInfo)
}

// Allow 判断是否允许指定的 PeerGroupInfo
// 参数:
//   - g: 要判断的 PeerGroupInfo
//
// 返回值:
//   - bool: 如果允许则返回 true,否则返回 false
func (m *mockPeerGroupFilter) Allow(g peerdiversity.PeerGroupInfo) (allow bool) {
	return m.allowFnc(g)
}

// PeerAddresses 返回指定 Peer 的地址列表
// 参数:
//   - p: 要获取地址的 Peer ID
//
// 返回值:
//   - []ma.Multiaddr: 该 Peer 的地址列表
func (m *mockPeerGroupFilter) PeerAddresses(p peer.ID) []ma.Multiaddr {
	return m.peerAddressFunc(p)
}

// Increment 增加指定 PeerGroupInfo 的计数
// 参数:
//   - g: 要增加计数的 PeerGroupInfo
func (m *mockPeerGroupFilter) Increment(g peerdiversity.PeerGroupInfo) {
	if m.incrementFnc != nil {
		m.incrementFnc(g)
	}
}

// Decrement 减少指定 PeerGroupInfo 的计数
// 参数:
//   - g: 要减少计数的 PeerGroupInfo
func (m *mockPeerGroupFilter) Decrement(g peerdiversity.PeerGroupInfo) {
	if m.decrementFnc != nil {
		m.decrementFnc(g)
	}
}

// CreateRoutingTable 创建带有多样性过滤器的路由表
// 参数:
//   - h: libp2p 主机实例
//   - opt: 配置选项
//   - noOpThreshold: 无操作阈值
//
// 返回值:
//   - *RoutingTable: 创建的路由表实例
//   - error: 如果创建失败则返回错误
func CreateRoutingTable(h host.Host, opt *fscfg.Options, noOpThreshold time.Duration) (*RoutingTable, error) {
	// 创建路由表分集过滤器
	cplCount := make(map[int]int)

	// 创建新的多样性过滤器
	df, err := peerdiversity.NewFilter(
		&mockPeerGroupFilter{
			// 返回节点的地址列表
			peerAddressFunc: func(p peer.ID) []ma.Multiaddr {
				return h.Peerstore().Addrs(p)
			},
			// 判断是否允许添加新节点
			allowFnc: func(g peerdiversity.PeerGroupInfo) bool {
				maxPerCpl := opt.GetMaxPeersPerCpl()
				return cplCount[g.Cpl] < maxPerCpl
			},
			// 当添加新节点时增加计数
			incrementFnc: func(g peerdiversity.PeerGroupInfo) {
				cplCount[g.Cpl]++
			},
			// 当移除节点时减少计数
			decrementFnc: func(g peerdiversity.PeerGroupInfo) {
				cplCount[g.Cpl]--
			},
		},
		"defs/diversity",
		func(p peer.ID) int {
			return CommonPrefixLen(
				ConvertPeerID(h.ID()),
				ConvertPeerID(p),
			)
		},
	)
	if err != nil {
		logger.Errorf("创建多样性过滤器失败: %v", err)
		return nil, err
	}

	// 创建路由表实例
	rt, err := NewRoutingTable(
		opt.GetBucketSize(),    // 从配置获取桶大小
		ConvertPeerID(h.ID()),  // 本地节点ID
		time.Hour,              // 刷新间隔
		peerstore.NewMetrics(), // 度量指标
		noOpThreshold,          // 无操作阈值
		df,                     // 多样性过滤器
	)
	if err != nil {
		logger.Errorf("创建路由表失败: %v", err)
		return nil, err
	}

	return rt, nil
}

// AddPeer 尝试将对等节点添加到路由表
// 参数:
//   - h: libp2p 主机实例
//   - pi: 要添加的对等节点地址信息
//   - mode: 运行模式
//   - queryPeer: 是否是查询对等节点
//   - isReplaceable: 是否可替换
//
// 返回值:
//   - bool: 如果对等节点成功添加则返回 true
//   - error: 如果添加失败则返回错误
func (rt *RoutingTable) AddPeer(ctx context.Context, h host.Host, pi peer.AddrInfo, mode int, queryPeer bool, isReplaceable bool) (bool, error) {
	// 检查节点是否有用
	if !rt.UsefulNewPeer(pi.ID) {
		logger.Errorf("对等节点不适合添加到路由表")
		return false, nil
	}

	// 执行握手,忽略返回的节点列表
	_, err := net.Handshake(ctx, h, pi)
	if err != nil {
		logger.Errorf("与节点握手失败: %v", err)
		return false, err
	}

	// 尝试添加到路由表
	success, err := rt.TryAddPeer(pi.ID, mode, queryPeer, isReplaceable)
	if err != nil {
		logger.Errorf("添加对等节点失败: %v", err)
		return false, err
	}

	if success {
		logger.Errorf("成功添加对等节点 %s", pi.ID.String())
	}

	return success, nil
}
