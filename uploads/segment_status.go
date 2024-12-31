// Package uploads 提供文件上传相关的功能实现
package uploads

import "sync"

// SegmentStatus 用于跟踪文件是否准备就绪的状态
type SegmentStatus struct {
	state             bool       // 当前状态(true:已就绪；false:未就绪)
	segmentStatusCond *sync.Cond // 用于同步等待状态变化的条件变量
}

// NewSegmentStatus 初始化并返回一个 SegmentStatus 实例
// 参数:
//   - locker: sync.Locker 用于同步的互斥锁
//
// 返回值:
//   - *SegmentStatus: 初始化后的 SegmentStatus 实例
func NewSegmentStatus(locker sync.Locker) *SegmentStatus {
	// 创建并返回新的 SegmentStatus 实例
	return &SegmentStatus{
		state:             false,                // 初始状态设置为未就绪
		segmentStatusCond: sync.NewCond(locker), // 使用提供的锁创建条件变量
	}
}

// SetState 设置状态，并通知所有等待的goroutine
// 参数:
//   - state: bool 要设置的新状态
func (s *SegmentStatus) SetState(state bool) {
	s.segmentStatusCond.L.Lock()         // 获取锁
	defer s.segmentStatusCond.L.Unlock() // 确保在函数返回时释放锁

	s.state = state                 // 更新状态
	s.segmentStatusCond.Broadcast() // 通知所有等待的goroutine
}

// GetState 获取当前状态
// 返回值:
//   - bool: 当前的状态值
func (s *SegmentStatus) GetState() bool {
	s.segmentStatusCond.L.Lock()         // 获取锁
	defer s.segmentStatusCond.L.Unlock() // 确保在函数返回时释放锁

	return s.state // 返回当前状态
}

// WaitForStateChange 阻塞当前goroutine，直到状态发生变化
func (s *SegmentStatus) WaitForStateChange() {
	s.segmentStatusCond.L.Lock()         // 获取锁
	defer s.segmentStatusCond.L.Unlock() // 确保在函数返回时释放锁

	s.segmentStatusCond.Wait() // 等待状态变化
}

// WaitForSpecificState 阻塞当前goroutine，直到达到指定状态
// 参数:
//   - targetState: bool 要等待的目标状态
func (s *SegmentStatus) WaitForSpecificState(targetState bool) {
	s.segmentStatusCond.L.Lock()         // 获取锁
	defer s.segmentStatusCond.L.Unlock() // 确保在函数返回时释放锁

	// 循环等待直到达到目标状态
	for s.state != targetState {
		s.segmentStatusCond.Wait() // 等待状态变化
	}
}
