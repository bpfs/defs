package kbucket

// import (
// 	"math/rand"
// 	"testing"
// 	"time"

// 	"github.com/dep2p/libp2p/core/peer"
// 	"github.com/dep2p/libp2p/core/test"

// 	"github.com/bpfs/defs/v2/kbucket/peerdiversity"

// 	pstore "github.com/dep2p/libp2p/p2p/host/peerstore"

// 	ma "github.com/multiformats/go-multiaddr"
// 	"github.com/stretchr/testify/require"
// )

// // NoOpThreshold 表示无操作的时间阈值，设定为 100 小时
// var NoOpThreshold = 100 * time.Hour

// // TestPrint 测试打印路由表的功能
// // 参数:
// //   - t: 测试对象
// func TestPrint(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的路由表
// 	// 参数1: 路由表的容量
// 	// 参数2: 本地 Peer ID 转换后的 DHT ID
// 	// 参数3: 路由表的刷新间隔
// 	// 参数4: 存储路由表统计信息的 Metrics 对象
// 	// 参数5: NoOpThreshold 表示无操作的时间阈值
// 	// 参数6: 可选的回调函数
// 	rt, err := NewRoutingTable(1, ConvertPeerID(local), time.Hour, pstore.NewMetrics(), NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 打印路由表
// 	rt.Print()
// }

// // TestBucket 测试桶（bucket）结构的基本功能
// // 参数:
// //   - t: 测试对象
// func TestBucket(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 创建两个测试时间
// 	testTime1 := time.Now()
// 	testTime2 := time.Now().AddDate(1, 0, 0)

// 	// 创建一个新的桶
// 	b := newBucket()

// 	// 创建一个包含 100 个元素的 peer.ID 切片
// 	peers := make([]peer.ID, 100)
// 	for i := 0; i < 100; i++ {
// 		// 生成一个随机的 peer.ID
// 		peers[i] = test.RandPeerIDFatal(t)

// 		// 将 PeerInfo 结构体添加到桶的前面
// 		b.pushFront(&PeerInfo{
// 			Id:                            peers[i],
// 			Mode:                          0,
// 			LastUsefulAt:                  testTime1,
// 			LastSuccessfulOutboundQueryAt: testTime2,
// 			AddedAt:                       testTime1,
// 			dhtId:                         ConvertPeerID(peers[i]),
// 		})
// 	}

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 将本地 Peer ID 转换为 DHT ID
// 	localID := ConvertPeerID(local)

// 	// 获取桶中的 PeerInfo 切片
// 	infos := b.peers()

// 	// 断言桶中的 PeerInfo 切片长度为 100
// 	require.Len(t, infos, 100)

// 	// 随机选择一个索引
// 	i := rand.Intn(len(peers))

// 	// 获取指定 peer.ID 对应的 PeerInfo
// 	p := b.getPeer(peers[i])

// 	// 断言获取到的 PeerInfo 不为 nil
// 	require.NotNil(t, p)

// 	// 断言获取到的 PeerInfo 的 ID 与指定的 peer.ID 相等
// 	require.Equal(t, peers[i], p.Id)

// 	// 断言获取到的 PeerInfo 的 DHT ID 与指定的 peer.ID 转换后的 DHT ID 相等
// 	require.Equal(t, ConvertPeerID(peers[i]), p.dhtId)

// 	// 断言获取到的 PeerInfo 的 LastUsefulAt 字段与指定的测试时间相等
// 	require.EqualValues(t, testTime1, p.LastUsefulAt)

// 	// 断言获取到的 PeerInfo 的 LastSuccessfulOutboundQueryAt 字段与指定的测试时间相等
// 	require.EqualValues(t, testTime2, p.LastSuccessfulOutboundQueryAt)

// 	// 创建两个新的时间
// 	t2 := time.Now().Add(1 * time.Hour)
// 	t3 := t2.Add(1 * time.Hour)

// 	// 更新获取到的 PeerInfo 的 LastSuccessfulOutboundQueryAt 和 LastUsefulAt 字段
// 	p.LastSuccessfulOutboundQueryAt = t2
// 	p.LastUsefulAt = t3

// 	// 再次获取指定 peer.ID 对应的 PeerInfo
// 	p = b.getPeer(peers[i])

// 	// 断言获取到的 PeerInfo 不为 nil
// 	require.NotNil(t, p)

// 	// 断言获取到的 PeerInfo 的 LastSuccessfulOutboundQueryAt 字段与更新后的时间相等
// 	require.EqualValues(t, t2, p.LastSuccessfulOutboundQueryAt)

// 	// 断言获取到的 PeerInfo 的 LastUsefulAt 字段与更新后的时间相等
// 	require.EqualValues(t, t3, p.LastUsefulAt)

// 	// 在索引为 0 的位置将桶进行分割，并获取左侧的桶
// 	spl := b.split(0, ConvertPeerID(local))
// 	llist := b.list

// 	// 遍历左侧桶的链表
// 	for e := llist.Front(); e != nil; e = e.Next() {
// 		p := ConvertPeerID(e.Value.(*PeerInfo).Id)
// 		cpl := CommonPrefixLen(p, localID)

