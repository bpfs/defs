// Package peerdiversity 实现了对等节点多样性过滤功能
package peerdiversity

import (
	"net"
	"sync"
	"testing"

	"github.com/dep2p/libp2p/core/peer"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// mockPeerGroupFilter 是一个模拟的 PeerIPGroupFilter 接口实现
var _ PeerIPGroupFilter = (*mockPeerGroupFilter)(nil)

// mockPeerGroupFilter 结构体用于实现 PeerGroupFilter 接口
type mockPeerGroupFilter struct {
	mu              sync.Mutex                     // mu 是一个互斥锁，用于保护 increments 和 decrements 的并发访问
	increments      map[peer.ID]struct{}           // increments 记录每个 peer.ID 对应的增量计数
	decrements      map[peer.ID]struct{}           // decrements 记录每个 peer.ID 对应的减量计数
	peerAddressFunc func(p peer.ID) []ma.Multiaddr // peerAddressFunc 用于获取特定 peer.ID 的多地址列表
	allowFnc        func(g PeerGroupInfo) bool     // allowFnc 用于判断是否允许特定的 PeerGroupInfo
}

// Allow 判断是否允许特定的 PeerGroupInfo
//
// 参数:
//   - g: 要判断的 PeerGroupInfo
//
// 返回值:
//   - bool: 如果允许则返回 true，否则返回 false
func (m *mockPeerGroupFilter) Allow(g PeerGroupInfo) (allow bool) {
	return m.allowFnc(g)
}

// PeerAddresses 获取特定 Peer 的地址列表
//
// 参数:
//   - p: 要获取地址的 peer.ID
//
// 返回值:
//   - []ma.Multiaddr: 返回该 peer 的多地址列表
func (m *mockPeerGroupFilter) PeerAddresses(p peer.ID) []ma.Multiaddr {
	return m.peerAddressFunc(p)
}

// Increment 增加特定 PeerGroupInfo 的计数
//
// 参数:
//   - g: 要增加计数的 PeerGroupInfo
func (m *mockPeerGroupFilter) Increment(g PeerGroupInfo) {
	m.mu.Lock()         // 获取互斥锁，保护对 increments 的并发访问
	defer m.mu.Unlock() // 在函数返回时释放互斥锁

	m.increments[g.Id] = struct{}{} // 将给定 PeerGroupInfo 的 ID 添加到 increments 映射中
}

// Decrement 减少特定 PeerGroupInfo 的计数
//
// 参数:
//   - g: 要减少计数的 PeerGroupInfo
func (m *mockPeerGroupFilter) Decrement(g PeerGroupInfo) {
	m.mu.Lock()         // 获取互斥锁，保护对 decrements 的并发访问
	defer m.mu.Unlock() // 在函数返回时释放互斥锁

	m.decrements[g.Id] = struct{}{} // 将给定 PeerGroupInfo 的 ID 添加到 decrements 映射中
}

// newMockPeerGroupFilter 创建并返回一个新的 mockPeerGroupFilter 实例
//
// 返回值:
//   - *mockPeerGroupFilter: 返回初始化后的 mockPeerGroupFilter 实例
func newMockPeerGroupFilter() *mockPeerGroupFilter {
	m := &mockPeerGroupFilter{
		increments: map[peer.ID]struct{}{}, // 初始化 increments 映射，用于记录增量计数
		decrements: map[peer.ID]struct{}{}, // 初始化 decrements 映射，用于记录减量计数

		peerAddressFunc: func(p peer.ID) []ma.Multiaddr {
			return nil
		},
		allowFnc: func(g PeerGroupInfo) bool {
			return false
		},
	}

	return m
}

// TestDiversityFilter 测试 DiversityFilter 的功能
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 测试简单的允许/拒绝场景
// 2. 测试一个地址被允许而另一个被拒绝的场景
// 3. 测试白名单对等节点的场景
// 4. 测试无地址对等节点的场景
func TestDiversityFilter(t *testing.T) {
	// tcs 是一个测试用例映射，用于存储不同的测试场景
	tcs := map[string]struct {
		peersForTest  func() []peer.ID             // peersForTest 返回要测试的对等节点列表
		mFnc          func(m *mockPeerGroupFilter) // mFnc 用于设置 mockPeerGroupFilter 的行为
		fFnc          func(f *Filter)              // fFnc 用于设置 Filter 的行为
		allowed       map[peer.ID]bool             // allowed 存储对等节点的允许状态
		isWhitelisted bool                         // isWhitelisted 指示对等节点是否在白名单中
	}{
		"simple allow": {
			peersForTest: func() []peer.ID {
				return []peer.ID{"p1", "p2"}
			},
			mFnc: func(m *mockPeerGroupFilter) {
				m.peerAddressFunc = func(id peer.ID) []ma.Multiaddr {
					return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0")}
				}
				m.allowFnc = func(g PeerGroupInfo) bool { return g.Id == "p1" }
			},
			allowed: map[peer.ID]bool{
				"p1": true,
				"p2": false,
			},
			fFnc: func(f *Filter) {},
		},

		"one address is allowed, one isn't": {
			peersForTest: func() []peer.ID {
				return []peer.ID{"p1", "p2"}
			},
			mFnc: func(m *mockPeerGroupFilter) {
				m.peerAddressFunc = func(id peer.ID) []ma.Multiaddr {
					if id == "p1" {
						return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0"),
							ma.StringCast("/ip4/127.0.0.1/tcp/0")}
					}
					return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0"),
						ma.StringCast("/ip4/192.168.1.1/tcp/0")}
				}
				m.allowFnc = func(g PeerGroupInfo) bool { return g.IPGroupKey == "127.0.0.0" }
			},
			allowed: map[peer.ID]bool{
				"p1": true,
				"p2": false,
			},
			fFnc: func(f *Filter) {},
		},

		"whitelisted peers": {
			peersForTest: func() []peer.ID {
				return []peer.ID{"p1", "p2"}
			},
			mFnc: func(m *mockPeerGroupFilter) {
				m.peerAddressFunc = func(id peer.ID) []ma.Multiaddr {
					if id == "p1" {
						return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0")}
					} else {
						return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0")}
					}
				}

				m.allowFnc = func(g PeerGroupInfo) bool {
					return false
				}
			},
			allowed: map[peer.ID]bool{
				"p1": false,
				"p2": true,
			},
			fFnc: func(f *Filter) {
				f.WhitelistPeers(peer.ID("p2"))
			},
			isWhitelisted: true,
		},
		"whitelist peers works even if peer has no addresses": {
			peersForTest: func() []peer.ID {
				return []peer.ID{"p1", "p2"}
			},
			mFnc: func(m *mockPeerGroupFilter) {
				m.peerAddressFunc = func(id peer.ID) []ma.Multiaddr {
					if id == "p1" {
						return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0")}
					} else {
						return nil
					}
				}

				m.allowFnc = func(g PeerGroupInfo) bool {
					return false
				}
			},
			allowed: map[peer.ID]bool{
				"p1": false,
				"p2": true,
			},
			fFnc: func(f *Filter) {
				f.WhitelistPeers(peer.ID("p2"))
			},
			isWhitelisted: true,
		},

		"peer has no addresses": {
			peersForTest: func() []peer.ID {
				return []peer.ID{"p1"}
			},
			mFnc: func(m *mockPeerGroupFilter) {
				m.peerAddressFunc = func(id peer.ID) []ma.Multiaddr {
					return nil
				}
				m.allowFnc = func(g PeerGroupInfo) bool {
					return true
				}
			},
			allowed: map[peer.ID]bool{
				"p1": false,
			},
			fFnc: func(f *Filter) {},
		},
	}

	// 遍历所有测试用例
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			// 创建新的过滤器实例
			m := newMockPeerGroupFilter()
			tc.mFnc(m)
			f, err := NewFilter(m, "test", func(p peer.ID) int { return 1 })
			require.NoError(t, err, name)
			tc.fFnc(f)

			// 遍历测试用例中的每个对等节点
			for _, p := range tc.peersForTest() {
				b := f.TryAdd(p)

				// 检查对等节点是否在允许列表中
				v, ok := tc.allowed[p]
				require.True(t, ok, string(p))
				require.Equal(t, v, b, string(p))

				// 根据允许状态执行相应的操作
				if v && !tc.isWhitelisted {
					// 检查增加计数是否存在
					m.mu.Lock()
					_, ok := m.increments[p]
					require.True(t, ok)
					m.mu.Unlock()

					// 从过滤器中移除对等节点
					f.Remove(p)

					// 检查减少计数是否存在
					m.mu.Lock()
					_, ok = m.decrements[p]
					require.True(t, ok)
					m.mu.Unlock()
				} else if v && tc.isWhitelisted {
					// 检查增加计数是否不存在
					m.mu.Lock()
					_, ok := m.increments[p]
					require.False(t, ok)
					m.mu.Unlock()

					// 从过滤器中移除对等节点
					f.Remove(p)

					// 检查减少计数是否不存在
					m.mu.Lock()
					_, ok = m.decrements[p]
					require.False(t, ok)
					m.mu.Unlock()
				}
			}
		})
	}
}

