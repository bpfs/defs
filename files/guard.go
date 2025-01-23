// Package files 提供文件和资源管理相关功能
//
/**
使用示例:

	// 创建资源守卫
	guard := NewResourceGuard(memManager,
		WithMemoryThreshold(0.85),
		WithCriticalThreshold(0.95),
		WithAsyncCleanup(true),
	)
	defer guard.Stop()

	// 获取资源(非阻塞)
	err := guard.AcquireResources(ctx)

	// 清理资源(支持异步)
	err = guard.CleanupResources(files, true)

	// 获取统计信息
	stats := guard.GetStats()

*/
package files

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryManagerInterface 定义内存管理器接口
type MemoryManagerInterface interface {
	GetMemoryStats() (*MemoryMetrics, error) // 获取内存统计
	TriggerGC()                              // 触发GC
}

// GuardOption 定义资源守卫配置选项
type GuardOption struct {
	MemoryThreshold   float64       // 内存使用警告阈值,当内存使用率超过此值时发出警告
	CriticalThreshold float64       // 内存使用临界阈值,当内存使用率超过此值触发强制GC
	GCInterval        time.Duration // GC间隔时间,定期触发GC的时间间隔
	MaxRetries        int           // 最大重试次数,资源释放失败时的最大重试次数
	RetryBackoff      time.Duration // 重试等待时间,重试之间的等待时间
	MaxBackoff        time.Duration // 最大重试等待时间,重试等待时间的上限
	MonitorInterval   time.Duration // 监控间隔时间,资源监控的时间间隔
	CleanupInterval   time.Duration // 清理间隔时间,定期清理的时间间隔
	AsyncCleanup      bool          // 是否异步清理,true表示异步清理,false表示同步清理
}

// 系统常量定义
const (
	GuardMemoryThreshold   = 0.9                    // 默认内存警告阈值(90%)
	GuardCriticalThreshold = 0.95                   // 默认内存临界阈值(95%)
	GuardGCInterval        = 30 * time.Second       // 默认GC间隔
	GuardMaxRetries        = 3                      // 默认最大重试次数
	GuardRetryBackoff      = 500 * time.Millisecond // 默认重试等待时间
	GuardMaxBackoff        = 10 * time.Second       // 最大重试等待时间
	GuardMonitorInterval   = 1 * time.Minute        // 默认监控间隔
	GuardCleanupInterval   = 5 * time.Minute        // 默认清理间隔
)

// DefaultGuardOption 返回默认配置选项
// 返回值:
// - *GuardOption: 包含默认配置值的GuardOption对象
func DefaultGuardOption() *GuardOption {
	return &GuardOption{
		MemoryThreshold:   GuardMemoryThreshold,   // 设置默认内存警告阈值
		CriticalThreshold: GuardCriticalThreshold, // 设置默认内存临界阈值
		GCInterval:        GuardGCInterval,        // 设置默认GC间隔
		MaxRetries:        GuardMaxRetries,        // 设置默认最大重试次数
		RetryBackoff:      GuardRetryBackoff,      // 设置默认重试等待时间
		MaxBackoff:        GuardMaxBackoff,        // 设置默认最大重试等待时间
		MonitorInterval:   GuardMonitorInterval,   // 设置默认监控间隔
		CleanupInterval:   GuardCleanupInterval,   // 设置默认清理间隔
		AsyncCleanup:      true,                   // 默认启用异步清理
	}
}

// ResourceStats 资源统计信息
type ResourceStats struct {
	TotalResources    int64         // 总资源数,记录所有已申请的资源数量
	ActiveResources   int64         // 活跃资源数,当前正在使用的资源数量
	ReleasedResources int64         // 已释放资源数,已成功释放的资源数量
	LastReleaseTime   time.Time     // 上次资源释放时间,最近一次资源释放的时间点
	TotalReleaseTime  time.Duration // 总释放时间,所有资源释放操作耗费的总时间
	FailedReleases    int64         // 释放失败次数,资源释放失败的总次数
	LastError         error         // 最后一次错误,最近一次操作发生的错误
	MemoryUsage       float64       // 当前内存使用率,系统当前内存使用百分比
}