// 		// 如果发现 cpl 大于 0 的 ID，则表示分割失败
// 		if cpl > 0 {
// 			t.Fatalf("split failed. found id with cpl > 0 in 0 bucket")
// 		}
// 	}

// 	rlist := spl.list

// 	// 遍历右侧桶的链表
// 	for e := rlist.Front(); e != nil; e = e.Next() {
// 		p := ConvertPeerID(e.Value.(*PeerInfo).Id)
// 		cpl := CommonPrefixLen(p, localID)

// 		// 如果发现 cpl 等于 0 的 ID，则表示分割失败
// 		if cpl == 0 {
// 			t.Fatalf("split failed. found id with cpl == 0 in non 0 bucket")
// 		}
// 	}
// }

// // TestNPeersForCpl 测试 NPeersForCpl 方法的功能
// // 参数:
// //   - t: 测试对象
// func TestNPeersForCpl(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 断言 cpl 为 0 时的邻居节点数为 0
// 	require.Equal(t, 0, rt.NPeersForCpl(0))

// 	// 断言 cpl 为 1 时的邻居节点数为 0
// 	require.Equal(t, 0, rt.NPeersForCpl(1))

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ := rt.GenRandPeerID(1)
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 断言 cpl 为 0 时的邻居节点数为 0
// 	require.Equal(t, 0, rt.NPeersForCpl(0))

// 	// 断言 cpl 为 1 时的邻居节点数为 1
// 	require.Equal(t, 1, rt.NPeersForCpl(1))

// 	// 断言 cpl 为 2 时的邻居节点数为 0
// 	require.Equal(t, 0, rt.NPeersForCpl(2))

// 	// 生成一个 cpl 为 0 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(0)
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 断言 cpl 为 0 时的邻居节点数为 1
// 	require.Equal(t, 1, rt.NPeersForCpl(0))

// 	// 断言 cpl 为 1 时的邻居节点数为 1
// 	require.Equal(t, 1, rt.NPeersForCpl(1))

// 	// 断言 cpl 为 2 时的邻居节点数为 0
// 	require.Equal(t, 0, rt.NPeersForCpl(2))

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(1)
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 断言 cpl 为 0 时的邻居节点数为 1
// 	require.Equal(t, 1, rt.NPeersForCpl(0))

// 	// 断言 cpl 为 1 时的邻居节点数为 2
// 	require.Equal(t, 2, rt.NPeersForCpl(1))

// 	// 断言 cpl 为 2 时的邻居节点数为 0
// 	require.Equal(t, 0, rt.NPeersForCpl(2))

// 	// 生成一个 cpl 为 0 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(0)
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 断言 cpl 为 0 时的邻居节点数为 2
// 	require.Equal(t, 2, rt.NPeersForCpl(0))
// }

// // TestUsefulNewPeer 测试 UsefulNewPeer 方法的功能
// // 参数:
// //   - t: 测试对象
// func TestUsefulNewPeer(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成一个 cpl 为 0 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ := rt.GenRandPeerID(0)

// 	// 断言 UsefulNewPeer 方法返回 true
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 断言 UsefulNewPeer 方法返回 false，因为 Peer ID 已经存在于 RoutingTable 中
// 	require.False(t, rt.UsefulNewPeer(p))

// 	// 生成一个 cpl 为 0 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(0)

// 	// 断言 UsefulNewPeer 方法返回 true
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 断言 UsefulNewPeer 方法返回 false，因为桶已满且无法替换
// 	require.False(t, rt.UsefulNewPeer(p))

// 	// 将桶进行展开

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(1)

// 	// 断言 UsefulNewPeer 方法返回 true
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 生成一个 cpl 为 2 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(2)

// 	// 断言 UsefulNewPeer 方法返回 true，尽管 cpl 为 2，但是该桶是最后一个桶
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 展开桶

// 	// 生成一个 cpl 为 2 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(2)

// 	// 断言 UsefulNewPeer 方法返回 true
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(1)

// 	// 断言 UsefulNewPeer 方法返回 true，因为该桶可以替换节点
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中，并设置可替换标志为 true
// 	rt.TryAddPeer(p, 0, true, true)

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(1)

// 	// 断言 UsefulNewPeer 方法返回 true，因为该桶可以替换节点
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中，并设置可替换标志为 true
// 	rt.TryAddPeer(p, 0, true, true)

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(1)

// 	// 断言 UsefulNewPeer 方法返回 true，因为该桶已满但无需替换节点
// 	require.True(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中，并设置可替换标志为 false
// 	rt.TryAddPeer(p, 0, true, false)

// 	// 生成一个 cpl 为 1 的随机 Peer ID，并将其添加到 RoutingTable 中
// 	p, _ = rt.GenRandPeerID(1)

// 	// 断言 UsefulNewPeer 方法返回 false，因为该桶已满且无法替换节点
// 	require.False(t, rt.UsefulNewPeer(p))

// 	// 将生成的 Peer ID 添加到 RoutingTable 中，并设置可替换标志为 false
// 	rt.TryAddPeer(p, 0, true, false)
// }

// // TestEmptyBucketCollapse 测试空桶的折叠功能
// // 参数:
// //   - t: 测试对象
// func TestEmptyBucketCollapse(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(1, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成 cpl 分别为 0、1、2 和 3 的随机 Peer ID
// 	p1, _ := rt.GenRandPeerID(0)
// 	p2, _ := rt.GenRandPeerID(1)
// 	p3, _ := rt.GenRandPeerID(2)
// 	p4, _ := rt.GenRandPeerID(3)

