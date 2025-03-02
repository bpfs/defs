package protocol

import (
	"sync"
	"sync/atomic"
	"time"
)

// FlowController 流量控制器
type FlowController struct {
	// 速率控制
	rate         int64        // 每秒允许的字节数
	currentBytes atomic.Int64 // 当前周期已使用字节数
	resetAt      time.Time    // 计数重置时间

	// 窗口控制
	window   int64        // 滑动窗口大小(字节)
	inFlight atomic.Int64 // 在途字节数

	// 拥塞控制
	threshold int64         // 拥塞阈值
	backoff   time.Duration // 退避时间

	mu sync.RWMutex
}

// NewFlowController 创建流量控制器
func NewFlowController(rate, window, threshold int64) *FlowController {
	return &FlowController{
		rate:      rate,
		window:    window * 2, // 增大窗口大小
		threshold: threshold,
		resetAt:   time.Now(),
		backoff:   time.Millisecond * 100,
	}
}

// Acquire 获取发送许可
func (fc *FlowController) Acquire(size int64) error {
	// 检查窗口
	if fc.inFlight.Load()+size > fc.window {
		// 等待一段时间后重试
		time.Sleep(fc.backoff)
		if fc.inFlight.Load()+size <= fc.window {
			goto ACQUIRE
		}
		return &ProtocolError{
			Code:    ErrCodeFlowControl,
			Message: "窗口已满",
		}
	}
ACQUIRE:
	// 检查速率
	fc.mu.Lock()
	defer fc.mu.Unlock()

	now := time.Now()
	if now.Sub(fc.resetAt) >= time.Second {
		fc.currentBytes.Store(0)
		fc.resetAt = now
	}

	if fc.currentBytes.Load()+size > fc.rate {
		return &ProtocolError{
			Code:    ErrCodeFlowControl,
			Message: "超出速率限制",
		}
	}

	// 更新计数
	fc.currentBytes.Add(size)
	fc.inFlight.Add(size)

	return nil
}

// Release 释放资源
func (fc *FlowController) Release(size int64) {
	fc.inFlight.Add(-size)
}
