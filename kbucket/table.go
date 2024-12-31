// Package kbucket implements a kademlia 'k-bucket' routing table.
package kbucket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/sirupsen/logrus"

	"github.com/bpfs/defs/kbucket/peerdiversity"
	"github.com/bpfs/defs/utils/logger"
)

// ErrPeerRejectedHighLatency：表示对等节点被拒绝的错误，原因是延迟太高。
var ErrPeerRejectedHighLatency = fmt.Errorf("对等节点被拒绝：延迟太高")

// ErrPeerRejectedNoCapacity：表示对等节点被拒绝的错误，原因是容量不足。
var ErrPeerRejectedNoCapacity = fmt.Errorf("对等节点被拒绝：容量不足")

// RoutingTable 定义了路由表。
type RoutingTable struct {
	ctx        context.Context    // 路由表的上下文
	ctxCancel  context.CancelFunc // 用于取消 RT 上下文的函数
	local      ID                 // 本地对等节点的 ID
	tabLock    sync.RWMutex       // 总体锁，为了性能优化会进行细化
	metrics    peerstore.Metrics  // 延迟指标
	maxLatency time.Duration      // 该集群中对等节点的最大可接受延迟
	// kBuckets 定义了与其他节点的所有联系。
	buckets        []*bucket          // 存储与其他节点的联系的桶（buckets）。
	bucketsize     int                // 桶的大小。
	cplRefreshLk   sync.RWMutex       // 用于刷新 Cpl 的锁。
	cplRefreshedAt map[uint]time.Time // 存储每个 Cpl 的刷新时间。
	// 通知函数
	PeerRemoved           func(peer.ID)         // 对等节点被移除时的通知函数。
	PeerAdded             func(peer.ID)         //  对等节点被添加时的通知函数。
	usefulnessGracePeriod time.Duration         // usefulnessGracePeriod 是我们给予桶中对等节点的最大宽限期，如果在此期限内对我们没有用处，我们将将其驱逐以为新对等节点腾出位置（如果桶已满）
	df                    *peerdiversity.Filter // 对等节点多样性过滤器。
}

// NewRoutingTable 使用给定的桶大小、本地 ID 和延迟容忍度创建一个新的路由表。
// 参数:
//   - bucketsize: 桶的大小
//   - localID: 本地节点的 ID
//   - latency: 最大可接受的延迟
//   - m: 延迟指标
//   - usefulnessGracePeriod: 对等节点的宽限期
//   - df: 多样性过滤器
//
// 返回值:
//   - *RoutingTable: 新创建的路由表
//   - error: 如果创建失败则返回错误
func NewRoutingTable(bucketsize int, localID ID, latency time.Duration, m peerstore.Metrics, usefulnessGracePeriod time.Duration,
	df *peerdiversity.Filter) (*RoutingTable, error) {
	logger.Info("创建新的路由表")
	rt := &RoutingTable{
		buckets:    []*bucket{newBucket()},
		bucketsize: bucketsize,
		local:      localID,

		maxLatency: latency,
		metrics:    m,

		cplRefreshedAt: make(map[uint]time.Time),

		PeerRemoved: func(peer.ID) {},
		PeerAdded:   func(peer.ID) {},

		usefulnessGracePeriod: usefulnessGracePeriod,

		df: df,
	}

	// 使用 context.WithCancel 函数创建一个后台上下文 ctx 和相应的取消函数 ctxCancel。
	rt.ctx, rt.ctxCancel = context.WithCancel(context.Background())
	logger.Infof("路由表创建成功,桶大小为:%d", bucketsize)
	return rt, nil
}

// Close 关闭路由表及其所有关联的进程。
// 可以安全地多次调用此函数。
//
// 返回值:
//   - error: 如果关闭失败则返回错误
func (rt *RoutingTable) Close() error {
	logger.Info("关闭路由表")
	rt.ctxCancel()
	return nil
}