// mockAsnStore 是一个模拟的 ASN 存储实现
type mockAsnStore struct {
	reply string // reply 存储响应字符串
}

// AsnForIPv6 根据 IPv6 地址获取 ASN
//
// 参数:
//   - net.IP: IPv6 地址
//
// 返回值:
//   - string: ASN 字符串
//   - error: 错误信息，如果没有错误则为 nil
func (m *mockAsnStore) AsnForIPv6(net.IP) (string, error) {
	return m.reply, nil
}

// TestIPGroupKey 测试 IPGroupKey 方法的功能
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 测试传统的 /8 网络
// 2. 测试 IPv4 /16 网络
// 3. 测试 IPv6 网络
func TestIPGroupKey(t *testing.T) {
	f, err := NewFilter(newMockPeerGroupFilter(), "test", func(p peer.ID) int { return 1 })
	f.asnStore = &mockAsnStore{"test"}
	require.NoError(t, err)

	// case 1: 测试传统的 /8 网络
	ip := net.ParseIP("17.111.0.1")
	require.NotNil(t, ip.To4())
	g, err := f.ipGroupKey(ip)
	require.NoError(t, err)
	require.Equal(t, "17.0.0.0", string(g))

	// case 2: 测试 IPv4 /16 网络
	ip = net.ParseIP("192.168.1.1")
	require.NotNil(t, ip.To4())
	g, err = f.ipGroupKey(ip)
	require.NoError(t, err)
	require.Equal(t, "192.168.0.0", string(g))

	// case 3: 测试 IPv6 网络
	ip = net.ParseIP("2a03:2880:f003:c07:face:b00c::2")
	g, err = f.ipGroupKey(ip)
	require.NoError(t, err)
	require.Equal(t, "test", string(g))
}

