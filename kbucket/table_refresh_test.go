// Package kbucket 实现了 Kademlia DHT 的路由表功能
package kbucket

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/test"

	pstore "github.com/libp2p/go-libp2p/p2p/host/peerstore"

	"github.com/stretchr/testify/require"
)

// TestGenRandPeerID 测试 GenRandPeerID 方法的功能
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 测试生成超过最大 CPL 的 PeerID 时应该失败
// 2. 测试生成不同 CPL 值的随机 PeerID
func TestGenRandPeerID(t *testing.T) {
	// 标记该测试可以并行执行
	t.Parallel()

	// 生成一个随机的本地 PeerID
	local := test.RandPeerIDFatal(t)
	// 创建一个新的 Metrics 实例
	m := pstore.NewMetrics()
	// 创建一个新的路由表实例
	rt, err := NewRoutingTable(1, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
	require.NoError(t, err)

	// 测试生成超过最大 CPL 的 PeerID 应该失败
	p, err := rt.GenRandPeerID(maxCplForRefresh + 1)
	require.Error(t, err)
	require.Empty(t, p)

	// 测试生成不同 CPL 值的随机 PeerID
	for cpl := uint(0); cpl <= maxCplForRefresh; cpl++ {
		peerID, err := rt.GenRandPeerID(cpl)
		require.NoError(t, err)

		// 验证生成的 PeerID 与本地节点的 CPL 值是否符合预期
		require.True(t, uint(CommonPrefixLen(ConvertPeerID(peerID), rt.local)) == cpl, "failed for cpl=%d", cpl)
	}
}

// TestGenRandomKey 测试 GenRandomKey 方法的功能
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 测试生成超过最大位数的密钥应该失败
// 2. 测试生成不同 CPL 值的随机密钥
func TestGenRandomKey(t *testing.T) {
	// 标记该测试可以并行执行
	t.Parallel()

	// 运行多次以确保测试结果稳定
	for i := 0; i < 100; i++ {
		// 生成一个随机的本地 PeerID
		local := test.RandPeerIDFatal(t)
		// 创建一个新的 Metrics 实例
		m := pstore.NewMetrics()
		// 创建一个新的路由表实例
		rt, err := NewRoutingTable(1, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
		require.NoError(t, err)

		// 测试生成超过最大位数的密钥应该失败
		_, err = rt.GenRandomKey(256)
		require.Error(t, err)
		_, err = rt.GenRandomKey(300)
		require.Error(t, err)

		// 测试 CPL = 0 的情况
		key0, err := rt.GenRandomKey(0)
		require.NoError(t, err)
		require.NotEqual(t, key0[0]>>7, rt.local[0]>>7) // 最高位应不同

		// 测试 CPL = 1 的情况
		key1, err := rt.GenRandomKey(1)
		require.NoError(t, err)
		require.Equal(t, key1[0]>>7, rt.local[0]>>7)              // 最高位应相同
		require.NotEqual(t, (key1[0]<<1)>>6, (rt.local[0]<<1)>>6) // 第二位应不同

		// 测试 CPL = 2 的情况
		key2, err := rt.GenRandomKey(2)
		require.NoError(t, err)
		require.Equal(t, key2[0]>>6, rt.local[0]>>6)              // 前两位应相同
		require.NotEqual(t, (key2[0]<<2)>>5, (rt.local[0]<<2)>>5) // 第三位应不同

		// 测试 CPL = 7 的情况
		key7, err := rt.GenRandomKey(7)
		require.NoError(t, err)
		require.Equal(t, key7[0]>>1, rt.local[0]>>1)    // 前七位应相同
		require.NotEqual(t, key7[0]<<7, rt.local[0]<<7) // 第八位应不同

		// 测试 CPL = 8 的情况
		key8, err := rt.GenRandomKey(8)
		require.NoError(t, err)
		require.Equal(t, key8[0], rt.local[0])          // 第一个字节应完全相同
		require.NotEqual(t, key8[1]>>7, rt.local[1]>>7) // 第九位应不同

		// 测试 CPL = 53 的情况
		key53, err := rt.GenRandomKey(53)
		require.NoError(t, err)
		require.Equal(t, key53[:6], rt.local[:6])                  // 前 6 个字节应相同
		require.Equal(t, key53[6]>>3, rt.local[6]>>3)              // 第 7 个字节的前 5 位应相同
		require.NotEqual(t, (key53[6]<<5)>>7, (rt.local[6]<<5)>>7) // 第 54 位应不同
	}
}

// TestRefreshAndGetTrackedCpls 测试路由表刷新和 CPL 跟踪功能
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 测试初始状态下的 CPL 跟踪
// 2. 测试添加和删除节点时的 CPL 跟踪变化
// 3. 测试重置刷新时间的功能
func TestRefreshAndGetTrackedCpls(t *testing.T) {
	// 标记该测试可以并行执行
	t.Parallel()

	// 定义测试常量
	const (
		minCpl  = 8  // 最小 CPL 值
		testCpl = 10 // 测试用的 CPL 值
		maxCpl  = 12 // 最大 CPL 值
	)

	// 创建路由表
	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
	require.NoError(t, err)

	// 验证初始状态
	trackedCpls := rt.GetTrackedCplsForRefresh()
	require.Len(t, trackedCpls, 1)

	// 生成测试用的 PeerID
	var peerIDs []peer.ID
	for i := minCpl; i <= maxCpl; i++ {
		id, err := rt.GenRandPeerID(uint(i))
		require.NoError(t, err)
		peerIDs = append(peerIDs, id)
	}

	// 添加 PeerID 并验证 CPL 跟踪的变化
	for i, id := range peerIDs {
		added, err := rt.TryAddPeer(id, 0, true, false)
		require.NoError(t, err)
		require.True(t, added)
		require.Len(t, rt.GetTrackedCplsForRefresh(), minCpl+i+1)
	}

	// 移除节点直到目标 CPL 值
	for i := maxCpl; i > testCpl; i-- {
		rt.RemovePeer(peerIDs[i-minCpl])
		require.Len(t, rt.GetTrackedCplsForRefresh(), i)
	}

	// 验证当前跟踪的 CPL 值
	trackedCpls = rt.GetTrackedCplsForRefresh()
	require.Len(t, trackedCpls, testCpl+1)
	for _, refresh := range trackedCpls {
		require.True(t, refresh.IsZero(), "跟踪的 CPL 值应为零")
	}

	// 添加本地节点并验证
	added, err := rt.TryAddPeer(local, 0, true, false)
	require.NoError(t, err)
	require.True(t, added)

	// 验证最大 CPL 值的跟踪
	trackedCpls = rt.GetTrackedCplsForRefresh()
	require.Len(t, trackedCpls, int(maxCplForRefresh)+1)
	for _, refresh := range trackedCpls {
		require.True(t, refresh.IsZero(), "跟踪的 CPL 值应为零")
	}

	// 测试重置刷新时间
	now := time.Now()
	rt.ResetCplRefreshedAtForID(ConvertPeerID(peerIDs[testCpl-minCpl]), now)

	// 验证刷新时间的重置结果
	trackedCpls = rt.GetTrackedCplsForRefresh()
	require.Len(t, trackedCpls, int(maxCplForRefresh)+1)
	for i, refresh := range trackedCpls {
		if i == testCpl {
			require.True(t, now.Equal(refresh), "测试用的 CPL 值应该有正确的刷新时间")
		} else {
			require.True(t, refresh.IsZero(), "其他 CPL 值应为零")
		}
	}
}
