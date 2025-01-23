package files

import (
	"context"
	"os"
	"testing"
	"time"
)

// mockMemoryManager 实现 MemoryManagerInterface 接口
type mockMemoryManager struct {
	memUsage float64
	gcCalled bool
}

// GetMemoryStats 实现接口方法
func (m *mockMemoryManager) GetMemoryStats() (*MemoryMetrics, error) {
	return &MemoryMetrics{
		UsedPercent: m.memUsage,
		Level:       getMemoryLevel(m.memUsage),
	}, nil
}

// TriggerGC 实现接口方法
func (m *mockMemoryManager) TriggerGC() {
	m.gcCalled = true
}

// getMemoryLevel 根据内存使用率返回对应的级别
func getMemoryLevel(usedPercent float64) MemoryLevel {
	if usedPercent >= 90 {
		return MemoryLevelCritical
	} else if usedPercent >= 80 {
		return MemoryLevelWarning
	}
	return MemoryLevelNormal
}

// TestNewResourceGuard 测试资源守卫创建
func TestNewResourceGuard(t *testing.T) {
	tests := []struct {
		name    string
		opts    []func(*GuardOption)
		wantErr bool
	}{
		{
			name:    "默认配置",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "自定义配置",
			opts: []func(*GuardOption){
				WithMemoryThreshold(0.85),
				WithGuardCriticalThreshold(0.95),
				WithAsyncCleanup(true),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memManager := &mockMemoryManager{}
			guard := NewResourceGuard(memManager, tt.opts...)
			defer guard.Stop()

			if guard == nil {
				t.Error("NewResourceGuard() returned nil")
			}

			stats := guard.GetStats()
			if stats == nil {
				t.Error("GetStats() returned nil")
			}
		})
	}
}

// TestAcquireResources 测试资源获取
func TestAcquireResources(t *testing.T) {
	memManager := &mockMemoryManager{memUsage: 0.8}
	guard := NewResourceGuard(memManager)
	defer guard.Stop()

	ctx := context.Background()
	err := guard.AcquireResources(ctx)
	if err != nil {
		t.Errorf("AcquireResources() error = %v", err)
	}

	stats := guard.GetStats()
	if stats.TotalResources != 1 {
		t.Errorf("Expected TotalResources = 1, got %d", stats.TotalResources)
	}
	if stats.ActiveResources != 1 {
		t.Errorf("Expected ActiveResources = 1, got %d", stats.ActiveResources)
	}
}

// TestMemoryThreshold 测试内存阈值
func TestMemoryThreshold(t *testing.T) {
	memManager := &mockMemoryManager{memUsage: 0.95}
	guard := NewResourceGuard(memManager,
		WithMemoryThreshold(0.9),
		WithGuardCriticalThreshold(0.95),
	)
	defer guard.Stop()

	ctx := context.Background()
	_ = guard.AcquireResources(ctx)

	// 等待异步检查完成
	time.Sleep(100 * time.Millisecond)

	if !memManager.gcCalled {
		t.Error("Expected GC to be triggered")
	}
}

// TestCleanupResources 测试资源清理
func TestCleanupResources(t *testing.T) {
	memManager := &mockMemoryManager{}
	guard := NewResourceGuard(memManager)
	defer guard.Stop()

	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	// 测试同步清理
	err = guard.CleanupResources([]*os.File{tmpfile}, false)
	if err != nil {
		t.Errorf("CleanupResources() error = %v", err)
	}

	// 测试异步清理
	tmpfile2, err := os.CreateTemp("", "test2")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile2.Name())

	guard = NewResourceGuard(memManager, WithAsyncCleanup(true))
	err = guard.CleanupResources([]*os.File{tmpfile2}, false)
	if err != nil {
		t.Errorf("CleanupResources() error = %v", err)
	}
}

// TestResourceRelease 测试资源释放
func TestResourceRelease(t *testing.T) {
	memManager := &mockMemoryManager{}
	guard := NewResourceGuard(memManager)
	defer guard.Stop()

	err := guard.ReleaseResources()
	if err != nil {
		t.Errorf("ReleaseResources() error = %v", err)
	}

	stats := guard.GetStats()
	if stats.ReleasedResources != 1 {
		t.Errorf("Expected ReleasedResources = 1, got %d", stats.ReleasedResources)
	}
}

// TestRetryMechanism 测试重试机制
func TestRetryMechanism(t *testing.T) {
	memManager := &mockMemoryManager{memUsage: 0.96}
	guard := NewResourceGuard(memManager,
		WithMemoryThreshold(0.9),
		WithGuardCriticalThreshold(0.95),
	)
	defer guard.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := guard.releaseResourcesWithRetry(ctx)
	if err != nil {
		t.Errorf("releaseResourcesWithRetry() error = %v", err)
	}
}

// TestBackgroundTasks 测试后台任务
func TestBackgroundTasks(t *testing.T) {
	memManager := &mockMemoryManager{}
	guard := NewResourceGuard(memManager,
		WithGuardMonitorInterval(100*time.Millisecond),
		WithGuardCleanupInterval(100*time.Millisecond),
	)

	// 等待后台任务执行
	time.Sleep(200 * time.Millisecond)

	// 停止守卫
	guard.Stop()

	// 验证是否正常停止
	select {
	case <-guard.stopChan:
		// 正常停止
	default:
		t.Error("Background tasks not stopped properly")
	}
}