// NPeersForCpl 返回给定 Cpl 的对等节点数量。
// 参数:
//   - cpl: 公共前缀长度
//
// 返回值:
//   - int: 具有给定 Cpl 的对等节点数量
func (rt *RoutingTable) NPeersForCpl(cpl uint) int {
	rt.tabLock.RLock()
	defer rt.tabLock.RUnlock()

	// 如果 Cpl 大于等于最后一个桶的索引
	if int(cpl) >= len(rt.buckets)-1 {
		count := 0
		b := rt.buckets[len(rt.buckets)-1]
		for _, p := range b.peers() {
			// 如果本地对等节点和当前对等节点的 DHT ID 的公共前缀长度等于 Cpl
			if CommonPrefixLen(rt.local, p.dhtId) == int(cpl) {
				count++
			}
		}
		logger.Debugf("cpl=%d count=%d", cpl, count)
		return count
	} else {
		// 返回索引为 Cpl 的桶中的对等节点数量
		count := rt.buckets[cpl].len()
		logger.Debugf("cpl=%d count=%d", cpl, count)
		return count
	}
}

// UsefulNewPeer 验证给定的 peer.ID 是否适合路由表。
// 如果对等节点尚未在路由表中，或者与 peer.ID 对应的桶没有满，或者它包含可替换的对等节点，或者它是最后一个桶且添加对等节点会拆分该桶，则返回 true。
//
// 参数:
//   - p: 要验证的对等节点 ID
//
// 返回值:
//   - bool: 如果对等节点适合路由表则返回 true，否则返回 false
func (rt *RoutingTable) UsefulNewPeer(p peer.ID) bool {
	rt.tabLock.RLock()
	defer rt.tabLock.RUnlock()

	// 与 p 对应的桶
	bucketID := rt.bucketIdForPeer(p)
	bucket := rt.buckets[bucketID]

	if bucket.getPeer(p) != nil {
		// 对��节点已经存在于路由表中，因此不是有用的
		logger.Debug("对等节点已存在于路由表中")
		return false
	}

	// 桶未满
	if bucket.len() < rt.bucketsize {
		logger.Debug("对等节点可以添加到未满的桶中")
		return true
	}

	// 桶已满，检查是否包含可替换的对等节点
	for e := bucket.list.Front(); e != nil; e = e.Next() {
		peer := e.Value.(*PeerInfo)
		if peer.replaceable {
			// 至少有一个可替换的对等节点
			logger.Debug("对等节点可以替换现有的可替换节点")
			return true
		}
	}

	// 最后一个桶可能包含具有不同 CPL 的对等节点 ID，并且如果需要，可以拆分为两个桶
	if bucketID == len(rt.buckets)-1 {
		peers := bucket.peers()
		cpl := CommonPrefixLen(rt.local, ConvertPeerID(p))
		for _, peer := range peers {
			// 如果至少有两个对等节点具有不同的 CPL，则新的对等节点是有用的，并且将触发桶的拆分
			if CommonPrefixLen(rt.local, peer.dhtId) != cpl {
				logger.Debug("对等节点可以触发最后一个桶的拆分")
				return true
			}
		}
	}

	// 适当的桶已满，且没有可替换的对等节点
	logger.Debug("对等节点不适合添加到路由表中")
	return false
}

// TryAddPeer 尝试将对��节点添加到路由表。
// 如果对等节点已经存在于路由表中并且之前已经查询过，则此调用不执行任何操作。
// 如果对等节点已经存在于路由表中但之前没有进行过查询，则将其 LastUsefulAt 值设置为当前时间。
// 这需要这样做是因为当我们第一次连接到对等节点时，我们不会将其标记为"有用"（通过设置 LastUsefulAt 值）。
//
// 如果对等节点是一个查询对等节点，即我们查询过它或它查询过我们，我们将 LastSuccessfulOutboundQuery 设置为当前时间。
// 如果对等节点只是一个我们连接到的对等节点/它连接到我们而没有进行任何 DHT 查询，则认为它没有 LastSuccessfulOutboundQuery。
//
// 如果对等节点所属的逻辑桶已满且不是最后一个桶，我们尝试用新的对等节点替换该桶中上次成功的出站查询时间超过允许阈值的现有对等节点。
// 如果该桶中不存在这样的对等节点，则不将对等节点添加到路由表中，并返回错误 "ErrPeerRejectedNoCapacity"。
//
// 参数:
//   - p: 要添加的对等节点 ID，用于唯一标识一个对等节点
//   - mode: 运行模式，指定路由表的工作模式，影响对等节点的添加策略
//   - queryPeer: 是否是查询对等节点，如果为 true 表示这是一个我们主动查询过或者查询过我们的节点，
//     这类节点在路由表中更价值，因为它们参与了 DHT 查询
//   - isReplaceable: 是否可替换，如果为 true 表示当桶满时，此节点可以被新的节点替换掉。
//     这通常用于控制路由表的更新策略，避免重要节点被随意替换
//
// 返回值:
//   - bool: 如果对等节点是新添加到路由表中的，则返回 true；否则返回 false
//   - error: 如果添加失败则返回错误
func (rt *RoutingTable) TryAddPeer(p peer.ID, mode int, queryPeer bool, isReplaceable bool) (bool, error) {
	rt.tabLock.Lock()
	defer rt.tabLock.Unlock()

	return rt.addPeer(p, mode, queryPeer, isReplaceable)
}

