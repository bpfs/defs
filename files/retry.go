//
/**
使用示例:

	// 创建重试操作实例
	retry, err := NewRetryableOperation(
		WithMaxRetries(5),
		WithInitialBackoff(time.Second),
		WithMaxBackoff(30 * time.Second),
		WithMaxElapsed(time.Minute),
		WithOnRetry(func(attempt int, err error) {
			log.Printf("第%d次重试失败: %v", attempt+1, err)
		}),
	)

	// 执行操作
	ctx := context.Background()
	err = retry.Execute(ctx, func() error {
		// 要重试的操作
		return nil
	})

*/
package files

import (
	"context"
	"fmt"
	"time"

	"github.com/bpfs/defs/utils/logger"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries     int                          // 最大重试次数
	InitialBackoff time.Duration                // 初始重试等待时间
	MaxBackoff     time.Duration                // 最大重试等待时间
	MaxElapsed     time.Duration                // 最大总耗时
	OnRetry        func(attempt int, err error) // 重试回调函数
}

// 默认配置常量
const (
	DefaultMaxRetries     = 3                // 默认最大重试次数
	DefaultInitialBackoff = time.Second      // 默认初始重试等待时间
	DefaultMaxBackoff     = 30 * time.Second // 默认最大重试等待时间
	DefaultMaxElapsed     = time.Minute      // 默认最大总耗时
)

// RetryableOperation 可重试操作结构体
type RetryableOperation struct {
	config *RetryConfig // 重试配置
}

// DefaultRetryConfig 返回默认重试配置
// 返回值：
//   - *RetryConfig: 默认配置对象
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     DefaultMaxRetries,
		InitialBackoff: DefaultInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
		MaxElapsed:     DefaultMaxElapsed,
		OnRetry:        nil,
	}
}

// NewRetryableOperation 创建新的可重试操作实例
// 参数：
//   - opts: 配置选项函数列表
//
// 返回值：
//   - *RetryableOperation: 可重试操作实例
//   - error: 创建过程中的错误
func NewRetryableOperation(opts ...func(*RetryConfig)) (*RetryableOperation, error) {
	// 创建默认配置
	config := DefaultRetryConfig()

	// 应用自定义配置
	for _, opt := range opts {
		opt(config)
	}

	// 验证配置
	if err := validateRetryConfig(config); err != nil {
		logger.Errorf("重试配置验证失败: %v", err)
		return nil, err
	}

	return &RetryableOperation{
		config: config,
	}, nil
}

// validateRetryConfig 验证重试配置
// 参数：
//   - config: 待验证的配置对象
//
// 返回值：
//   - error: 验证错误
func validateRetryConfig(config *RetryConfig) error {
	if config.MaxRetries < 0 {
		logger.Error("最大重试次数不能为负数")
		return fmt.Errorf("最大重试次数不能为负数")
	}
	if config.InitialBackoff <= 0 {
		logger.Error("初始重试等待时间必须大于0")
		return fmt.Errorf("初始重试等待时间必须大于0")
	}
	if config.MaxBackoff < config.InitialBackoff {
		logger.Error("最大重试等待时间不能小于初始重试等待时间")
		return fmt.Errorf("最大重试等待时间不能小于初始重试等待时间")
	}
	if config.MaxElapsed <= 0 {
		logger.Error("最大总耗时必须大于0")
		return fmt.Errorf("最大总耗时必须大于0")
	}
	return nil
}

// WithMaxRetries 设置最大重试次数
// 参数：
//   - maxRetries: 最大重试次数
//
// 返回值：
//   - func(*RetryConfig): 配置函数
func WithMaxRetries(maxRetries int) func(*RetryConfig) {
	return func(c *RetryConfig) {
		c.MaxRetries = maxRetries
	}
}

// WithInitialBackoff 设置初始重试等待时间
// 参数：
//   - backoff: 初始重试等待时间
//
// 返回值：
//   - func(*RetryConfig): 配置函数
func WithInitialBackoff(backoff time.Duration) func(*RetryConfig) {
	return func(c *RetryConfig) {
		c.InitialBackoff = backoff
	}
}

// WithMaxBackoff 设置最大重试等待时间
// 参数：
//   - maxBackoff: 最大重试等待时间
//
// 返回值：
//   - func(*RetryConfig): 配置函数
func WithMaxBackoff(maxBackoff time.Duration) func(*RetryConfig) {
	return func(c *RetryConfig) {
		c.MaxBackoff = maxBackoff
	}
}