// 	// 移除空桶中的 Peer 不应导致错误
// 	rt.RemovePeer(p1)

// 	// 将 cpl 为 0 的 Peer 添加到 RoutingTable 中，并立即移除
// 	b, err := rt.TryAddPeer(p1, 0, true, false)
// 	require.True(t, b)
// 	require.NoError(t, err)
// 	rt.RemovePeer(p1)

// 	// 断言 RoutingTable 中仍然存在一个空桶，因为它是唯一的桶
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 1)
// 	rt.tabLock.Unlock()

// 	// 断言 RoutingTable 中没有 Peer
// 	require.Empty(t, rt.ListPeers())

// 	// 将 cpl 为 0 和 cpl 为 1 的 Peer 添加到 RoutingTable 中，验证存在两个桶
// 	b, err = rt.TryAddPeer(p1, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	b, err = rt.TryAddPeer(p2, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 2)
// 	rt.tabLock.Unlock()

// 	// 从最后一个桶中移除一个 Peer，该桶将被折叠
// 	rt.RemovePeer(p2)
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 1)
// 	rt.tabLock.Unlock()

// 	// 断言 RoutingTable 中仅有一个 Peer
// 	require.Len(t, rt.ListPeers(), 1)
// 	require.Contains(t, rt.ListPeers(), p1)

// 	// 再次将 p2 添加到 RoutingTable 中
// 	b, err = rt.TryAddPeer(p2, 0, true, false)
// 	require.True(t, b)
// 	require.NoError(t, err)
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 2)
// 	rt.tabLock.Unlock()

// 	// 现在从倒数第二个桶（即第一个桶）中移除一个 Peer，并确保它被折叠
// 	rt.RemovePeer(p1)
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 1)
// 	rt.tabLock.Unlock()

// 	// 断言 RoutingTable 中仅有一个 Peer
// 	require.Len(t, rt.ListPeers(), 1)
// 	require.Contains(t, rt.ListPeers(), p2)

// 	// 现在让我们总共有 4 个桶
// 	rt.TryAddPeer(p1, 0, true, false)
// 	rt.TryAddPeer(p2, 0, true, false)
// 	rt.TryAddPeer(p3, 0, true, false)
// 	rt.TryAddPeer(p4, 0, true, false)

// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 4)
// 	rt.tabLock.Unlock()

// 	// 从第二、三和第四个桶中依次移除 Peer，最终只剩下一个桶
// 	rt.RemovePeer(p2)
// 	rt.RemovePeer(p3)
// 	rt.RemovePeer(p4)
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 1)
// 	rt.tabLock.Unlock()

// 	// 断言 RoutingTable 中仅有一个 Peer
// 	require.Len(t, rt.ListPeers(), 1)
// 	require.Contains(t, rt.ListPeers(), p1)

// 	// 中间的空桶不会导致其他桶折叠
// 	rt.TryAddPeer(p1, 0, true, false)
// 	rt.TryAddPeer(p2, 0, true, false)
// 	rt.TryAddPeer(p3, 0, true, false)
// 	rt.TryAddPeer(p4, 0, true, false)

// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 4)
// 	rt.tabLock.Unlock()

// 	rt.RemovePeer(p2)
// 	rt.tabLock.Lock()
// 	require.Len(t, rt.buckets, 4)
// 	rt.tabLock.Unlock()

// 	// 断言 RoutingTable 中不包含 p2
// 	require.NotContains(t, rt.ListPeers(), p2)
// }

// // TestRemovePeer 测试移除 Peer 的功能
// // 参数:
// //   - t: 测试对象
// func TestRemovePeer(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成两个 cpl 为 0 的随机 Peer ID
// 	p1, _ := rt.GenRandPeerID(0)
// 	p2, _ := rt.GenRandPeerID(0)

// 	// 将 p1 和 p2 添加到 RoutingTable 中
// 	b, err := rt.TryAddPeer(p1, 0, true, false)
// 	require.True(t, b)
// 	require.NoError(t, err)
// 	b, err = rt.TryAddPeer(p2, 0, true, false)
// 	require.True(t, b)
// 	require.NoError(t, err)

// 	// 确保 p1 和 p2 存在于 RoutingTable 中
// 	require.Len(t, rt.ListPeers(), 2)
// 	require.Contains(t, rt.ListPeers(), p1)
// 	require.Contains(t, rt.ListPeers(), p2)

// 	// 移除一个 Peer 并确保它不在 RoutingTable 中
// 	require.NotEmpty(t, rt.Find(p1))
// 	rt.RemovePeer(p1)
// 	require.Empty(t, rt.Find(p1))
// 	require.NotEmpty(t, rt.Find(p2))
// }

// // TestTableCallbacks 测试 RoutingTable 的回调函数功能
// // 参数:
// //   - t: 测试对象
// func TestTableCallbacks(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 创建一个包含 100 个随机 Peer ID 的切片
// 	peers := make([]peer.ID, 100)
// 	for i := 0; i < 100; i++ {
// 		peers[i] = test.RandPeerIDFatal(t)
// 	}