// ResourceGuard 资源守卫实现
type ResourceGuard struct {
	memManager MemoryManagerInterface // 改为使用接口类型
	stats      *ResourceStats
	option     *GuardOption
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// NewResourceGuard 创建新的资源守卫
// 参数:
// - memManager: 内存管理器实例
// - opts: 可选的配置选项函数列表
// 返回值:
// - *ResourceGuard: 新创建的资源守卫实例
func NewResourceGuard(memManager MemoryManagerInterface, opts ...func(*GuardOption)) *ResourceGuard {
	// 创建默认配置选项
	option := DefaultGuardOption()
	// 应用自定义配置
	for _, opt := range opts {
		opt(option)
	}

	// 创建资源守卫实例
	guard := &ResourceGuard{
		memManager: memManager,
		stats:      &ResourceStats{},
		option:     option,
		stopChan:   make(chan struct{}),
	}

	// 启动后台监控任务
	guard.startBackgroundTasks()

	return guard
}

// startBackgroundTasks 启动后台任务
// 启动资源监控和定期清理两个后台协程
func (g *ResourceGuard) startBackgroundTasks() {
	// 添加两个台任务的计数
	g.wg.Add(2)

	// 启动资源监控协程
	go func() {
		defer g.wg.Done()
		g.monitorRoutine()
	}()

	// 启动定期清理协程
	go func() {
		defer g.wg.Done()
		g.cleanupRoutine()
	}()
}

// AcquireResources 获取资源(非阻塞)
// 参数:
// - ctx: 上下文对象,用于传递取消信号
// 返回值:
// - error: 获取资源过程中的错误,如果成功则返回nil
func (g *ResourceGuard) AcquireResources(ctx context.Context) error {
	// 增加总资源计数
	atomic.AddInt64(&g.stats.TotalResources, 1)
	// 增加活跃资源计数
	atomic.AddInt64(&g.stats.ActiveResources, 1)

	// 异步检查内存使用情况
	go g.checkMemoryUsage(ctx)

	return nil
}

// checkMemoryUsage 检查内存使用情况
// 参数:
// - ctx: 上下文对象,用于传递取消信号
func (g *ResourceGuard) checkMemoryUsage(ctx context.Context) {
	// 获取内存统计信息
	memStats, err := g.memManager.GetMemoryStats()
	if err != nil {
		logger.Warnf("获取内存统计失败: %v", err)
		return
	}

	// 更新当前内存使用率
	g.stats.MemoryUsage = memStats.UsedPercent

	// 检查是否超过警告阈值
	if memStats.UsedPercent > g.option.MemoryThreshold {
		logger.Warnf("内存使用率较高: %.2f%%", memStats.UsedPercent*100)

		// 异步释放资源
		go func() {
			if err := g.releaseResourcesWithRetry(ctx); err != nil {
				logger.Warnf("资源释放失败: %v", err)
				atomic.AddInt64(&g.stats.FailedReleases, 1)
				g.stats.LastError = err
			}
		}()
	}

	// 检查是否超过临界阈值
	if memStats.UsedPercent > g.option.CriticalThreshold {
		logger.Warn("内存使用率超过临界值，执行强制GC")
		g.memManager.TriggerGC()
	}
}

// ReleaseResources 释放资源
// 返回值:
// - error: 释放资源过程中的错误,如果成功则返回nil
func (g *ResourceGuard) ReleaseResources() error {
	// 记录开始时间
	startTime := time.Now()

	// 触发垃圾回收
	g.memManager.TriggerGC()

	// 更新统计信息
	atomic.AddInt64(&g.stats.ReleasedResources, 1)
	atomic.AddInt64(&g.stats.ActiveResources, -1)
	g.stats.LastReleaseTime = time.Now()
	g.stats.TotalReleaseTime += time.Since(startTime)

	return nil
}

// releaseResourcesWithRetry 带重试的资源释放
// 参数:
// - ctx: 上下文对象,用于传递取消信号
// 返回值:
// - error: 释放资源过程中的错误,如果成功则返回nil
func (g *ResourceGuard) releaseResourcesWithRetry(ctx context.Context) error {
	retries := 0                     // 当前重试次数
	backoff := g.option.RetryBackoff // 当前重试等待时间

	// 循环重试直到达到最大重试次数
	for retries < g.option.MaxRetries {
		// 尝试释放资源
		if err := g.ReleaseResources(); err == nil {
			return nil
		}

		// 等待重试或检查取消信号
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			retries++
			// 指数退避增加等待时间
			backoff *= 2
			// 确保不超过最大等待时间
			if backoff > g.option.MaxBackoff {
				backoff = g.option.MaxBackoff
			}
		}
	}

	return fmt.Errorf("释放资源重试次数超过限制(%d)", g.option.MaxRetries)
}

