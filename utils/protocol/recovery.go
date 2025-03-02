package protocol

import (
	"fmt"
	"runtime"
	"time"
)

// RecoveryHandler 处理panic恢复
type RecoveryHandler struct {
	MaxRetries    int                       // 最大重试次数
	RetryInterval time.Duration             // 重试间隔
	OnPanic       func(interface{}, []byte) // panic处理回调
	OnDisconnect  func(error)               // 断开连接处理
}

// SafeHandle 安全处理通信
func (h *Handler) SafeHandle(operation func() error) (err error) {
	logger.Infof("开始安全处理操作")
	defer func() {
		if r := recover(); r != nil {
			// 获取堆栈信息
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)

			logger.Errorf("协议处理panic: %v\n%s", r, string(buf[:n]))

			err = &ProtocolError{
				Code:    ErrCodePanic,
				Message: fmt.Sprintf("协议处理panic: %v", r),
			}

			// 触发panic回调
			if h.recovery.OnPanic != nil {
				h.recovery.OnPanic(r, buf[:n])
			}
		}
	}()

	return operation()
}