// addPeer 方法将对等节点添加到路由表。
//
// 参数:
//   - p: 要添加的对等节点 ID
//   - mode: 运行模式
//   - queryPeer: 是否是查询对等节点
//   - isReplaceable: 是否可替换
//
// 返回值:
//   - bool: 如果对等节点是新添加到路由表中的，则返回 true；否则返回 false
//   - error: 如果添加失败则返回错误
//
// 注意：调用 addPeer 方法之前，需要确保路由表的写入操作已经加锁。
func (rt *RoutingTable) addPeer(p peer.ID, mode int, queryPeer bool, isReplaceable bool) (bool, error) {
	// 根据对等节点的 peer.ID 计算桶的 ID。
	bucketID := rt.bucketIdForPeer(p)
	// 获取对应桶的引用。
	bucket := rt.buckets[bucketID]

	// 获取当前���间。
	now := time.Now()
	var lastUsefulAt time.Time
	if queryPeer {
		lastUsefulAt = now
	}

	// 对等节点已经存在于路由表中。
	if peerInfo := bucket.getPeer(p); peerInfo != nil {
		// 如果我们在添加对等节点之后第一次查询它，让我们给它一个有用性提升。这只会发生一次。
		if peerInfo.LastUsefulAt.IsZero() && queryPeer {
			peerInfo.LastUsefulAt = lastUsefulAt
			logger.Debug("更新对等节点的LastUsefulAt时间")
		}
		return false, nil
	}

	// 对等节点的延迟阈值不可接受。
	if rt.metrics.LatencyEWMA(p) > rt.maxLatency {
		// 连接不符合要求，跳过！
		logger.Warn("对等节点的延迟超过阈值")
		return false, ErrPeerRejectedHighLatency
	}

	// 将对等节点添加到多样性过滤器中。
	// 如果我们无法在表中找到对等节点的位置，我们将在稍后从过滤器中将其删除。
	if rt.df != nil {
		if !rt.df.TryAdd(p) {
			logger.Warn("对等节点被多样性过滤器拒绝")
			return false, fmt.Errorf("对等节点被多样性过滤器拒绝")
		}
	}

	// 我们在桶中有足够的空间（无论是生成的桶还是分的桶）。
	if bucket.len() < rt.bucketsize {
		// 创建新的对等节点信息将其添加到桶的前面。
		bucket.pushFront(&PeerInfo{
			Id:                            p,                // 对等节点的 ID
			Mode:                          mode,             // 当前的运行模式
			LastUsefulAt:                  lastUsefulAt,     // 对等节点上次对我们有用的时间点（请参阅 DHT 文档以了解有用性的定义）
			LastSuccessfulOutboundQueryAt: now,              // 我们最后一次从对等节点获得成功的查询响应的时间点
			AddedAt:                       now,              // 将此对等节点添加到路由表的时间点
			dhtId:                         ConvertPeerID(p), // 对等节点在 DHT XOR keyspace 中的 ID
			replaceable:                   isReplaceable,    // 如果一个桶已满，此对等节点可以被替换以为新对等节点腾出空间
		})
		// 更新路由表的相关状态。
		rt.PeerAdded(p)
		logger.Infof("成功将对等节点添加到桶 %d", bucketID)
		return true, nil
	}

	if bucketID == len(rt.buckets)-1 {
		// 如果桶太大，并且这是最后一个桶（即通配符桶），则展开它。
		rt.nextBucket()
		// 表的结构已经改变，因此让我们重新检查对等节点是否现在有一专用的桶。
		bucketID = rt.bucketIdForPeer(p)
		bucket = rt.buckets[bucketID]

		// 仅在拆分后的桶不会溢出时才将对等节点推入。
		if bucket.len() < rt.bucketsize {
			// 创建新的对等节点信息并将其添加到桶的前面。
			bucket.pushFront(&PeerInfo{
				Id:                            p,                // 对等节点的 ID
				Mode:                          mode,             // 当前的运行模式
				LastUsefulAt:                  lastUsefulAt,     // 对等节点上次对我们有用的时间点（请参阅 DHT 文档以了解有用性的定义）
				LastSuccessfulOutboundQueryAt: now,              // 我们最后一次从对等节点获得成功的查询响应的时间点
				AddedAt:                       now,              // 将此对等节点添加到路由表的时间点
				dhtId:                         ConvertPeerID(p), // 对等节点在 DHT XOR keyspace 中的 ID
				replaceable:                   isReplaceable,    // 如果一个桶已满，此对等节点可以被替换以为新对等节点腾出空间
			})
			// 更新路由表的相关状态。
			rt.PeerAdded(p)
			logger.Infof("成功将对等节点添加到拆分后的桶 %d", bucketID)
			return true, nil
		}
	}

	// 对等节点所属的桶已满。让我们尝试在该桶中找到一个可替换的对等节点。
	// 在这里我不需要稳定排序，因为无论替换哪个对等节点，都无关紧要，只要它是可替换的对等节点即可。
	replaceablePeer := bucket.min(func(p1 *PeerInfo, p2 *PeerInfo) bool {
		return p1.replaceable
	})

	if replaceablePeer != nil && replaceablePeer.replaceable {
		// 我们找到了一个可替换的对等节点，让我们用新的对等节点替换它。

		// 将新的对等节点添加到桶中。在删除可替换的对等节点之前需要这样做，
		// 因为如果桶的大小为 1，我们将删除唯一的对等节点，并删除桶。
		bucket.pushFront(&PeerInfo{
			Id:                            p,                // 对等节点的 ID
			Mode:                          mode,             // 当前的运行模式
			LastUsefulAt:                  lastUsefulAt,     // 对等节点上次对我们有用的时间点（请参阅 DHT 文档以了解有用性的定义）
			LastSuccessfulOutboundQueryAt: now,              // 我们最后一次从对等节点获得成功的查询响应的时间点
			AddedAt:                       now,              // 将此对等节点添加到路由表的时间点
			dhtId:                         ConvertPeerID(p), // 对等节点在 DHT XOR keyspace 中的 ID
			replaceable:                   isReplaceable,    // 如果一个桶已满，此对等节点可以被替换以为新对等节点腾出空间
		})
		// 更新路由表的相关状态。
		rt.PeerAdded(p)

		// 移除被替换的对等节点。
		rt.removePeer(replaceablePeer.Id)
		logger.Infof("成功用对等节点替换了可替换节点")
		return true, nil
	}

	// 我们无法找到对等节点的位置，从过滤器中将其删除。
	if rt.df != nil {
		rt.df.Remove(p)
	}
	logger.Warn("无法为对等节点找到位置，容量不足")
	return false, ErrPeerRejectedNoCapacity
}