// TestGetDiversityStats 测试 GetDiversityStats 方法的功能
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 测试不同 CPL 值的对等节点统计
// 2. 测试具有多个地址的对等节点统计
func TestGetDiversityStats(t *testing.T) {
	// 初始化测试数据
	p1 := peer.ID("a")
	p2 := peer.ID("b")
	p3 := peer.ID("aa")
	p4 := peer.ID("bb")

	// 设置测试用的地址映射
	paddrs := map[peer.ID][]ma.Multiaddr{
		p1: {ma.StringCast("/ip4/17.0.0.1/tcp/0"), ma.StringCast("/ip4/19.1.1.0")},
		p2: {ma.StringCast("/ip4/18.1.0.1/tcp/0")},
		p3: {ma.StringCast("/ip4/19.2.0.1/tcp/0")},
		p4: {ma.StringCast("/ip4/20.3.0.1/tcp/0")},
	}

	// 创建并配置模拟过滤器
	m := newMockPeerGroupFilter()
	m.peerAddressFunc = func(p peer.ID) []ma.Multiaddr {
		return paddrs[p]
	}
	m.allowFnc = func(g PeerGroupInfo) bool {
		return true
	}

	// 创建过滤器实例
	f, err := NewFilter(m, "test", func(p peer.ID) int {
		return len(string(p))
	})
	require.NoError(t, err)

	// 添加测试对等节点
	require.True(t, f.TryAdd(p1))
	require.True(t, f.TryAdd(p2))
	require.True(t, f.TryAdd(p3))
	require.True(t, f.TryAdd(p4))

	// 获取并验证多样性统计信息
	stats := f.GetDiversityStats()
	require.Len(t, stats, 2)
	require.Equal(t, stats[0].Cpl, 1)
	require.Len(t, stats[0].Peers[p1], 2)
	require.Len(t, stats[0].Peers[p2], 1)

	require.Equal(t, stats[1].Cpl, 2)
	require.Len(t, stats[1].Peers[p3], 1)
	require.Len(t, stats[1].Peers[p4], 1)
}
