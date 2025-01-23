package files

import (
	"testing"
	"time"
)

// TestNewMemoryManager 测试内存管理器创建
func TestNewMemoryManager(t *testing.T) {
	tests := []struct {
		name    string
		opts    []func(*MemoryConfig)
		wantErr bool
	}{
		{
			name:    "默认配置",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "自定义配置",
			opts: []func(*MemoryConfig){
				WithWarningThreshold(0.75),
				WithCriticalThreshold(0.85),
				WithMonitorInterval(10 * time.Second),
				WithAutoTuneGC(true),
			},
			wantErr: false,
		},
		{
			name: "无效的警告阈值",
			opts: []func(*MemoryConfig){
				WithWarningThreshold(-0.1),
			},
			wantErr: true,
		},
		{
			name: "无效的危险阈值",
			opts: []func(*MemoryConfig){
				WithWarningThreshold(0.7),
				WithCriticalThreshold(0.6),
			},
			wantErr: true,
		},
		{
			name: "无效的监控间隔",
			opts: []func(*MemoryConfig){
				WithMonitorInterval(500 * time.Millisecond),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mm, err := NewMemoryManager(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMemoryManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && mm == nil {
				t.Error("NewMemoryManager() returned nil")
			}
			if mm != nil {
				mm.Stop()
			}
		})
	}
}

// TestMemoryManager_GetMetrics 测试获取内存指标
func TestMemoryManager_GetMetrics(t *testing.T) {
	mm, err := NewMemoryManager()
	if err != nil {
		t.Fatalf("NewMemoryManager() error = %v", err)
	}
	defer mm.Stop()

	metrics := mm.GetMetrics()
	if metrics == nil {
		t.Fatal("GetMetrics() returned nil")
	}

	// 验证指标字段
	if metrics.UsedPercent < 0 || metrics.UsedPercent > 100 {
		t.Errorf("Invalid UsedPercent: %v", metrics.UsedPercent)
	}
	if metrics.UsedBytes == 0 {
		t.Error("UsedBytes is 0")
	}
	if metrics.TotalBytes == 0 {
		t.Error("TotalBytes is 0")
	}
}

// TestMemoryManager_TriggerGC 测试触发GC
func TestMemoryManager_TriggerGC(t *testing.T) {
	mm, err := NewMemoryManager()
	if err != nil {
		t.Fatalf("NewMemoryManager() error = %v", err)
	}
	defer mm.Stop()

	// 记录初始GC次数
	initialMetrics := mm.GetMetrics()
	initialGCCount := initialMetrics.GCCount

	// 触发GC
	mm.TriggerGC()

	// 等待GC完成
	time.Sleep(100 * time.Millisecond)

	// 验证GC是否执行
	currentMetrics := mm.GetMetrics()
	if currentMetrics.GCCount <= initialGCCount {
		t.Error("GC count did not increase after TriggerGC()")
	}
}

// TestMemoryManager_Stop 测试停止内存管理器
func TestMemoryManager_Stop(t *testing.T) {
	mm, err := NewMemoryManager()
	if err != nil {
		t.Fatalf("NewMemoryManager() error = %v", err)
	}

	// 停止管理器
	mm.Stop()

	// 验证是否可以重复停止
	mm.Stop() // 不应该panic

	// 验证运行状态
	if mm.isRunning != 0 {
		t.Error("Memory manager is still running after Stop()")
	}
}

// TestMemoryManager_GetMemoryStats 测试获取内存统计信息
func TestMemoryManager_GetMemoryStats(t *testing.T) {
	mm, err := NewMemoryManager(
		WithWarningThreshold(0.7),
		WithCriticalThreshold(0.8),
	)
	if err != nil {
		t.Fatalf("NewMemoryManager() error = %v", err)
	}
	defer mm.Stop()

	stats, err := mm.GetMemoryStats()
	if err != nil {
		t.Fatalf("GetMemoryStats() error = %v", err)
	}

	// 验证统计信息
	if stats == nil {
		t.Fatal("GetMemoryStats() returned nil")
	}
	if stats.UsedPercent < 0 || stats.UsedPercent > 100 {
		t.Errorf("Invalid UsedPercent: %v", stats.UsedPercent)
	}
	if stats.UsedBytes == 0 {
		t.Error("UsedBytes is 0")
	}
	if stats.TotalBytes == 0 {
		t.Error("TotalBytes is 0")
	}

	// 验证内存级别判断
	expectedLevel := MemoryLevelNormal
	if stats.UsedPercent >= 80 {
		expectedLevel = MemoryLevelCritical
	} else if stats.UsedPercent >= 70 {
		expectedLevel = MemoryLevelWarning
	}
	if stats.Level != expectedLevel {
		t.Errorf("Wrong memory level, got %v, want %v", stats.Level, expectedLevel)
	}
}

// TestMemoryManager_MonitorRoutine 测试监控例程
func TestMemoryManager_MonitorRoutine(t *testing.T) {
	mm, err := NewMemoryManager(
		WithMonitorInterval(100*time.Millisecond),
		WithWarningThreshold(0.1), // 设置较低的阈值以触发GC
	)
	if err != nil {
		t.Fatalf("NewMemoryManager() error = %v", err)
	}
	defer mm.Stop()

	// 等待监控例程执行几次
	time.Sleep(250 * time.Millisecond)

	// 验证指标是否被更新
	metrics := mm.GetMetrics()
	if metrics.GCCount == 0 {
		t.Error("Monitor routine did not trigger any GC")
	}
}