// MarkAllPeersIrreplaceable 将路由表中的所有对等节点标记为不可替换。
// 这意味着我们永远不会替换表中的现有对等节点以为新对等节点腾出空间。
// 但是，可以通过调用 `RemovePeer` API 来删除它们。
func (rt *RoutingTable) MarkAllPeersIrreplaceable() {
	rt.tabLock.Lock()         // 锁定路由表，确保并发安全。
	defer rt.tabLock.Unlock() // 在函数返回时解锁路由表。

	for i := range rt.buckets {
		b := rt.buckets[i]
		b.updateAllWith(func(p *PeerInfo) {
			p.replaceable = false // 将每个对等节点的可替换属性设置为 false，标记为不可替换。
		})
	}
	logger.Info("已将所有对等节点标记为不可替换")
}

// GetPeerInfos 返回我们在桶中存储的对等节点信息。
//
// 返回值:
//   - []PeerInfo: 包含所有对等节点信息的切片
func (rt *RoutingTable) GetPeerInfos() []PeerInfo {
	rt.tabLock.RLock()         // 以读取模式锁定路由表，确保并发安全。
	defer rt.tabLock.RUnlock() // 在函数返回时解锁路由表。

	var pis []PeerInfo
	for _, b := range rt.buckets {
		pis = append(pis, b.peers()...) // 将每个桶中的对等节点信息追加到 pis 切片中。
	}
	logger.Debugf("返回 %d 个对等节点信息", len(pis))
	return pis // 返回包含所有对等节点信息的切片。
}