// WithMaxElapsed 设置最大总耗时
// 参数：
//   - maxElapsed: 最大总耗时
//
// 返回值：
//   - func(*RetryConfig): 配置函数
func WithMaxElapsed(maxElapsed time.Duration) func(*RetryConfig) {
	return func(c *RetryConfig) {
		c.MaxElapsed = maxElapsed
	}
}

// WithOnRetry 设置重试回调函数
// 参数：
//   - onRetry: 重试回调函数
//
// 返回值：
//   - func(*RetryConfig): 配置函数
func WithOnRetry(onRetry func(attempt int, err error)) func(*RetryConfig) {
	return func(c *RetryConfig) {
		c.OnRetry = onRetry
	}
}

// Execute 执行可重试操作
// 参数：
//   - ctx: 上下文，用于取消操作
//   - op: 要执行的操作函数
//
// 返回值：
//   - error: 执行过程中的错误
func (ro *RetryableOperation) Execute(ctx context.Context, op func() error) error {
	var lastErr error
	startTime := time.Now()

	// 在最大重试次数内尝试执行操作
	for attempt := 0; attempt < ro.config.MaxRetries; attempt++ {
		// 检查是否超过最大总耗时
		if time.Since(startTime) > ro.config.MaxElapsed {
			logger.Errorf("操作超过最大总耗时 %v", ro.config.MaxElapsed)
			return fmt.Errorf("操作超过最大总耗时 %v", ro.config.MaxElapsed)
		}

		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			logger.Errorf("操作被取消: %v", ctx.Err())
			return ctx.Err()
		default:
			// 执行操作
			if err := op(); err == nil {
				return nil
			} else {
				lastErr = err
				logger.Errorf("第%d次重试失败: %v", attempt+1, err)
				// 调用重试回调函数
				if ro.config.OnRetry != nil {
					ro.config.OnRetry(attempt, err)
				}

				// 如果不是最后一次尝试，则等待后重试
				if attempt < ro.config.MaxRetries-1 {
					backoff := ro.calculateBackoff(attempt)
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						logger.Errorf("等待重试时操作被取消: %v", ctx.Err())
						return ctx.Err()
					case <-timer.C:
						// 继续重试
					}
				}
			}
		}
	}

	return lastErr
}

// ExecuteWithFallback 执行可重试操作，带有降级处理
// 参数：
//   - ctx: 上下文，用于取消操作
//   - op: 主要操作函数
//   - fallback: 降级操作函数
//
// 返回值：
//   - error: 执行过程中的错误
func (ro *RetryableOperation) ExecuteWithFallback(
	ctx context.Context,
	op func() error,
	fallback func() error,
) error {
	// 先尝试执行主要操作
	err := ro.Execute(ctx, op)
	if err == nil {
		return nil
	}

	logger.Errorf("主要操作执行失败，尝试降级处理: %v", err)

	// 如果主要操作失败且提供了降级操作，则执行降级
	if fallback != nil {
		if err := fallback(); err != nil {
			logger.Errorf("降级处理失败: %v", err)
			return err
		}
		return nil
	}

	return err
}

// calculateBackoff 计算重试等待时间
// 参数：
//   - attempt: 当前尝试次数
//
// 返回值：
//   - time.Duration: 计算得到的等待时间
func (ro *RetryableOperation) calculateBackoff(attempt int) time.Duration {
	// 使用指数退避策略计算等待时间
	backoff := ro.config.InitialBackoff * time.Duration(1<<uint(attempt))

	// 确保不超过最大等待时间
	if backoff > ro.config.MaxBackoff {
		backoff = ro.config.MaxBackoff
	}

	return backoff
}

// IsRetryable 判断错误是否可重试
// 参数：
//   - err: 要判断的错误
//
// 返回值：
//   - bool: 是否可以重试该错误
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	return isTimeoutError(err) || isTemporaryError(err)
}

// isTimeoutError 判断是否为超时错误
// 参数：
//   - err: 要判断的错误
//
// 返回值：
//   - bool: 是否为超时错误
func isTimeoutError(err error) bool {
	type timeout interface {
		Timeout() bool
	}

	te, ok := err.(timeout)
	return ok && te.Timeout()
}

// isTemporaryError 判断是否为临时性错误
// 参数：
//   - err: 要判断的错误
//
// 返回值：
//   - bool: 是否为临时性错误
func isTemporaryError(err error) bool {
	type temporary interface {
		Temporary() bool
	}

	te, ok := err.(temporary)
	return ok && te.Temporary()
}
