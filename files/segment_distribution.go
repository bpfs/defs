package files

import (
	"container/list"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// SegmentDistribution 分片分配管理器
// 用于管理文件分片在不同节点间的分配关系
type SegmentDistribution struct {
	mu   sync.RWMutex // 用于保护并发访问的互斥锁
	list *list.List   // 存储分片分配信息的双向链表
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
//   - distribution: map[peer.ID][]string 节点ID到分片ID列表的映射
func (sd *SegmentDistribution) AddDistribution(distribution map[peer.ID][]string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// 直接将映射添加到列表
	sd.list.PushBack(distribution)
}

// GetNextDistribution 获取并移除下一个待处理的分配
// 返回值:
//   - map[peer.ID][]string: 下一个待处理的节点分片映射
//   - bool: 是否成功获取到分配信息
func (sd *SegmentDistribution) GetNextDistribution() (map[peer.ID][]string, bool) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// 获取链表第一个元素
	if element := sd.list.Front(); element != nil {
		// 移除并返回该元素
		distribution := sd.list.Remove(element).(map[peer.ID][]string)
		return distribution, true
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