// UpdateLastSuccessfulOutboundQueryAt 更新对等节点最后一次成功响应我们查询的时间。
// 这个时间戳用于评估对等节点的活跃度和可靠性。
// 如果一个对等节点长期没有成功响应查询，说明它可能已经离线或不可用，可以考虑将其从路由表中移除。
//
// 参数:
//   - p: 要更新的对等节点 ID
//   - t: 新的时间戳
//
// 返回值:
//   - bool: 如果更新成功则返回 true，否则返回 false
func (rt *RoutingTable) UpdateLastSuccessfulOutboundQueryAt(p peer.ID, t time.Time) bool {
	rt.tabLock.Lock()         // 锁定路由表，确保并发安全。
	defer rt.tabLock.Unlock() // 在函数返回时解锁路由表。

	bucketID := rt.bucketIdForPeer(p) // 获取对等节点所在的桶 ID。
	bucket := rt.buckets[bucketID]    // 获取对应的桶。

	if pc := bucket.getPeer(p); pc != nil {
		pc.LastSuccessfulOutboundQueryAt = t // 更新对等节点的 LastSuccessfulOutboundQueryAt 时间。
		logger.Debugf("更新对等节点的查询时间为 %s", t)
		return true // 更新成功，返回 true。
	}
	logger.Warn("未找到对等节点")
	return false // 更新失败，返回 false。
}

// UpdateLastUsefulAt 更新对等节点最后一次对我们有帮助的时间。
// 对等节点为我们的查询提供有价值的响应时(例如返回我们需要的数据或者提供有效的路由信息)，应该更新这个时间戳。
// 这个时间戳用于评估对等节点的价值，帮助我们在路由表空间有限时决定保留哪些节点。经常提供帮助的节点更有可能被保留在路由表中。
//
// 参数:
//   - p: 要更新的对等节点 ID
//   - t: 新的时间戳
//
// 返回值:
//   - bool: 如果更新成功则返回 true，否则返回 false
func (rt *RoutingTable) UpdateLastUsefulAt(p peer.ID, t time.Time) bool {
	rt.tabLock.Lock()         // 锁定路由表，确保并发安全。
	defer rt.tabLock.Unlock() // 在函数返回时解锁路由表。

	bucketID := rt.bucketIdForPeer(p) // 获取对等节点所在的桶 ID。
	bucket := rt.buckets[bucketID]    // 获取对应的桶。

	if pc := bucket.getPeer(p); pc != nil {
		pc.LastUsefulAt = t // 更新对等节点的 LastUsefulAt 时间。
		logger.Debugf("更新对等节点的有用时间为 %s", t)
		return true // 更新成功，返回 true。
	}
	logger.Warn("未找到对等节点")
	return false // 更新失败，返回 false。
}

// RemovePeer 在调用者确定某个对等节点对查询不再有用时应调用此方法。
// 例如：对等节点可能停止支持 DHT 协议。
// 它从路由表中驱逐该对等节点。
//
// 参数:
//   - p: 要移除的对等节点 ID
func (rt *RoutingTable) RemovePeer(p peer.ID) {
	rt.tabLock.Lock()         // 锁定路由表，确保并发安全。
	defer rt.tabLock.Unlock() // 在函数返回时解锁路由表。
	rt.removePeer(p)          // 调用内部的 removePeer 方法，从路由表中移除对等节点。
}

// removePeer locking is the responsibility of the caller
// 锁定的责任由调用者承担
//
// 参数:
//   - p: 要移除的对等节点 ID
//
// 返回值:
//   - bool: 如果移除成功则返回 true，否则返回 false
func (rt *RoutingTable) removePeer(p peer.ID) bool {
	bucketID := rt.bucketIdForPeer(p) // 获取对等节点所在的桶 ID。
	bucket := rt.buckets[bucketID]    // 获取对应的桶。

	if bucket.remove(p) { // 如果从桶中成功移除对等节点。
		if rt.df != nil {
			rt.df.Remove(p) // 如果存在默认路由函数，则从默认路由函数中移除对等节点。
		}

		for {
			lastBucketIndex := len(rt.buckets) - 1

			// 如果最后一个桶为空且不是唯一的桶，则移除最后一个桶。
			if len(rt.buckets) > 1 && rt.buckets[lastBucketIndex].len() == 0 {
				rt.buckets[lastBucketIndex] = nil
				rt.buckets = rt.buckets[:lastBucketIndex]
			} else if len(rt.buckets) >= 2 && rt.buckets[lastBucketIndex-1].len() == 0 {
				// 如果倒数第二个桶刚变为空，并且至少有两个桶，则移除倒数第二个桶，并用最后一个桶替换它。
				rt.buckets[lastBucketIndex-1] = rt.buckets[lastBucketIndex]
				rt.buckets[lastBucketIndex] = nil
				rt.buckets = rt.buckets[:lastBucketIndex]
			} else {
				break
			}
		}

		rt.PeerRemoved(p) // 对等节点移除回调函数。
		return true       // 移除成功，返回 true。
	}
	return false // 移除失败，返回 false。
}