// 	// 创建一个空的 Peer 集合 pset，并定义 PeerAdded 回调函数将 Peer 添加到 pset 中
// 	pset := make(map[peer.ID]struct{})
// 	rt.PeerAdded = func(p peer.ID) {
// 		pset[p] = struct{}{}
// 	}

// 	// 定义 PeerRemoved 回调函数将 Peer 从 pset 中移除
// 	rt.PeerRemoved = func(p peer.ID) {
// 		delete(pset, p)
// 	}

// 	// 将 peers[0] 添加到 RoutingTable 中，并确保它存在于 pset 中
// 	rt.TryAddPeer(peers[0], 0, true, false)
// 	if _, ok := pset[peers[0]]; !ok {
// 		t.Fatal("should have this peer")
// 	}

// 	// 从 RoutingTable 中移除 peers[0]，并确保它不在 pset 中
// 	rt.RemovePeer(peers[0])
// 	if _, ok := pset[peers[0]]; ok {
// 		t.Fatal("should not have this peer")
// 	}

// 	// 将 peers 中的所有 Peer 添加到 RoutingTable 中
// 	for _, p := range peers {
// 		rt.TryAddPeer(p, 0, true, false)
// 	}

// 	// 获取 RoutingTable 中的所有 Peer
// 	out := rt.ListPeers()

// 	// 遍历输出的 Peer，确保它们存在于 pset 中，并从 pset 中移除
// 	for _, outp := range out {
// 		if _, ok := pset[outp]; !ok {
// 			t.Fatal("should have peer in the peerset")
// 		}
// 		delete(pset, outp)
// 	}

// 	// 断言 pset 中没有剩余的 Peer
// 	if len(pset) > 0 {
// 		t.Fatal("have peers in peerset that were not in the table", len(pset))
// 	}
// }

// // TestTryAddPeerLoad 测试 TryAddPeer 方法在高负载情况下的性能和稳定性
// // 参数:
// //   - t: 测试对象
// func TestTryAddPeerLoad(t *testing.T) {
// 	// 标记该测试可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 创建一个包含 100 个随机 Peer ID 的切片
// 	peers := make([]peer.ID, 100)
// 	for i := 0; i < 100; i++ {
// 		peers[i] = test.RandPeerIDFatal(t)
// 	}

// 	// 在循环中多次调用 TryAddPeer 方法，模拟高负载情况
// 	for i := 0; i < 10000; i++ {
// 		rt.TryAddPeer(peers[rand.Intn(len(peers))], 0, true, false)
// 	}

// 	// 在循环中进行 NearestPeers 方法的调用，检查是否能正确找到最近的 Peer
// 	for i := 0; i < 100; i++ {
// 		// 生成一个随机的 Peer ID
// 		id := ConvertPeerID(test.RandPeerIDFatal(t))

// 		// 调用 NearestPeers 方法查找与给定 Peer ID 最近的 5 个 Peer
// 		ret := rt.NearestPeers(id, 5)

// 		// 如果返回结果为空，则表示查找失败
// 		if len(ret) == 0 {
// 			t.Fatal("Failed to find node near ID.")
// 		}
// 	}
// }

// // TestTableFind 是一个测试函数，用于测试 RoutingTable 的 NearestPeer 方法
// func TestTableFind(t *testing.T) {
// 	// t.Parallel() 表示该测试函数可以并行执行
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 创建一个包含 100 个随机 Peer ID 的切片
// 	peers := make([]peer.ID, 100)
// 	for i := 0; i < 5; i++ {
// 		peers[i] = test.RandPeerIDFatal(t)
// 		rt.TryAddPeer(peers[i], 0, true, false)
// 	}

// 	// 打印要查找的 Peer
// 	t.Logf("Searching for peer: '%s'", peers[2])

// 	// 调用 NearestPeer 方法查找与给定 Peer ID 最近的 Peer
// 	found := rt.NearestPeer(ConvertPeerID(peers[2]))

// 	// 如果找到的 Peer 不等于预期的 peers[2]，则表示查找失败
// 	if !(found == peers[2]) {
// 		t.Fatalf("Failed to lookup known node...")
// 	}
// }

// // TestUpdateLastSuccessfulOutboundQueryAt 是一个测试函数，用于测试 UpdateLastSuccessfulOutboundQueryAt 方法的功能
// func TestUpdateLastSuccessfulOutboundQueryAt(t *testing.T) {
// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成一个随机的 Peer ID
// 	p := test.RandPeerIDFatal(t)

// 	// 尝试将 Peer 添加到 RoutingTable 中
// 	b, err := rt.TryAddPeer(p, 0, true, false)
// 	require.True(t, b)
// 	require.NoError(t, err)

// 	// 生成一个新的时间 t2
// 	t2 := time.Now().Add(1 * time.Hour)

// 	// 调用 UpdateLastSuccessfulOutboundQueryAt 方法更新 Peer 的 LastSuccessfulOutboundQueryAt 字段为 t2
// 	rt.UpdateLastSuccessfulOutboundQueryAt(p, t2)

// 	// 获取锁以访问 RoutingTable 的内部数据结构
// 	rt.tabLock.Lock()

