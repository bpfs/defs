//
/**
使用示例:

	// 创建带默认配置的读写锁
	mutex, err := NewSafeRWMutex()

	// 创建带自定义配置的读写锁
	mutex, err := NewSafeRWMutex(
		WithTimeout(3 * time.Second),
		WithRetries(5),
		WithBackoff(500 * time.Millisecond),
	)

	// 使用锁
	ctx := context.Background()
	if err := mutex.TryLockTimeout(ctx); err != nil {
		// 处理错误
	}
	defer mutex.Unlock()

*/
package files

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SafeRWMutex 安全的读写锁结构体
type SafeRWMutex struct {
	sync.RWMutex               // 内嵌标准读写锁
	timeout      time.Duration // 锁操作超时时间
	retries      int           // 默认重试次数
	backoff      time.Duration // 默认重试间隔
}

// MutexConfig 互斥锁配置
type MutexConfig struct {
	Timeout time.Duration // 锁超时时间
	Retries int           // 重试次数
	Backoff time.Duration // 重试间隔
}

// 默认配置常量
const (
	DefaultTimeout = 5 * time.Second  // 默认超时时间
	DefaultRetries = 3                // 默认重试次数
	DefaultBackoff = time.Second      // 默认重试间隔
	MaxTimeout     = time.Minute      // 最大超时时间
	MaxRetries     = 10               // 最大重试次数
	MaxBackoff     = 10 * time.Second // 最大重试间隔
)

// DefaultMutexConfig 返回默认配置
// 返回值：
//   - *MutexConfig: 默认配置对象
func DefaultMutexConfig() *MutexConfig {
	return &MutexConfig{
		Timeout: DefaultTimeout,
		Retries: DefaultRetries,
		Backoff: DefaultBackoff,
	}
}

// NewSafeRWMutex 创建新的安全读写锁
// 参数：
//   - opts: 配置选项函数列表
//
// 返回值：
//   - *SafeRWMutex: 安全读写锁实例
//   - error: 创建过程中的错误
func NewSafeRWMutex(opts ...func(*MutexConfig)) (*SafeRWMutex, error) {
	// 创建默认配置
	config := DefaultMutexConfig()

	// 应用自定义配置
	for _, opt := range opts {
		opt(config)
	}

	// 验证配置
	if err := validateMutexConfig(config); err != nil {
		logger.Errorf("配置无效: %v", err)
		return nil, err

	}

	// 创建实例
	return &SafeRWMutex{
		timeout: config.Timeout,
		retries: config.Retries,
		backoff: config.Backoff,
	}, nil
}

// validateMutexConfig 验证配置有效性
// 参数：
//   - config: 待验证的配置对象
//
// 返回值：
//   - error: 验证错误
func validateMutexConfig(config *MutexConfig) error {
	if config.Timeout <= 0 || config.Timeout > MaxTimeout {
		return fmt.Errorf("超时时间必须在0到%v之间", MaxTimeout)
	}
	if config.Retries < 0 || config.Retries > MaxRetries {
		return fmt.Errorf("重试次数必须在0到%d之间", MaxRetries)
	}
	if config.Backoff <= 0 || config.Backoff > MaxBackoff {
		return fmt.Errorf("重试间隔必须在0到%v之间", MaxBackoff)
	}
	return nil
}

// TryLockTimeout 尝试在指定时间内获取写锁
// 参数：
//   - ctx: 上下文，用于取消操作
//
// 返回值：
//   - error: 获取锁过程中的错误
func (m *SafeRWMutex) TryLockTimeout(ctx context.Context) error {
	logger.Debugf("尝试获取写锁")
	// 创建一个用于通知锁获取成功的通道
	done := make(chan struct{})

	// 在新的goroutine中尝试获取锁
	go func() {
		m.Lock()
		close(done)
	}()

	// 等待锁获取成功或超时
	select {
	case <-done:
		logger.Debugf("成功获取写锁")
		return nil
	case <-time.After(m.timeout):
		err := fmt.Errorf("获取锁超时 (超时时间: %v)", m.timeout)
		logger.Errorf("获取写锁失败: %v", err)
		return err
	case <-ctx.Done():
		err := ctx.Err()
		logger.Errorf("获取写锁失败: %v", err)
		return err
	}
}