// nextBucket 是展开路由表中的下一个桶
func (rt *RoutingTable) nextBucket() {
	// 这是最后一个桶，据说它是一个混合的容器，包含那些不属于专用（展开的）桶的节点。
	// 这里使用 "_allegedly_" 来表示最后一个桶中的 *所有* 节点可能实际上属于其他桶。
	// 这可能发生在我们展开了4个桶之后，最后一个桶中的所有节点实际上属于第8个桶。
	bucket := rt.buckets[len(rt.buckets)-1]

	// 将最后一个桶分割成两个桶，第一个桶保留原来的节点，第二个桶包含一部分节点。
	newBucket := bucket.split(len(rt.buckets)-1, rt.local)
	rt.buckets = append(rt.buckets, newBucket)

	// 新形成的桶仍然包含太多的节点。可能是因为我展开了一个空的桶。
	if newBucket.len() >= rt.bucketsize {
		// 持续展路由表，直到最后一个桶不再溢出。
		rt.nextBucket()
	}
}

// Find 根据给定的 ID 查找特定的节点，如果找不到则返回 nil
//
// 参数:
//   - id: 要查找的对等节点 ID
//
// 返回值:
//   - peer.ID: 找到的对等节点 ID，如果未找到则返回空字符串
func (rt *RoutingTable) Find(id peer.ID) peer.ID {
	// 调用 NearestPeers 方法查找离给定 ID 最近的节点，最多返回一个节点
	srch := rt.NearestPeers(ConvertPeerID(id), 1)

	// 如果找不到节点或者找到的节点与给定的 ID 不匹配，则返回空字符串
	if len(srch) == 0 || srch[0] != id {
		return ""
	}

	// 返回找到的节点 ID
	return srch[0]
}

// NearestPeer  返回距离给定 ID 最近的单个节点
//
// 参数:
//   - id: 目标 ID
//
// 返回值:
//   - peer.ID: 最近的对等节点 ID，如果未找到则返回空字符串
func (rt *RoutingTable) NearestPeer(id ID) peer.ID {
	// 调用 NearestPeers 方法查找距离给定 ID 最近的节点，最多返回一个节点
	peers := rt.NearestPeers(id, 1)

	// 如果找到了节点，则返回第一个节点的 ID
	if len(peers) > 0 {
		return peers[0]
	}

	// 打印调试信息，表示没有找到最近的节点，同时输出当前路由表的大小
	logrus.Debugf("NearestPeer: 返回空值，表的大小为 %d", rt.Size())

	// 返回空字符串表示没有找到最近的节点
	return ""
}