// 	// 从第一个桶中获取 Peer 的信息
// 	pi := rt.buckets[0].getPeer(p)

// 	// 断言 Peer 的信息不为空
// 	require.NotNil(t, pi)

// 	// 断言 Peer 的 LastSuccessfulOutboundQueryAt 字段的值等于 t2
// 	require.EqualValues(t, t2, pi.LastSuccessfulOutboundQueryAt)

// 	// 释放锁
// 	rt.tabLock.Unlock()
// }

// // TestUpdateLastUsefulAt 是一个测试函数，用于测试 UpdateLastUsefulAt 方法的功能
// func TestUpdateLastUsefulAt(t *testing.T) {
// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成一个随机的 Peer ID
// 	p := test.RandPeerIDFatal(t)

// 	// 尝试将 Peer 添加到 RoutingTable 中
// 	b, err := rt.TryAddPeer(p, 0, true, false)
// 	require.True(t, b)
// 	require.NoError(t, err)

// 	// 生成一个新的时间 t2
// 	t2 := time.Now().Add(1 * time.Hour)

// 	// 调用 UpdateLastUsefulAt 方法更新 Peer 的 LastUsefulAt 字段为 t2
// 	rt.UpdateLastUsefulAt(p, t2)

// 	// 获取锁以访问 RoutingTable 的内部数据结构
// 	rt.tabLock.Lock()

// 	// 从第一个桶中获取 Peer 的信息
// 	pi := rt.buckets[0].getPeer(p)

// 	// 断言 Peer 的信息不为空
// 	require.NotNil(t, pi)

// 	// 断言 Peer 的 LastUsefulAt 字段的值等于 t2
// 	require.EqualValues(t, t2, pi.LastUsefulAt)

// 	// 释放锁
// 	rt.tabLock.Unlock()
// }

// // TestTryAddPeer 是一个测试函数，用于测试 TryAddPeer 方法的功能
// func TestTryAddPeer(t *testing.T) {
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成两个 Peer，用于填满 cpl=0 的第一个桶
// 	p1, _ := rt.GenRandPeerID(0)
// 	b, err := rt.TryAddPeer(p1, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	p2, _ := rt.GenRandPeerID(0)
// 	b, err = rt.TryAddPeer(p2, 0, true, true)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	require.Equal(t, p1, rt.Find(p1))
// 	require.Equal(t, p2, rt.Find(p2))

// 	// 尝试添加一个 cpl=0 的 Peer，由于 p2 可替换，因此添加成功
// 	p3, _ := rt.GenRandPeerID(0)
// 	b, err = rt.TryAddPeer(p3, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	require.Equal(t, p3, rt.Find(p3))
// 	// p2 已被移除
// 	require.Empty(t, rt.Find(p2))

// 	// 尝试添加一个 Peer 失败，因为没有更多可替换的 Peer。
// 	p5, err := rt.GenRandPeerID(0)
// 	require.NoError(t, err)
// 	b, err = rt.TryAddPeer(p5, 0, true, false)
// 	require.Error(t, err)
// 	require.False(t, b)

// 	// 尝试添加一个 cpl=1 的 Peer 成功
// 	p4, _ := rt.GenRandPeerID(1)
// 	b, err = rt.TryAddPeer(p4, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	require.Equal(t, p4, rt.Find(p4))

// 	// 添加一个非查询 Peer
// 	p6, err := rt.GenRandPeerID(3)
// 	require.NoError(t, err)
// 	b, err = rt.TryAddPeer(p6, 0, false, false)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	rt.tabLock.Lock()
// 	pi := rt.buckets[rt.bucketIdForPeer(p6)].getPeer(p6)
// 	require.NotNil(t, p6)
// 	require.True(t, pi.LastUsefulAt.IsZero())
// 	rt.tabLock.Unlock()
// }

// // TestReplacePeerWithBucketSize1 是一个测试函数，用于测试在桶大小为1时替换 Peer 的功能
// func TestReplacePeerWithBucketSize1(t *testing.T) {
// 	localID := test.RandPeerIDFatal(t)

// 	// 创建一个新的 RoutingTable 实例，桶大小为1
// 	rt, err := NewRoutingTable(1, ConvertPeerID(localID), time.Hour, pstore.NewMetrics(), NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成两个 Peer，targetCpl > 0
// 	p1, _ := rt.GenRandPeerID(1)
// 	p2, _ := rt.GenRandPeerID(1)

// 	// 尝试添加 p1，作为可替换的 Peer
// 	rt.TryAddPeer(p1, 0, true, true)

// 	// 尝试添加 p2，替换 p1
// 	success, err := rt.TryAddPeer(p2, 0, true, true)

// 	require.NoError(t, err)
// 	require.True(t, success)

// 	// 断言 p1 已被替换
// 	require.Equal(t, peer.ID(""), rt.Find(p1))

// 	// 断言 p2 已成功添加
// 	require.Equal(t, p2, rt.Find(p2))

// 	// 断言 RoutingTable 的大小为1
// 	require.Equal(t, rt.Size(), 1)
// }

