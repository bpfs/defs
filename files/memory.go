// Package files 提供内存管理相关功能
//
/**
使用示例:

	// 创建内存管理器
	mm, err := NewMemoryManager(
		WithWarningThreshold(0.75),
		WithCriticalThreshold(0.85),
		WithMonitorInterval(10 * time.Second),
		WithAutoTuneGC(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer mm.Stop()

	// 获取内存指标
	metrics := mm.GetMetrics()
	fmt.Printf("内存使用率: %.2f%%\n", metrics.UsedPercent)

	// 手动触发GC
	mm.TriggerGC()

*/
package files

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/mem"
)

// MemoryConfig 内存管理器配置
type MemoryConfig struct {
	WarningThreshold  float64       // 内存使用预警阈值（默认80%）
	CriticalThreshold float64       // 内存使用危险阈值（默认90%）
	MinGCInterval     time.Duration // 最小GC间隔（默认1秒）
	MaxGCTimeout      time.Duration // 最大GC超时时间（默认30秒）
	AutoTuneGC        bool          // 是否自动调整GC（默认true）
	MonitorInterval   time.Duration // 监控间隔（默认5秒）
}

// MemoryLevel 内存使用级别
type MemoryLevel int

// 内存使用级别常量定义
const (
	MemoryLevelNormal   MemoryLevel = iota // 正常
	MemoryLevelWarning                     // 警告
	MemoryLevelCritical                    // 危险
)

// 系统默认配置常量
const (
	DefaultWarningThreshold  = 0.8              // 默认内存使用预警阈值（80%）
	DefaultCriticalThreshold = 0.9              // 默认内存使用危险阈值（90%）
	DefaultMinGCInterval     = time.Second      // 默认最小GC间隔
	DefaultMaxGCTimeout      = 30 * time.Second // 默认GC超时时间
	DefaultMonitorInterval   = 5 * time.Second  // 默认监控间隔
)

// MemoryMetrics 内存指标统计
type MemoryMetrics struct {
	UsedPercent    float64       // 内存使用百分比
	UsedBytes      uint64        // 已用内存字节数
	TotalBytes     uint64        // 总内存字节数
	Level          MemoryLevel   // 内存使用级别
	GCCount        int64         // GC次数
	TotalGCTime    time.Duration // 总GC时间
	AverageGCPause time.Duration // 平均GC暂停时间
	MaxGCPause     time.Duration // 最大GC暂停时间
	LastGCTime     time.Time     // 上次GC时间
	LastGCDuration time.Duration // 上次GC持续时间
}

// MemoryManager 内存管理器
type MemoryManager struct {
	config    *MemoryConfig  // 配置信息
	metrics   *MemoryMetrics // 指标统计
	mu        sync.RWMutex   // 读写锁
	stopChan  chan struct{}  // 停止信号通道
	gcTrigger chan struct{}  // GC触发信号通道
	wg        sync.WaitGroup // 等待组
	isRunning int32          // 运行状态标志
}

// DefaultMemoryConfig 返回默认配置
// 返回值：
//   - *MemoryConfig: 默认配置对象
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		WarningThreshold:  DefaultWarningThreshold,
		CriticalThreshold: DefaultCriticalThreshold,
		MinGCInterval:     DefaultMinGCInterval,
		MaxGCTimeout:      DefaultMaxGCTimeout,
		AutoTuneGC:        true,
		MonitorInterval:   DefaultMonitorInterval,
	}
}

// NewMemoryManager 创建新的内存管理器
// 参数：
//   - opts: 配置选项函数列表
//
// 返回值：
//   - *MemoryManager: 内存管理器实例
//   - error: 创建过程中的错误
func NewMemoryManager(opts ...func(*MemoryConfig)) (*MemoryManager, error) {
	// 创建默认配置
	config := DefaultMemoryConfig()

	// 应用自定义配置
	for _, opt := range opts {
		opt(config)
	}

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("配置无效: %v", err)
	}

	// 创建管理器实例
	mm := &MemoryManager{
		config:    config,
		metrics:   &MemoryMetrics{},
		stopChan:  make(chan struct{}),
		gcTrigger: make(chan struct{}, 1),
	}

	// 启动后台任务
	mm.startBackgroundTasks()
	return mm, nil
}

// validateConfig 验证配置有效性
// 参数：
//   - config: 待验证的配置对象
//
// 返回值：
//   - error: 验证错误
func validateConfig(config *MemoryConfig) error {
	if config.WarningThreshold <= 0 || config.WarningThreshold >= 1 {
		return fmt.Errorf("警告阈值必须在0-1之间")
	}
	if config.CriticalThreshold <= config.WarningThreshold || config.CriticalThreshold >= 1 {
		return fmt.Errorf("危险阈值必须大于警告阈值且小于1")
	}
	if config.MinGCInterval < time.Second {
		return fmt.Errorf("最小GC间隔不能小于1秒")
	}
	if config.MonitorInterval < time.Second {
		return fmt.Errorf("监控间隔不能小于1秒")
	}
	return nil
}

// startBackgroundTasks 启动后台任务
func (mm *MemoryManager) startBackgroundTasks() {
	// 设置运行状态
	atomic.StoreInt32(&mm.isRunning, 1)

	// 启动监控和GC协程
	mm.wg.Add(2)
	go mm.monitorRoutine()
	go mm.gcRoutine()
}

// monitorRoutine 内存监控例程
func (mm *MemoryManager) monitorRoutine() {
	defer mm.wg.Done()

	// 创建定时器
	ticker := time.NewTicker(mm.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mm.stopChan:
			return
		case <-ticker.C:
			// 检查并更新指标
			if err := mm.checkAndUpdateMetrics(); err != nil {
				logger.Errorf("内存监控失败: %v", err)
			}
		}
	}
}