// NearestPeers 返回距离给定 ID 最近的 'count' 个节点的列表
//
// 参数:
//   - id: 目标 ID
//   - count: 要返回的最近节点数量
//   - mode: 可选参数，用于约束目标节点的运行模式
//
// 返回值:
//   - []peer.ID: 最近节点的 ID 列表
func (rt *RoutingTable) NearestPeers(id ID, count int, mode ...int) []peer.ID {
	// 计算目标 ID 与本地节点 ID 的公共前缀长度
	// 该桶中的所有节点与我们共享 cpl 位，因此它们与给定键至少共享 cpl+1 位
	// +1 是因为目标和该桶中的所有节点在 cpl 位上与我们不同
	cpl := CommonPrefixLen(id, rt.local)

	// 获取路由表的读锁
	rt.tabLock.RLock()

	// 如果 cpl 超出桶的范围，则将其设置为最后一个桶的索引
	if cpl >= len(rt.buckets) {
		cpl = len(rt.buckets) - 1
	}

	// 初始化节点距离排序器
	pds := peerDistanceSorter{
		peers:  make([]peerDistance, 0, count+rt.bucketsize),
		target: id,
	}

	// 从目标桶（cpl+1 个共享位）添加节点
	pds.appendPeersFromList(rt.buckets[cpl].list, mode...)

	// 如果节点数量不足，从右侧桶中添加节点
	// 右侧桶共享 cpl 位（与 cpl 桶中的节点共享 cpl+1 位不同）
	if pds.Len() < count {
		for i := cpl + 1; i < len(rt.buckets); i++ {
			pds.appendPeersFromList(rt.buckets[i].list, mode...)
		}
	}

	// 如果节点数量仍然不足，从左侧桶中添加节点
	// 每个桶与上一个桶相比共享的位数少 1
	for i := cpl - 1; i >= 0 && pds.Len() < count; i-- {
		pds.appendPeersFromList(rt.buckets[i].list, mode...)
	}

	// 释放路由表的读锁
	rt.tabLock.RUnlock()

	// 按与目标 ID 的距离对节点进行排序
	pds.sort()

	// 如果找到的节点数量超过要求的数量，则截取前 count 个节点
	if count < pds.Len() {
		pds.peers = pds.peers[:count]
	}

	// 构建输出列表
	out := make([]peer.ID, 0, pds.Len())
	for _, p := range pds.peers {
		// 如果没有指定运行模式，直接添加节点
		if len(mode) == 0 {
			out = append(out, p.p)
			continue
		}
		// 如果指定了运行模式，只添加匹配模式的节点
		if p.mode == mode[0] {
			out = append(out, p.p)
		}
	}

	// 返回最近节点的 ID 列表
	return out
}

// Size 返回路由表中的节点总数
// 参数:
//   - mode: 可选参数，用于过滤特定运行模式的节点数量
//     如果不指定，则返回所有节点数量
//     如果指定，则只返回该运行模式下的节点数量
//
// 返回值:
//   - int: 符合条件的节点总数
//
// 示例:
//
//	// 获取所有节点数量
//	totalSize := rt.Size()
//	// 获取服务器模式(mode=1)的节点数量
//	serverSize := rt.Size(1)
//	// 获取客户端模式(mode=0)的节点数量
//	clientSize := rt.Size(0)
func (rt *RoutingTable) Size(mode ...int) int {
	var tot int
	// 获取路由表的读锁
	rt.tabLock.RLock()
	defer rt.tabLock.RUnlock()

	// 遍历所有桶
	for _, buck := range rt.buckets {
		// 如果没有指定mode，直接累加桶中所有节点
		if len(mode) == 0 {
			tot += buck.len()
			continue
		}

		// 如果指定了mode，只统计匹配mode的节点
		for e := buck.list.Front(); e != nil; e = e.Next() {
			if peer := e.Value.(*PeerInfo); peer.Mode == mode[0] {
				tot++
			}
		}
	}

	logger.Debugf("路由表大小: %d (mode=%v)", tot, mode)
	return tot
}

// ListPeers 返回路由表中所有节点的 ID 列表
// 参数:
//   - mode: 可选参数，用于过滤特定运行模式的节点
//     如果不指定，则返回所有节点
//     如果指定，则只返回该运行模式下的节点
//
// 返回值:
//   - []peer.ID: 符合条件的节点 ID 列表
//
// 示例:
//
//	// 获取所有节点列表
//	allPeers := rt.ListPeers()
//	// 获取服务器模式(mode=1)的节点列表
//	serverPeers := rt.ListPeers(1)
//	// 获取客户端模式(mode=0)的节点列表
//	clientPeers := rt.ListPeers(0)
func (rt *RoutingTable) ListPeers(mode ...int) []peer.ID {
	// 获取路由表的读锁
	rt.tabLock.RLock()
	defer rt.tabLock.RUnlock()

	var peers []peer.ID

	// 遍历所有桶
	for _, buck := range rt.buckets {
		// 如果没有指定mode，获取桶中所有节点
		if len(mode) == 0 {
			peers = append(peers, buck.peerIds()...)
			continue
		}

		// 如果指定了mode，只获取匹配mode的节点
		for e := buck.list.Front(); e != nil; e = e.Next() {
			if peer := e.Value.(*PeerInfo); peer.Mode == mode[0] {
				peers = append(peers, peer.Id)
			}
		}
	}

	logger.Debugf("获取到 %d 个节点 (mode=%v)", len(peers), mode)
	return peers
}

