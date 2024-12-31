//go:generate go run ./generate

package kbucket

import (
	"container/list"
	"time"

	"github.com/bpfs/defs/utils/logger"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerInfo 包含了 K-Bucket 中一个对等节点的所有相关信息。
type PeerInfo struct {
	Id                            peer.ID   // 对等节点的唯一标识符
	Mode                          int       // 对等节点的运行模式(如 DHT 服务器/客户端)
	LastUsefulAt                  time.Time // 对等节点上次对我们有用的时间点(参见 DHT 文档中有用性的定义)
	LastSuccessfulOutboundQueryAt time.Time // 我们最后一次从该对等节点获得成功查询响应的时间点
	AddedAt                       time.Time // 该对等节点被添加到路由表的时间点
	dhtId                         ID        // 对等节点在 DHT XOR 密钥空间中的 ID
	replaceable                   bool      // 当桶已满时,该对等节点是否可以被替换以容纳新节点
}

// bucket 是一个对等节点列表。
// 所有对 bucket 的访问都在路由表的锁保护下进行同步,
// 因此 bucket 本身不需要任何锁。
// 如果将来需要避免在访问 bucket 时锁定整个表,
// 调用者将需要负责同步对 bucket 的所有访问。
type bucket struct {
	list *list.List // 存储对等节点信息的双向链表
}

// newBucket 创建并返回一个新的空 bucket。
//
// 返回值:
//   - *bucket: 新创建的空 bucket
func newBucket() *bucket {
	logger.Debugf("创建新的 bucket")
	b := new(bucket)    // 分配新的 bucket 结构体
	b.list = list.New() // 初始化双向链表
	return b            // 返回新创建的 bucket
}

// peers 返回桶中所有对等节点的信息列表。
// 返回的是一个防御性副本,调用者可以安全地修改返回的切片。
//
// 返回值:
//   - []PeerInfo: 包含所有对等节点信息的切片
func (b *bucket) peers() []PeerInfo {
	logger.Debugf("获取 bucket 中所有对等节点信息")
	ps := make([]PeerInfo, 0, b.len())                // 创建容量为桶大小的切片
	for e := b.list.Front(); e != nil; e = e.Next() { // 遍历桶中所有节点
		p := e.Value.(*PeerInfo) // 获取节点的 PeerInfo
		ps = append(ps, *p)      // 将 PeerInfo 的副本添加到切片
	}
	return ps // 返回包含所有节点信息的切片
}

// min 根据给定的比较函数返回桶中的"最小"对等节点。
// 注意:比较函数会直接修改传入的 PeerInfo 指针,返回值也不应被修改。
//
// 参数:
//   - lessThan: 用于比较两个节点大小的函数
//
// 返回值:
//   - *PeerInfo: 最"小"的节点,如果桶为空则返回 nil
func (b *bucket) min(lessThan func(p1 *PeerInfo, p2 *PeerInfo) bool) *PeerInfo {
	logger.Debugf("查找 bucket 中的最小对等节点")
	if b.list.Len() == 0 { // 如果桶为空
		logger.Debugf("bucket 为空")
		return nil // 返回 nil
	}

	minVal := b.list.Front().Value.(*PeerInfo) // 初始化最小值为第一个节点

	for e := b.list.Front().Next(); e != nil; e = e.Next() { // 遍历剩余节点
		val := e.Value.(*PeerInfo) // 获取当前节点
		if lessThan(val, minVal) { // 如果当前节点更小
			minVal = val // 更新最小值
		}
	}

	return minVal // 返回最小节点
}

// updateAllWith 使用给定的更新函数更新桶中的所有对等节点。
//
// 参数:
//   - updateFnc: 用于更新节点信息的函数
func (b *bucket) updateAllWith(updateFnc func(p *PeerInfo)) {
	logger.Debugf("更新 bucket 中所有对等节点")
	for e := b.list.Front(); e != nil; e = e.Next() { // 遍历所有节点
		val := e.Value.(*PeerInfo) // 获取节点信息
		updateFnc(val)             // 应用更新函数
	}
}

// peerIds 返回桶中所有对等节点的 ID 列表。
//
// 返回值:
//   - []peer.ID: 包含所有节点 ID 的切片
func (b *bucket) peerIds() []peer.ID {
	logger.Debugf("获取 bucket 中所有对等节点 ID")
	ps := make([]peer.ID, 0, b.list.Len())            // 创建容量为桶大小的切片
	for e := b.list.Front(); e != nil; e = e.Next() { // 遍历所有节点
		p := e.Value.(*PeerInfo) // 获取节点信息
		ps = append(ps, p.Id)    // 添加节点 ID
	}
	return ps // 返回 ID 列表
}

// getPeer 根据给定的 ID 查找并返回对应的对等节点信息。
//
// 参数:
//   - p: 要查找的对等节点 ID
//
// 返回值:
//   - *PeerInfo: 找到的节点信息,如果不存在则返回 nil
func (b *bucket) getPeer(p peer.ID) *PeerInfo {
	logger.Debugf("查找对等节点: %s", p)
	for e := b.list.Front(); e != nil; e = e.Next() { // 遍历所有节点
		if e.Value.(*PeerInfo).Id == p { // 如果找到匹配的 ID
			return e.Value.(*PeerInfo) // 返回节点信息
		}
	}
	logger.Debugf("未找到对等节点: %s", p)
	return nil // 未找到则返回 nil
}

// remove 从桶中移除指定 ID 的对等节点。
//
// 参数:
//   - id: 要移除的对等节点 ID
//
// 返回值:
//   - bool: 如果成功移除返回 true,节点不存在返回 false
func (b *bucket) remove(id peer.ID) bool {
	logger.Debugf("移除对等节点: %s", id)
	for e := b.list.Front(); e != nil; e = e.Next() { // 遍历所有节点
		if e.Value.(*PeerInfo).Id == id { // 如果找到匹配的 ID
			b.list.Remove(e) // 从链表中移除
			logger.Debugf("成功移除对等节点: %s", id)
			return true // 返回移除成功
		}
	}
	logger.Debugf("未找到要移除的对等节点: %s", id)
	return false // 未找到则返回失败
}

// pushFront 将给定的对等节点信息添加到桶的前端。
//
// 参数:
//   - p: 要添加的对等节点信息
func (b *bucket) pushFront(p *PeerInfo) {
	logger.Debugf("添加对等节点到 bucket 前端: %s", p.Id)
	b.list.PushFront(p) // 将节点添加到链表前端
}

// len 返回桶中对等节点的数量。
//
// 返回值:
//   - int: 桶中节点的数量
func (b *bucket) len() int {
	length := b.list.Len()
	logger.Debugf("bucket 当前长度: %d", length)
	return length // 返回链表长度
}

// split 将桶中的节点按照与目标的共同前缀长度(CPL)分割成两个桶。
// 原桶保留 CPL 等于 cpl 的节点,返回的新桶包含 CPL 大于 cpl 的节点。
//
// 参数:
//   - cpl: 用于分割的共同前缀长度
//   - target: 目标节点 ID
//
// 返回值:
//   - *bucket: 包含 CPL 大于 cpl 的节点的新桶
func (b *bucket) split(cpl int, target ID) *bucket {
	logger.Debugf("分割 bucket, CPL: %d, 目标: %v", cpl, target)
	out := list.New()      // 创建新链表
	newbuck := newBucket() // 创建新桶
	newbuck.list = out     // 设置新桶的链表
	e := b.list.Front()    // 获取第一个节点
	for e != nil {
		pDhtId := e.Value.(*PeerInfo).dhtId        // 获取节点的 DHT ID
		peerCPL := CommonPrefixLen(pDhtId, target) // 计算与目标的 CPL
		if peerCPL > cpl {                         // 如果 CPL 大于分割点
			cur := e              // 保存当前节点
			out.PushBack(e.Value) // 添加到新桶
			e = e.Next()          // 移动到下一个节点
			b.list.Remove(cur)    // 从原桶移除
			continue
		}
		e = e.Next() // 移动到下一个节点
	}
	logger.Debugf("bucket 分割完成, 新桶大小: %d", newbuck.len())
	return newbuck // 返回新桶
}

// maxCommonPrefix 计算桶中所有节点与目标节点的最大共同前缀长度。
//
// 参数:
//   - target: 目标节点 ID
//
// 返回值:
//   - uint: 最大共同前缀长度
func (b *bucket) maxCommonPrefix(target ID) uint {
	logger.Debugf("计算与目标的最大共同前缀长度, 目标: %v", target)
	maxCpl := uint(0)                                 // 初始化最大 CPL
	for e := b.list.Front(); e != nil; e = e.Next() { // 遍历所有节点
		cpl := uint(CommonPrefixLen(e.Value.(*PeerInfo).dhtId, target)) // 计算当前节点的 CPL
		if cpl > maxCpl {                                               // 如果当前 CPL 更大
			maxCpl = cpl // 更新最大 CPL
		}
	}
	logger.Debugf("最大共同前缀长度: %d", maxCpl)
	return maxCpl // 返回最大 CPL
}