// gcRoutine GC处理例程
func (mm *MemoryManager) gcRoutine() {
	defer mm.wg.Done()

	for {
		select {
		case <-mm.stopChan:
			return
		case <-mm.gcTrigger:
			// 执行GC
			if err := mm.performGC(); err != nil {
				logger.Errorf("GC执行失败: %v", err)
			}
		}
	}
}

// checkAndUpdateMetrics 检查并更新内存指标
// 返回值：
//   - error: 更新过程中的错误
func (mm *MemoryManager) checkAndUpdateMetrics() error {
	// 获取系统内存信息
	v, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("获取内存信息失败: %v", err)
	}

	// 确定内存使用级别
	level := MemoryLevelNormal
	if v.UsedPercent >= mm.config.CriticalThreshold*100 {
		level = MemoryLevelCritical
	} else if v.UsedPercent >= mm.config.WarningThreshold*100 {
		level = MemoryLevelWarning
	}

	// 更新指标
	mm.mu.Lock()
	mm.metrics.UsedPercent = v.UsedPercent
	mm.metrics.UsedBytes = v.Used
	mm.metrics.TotalBytes = v.Total
	mm.metrics.Level = level
	mm.mu.Unlock()

	// 根据级别触发GC
	if level >= MemoryLevelWarning {
		mm.TriggerGC()
	}

	return nil
}

// performGC 执行垃圾回收
// 返回值：
//   - error: GC执行过程中的错误
func (mm *MemoryManager) performGC() error {
	startTime := time.Now()

	// 执行GC
	runtime.GC()
	if mm.metrics.Level == MemoryLevelCritical {
		debug.FreeOSMemory()
	}

	// 更新GC统计信息
	duration := time.Since(startTime)
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.metrics.GCCount++
	mm.metrics.TotalGCTime += duration
	mm.metrics.LastGCTime = startTime
	mm.metrics.LastGCDuration = duration
	if duration > mm.metrics.MaxGCPause {
		mm.metrics.MaxGCPause = duration
	}
	mm.metrics.AverageGCPause = time.Duration(int64(mm.metrics.TotalGCTime) / mm.metrics.GCCount)

	return nil
}

// TriggerGC 触发垃圾回收
func (mm *MemoryManager) TriggerGC() {
	select {
	case mm.gcTrigger <- struct{}{}:
		logger.Info("已触发GC")
	default:
		logger.Info("GC已在队列中")
	}
}

// GetMetrics 获取内存指标
// 返回值：
//   - *MemoryMetrics: 当前内存指标快照
func (mm *MemoryManager) GetMetrics() *MemoryMetrics {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	// 返回指标副本
	metrics := *mm.metrics
	return &metrics
}

// Stop 停止内存管理器
func (mm *MemoryManager) Stop() {
	// 确保只执行一次停止操作
	if !atomic.CompareAndSwapInt32(&mm.isRunning, 1, 0) {
		return
	}

	// 关闭停止信号通道
	close(mm.stopChan)
	// 等待所有后台任务完成
	mm.wg.Wait()
}

// WithWarningThreshold 设置警告阈值
// 参数：
//   - threshold: 警告阈值(0.0-1.0)
//
// 返回值：
//   - func(*MemoryConfig): 配置函数
func WithWarningThreshold(threshold float64) func(*MemoryConfig) {
	return func(c *MemoryConfig) {
		c.WarningThreshold = threshold
	}
}

// WithCriticalThreshold 设置危险阈值
// 参数：
//   - threshold: 危险阈值(0.0-1.0)
//
// 返回值：
//   - func(*MemoryConfig): 配置函数
func WithCriticalThreshold(threshold float64) func(*MemoryConfig) {
	return func(c *MemoryConfig) {
		c.CriticalThreshold = threshold
	}
}

// WithMonitorInterval 设置监控间隔
// 参数：
//   - interval: 监控间隔时间
//
// 返回值：
//   - func(*MemoryConfig): 配置函数
func WithMonitorInterval(interval time.Duration) func(*MemoryConfig) {
	return func(c *MemoryConfig) {
		c.MonitorInterval = interval
	}
}

// WithAutoTuneGC 设置是否自动调整GC
// 参数：
//   - enable: 是否启用
//
// 返回值：
//   - func(*MemoryConfig): 配置函数
func WithAutoTuneGC(enable bool) func(*MemoryConfig) {
	return func(c *MemoryConfig) {
		c.AutoTuneGC = enable
	}
}

// GetMemoryStats 获取内存统计信息
// 返回值：
//   - *MemoryMetrics: 内存统计信息
//   - error: 获取过程中的错误
func (mm *MemoryManager) GetMemoryStats() (*MemoryMetrics, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	// 获取系统内存信息
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("获取系统内存信息失败: %v", err)
	}

	// 复制当前指标
	metrics := *mm.metrics

	// 更新最新数据
	metrics.UsedPercent = vmStat.UsedPercent
	metrics.UsedBytes = vmStat.Used
	metrics.TotalBytes = vmStat.Total

	// 设置内存级别
	if metrics.UsedPercent >= mm.config.CriticalThreshold*100 {
		metrics.Level = MemoryLevelCritical
	} else if metrics.UsedPercent >= mm.config.WarningThreshold*100 {
		metrics.Level = MemoryLevelWarning
	} else {
		metrics.Level = MemoryLevelNormal
	}

	return &metrics, nil
}