// Print 打印路由表的详细信息到标准输出
// 打印内容包括:
// - 路由表的基本配置(桶大小、最大延迟等)
// - 每个桶中节点的详细信息(ID、模式、延迟等)
// - 节点统计信息(总数、过滤后数量等)
//
// 参数:
//   - mode: 可选参数，用于过滤特定运行模式的节点
//     如果不指定，则打印所有节点
//     如果指定，则只打印该运行模式下的节点
//
// 示例:
//
//	// 打印所有节点信息
//	rt.Print()
//	// 打印服务器模式(mode=1)的节点信息
//	rt.Print(1)
//	// 打印客户端模式(mode=0)的节点信息
//	rt.Print(0)
func (rt *RoutingTable) Print(mode ...int) {
	// 打印路由表的基本信息
	fmt.Printf("路由表, 桶大小 = %d, 最大延迟 = %d\n", rt.bucketsize, rt.maxLatency)
	if len(mode) > 0 {
		fmt.Printf("按模式过滤: %d\n", mode[0])
	}

	// 获取路由表的读锁
	rt.tabLock.RLock()
	defer rt.tabLock.RUnlock()

	// 用于统计节点数量
	totalNodes := 0
	filteredNodes := 0

	// 遍历所有桶
	for i, b := range rt.buckets {
		// 用于存储当前桶中符合条件的节点
		var bucketNodes []string

		// 遍历当前桶中的所有节点
		for e := b.list.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*PeerInfo)
			totalNodes++

			// 如果指定了mode且不匹配，则跳过
			if len(mode) > 0 && peer.Mode != mode[0] {
				continue
			}
			filteredNodes++

			// 格式化节点信息
			nodeInfo := fmt.Sprintf("\t\t- %s mode=%d latency=%s\n",
				peer.Id.String(),
				peer.Mode,
				rt.metrics.LatencyEWMA(peer.Id).String())
			bucketNodes = append(bucketNodes, nodeInfo)
		}

		// 如果桶中有符合条件的节点，才打印桶信息
		if len(bucketNodes) > 0 {
			fmt.Printf("\tbucket: %d\n", i)
			for _, nodeInfo := range bucketNodes {
				fmt.Print(nodeInfo)
			}
		}
	}

	// 打印统计信息
	if len(mode) > 0 {
		fmt.Printf("\n总节点数: %d, 过滤后节点数 (模式=%d): %d\n",
			totalNodes, mode[0], filteredNodes)
	} else {
		fmt.Printf("\n总节点数: %d\n", totalNodes)
	}
}

// GetDiversityStats 返回路由表的多样性统计信息
//
// 返回值:
//   - []peerdiversity.CplDiversityStats: 多样性统计信息，如果未配置多样性过滤器则返回 nil
func (rt *RoutingTable) GetDiversityStats() []peerdiversity.CplDiversityStats {
	if rt.df != nil {
		return rt.df.GetDiversityStats()
	}
	return nil
}

// bucketIdForPeer 返回给定节点 ID 应该所在的桶的索引
//
// 参数:
//   - p: 节点 ID
//
// 返回值:
//   - int: 桶的索引
//
// 注意: 调用者负责加锁
func (rt *RoutingTable) bucketIdForPeer(p peer.ID) int {
	// 将节点 ID 转换为内部格式
	peerID := ConvertPeerID(p)

	// 计算与本地节点的公共前缀长度
	cpl := CommonPrefixLen(peerID, rt.local)
	bucketID := cpl

	// 如果 bucketID 超出桶的范围，则将其设置为最后一个桶的索引
	if bucketID >= len(rt.buckets) {
		bucketID = len(rt.buckets) - 1
	}

	return bucketID
}

// maxCommonPrefix 返回路由表中任意节点与本地节点之间的最大公共前缀长度
//
// 返回值:
//   - uint: 最大公共前缀长度
func (rt *RoutingTable) maxCommonPrefix() uint {
	// 获取路由表的读锁
	rt.tabLock.RLock()
	defer rt.tabLock.RUnlock()

	// 从最后一个桶开始向前遍历
	for i := len(rt.buckets) - 1; i >= 0; i-- {
		// 如果当前��中有节点
		if rt.buckets[i].len() > 0 {
			// 返回当前桶中节点与本地节点的最大公共前缀长度
			return rt.buckets[i].maxCommonPrefix(rt.local)
		}
	}

	// 如果路由表为空，则返回 0
	return 0
}
