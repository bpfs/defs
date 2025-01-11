package kbucket

import (
	"container/list"
	"sort"

	"github.com/dep2p/libp2p/core/peer"
)

// peerDistance 是一个辅助结构，用于按照与本地节点的距离对对等节点进行排序。
type peerDistance struct {
	p        peer.ID // 对等节点的 ID
	mode     int     // 对等节点的运行模式
	distance ID      // 对等节点与本地节点的距离
}

// peerDistanceSorter 实现 sort.Interface 接口，用于按照异或距离对对等节点进行排序。
type peerDistanceSorter struct {
	peers  []peerDistance // 待排序的对等节点列表
	target ID             // 排序的目标节点 ID，与目标距离最近的节点将排在前面
}

// Len 返回 peerDistanceSorter 中的节点数量。
//
// 返回值:
//   - int: 节点数量
func (pds *peerDistanceSorter) Len() int { return len(pds.peers) }

// Swap 交换 peerDistanceSorter 中两个位置的节点。
//
// 参数:
//   - a: 第一个节点的索引
//   - b: 第二个节点的索引
func (pds *peerDistanceSorter) Swap(a, b int) {
	pds.peers[a], pds.peers[b] = pds.peers[b], pds.peers[a]
}

// Less 比较 peerDistanceSorter 中两个位置的节点的距离大小。
//
// 参数:
//   - a: 第一个节点的索引
//   - b: 第二个节点的索引
//
// 返回值:
//   - bool: 如果第一个节点距离小于第二个节点距离，则返回 true
func (pds *peerDistanceSorter) Less(a, b int) bool {
	return pds.peers[a].distance.less(pds.peers[b].distance)
}

// appendPeer 将 peer.ID 添加到排序器的切片中，可能会导致切片不再有序。
//
// 参数:
//   - p: 要添加的对等节点 ID
//   - pDhtId: 对等节点的 DHT ID
//   - mode: 可选的运行模式
func (pds *peerDistanceSorter) appendPeer(p peer.ID, pDhtId ID, mode ...int) {
	peerDistance := peerDistance{
		p:        p,
		distance: xor(pds.target, pDhtId),
	}

	// 如果指定了mode，则设置mode值
	if len(mode) > 0 {
		peerDistance.mode = mode[0]
	}

	pds.peers = append(pds.peers, peerDistance)
	logger.Debugf("添加对等节点到排序器: %s", p)
}

// appendPeersFromList 将列表中的 peer.ID 值添加到排序器的切片中，可能会导致切片不再有序。
//
// 参数:
//   - l: 包含对等节点信息的链表
//   - mode: 可选的运行模式过滤条件
func (pds *peerDistanceSorter) appendPeersFromList(l *list.List, mode ...int) {
	logger.Debugf("开始从链表添加对等节点到排序器")
	count := 0
	// 遍历链表中的每个节点
	for e := l.Front(); e != nil; e = e.Next() {
		// 如果指定了mode且不匹配，则跳过该节点
		if len(mode) > 0 && e.Value.(*PeerInfo).Mode != mode[0] {
			continue
		}

		// 添加节点到排序器
		pds.appendPeer(e.Value.(*PeerInfo).Id, e.Value.(*PeerInfo).dhtId, mode...)

		count++
	}
	logger.Debugf("完成从链表添加对等节点，共添加 %d 个节点", count)
}

// sort 对 peerDistanceSorter 进行排序。
func (pds *peerDistanceSorter) sort() {
	logger.Debugf("开始对 %d 个对等节点进行排序", len(pds.peers))
	sort.Sort(pds)
	logger.Debugf("完成对等节点排序")
}

// SortClosestPeers 按照与目标节点的升序距离对给定的对等节点进行排序。
//
// 参数:
//   - peers: 要排序的对等节点 ID 列表
//   - target: 目标节点 ID
//
// 返回值:
//   - []peer.ID: 按距离排序后的对等节点 ID 列表
func SortClosestPeers(peers []peer.ID, target ID) []peer.ID {
	logger.Debugf("开始对 %d 个对等节点进行最近节点排序", len(peers))
	// 创建一个排序器实例
	sorter := peerDistanceSorter{
		peers:  make([]peerDistance, 0, len(peers)), // 初始化存储节点距离的切片
		target: target,                              // 设置目标节点 ID
	}

	// 遍历对等节点列表，将节点信息添加到排序器中
	for _, p := range peers {
		sorter.appendPeer(p, ConvertPeerID(p))
	}

	// 对排序器中的节点进行排序
	sorter.sort()

	// 创建一个切片用于存储排序后的节点列表
	out := make([]peer.ID, 0, sorter.Len())

	// 遍历排序后的节点信息，将节点 ID 添加到输出切片中
	for _, p := range sorter.peers {
		out = append(out, p.p)
	}

	logger.Debugf("完成最近节点排序，共排序 %d 个节点", len(out))
	// 返回排序后的节点列表
	return out
}