// // TestMarkAllPeersIrreplaceable 是一个测试函数，用于测试将所有 Peer 标记为不可替换的功能
// func TestMarkAllPeersIrreplaceable(t *testing.T) {
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	// 生成两个 Peer
// 	p1, _ := rt.GenRandPeerID(0)
// 	b, err := rt.TryAddPeer(p1, 0, true, true)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	p2, _ := rt.GenRandPeerID(0)
// 	b, err = rt.TryAddPeer(p2, 0, true, true)
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	require.Equal(t, p1, rt.Find(p1))
// 	require.Equal(t, p2, rt.Find(p2))

// 	// 将所有 Peer 标记为不可替换
// 	rt.MarkAllPeersIrreplaceable()

// 	// 获取所有 Peer 的信息
// 	ps := rt.GetPeerInfos()

// 	// 断言所有 Peer 的 replaceable 属性为 false
// 	for i := range ps {
// 		require.False(t, ps[i].replaceable)
// 	}
// }

// // TestTableFindMultiple 是一个测试函数，用于测试查找多个最近的 Peer 的功能
// func TestTableFindMultiple(t *testing.T) {
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	rt, err := NewRoutingTable(20, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	peers := make([]peer.ID, 100)
// 	for i := 0; i < 18; i++ {
// 		peers[i] = test.RandPeerIDFatal(t)
// 		rt.TryAddPeer(peers[i], 0, true, false)
// 	}

// 	t.Logf("Searching for peer: '%s'", peers[2])

// 	// 查找与指定 Peer 最近的 15 个 Peer
// 	found := rt.NearestPeers(ConvertPeerID(peers[2]), 15)

// 	// 断言返回的 Peer 数量与预期相同
// 	if len(found) != 15 {
// 		t.Fatalf("Got back different number of peers than we expected.")
// 	}
// }

// // TestTableFindMultipleBuckets 是一个测试函数，用于测试在多个桶中查找多个最近的 Peer 的功能
// func TestTableFindMultipleBuckets(t *testing.T) {
// 	t.Parallel()

// 	// 生成一个随机的本地 Peer ID
// 	local := test.RandPeerIDFatal(t)

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例，桶大小为5
// 	rt, err := NewRoutingTable(5, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	peers := make([]peer.ID, 100)
// 	for i := 0; i < 100; i++ {
// 		peers[i] = test.RandPeerIDFatal(t)
// 		rt.TryAddPeer(peers[i], 0, true, false)
// 	}

// 	// 对所有 Peer 进行排序，以找到与指定 Peer 最近的 Peer
// 	closest := SortClosestPeers(rt.ListPeers(), ConvertPeerID(peers[2]))

// 	t.Logf("Searching for peer: '%s'", peers[2])

// 	// 应该能够找到至少 30 个 Peer
// 	// 大约 31 个 (logtwo(100) * 5)
// 	found := rt.NearestPeers(ConvertPeerID(peers[2]), 20)
// 	if len(found) != 20 {
// 		t.Fatalf("asked for 20 peers, got %d", len(found))
// 	}
// 	for i, p := range found {
// 		if p != closest[i] {
// 			t.Fatalf("unexpected peer %d", i)
// 		}
// 	}

// 	// 现在让我们尝试找到所有的 Peer
// 	found = rt.NearestPeers(ConvertPeerID(peers[2]), 100)
// 	if len(found) != rt.Size() {
// 		t.Fatalf("asked for %d peers, got %d", rt.Size(), len(found))
// 	}

// 	for i, p := range found {
// 		if p != closest[i] {
// 			t.Fatalf("unexpected peer %d", i)
// 		}
// 	}
// }

// // TestTableMultithreaded 是一个测试函数，用于在表操作中查找竞态条件。
// // 为了更加“确定”的测试结果，可以将循环计数器从1000增加到一个更大的数字，并将 GOMAXPROCS 设置为大于1。
// func TestTableMultithreaded(t *testing.T) {
// 	t.Parallel()

// 	// 本地 Peer ID
// 	local := peer.ID("localPeer")

// 	// 创建一个新的 Metrics 实例
// 	m := pstore.NewMetrics()

// 	// 创建一个新的 RoutingTable 实例
// 	tab, err := NewRoutingTable(20, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	var peers []peer.ID
// 	for i := 0; i < 500; i++ {
// 		peers = append(peers, test.RandPeerIDFatal(t))
// 	}

// 	done := make(chan struct{})

// 	// 启动第一个 goroutine，进行 1000 次随机添加 Peer 的操作
// 	go func() {
// 		for i := 0; i < 1000; i++ {
// 			n := rand.Intn(len(peers))
// 			tab.TryAddPeer(peers[n], 0, true, false)
// 		}
// 		done <- struct{}{}
// 	}()

// 	// 启动第二个 goroutine，进行 1000 次随机添加 Peer 的操作
// 	go func() {
// 		for i := 0; i < 1000; i++ {
// 			n := rand.Intn(len(peers))
// 			tab.TryAddPeer(peers[n], 0, true, false)
// 		}
// 		done <- struct{}{}
// 	}()

// 	// 启动第三个 goroutine，进行 1000 次随机查找 Peer 的操作
// 	go func() {
// 		for i := 0; i < 1000; i++ {
// 			n := rand.Intn(len(peers))
// 			tab.Find(peers[n])
// 		}
// 		done <- struct{}{}
// 	}()

// 	// 等待三个 goroutine 完成
// 	<-done
// 	<-done
// 	<-done
// }

// // // mockPeerGroupFilter 是一个模拟的 PeerGroupFilter 结构体
// // type mockPeerGroupFilter struct {
// // 	peerAddressFunc func(p peer.ID) []ma.Multiaddr           // 用于返回指定 Peer 的地址列表的函数
// // 	allowFnc        func(g peerdiversity.PeerGroupInfo) bool // 用于判断是否允许指定的 PeerGroupInfo 的函数

// // 	incrementFnc func(g peerdiversity.PeerGroupInfo) // 增加指定 PeerGroupInfo 计数的函数
// // 	decrementFnc func(p peerdiversity.PeerGroupInfo) // 减少指定 PeerGroupInfo 计数的函数
// // }

// // // Allow 方法用于判断是否允许指定的 PeerGroupInfo
// // func (m *mockPeerGroupFilter) Allow(g peerdiversity.PeerGroupInfo) (allow bool) {
// // 	return m.allowFnc(g)
// // }

// // // PeerAddresses 方法返回指定 Peer 的地址列表
// // func (m *mockPeerGroupFilter) PeerAddresses(p peer.ID) []ma.Multiaddr {
// // 	return m.peerAddressFunc(p)
// // }

// // // Increment 方法用于增加指定的 PeerGroupInfo 的计数
// // func (m *mockPeerGroupFilter) Increment(g peerdiversity.PeerGroupInfo) {
// // 	if m.incrementFnc != nil {
// // 		m.incrementFnc(g)
// // 	}
// // }

// // // Decrement 方法用于减少指定的 PeerGroupInfo 的计数
// // func (m *mockPeerGroupFilter) Decrement(g peerdiversity.PeerGroupInfo) {
// // 	if m.decrementFnc != nil {
// // 		m.decrementFnc(g)
// // 	}
// // }

// // TestDiversityFiltering 是用于测试多样性过滤的函数
// func TestDiversityFiltering(t *testing.T) {
// 	local := test.RandPeerIDFatal(t) // 生成一个随机的本地 PeerID
// 	cplCount := make(map[int]int)    // 用于记录每个 Common Prefix Length (Cpl) 的计数的映射
// 	mg := &mockPeerGroupFilter{}     // 创建一个模拟的 PeerGroupFilter 结构体实例
// 	mg.peerAddressFunc = func(p peer.ID) []ma.Multiaddr {
// 		return []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/0")} // 返回指定 Peer 的地址列表
// 	}
// 	mg.allowFnc = func(g peerdiversity.PeerGroupInfo) bool {
// 		return cplCount[g.Cpl] < 1 // 判断指定的 PeerGroupInfo 是否允许添加到路由表中
// 	}

// 	mg.incrementFnc = func(g peerdiversity.PeerGroupInfo) {
// 		cplCount[g.Cpl] = cplCount[g.Cpl] + 1 // 增加指定 PeerGroupInfo 的计数
// 	}

// 	mg.decrementFnc = func(g peerdiversity.PeerGroupInfo) {
// 		cplCount[g.Cpl] = cplCount[g.Cpl] - 1 // 减少指定 PeerGroupInfo 的计数
// 	}

// 	df, err := peerdiversity.NewFilter(mg, "appname", func(p peer.ID) int {
// 		return CommonPrefixLen(ConvertPeerID(local), ConvertPeerID(p)) // 计算指定 Peer 与本地 Peer 的 Common Prefix Length (Cpl)
// 	})
// 	require.NoError(t, err)

// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, pstore.NewMetrics(), NoOpThreshold, df)
// 	require.NoError(t, err)
// 	p, _ := rt.GenRandPeerID(2)                // 生成指定 Common Prefix Length (Cpl) 的随机 PeerID
// 	b, err := rt.TryAddPeer(p, 0, true, false) // 尝试将 Peer 添加到路由表中
// 	require.NoError(t, err)
// 	require.True(t, b) // 断言添加 Peer 成功

// 	p2, _ := rt.GenRandPeerID(2)               // 生成另一个指定 Common Prefix Length (Cpl) 的随机 PeerID
// 	b, err = rt.TryAddPeer(p2, 0, true, false) // 尝试将另一个 Peer 添加到路由表中
// 	require.Error(t, err)                      // 断言添加 Peer 失败
// 	require.False(t, b)

// 	rt.RemovePeer(p)                           // 从路由表中移除 Peer
// 	b, err = rt.TryAddPeer(p2, 0, true, false) // 再次尝试将 Peer 添加到路由表中
// 	require.NoError(t, err)
// 	require.True(t, b) // 断言添加 Peer 成功
// }

// // TestGetPeerInfos 是用于测试获取 Peer 信息的函数
// func TestGetPeerInfos(t *testing.T) {
// 	local := test.RandPeerIDFatal(t) // 生成一个随机的本地 PeerID
// 	m := pstore.NewMetrics()         // 创建一个新的存储度量指标的实例
// 	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)

// 	require.Empty(t, rt.GetPeerInfos()) // 断言获取 Peer 信息为空

// 	p1 := test.RandPeerIDFatal(t) // 生成一个随机的 PeerID
// 	p2 := test.RandPeerIDFatal(t) // 生成另一个随机的 PeerID

// 	b, err := rt.TryAddPeer(p1, 0, false, false) // 尝试将 Peer1 添加到路由表中
// 	require.True(t, b)                           // 断言添加 Peer1 成功
// 	require.NoError(t, err)
// 	b, err = rt.TryAddPeer(p2, 0, true, false) // 尝试将 Peer2 添加到路由表中
// 	require.True(t, b)                         // 断言添加 Peer2 成功
// 	require.NoError(t, err)

// 	ps := rt.GetPeerInfos() // 获取所有 Peer 的信息
// 	require.Len(t, ps, 2)   // 断言 Peer 数量为2
// 	ms := make(map[peer.ID]PeerInfo)
// 	for _, p := range ps {
// 		ms[p.Id] = p // 将 PeerInfo 以 PeerID 为键保存到映射中
// 	}

// 	require.Equal(t, p1, ms[p1].Id)                // 断言 Peer1 的 PeerID 正确
// 	require.True(t, ms[p1].LastUsefulAt.IsZero())  // 断言 Peer1 的 LastUsefulAt 为零值
// 	require.Equal(t, p2, ms[p2].Id)                // 断言 Peer2 的 PeerID 正确
// 	require.False(t, ms[p2].LastUsefulAt.IsZero()) // 断言 Peer2 的 LastUsefulAt 不为零值
// }

// // TestPeerRemovedNotificationWhenPeerIsEvicted 是用于测试在 Peer 被驱逐时的 Peer 移除通知的函数
// func TestPeerRemovedNotificationWhenPeerIsEvicted(t *testing.T) {
// 	t.Parallel()

// 	local := test.RandPeerIDFatal(t) // 生成一个随机的本地 PeerID
// 	m := pstore.NewMetrics()         // 创建一个新的存储度量指标的实例
// 	rt, err := NewRoutingTable(1, ConvertPeerID(local), time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(t, err)
// 	pset := make(map[peer.ID]struct{})
// 	rt.PeerAdded = func(p peer.ID) {
// 		pset[p] = struct{}{} // 将添加的 PeerID 存储到 pset 映射中
// 	}
// 	rt.PeerRemoved = func(p peer.ID) {
// 		delete(pset, p) // 从 pset 映射中删除移除的 PeerID
// 	}

// 	p1, _ := rt.GenRandPeerID(0) // 生成一个随机的 PeerID
// 	p2, _ := rt.GenRandPeerID(0) // 生成另一个随机的 PeerID

// 	// 第一个 Peer 添加成功
// 	b, err := rt.TryAddPeer(p1, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, b)

// 	// 由于容量限制，第二个 Peer 被拒绝添加
// 	b, err = rt.TryAddPeer(p2, 0, true, false)
// 	require.False(t, b)
// 	require.Error(t, err)

// 	// pset 中包含第一个 Peer
// 	require.Contains(t, pset, p1)
// 	require.NotContains(t, pset, p2)

// 	// 标记 Peer 可替换，以便可以驱逐
// 	i := rt.bucketIdForPeer(p1)
// 	rt.tabLock.Lock()
// 	bucket := rt.buckets[i]
// 	rt.tabLock.Unlock()
// 	bucket.getPeer(p1).replaceable = true

// 	b, err = rt.TryAddPeer(p2, 0, true, false) // 尝试添加第二个 Peer
// 	require.NoError(t, err)
// 	require.True(t, b)
// 	require.Contains(t, pset, p2)
// 	require.NotContains(t, pset, p1)
// }

// // BenchmarkAddPeer 用于对添加 Peer 的性能进行基准测试的函数
// func BenchmarkAddPeer(b *testing.B) {
// 	b.StopTimer()
// 	local := ConvertKey("localKey") // 将字符串转换为本地键
// 	m := pstore.NewMetrics()        // 创建一个新的存储度量指标的实例
// 	tab, err := NewRoutingTable(20, local, time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(b, err)

// 	var peers []peer.ID
// 	for i := 0; i < b.N; i++ {
// 		peers = append(peers, test.RandPeerIDFatal(b)) // 生成随机的 PeerID 并添加到 peers 切片中
// 	}

// 	b.StartTimer()
// 	for i := 0; i < b.N; i++ {
// 		tab.TryAddPeer(peers[i], 0, true, false) // 尝试添加 Peer
// 	}
// }

// // BenchmarkFinds 用于对查找 Peer 的性能进行基准测试的函数
// func BenchmarkFinds(b *testing.B) {
// 	b.StopTimer()
// 	local := ConvertKey("localKey") // 将字符串转换为本地键
// 	m := pstore.NewMetrics()        // 创建一个新的存储度量指标的实例
// 	tab, err := NewRoutingTable(20, local, time.Hour, m, NoOpThreshold, nil)
// 	require.NoError(b, err)

// 	var peers []peer.ID
// 	for i := 0; i < b.N; i++ {
// 		peers = append(peers, test.RandPeerIDFatal(b)) // 生成随机的 PeerID 并添加到 peers 切片中
// 		tab.TryAddPeer(peers[i], 0, true, false)       // 尝试添加 Peer
// 	}

// 	b.StartTimer()
// 	for i := 0; i < b.N; i++ {
// 		tab.Find(peers[i]) // 查找 Peer
// 	}
// }
