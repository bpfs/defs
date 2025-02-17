package files

import (
	"container/list"
	"sync"

	"github.com/dep2p/go-dep2p/core/peer"
)

// SegmentDistributionItem 表示一个分片分配项
type SegmentDistributionItem struct {
	PeerInfo peer.AddrInfo // 节点信息
	Segments []string      // 分片ID列表
}

// SegmentDistribution 分片分配管理器
// 用于管理文件分片在不同节点间的分配关系
type SegmentDistribution struct {
	mu   sync.RWMutex // 用于保护并发访问的互斥锁
	list *list.List   // 存储 SegmentDistributionItem 的列表
}

// NewSegmentDistribution 创建新的分片分配管理器
// 返回值:
//   - *SegmentDistribution: 初始化后的分片分配管理器实例
func NewSegmentDistribution() *SegmentDistribution {
	return &SegmentDistribution{
		list: list.New(), // 初始化空链表
	}
}

// AddDistribution 添加分片分配映射
// 参数:
//   - peerInfo: peer.AddrInfo 节点信息
//   - segments: []string 分片ID列表
func (sd *SegmentDistribution) AddDistribution(peerInfo peer.AddrInfo, segments []string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	item := &SegmentDistributionItem{
		PeerInfo: peerInfo,
		Segments: segments,
	}
	sd.list.PushBack(item)
}

// GetNextDistribution 获取并移除下一个待处理的分配
// 返回值:
//   - *SegmentDistributionItem: 下一个待处理的节点分片映射
//   - bool: 是否成功获取到分配信息
func (sd *SegmentDistribution) GetNextDistribution() (*SegmentDistributionItem, bool) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// 获取第一个元素
	if element := sd.list.Front(); element != nil {
		// 移除第一个元素
		item := sd.list.Remove(element).(*SegmentDistributionItem)
		return item, true
	}
	return nil, false
}

// GetLength 获取当前列表长度
// 返回值:
//   - int: 当前列表中的元素数量
func (sd *SegmentDistribution) GetLength() int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.list.Len()
}

// Clear 清空分片分配列表
// 移除所有已添加的分配信息
func (sd *SegmentDistribution) Clear() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.list = list.New()
}
