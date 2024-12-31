package files

import (
	"context"
	"testing"
	"time"
)

// TestNewSafeRWMutex 测试创建安全读写锁
func TestNewSafeRWMutex(t *testing.T) {
	tests := []struct {
		name    string
		opts    []func(*MutexConfig)
		wantErr bool
	}{
		{
			name:    "默认配置",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "自定义配置",
			opts: []func(*MutexConfig){
				WithTimeout(3 * time.Second),
				WithRetries(5),
				WithBackoff(500 * time.Millisecond),
			},
			wantErr: false,
		},
		{
			name: "无效的超时时间",
			opts: []func(*MutexConfig){
				WithTimeout(-1 * time.Second),
			},
			wantErr: true,
		},
		{
			name: "无效的重试次数",
			opts: []func(*MutexConfig){
				WithRetries(-1),
			},
			wantErr: true,
		},
		{
			name: "无效的重试间隔",
			opts: []func(*MutexConfig){
				WithBackoff(-1 * time.Second),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutex, err := NewSafeRWMutex(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSafeRWMutex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && mutex == nil {
				t.Error("NewSafeRWMutex() returned nil without error")
			}
		})
	}
}

// TestSafeRWMutex_TryLockTimeout 测试写锁超时
func TestSafeRWMutex_TryLockTimeout(t *testing.T) {
	mutex, err := NewSafeRWMutex(WithTimeout(100 * time.Millisecond))
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	// 测试正常获取锁
	ctx := context.Background()
	if err := mutex.TryLockTimeout(ctx); err != nil {
		t.Errorf("TryLockTimeout() error = %v", err)
	}
	mutex.Unlock()

	// 测试超时情况
	// 先获取锁，阻塞后续获取
	mutex.Lock()
	defer mutex.Unlock()

	// 尝试再次获取锁，应该超时
	if err := mutex.TryLockTimeout(ctx); err == nil {
		t.Error("TryLockTimeout() expected timeout error, got nil")
	}
}

// TestSafeRWMutex_TryRLockTimeout 测试读锁超时
func TestSafeRWMutex_TryRLockTimeout(t *testing.T) {
	mutex, err := NewSafeRWMutex(WithTimeout(100 * time.Millisecond))
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	// 测试正常获取读锁
	ctx := context.Background()
	if err := mutex.TryRLockTimeout(ctx); err != nil {
		t.Errorf("TryRLockTimeout() error = %v", err)
	}
	mutex.RUnlock()

	// 测试超时情况
	// 先获取写锁，阻塞后续读锁获取
	mutex.Lock()
	defer mutex.Unlock()

	// 尝试获取读锁，应该超时
	if err := mutex.TryRLockTimeout(ctx); err == nil {
		t.Error("TryRLockTimeout() expected timeout error, got nil")
	}
}

// TestSafeRWMutex_LockWithDeadline 测试截止时间锁
func TestSafeRWMutex_LockWithDeadline(t *testing.T) {
	mutex, err := NewSafeRWMutex()
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	// 测试过期的截止时间
	pastDeadline := time.Now().Add(-time.Second)
	if err := mutex.LockWithDeadline(pastDeadline); err == nil {
		t.Error("LockWithDeadline() expected error for past deadline")
		mutex.Unlock()
	}

	// 测试未来的截止时间
	futureDeadline := time.Now().Add(time.Second)
	if err := mutex.LockWithDeadline(futureDeadline); err != nil {
		t.Errorf("LockWithDeadline() error = %v", err)
	} else {
		mutex.Unlock()
	}
}

// TestSafeRWMutex_RLockWithDeadline 测试截止时间读锁
func TestSafeRWMutex_RLockWithDeadline(t *testing.T) {
	mutex, err := NewSafeRWMutex()
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	// 测试过期的截止时间
	pastDeadline := time.Now().Add(-time.Second)
	if err := mutex.RLockWithDeadline(pastDeadline); err == nil {
		t.Error("RLockWithDeadline() expected error for past deadline")
		mutex.RUnlock()
	}

	// 测试未来的截止时间
	futureDeadline := time.Now().Add(time.Second)
	if err := mutex.RLockWithDeadline(futureDeadline); err != nil {
		t.Errorf("RLockWithDeadline() error = %v", err)
	} else {
		mutex.RUnlock()
	}
}

// TestSafeRWMutex_LockWithRetry 测试重试机制
func TestSafeRWMutex_LockWithRetry(t *testing.T) {
	mutex, err := NewSafeRWMutex(
		WithRetries(3),
		WithBackoff(50*time.Millisecond),
		WithTimeout(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	// 先获取锁以触发重试
	mutex.Lock()

	// 在另一个goroutine中尝试获取锁
	ctx := context.Background()
	done := make(chan error)
	go func() {
		done <- mutex.LockWithRetry(ctx)
	}()

	// 等待一段时间后释放锁
	time.Sleep(100 * time.Millisecond)
	mutex.Unlock()

	// 检查重试是否成功
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("LockWithRetry() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Error("LockWithRetry() timeout waiting for lock")
	}
}

// TestSafeRWMutex_RLockWithRetry 测试读锁重试机制
func TestSafeRWMutex_RLockWithRetry(t *testing.T) {
	mutex, err := NewSafeRWMutex(
		WithRetries(3),
		WithBackoff(50*time.Millisecond),
		WithTimeout(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	// 先获取写锁以触发读锁重试
	mutex.Lock()

	// 在另一个goroutine中尝试获取读锁
	ctx := context.Background()
	done := make(chan error)
	go func() {
		done <- mutex.RLockWithRetry(ctx)
	}()

	// 等待一段时间后释放写锁
	time.Sleep(100 * time.Millisecond)
	mutex.Unlock()

	// 检查重试是否成功
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RLockWithRetry() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Error("RLockWithRetry() timeout waiting for lock")
	}
}

// TestSafeRWMutex_ConcurrentAccess 测试并发访问
func TestSafeRWMutex_ConcurrentAccess(t *testing.T) {
	mutex, err := NewSafeRWMutex()
	if err != nil {
		t.Fatalf("NewSafeRWMutex() error = %v", err)
	}

	const goroutines = 10
	done := make(chan bool, goroutines)

	// 启动多个goroutine并发访问
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			ctx := context.Background()

			// 交替使用读锁和写锁
			if id%2 == 0 {
				if err := mutex.TryLockTimeout(ctx); err == nil {
					time.Sleep(10 * time.Millisecond)
					mutex.Unlock()
				}
			} else {
				if err := mutex.TryRLockTimeout(ctx); err == nil {
					time.Sleep(10 * time.Millisecond)
					mutex.RUnlock()
				}
			}

			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < goroutines; i++ {
		select {
		case <-done:
			// goroutine完成
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for goroutines")
		}
	}
}
