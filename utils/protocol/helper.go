package protocol

import (
	"context"
	"time"
)

// WithTimeout 设置超时上下文
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(parent, timeout)
}

// RetryWithBackoff 实现指数退避重试
func RetryWithBackoff(operation func() error) error {
	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
			// 检查是否是不可重试的错误
			if pe, ok := err.(*ProtocolError); ok {
				if pe.Code == ErrCodeInvalidLength ||
					pe.Code == ErrCodeSerialize ||
					pe.Code == ErrCodeDeserialize {
					return err
				}
			}
		}

		// 计算退避时间
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		time.Sleep(backoff)
	}
	return lastErr
}

// IsTemporaryError 判断是否为临时错误
func IsTemporaryError(err error) bool {
	if pe, ok := err.(*ProtocolError); ok {
		return pe.Code == ErrCodeTimeout ||
			pe.Code == ErrCodeRead ||
			pe.Code == ErrCodeWrite
	}
	return false
}
