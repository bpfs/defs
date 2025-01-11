package kbucket

// import (
// 	"testing"
// 	"time"

// 	"github.com/dep2p/libp2p/core/test"

// 	"github.com/stretchr/testify/require"
// )

// // TestBucketMinimum 测试桶的最小值查找功能
// // 参数:
// //   - t: 测试用例上下文
// func TestBucketMinimum(t *testing.T) {
// 	// 启用并行测试
// 	t.Parallel()

// 	// 创建一个新的桶
// 	b := newBucket()
// 	// 测试空桶情况,应返回nil
// 	require.Nil(t, b.min(func(p1 *PeerInfo, p2 *PeerInfo) bool { return true }))

// 	// 生成三个随机的对等节点ID
// 	pid1 := test.RandPeerIDFatal(t)
// 	pid2 := test.RandPeerIDFatal(t)
// 	pid3 := test.RandPeerIDFatal(t)

// 	// 测试场景1: 只有一个节点时,该节点为最小值
// 	b.pushFront(&PeerInfo{Id: pid1, LastUsefulAt: time.Now()})
// 	require.Equal(t, pid1, b.min(func(first *PeerInfo, second *PeerInfo) bool {
// 		return first.LastUsefulAt.Before(second.LastUsefulAt)
// 	}).Id)

// 	// 测试场景2: 添加一个更新的节点,第一个节点仍为最小值
// 	b.pushFront(&PeerInfo{Id: pid2, LastUsefulAt: time.Now().AddDate(1, 0, 0)})
// 	require.Equal(t, pid1, b.min(func(first *PeerInfo, second *PeerInfo) bool {
// 		return first.LastUsefulAt.Before(second.LastUsefulAt)
// 	}).Id)

// 	// 测试场景3: 添加一个更早的节点,新节点成为最小值
// 	b.pushFront(&PeerInfo{Id: pid3, LastUsefulAt: time.Now().AddDate(-1, 0, 0)})
// 	require.Equal(t, pid3, b.min(func(first *PeerInfo, second *PeerInfo) bool {
// 		return first.LastUsefulAt.Before(second.LastUsefulAt)
// 	}).Id)
// }

// // TestUpdateAllWith 测试批量更新桶中所有对等节点信息的功能
// // 参数:
// //   - t: 测试用例上下文
// func TestUpdateAllWith(t *testing.T) {
// 	// 启用并行测试
// 	t.Parallel()

// 	// 创建一个新的桶
// 	b := newBucket()
// 	// 测试空桶情况,确保不会崩溃
// 	b.updateAllWith(func(p *PeerInfo) {})

// 	// 生成三个随机的对等节点ID
// 	pid1 := test.RandPeerIDFatal(t)
// 	pid2 := test.RandPeerIDFatal(t)
// 	pid3 := test.RandPeerIDFatal(t)

// 	// 测试场景1: 更新单个节点
// 	b.pushFront(&PeerInfo{Id: pid1, replaceable: false})
// 	b.updateAllWith(func(p *PeerInfo) {
// 		p.replaceable = true
// 	})
// 	require.True(t, b.getPeer(pid1).replaceable)

// 	// 测试场景2: 选择性更新多个节点
// 	b.pushFront(&PeerInfo{Id: pid2, replaceable: false})
// 	b.updateAllWith(func(p *PeerInfo) {
// 		if p.Id == pid1 {
// 			p.replaceable = false
// 		} else {
// 			p.replaceable = true
// 		}
// 	})
// 	require.True(t, b.getPeer(pid2).replaceable)
// 	require.False(t, b.getPeer(pid1).replaceable)

// 	// 测试场景3: 批量更新所有节点
// 	b.pushFront(&PeerInfo{Id: pid3, replaceable: false})
// 	require.False(t, b.getPeer(pid3).replaceable)
// 	b.updateAllWith(func(p *PeerInfo) {
// 		p.replaceable = true
// 	})
// 	require.True(t, b.getPeer(pid1).replaceable)
// 	require.True(t, b.getPeer(pid2).replaceable)
// 	require.True(t, b.getPeer(pid3).replaceable)
// }
