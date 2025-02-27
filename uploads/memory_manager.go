package uploads

import (
	"runtime"
)

const (
	// 内存相关常量
	maxMemoryUsage   = 1 << 30 // 1GB 最大内存使用
	warningThreshold = 0.8     // 80% 警告阈值
	maxConcurrentOps = 4       // 最大并发操作数
)

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	semaphore chan struct{} // 并发控制
}

var (
	// 全局内存监控器
	globalMemMonitor = NewMemoryMonitor()
)

// NewMemoryMonitor 创建新的内存监控器
func NewMemoryMonitor() *MemoryMonitor {
	return &MemoryMonitor{
		semaphore: make(chan struct{}, maxConcurrentOps),
	}
}

// AcquireToken 获取并发令牌
func (mm *MemoryMonitor) AcquireToken() {
	mm.semaphore <- struct{}{}
}

// ReleaseToken 释放并发令牌
func (mm *MemoryMonitor) ReleaseToken() {
	<-mm.semaphore
}

// CheckMemory 检查内存使用情况
func (mm *MemoryMonitor) CheckMemory() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	threshold := uint64(maxMemoryUsage * uint64(warningThreshold*100) / 100)
	if m.Alloc > threshold {
		logger.Warnf("内存使用接近阈值: %d/%d", m.Alloc, maxMemoryUsage)
		runtime.GC()
	}
}

// Global 获取全局内存监控器实例
func (mm *MemoryMonitor) Global() *MemoryMonitor {
	return globalMemMonitor
}
