package protocol

import (
	"sync/atomic"
	"time"
)

// AtomicDuration 原子操作的 Duration 类型
type AtomicDuration struct {
	v atomic.Int64
}

// Store 存储 Duration
func (d *AtomicDuration) Store(val time.Duration) {
	d.v.Store(int64(val))
}

// Load 加载 Duration
func (d *AtomicDuration) Load() time.Duration {
	return time.Duration(d.v.Load())
}

// Add 添加 Duration
func (d *AtomicDuration) Add(val time.Duration) time.Duration {
	return time.Duration(d.v.Add(int64(val)))
}