// CleanupResources 清理资源列表
// 参数:
// - files: 需要清理的文件列表
// - removeFile: 是否同时删除文件
// 返回值:
// - error: 清理过程中的错误,如果成功则返回nil
func (g *ResourceGuard) CleanupResources(files []*os.File, removeFile bool) error {
	var errs []error // 错误列表

	// 定义清理单个文件的函数
	cleanup := func(f *os.File) {
		if f == nil {
			return
		}

		path := f.Name()
		// 关闭文件
		if err := f.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭文件失败 [%s]: %v", path, err))
			return
		}

		// 如果需要删除文件
		if removeFile {
			if err := os.Remove(path); err != nil {
				errs = append(errs, fmt.Errorf("删除文件失败 [%s]: %v", path, err))
			}
		}
	}

	// 根据配置选择同步或异步清理
	if g.option.AsyncCleanup {
		// 异步清理每个文件
		for _, f := range files {
			go cleanup(f)
		}
		return nil
	} else {
		// 同步清理每个文件
		for _, f := range files {
			cleanup(f)
		}
	}

	// 如果有错误发生,返回错误信息
	if len(errs) > 0 {
		return fmt.Errorf("清理资源时发生错误: %v", errs)
	}

	return nil
}

// monitorRoutine 资源监控例程
func (g *ResourceGuard) monitorRoutine() {
	// 创建定时器
	ticker := time.NewTicker(g.option.MonitorInterval)
	defer ticker.Stop()

	// 循环监控
	for {
		select {
		case <-g.stopChan:
			return
		case <-ticker.C:
			g.checkResourceUsage()
		}
	}
}

// cleanupRoutine 定期清理例程
func (g *ResourceGuard) cleanupRoutine() {
	// 创建定时器
	ticker := time.NewTicker(g.option.CleanupInterval)
	defer ticker.Stop()

	// 循环清理
	for {
		select {
		case <-g.stopChan:
			return
		case <-ticker.C:
			g.performCleanup()
		}
	}
}

// checkResourceUsage 检查资源使用情况
func (g *ResourceGuard) checkResourceUsage() {
	// 获取内存统计信息
	memStats, err := g.memManager.GetMemoryStats()
	if err != nil {
		logger.Warnf("获取内存统计失败: %v", err)
		return
	}

	// 新内存使用率
	g.stats.MemoryUsage = memStats.UsedPercent

	// 如果超过警告阈值,触发GC
	if memStats.UsedPercent > g.option.MemoryThreshold*100 {
		logger.Warnf("内存使用率: %.2f%%", memStats.UsedPercent*100)
		g.memManager.TriggerGC()
	}
}

// performCleanup 执行清理操作
func (g *ResourceGuard) performCleanup() {
	// 可以添加额外的清理逻辑
}

// Stop 停止资源守卫
func (g *ResourceGuard) Stop() {
	close(g.stopChan) // 关闭停止信号通道
	g.wg.Wait()       // 等待所有后台任务完成
}

// GetStats 获取资源统计信息
// 返回值:
// - *ResourceStats: 当前的资源统计信息
func (g *ResourceGuard) GetStats() *ResourceStats {
	return g.stats
}

// WithMemoryThreshold 设置内���警告阈值
// 参数:
// - threshold: 新的内存警告阈值
// 返回值:
// - func(*GuardOption): 返回一个修改配置的函数
func WithMemoryThreshold(threshold float64) func(*GuardOption) {
	return func(opt *GuardOption) {
		opt.MemoryThreshold = threshold
	}
}

// WithAsyncCleanup 设置是否异步清理
// 参数:
// - async: 是否启用异步清理
// 返回值:
// - func(*GuardOption): 返回一个修改配置的函数
func WithAsyncCleanup(async bool) func(*GuardOption) {
	return func(opt *GuardOption) {
		opt.AsyncCleanup = async
	}
}

// WithCleanupInterval 设置清理间隔时间
// 参数:
// - interval: 新的清理间隔时间
// 返回值:
// - func(*GuardOption): 返回一个修改配置的函数
func WithCleanupInterval(interval time.Duration) func(*GuardOption) {
	return func(opt *GuardOption) {
		opt.CleanupInterval = interval
	}
}

// WithGuardMonitorInterval 设置资源守卫的监控间隔时间
// 参数:
// - interval: 新的监控间隔时间
// 返回值:
// - func(*GuardOption): 返回一个修改配置的函数
func WithGuardMonitorInterval(interval time.Duration) func(*GuardOption) {
	return func(opt *GuardOption) {
		opt.MonitorInterval = interval
	}
}

// WithGuardCleanupInterval 设置资源守卫的清��间隔时间
// 参数:
// - interval: 新的清理间隔时间
// 返回值:
// - func(*GuardOption): 返回一个修改配置的函数
func WithGuardCleanupInterval(interval time.Duration) func(*GuardOption) {
	return func(opt *GuardOption) {
		opt.CleanupInterval = interval
	}
}

// WithGuardCriticalThreshold 设置资源守卫的内存临界阈值
// 参数:
// - threshold: 新的内存临界阈值(0.0-1.0)
// 返回值:
// - func(*GuardOption): 返回一个修改配置的函数
func WithGuardCriticalThreshold(threshold float64) func(*GuardOption) {
	return func(opt *GuardOption) {
		opt.CriticalThreshold = threshold
	}
}