// TryRLockTimeout 尝试在指定时间内获取读锁
// 参数：
//   - ctx: 上下文，用于取消操作
//
// 返回值：
//   - error: 获取锁过程中的错误
func (m *SafeRWMutex) TryRLockTimeout(ctx context.Context) error {
	logger.Debugf("尝试获取读锁")
	// 创建一个用于通知锁获取成功的通道
	done := make(chan struct{})

	// 在新的goroutine中尝试获取读锁
	go func() {
		m.RLock()
		close(done)
	}()

	// 等待锁获取成功或超时
	select {
	case <-done:
		logger.Debugf("成功获取读锁")
		return nil
	case <-time.After(m.timeout):
		err := fmt.Errorf("获取读锁超时 (超时时间: %v)", m.timeout)
		logger.Errorf("获取读锁失败: %v", err)
		return err
	case <-ctx.Done():
		err := ctx.Err()
		logger.Errorf("获取读锁失败: %v", err)
		return err
	}
}

// LockWithDeadline 在指定截止时间前获取写锁
// 参数：
//   - deadline: 截止时间
//
// 返回值：
//   - error: 获取锁过程中的错误
func (m *SafeRWMutex) LockWithDeadline(deadline time.Time) error {
	logger.Debugf("尝试在截止时间前获取写锁: %v", deadline)
	timeout := time.Until(deadline)
	if timeout <= 0 {
		err := fmt.Errorf("截止时间已过")
		logger.Errorf("获取写锁失败: %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return m.TryLockTimeout(ctx)
}

// RLockWithDeadline 在指定截止时间前获取读锁
// 参数：
//   - deadline: 截止时间
//
// 返回值：
//   - error: 获取锁过程中的错误
func (m *SafeRWMutex) RLockWithDeadline(deadline time.Time) error {
	logger.Debugf("尝试在截止时间前获取读锁: %v", deadline)
	timeout := time.Until(deadline)
	if timeout <= 0 {
		err := fmt.Errorf("截止时间已过")
		logger.Errorf("获取读锁失败: %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return m.TryRLockTimeout(ctx)
}

// LockWithRetry 带重试机制的写锁获取
// 参数：
//   - ctx: 上下文，用于取消操作
//
// 返回值：
//   - error: 获取锁过程中的错误
func (m *SafeRWMutex) LockWithRetry(ctx context.Context) error {
	logger.Debugf("开始尝试获取写锁，最大重试次数: %d", m.retries)
	for i := 0; i < m.retries; i++ {
		err := m.TryLockTimeout(ctx)
		if err == nil {
			return nil
		}
		logger.Debugf("第%d次获取写锁失败，准备重试", i+1)

		// 最后一次重试不需要等待
		if i < m.retries-1 {
			select {
			case <-ctx.Done():
				err := ctx.Err()
				logger.Errorf("获取写锁失败: %v", err)
				return err
			case <-time.After(m.backoff):
				// 继续重试
			}
		}
	}
	err := fmt.Errorf("获取锁失败，已重试%d次", m.retries)
	logger.Errorf("获取写锁失败: %v", err)
	return err
}

// RLockWithRetry 带重试机制的读锁获取
// 参数：
//   - ctx: 上下文，用于取消操作
//
// 返回值：
//   - error: 获取锁过程中的错误
func (m *SafeRWMutex) RLockWithRetry(ctx context.Context) error {
	logger.Debugf("开始尝试获取读锁，最大重试次数: %d", m.retries)
	for i := 0; i < m.retries; i++ {
		err := m.TryRLockTimeout(ctx)
		if err == nil {
			return nil
		}
		logger.Debugf("第%d次获取读锁失败，准备重试", i+1)

		// 最后一次重试不需要等待
		if i < m.retries-1 {
			select {
			case <-ctx.Done():
				err := ctx.Err()
				logger.Errorf("获取读锁失败: %v", err)
				return err
			case <-time.After(m.backoff):
				// 继续重试
			}
		}
	}
	err := fmt.Errorf("获取读锁失败，已重试%d次", m.retries)
	logger.Errorf("获取读锁失败: %v", err)
	return err
}

// WithTimeout 设置超时时间配置选项
// 参数：
//   - timeout: 超时时间
//
// 返回值：
//   - func(*MutexConfig): 配置函数
func WithTimeout(timeout time.Duration) func(*MutexConfig) {
	return func(c *MutexConfig) {
		c.Timeout = timeout
	}
}

// WithRetries 设置重试次数配置选项
// 参数：
//   - retries: 重试次数
//
// 返回值：
//   - func(*MutexConfig): 配置函数
func WithRetries(retries int) func(*MutexConfig) {
	return func(c *MutexConfig) {
		c.Retries = retries
	}
}

// WithBackoff 设置重试间隔配置选项
// 参数：
//   - backoff: 重试间隔
//
// 返回值：
//   - func(*MutexConfig): 配置函数
func WithBackoff(backoff time.Duration) func(*MutexConfig) {
	return func(c *MutexConfig) {
		c.Backoff = backoff
	}
}
