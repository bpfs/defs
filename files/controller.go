// Package files 提供文件操作相关功能
//
/**
使用示例:

	// 创建控制器
	controller, err := NewController(
	    WithMaxWorkers(10),
	    WithMinWorkers(2),
	    WithQueueSize(1000),
	)
	if err != nil {
	    log.Fatal(err)
	}

	// 执行任务
	err = controller.ExecuteTaskWithPriority(
	    context.Background(),
	    func(ctx context.Context, task *TaskInfo) error {
	        // 任务处理逻辑
	        return nil
	    },
	    1, // taskID
	    PriorityHigh,
	    map[string]interface{}{
	        "source": "api",
	    },
	)

	// 获取指标
	metrics, err := controller.GetMetrics()

	// 停止控制器
	err = controller.Stop()

*/
package files

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// 控制器常量定义
const (
	CtrlMinWorkers         = 1                // 最小工作协程数
	CtrlMaxWorkers         = 10000            // 最大工作协程数
	CtrlDefaultMinWorkers  = 1                // 默认最小工作协程数
	CtrlDefaultMaxWorkers  = 4                // 默认最大工作协程数
	CtrlDefaultQueueSize   = 1000             // 默认任务队列大小
	CtrlDefaultRetryTimes  = 3                // 默认重试次数
	CtrlDefaultTaskTimeout = 10 * time.Minute // 默认任务超时时间
	CtrlShutdownTimeout    = time.Minute      // 默认关闭超时时间
	CtrlMetricsInterval    = time.Second      // 默认指标收集间隔
	CtrlCleanupInterval    = 5 * time.Minute  // 默认清理间隔
)

// 控制器错误码定义
const (
	CtrlErrInvalidConfig = iota + 1000 // 无效配置错误码
	CtrlErrTaskNotFound                // 任务未找到错误码
	CtrlErrTaskTimeout                 // 任务超时错误码
	CtrlErrTaskCanceled                // 任务取消错误码
	CtrlErrSystemBusy                  // 系统繁忙错误码
)

// TaskPriority 定义任务优先级类型
type TaskPriority int

// 任务优先级常量定义
const (
	PriorityLow      TaskPriority = iota // 低优先级
	PriorityNormal                       // 普通优先级
	PriorityHigh                         // 高优先级
	PriorityCritical                     // 关键优先级
)

// TaskStatus 定义任务状态类型
type TaskStatus int32

// 任务状态常量定义
const (
	TaskStatusPending  TaskStatus = iota // 等待中
	TaskStatusRunning                    // 运行中
	TaskStatusComplete                   // 已完成
	TaskStatusFailed                     // 已失败
)

// ControllerOption 定义控制器配置选项
type ControllerOption struct {
	MaxWorkers      int           // 最大工作协程数
	MinWorkers      int           // 最小工作协程数
	QueueSize       int           // 队列大小
	RetryTimes      int           // 重试次数
	TaskTimeout     time.Duration // 任务超时时间
	ShutdownTimeout time.Duration // 关闭超时时间
	MetricsInterval time.Duration // 指标收集间隔
	CleanupInterval time.Duration // 清理间隔
}

// DefaultOption 返回默认配置选项
// 返回值:
//   - *ControllerOption: 默认配置选项实例
func DefaultOption() *ControllerOption {
	return &ControllerOption{
		MaxWorkers:      CtrlMaxWorkers,         // 设置默认最大工作协程数
		MinWorkers:      CtrlMinWorkers,         // 设置默认最小工作协程数
		QueueSize:       CtrlDefaultQueueSize,   // 设置默认队列大小
		RetryTimes:      CtrlDefaultRetryTimes,  // 设置默认重试次数
		TaskTimeout:     CtrlDefaultTaskTimeout, // 设置默认任务超时时间
		ShutdownTimeout: CtrlShutdownTimeout,    // 设置默认关闭超时时间
		MetricsInterval: CtrlMetricsInterval,    // 设置默认指标收集间隔
		CleanupInterval: CtrlCleanupInterval,    // 设置默认清理间隔
	}
}

// ControllerError 定义错误类型
type ControllerError struct {
	Code    int    // 错误码
	Message string // 错误信息
	Err     error  // 原始错误
}

// Error 实现error接口
// 返回值:
//   - string: 格式化的错误信息
func (e *ControllerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// NewError 创建新的控制器错误
// 参数:
//   - code: 错误码
//   - message: 错误信息
//   - err: 原始错误
//
// 返回值:
//   - *ControllerError: 新创建的错误实例
func NewError(code int, message string, err error) *ControllerError {
	return &ControllerError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// TaskInfo 存储任务的详细信息
type TaskInfo struct {
	TaskID       int64                  // 任务ID
	Status       TaskStatus             // 任务状态
	Priority     TaskPriority           // 任务优先级
	CreateTime   time.Time              // 创建时间
	StartTime    time.Time              // 开始时间
	EndTime      time.Time              // 结束时间
	RetryCount   int32                  // 重试次数
	ErrorMessage string                 // 错误信息
	Metadata     map[string]interface{} // 元数据
	ctx          context.Context        // 上下文
	cancel       context.CancelFunc     // 取消函数
	handler      TaskHandler            // 任务处理函数
}

// ExtendedMetrics 扩展的指标统计
type ExtendedMetrics struct {
	WorkerMetrics                 // 基础指标
	QueueLatency    time.Duration // 队列延迟
	PriorityMetrics map[TaskPriority]struct {
		Count      int64         // 任务数量
		AvgLatency time.Duration // 平均延迟
	}
	ErrorCounts   map[string]int64     // 错误计数
	TasksByStatus map[TaskStatus]int64 // 各状态任务数量
}

// WorkerMetrics 基础指标统计
type WorkerMetrics struct {
	TotalTasks     int64         // 总任务数
	CompletedTasks int64         // 已完成任务数
	FailedTasks    int64         // 失败任务数
	ProcessingTime time.Duration // 处理时间
	MaxProcessTime time.Duration // 最大处理时间
	MinProcessTime time.Duration // 最小处理时间
	AvgProcessTime time.Duration // 平均处理时间
	RetryCount     int64         // 重试次数
	QueueLength    int32         // 队列长度
}

// ConcurrencyController 并发控制器结构体
type ConcurrencyController struct {
	option     *ControllerOption   // 配置选项
	sem        chan struct{}       // 信号量通道
	taskQueue  chan *TaskInfo      // 任务队列
	active     int32               // 活跃任务数
	mu         sync.RWMutex        // 读写锁
	metrics    *ExtendedMetrics    // 指标统计
	tasks      map[int64]*TaskInfo // 任务映射表
	stopChan   chan struct{}       // 停止信号通道
	workerPool sync.Pool           // 工作协程池
	wg         sync.WaitGroup      // 等待组
}

// TaskHandler 定义任务处理函数的类型
type TaskHandler func(context.Context, *TaskInfo) error

// validateOption 验证配置选项
// 参数:
//   - opt: 配置选项
//
// 返回值:
//   - error: 验证错误
func validateOption(opt *ControllerOption) error {
	// 验证最大工作协程数
	if opt.MaxWorkers < CtrlMinWorkers || opt.MaxWorkers > CtrlMaxWorkers {
		return NewError(CtrlErrInvalidConfig,
			fmt.Sprintf("invalid MaxWorkers: %d", opt.MaxWorkers), nil)
	}
	// 验证最小工作协程数
	if opt.MinWorkers < CtrlMinWorkers || opt.MinWorkers > opt.MaxWorkers {
		return NewError(CtrlErrInvalidConfig,
			fmt.Sprintf("invalid MinWorkers: %d", opt.MinWorkers), nil)
	}
	// 验证队列大小
	if opt.QueueSize <= 0 {
		return NewError(CtrlErrInvalidConfig,
			fmt.Sprintf("invalid QueueSize: %d", opt.QueueSize), nil)
	}
	return nil
}

// NewController 创建新的并发控制器
// 参数:
//   - opts: 配置选项函数列表
//
// 返回值:
//   - *ConcurrencyController: 新创建的控制器实例
//   - error: 创建过程中的错误
func NewController(opts ...func(*ControllerOption)) (*ConcurrencyController, error) {
	// 创建默认配置
	option := DefaultOption()
	// 应用自定义配置
	for _, opt := range opts {
		opt(option)
	}

	// 验证配置
	if err := validateOption(option); err != nil {
		return nil, err
	}

	// 创建控制器实例
	cc := &ConcurrencyController{
		option:    option,
		sem:       make(chan struct{}, option.MaxWorkers),
		taskQueue: make(chan *TaskInfo, option.QueueSize),
		metrics:   newExtendedMetrics(),
		tasks:     make(map[int64]*TaskInfo),
		stopChan:  make(chan struct{}),
	}

	// 初始化工作协程池
	cc.workerPool.New = func() interface{} {
		return make(chan struct{}, 1)
	}

	// 初始化控制器
	if err := cc.init(); err != nil {
		return nil, err
	}

	return cc, nil
}

// init 初始化控制器
// 返回值:
//   - error: 初始化错误
func (cc *ConcurrencyController) init() error {
	// 启动工作协程
	for i := 0; i < cc.option.MinWorkers; i++ {
		go cc.worker()
	}

	// 启动指标收集
	go cc.collectMetrics()

	// 启动清理协程
	go cc.cleanupRoutine()

	return nil
}

// WithMaxWorkers 设置最大工作协程数
// 参数:
//   - max: 最大工作协程数
//
// 返回值:
//   - func(*ControllerOption): 配置函数
func WithMaxWorkers(max int) func(*ControllerOption) {
	return func(opt *ControllerOption) {
		opt.MaxWorkers = max
	}
}

// WithMinWorkers 设置最小工作协程数
// 参数:
//   - min: 最小工作协程数
//
// 返回值:
//   - func(*ControllerOption): 配置函数
func WithMinWorkers(min int) func(*ControllerOption) {
	return func(opt *ControllerOption) {
		opt.MinWorkers = min
	}
}

// WithQueueSize 设置队列大小
// 参数:
//   - size: 队列大小
//
// 返回值:
//   - func(*ControllerOption): 配置函数
func WithQueueSize(size int) func(*ControllerOption) {
	return func(opt *ControllerOption) {
		opt.QueueSize = size
	}
}

// ExecuteTaskWithPriority 执行带优先级的任务
// 参数:
//   - ctx: 上下文
//   - handler: 任务处理函数
//   - taskID: 任务ID
//   - priority: 任务优先级
//   - metadata: 任务元数据
//
// 返回值:
//   - error: 执行错误
func (cc *ConcurrencyController) ExecuteTaskWithPriority(
	ctx context.Context,
	handler TaskHandler,
	taskID int64,
	priority TaskPriority,
	metadata map[string]interface{},
) error {
	// 验证处理函数
	if handler == nil {
		return NewError(CtrlErrInvalidConfig, "task handler is nil", nil)
	}

	// 创建任务上下文
	taskCtx, cancel := context.WithTimeout(ctx, cc.option.TaskTimeout)

	// 创建任务信息
	taskInfo := &TaskInfo{
		TaskID:     taskID,
		Status:     TaskStatusPending,
		Priority:   priority,
		CreateTime: time.Now(),
		Metadata:   metadata,
		ctx:        taskCtx,
		cancel:     cancel,
		handler:    handler,
	}

	// 添加任务
	if err := cc.addTask(taskInfo); err != nil {
		cancel()
		return err
	}

	// 将任务加入队列
	select {
	case cc.taskQueue <- taskInfo:
		return cc.waitForTaskComplete(ctx, taskID)
	case <-ctx.Done():
		cc.removeTask(taskID)
		return NewError(CtrlErrTaskCanceled, "task was canceled", ctx.Err())
	}
}

// waitForTaskComplete 等待任务完成
// 参数:
//   - ctx: 上下文
//   - taskID: 任务ID
//
// 返回值:
//   - error: 等待错误
func (cc *ConcurrencyController) waitForTaskComplete(ctx context.Context, taskID int64) error {
	// 创建定时器
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// 循环检查任务状态
	for {
		select {
		case <-ctx.Done():
			return NewError(CtrlErrTaskCanceled, "context canceled", ctx.Err())
		case <-ticker.C:
			task, err := cc.getTask(taskID)
			if err != nil {
				return err
			}

			switch task.Status {
			case TaskStatusComplete:
				return nil
			case TaskStatusFailed:
				return NewError(CtrlErrTaskTimeout, task.ErrorMessage, nil)
			}
		}
	}
}

// worker 工作协程
func (cc *ConcurrencyController) worker() {
	// 增加等待组计数
	cc.wg.Add(1)
	defer cc.wg.Done()

	// 循环处理任务
	for {
		select {
		case <-cc.stopChan:
			return
		case task := <-cc.taskQueue:
			if task == nil {
				continue
			}

			// 更新活跃任务数
			atomic.AddInt32(&cc.active, 1)
			startTime := time.Now()

			var err error
			// 重试执行任务
			for retry := 0; retry <= cc.option.RetryTimes; retry++ {
				if retry > 0 {
					time.Sleep(time.Duration(retry) * time.Second)
				}

				err = cc.executeTask(task)
				if err == nil {
					break
				}
			}

			// 更新任务状态和指标
			cc.updateTaskStatus(task, err)
			cc.updateMetrics(task, startTime)
			atomic.AddInt32(&cc.active, -1)
		}
	}
}

// executeTask 执行任务
// 参数:
//   - task: 任务信息
//
// 返回值:
//   - error: 执行错误
func (cc *ConcurrencyController) executeTask(task *TaskInfo) error {
	select {
	case cc.sem <- struct{}{}:
		defer func() { <-cc.sem }()

		// 更新任务状态
		task.StartTime = time.Now()
		task.Status = TaskStatusRunning

		// 执行任务处理函数
		if err := task.handler(task.ctx, task); err != nil {
			task.Status = TaskStatusFailed
			task.ErrorMessage = err.Error()
			return err
		}

		task.Status = TaskStatusComplete
		return nil

	case <-task.ctx.Done():
		return NewError(CtrlErrTaskTimeout, "task execution timeout", task.ctx.Err())
	}
}

// updateTaskStatus 更新任务状态
// 参数:
//   - task: 任务信息
//   - err: 执行错误
func (cc *ConcurrencyController) updateTaskStatus(task *TaskInfo, err error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 更新任务状态
	if t, exists := cc.tasks[task.TaskID]; exists {
		t.Status = task.Status
		t.EndTime = time.Now()
		if err != nil {
			t.ErrorMessage = err.Error()
		}
	}
}

// updateMetrics 更新指标
// 参数:
//   - task: 任务信息
//   - startTime: 开始时间
func (cc *ConcurrencyController) updateMetrics(task *TaskInfo, startTime time.Time) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 计算执行时间
	duration := time.Since(startTime)

	// 更新基础指标
	cc.metrics.TotalTasks++
	if task.Status == TaskStatusComplete {
		cc.metrics.CompletedTasks++
	} else {
		cc.metrics.FailedTasks++
	}

	// 更新优先级指标
	if pm, exists := cc.metrics.PriorityMetrics[task.Priority]; exists {
		pm.Count++
		pm.AvgLatency = (pm.AvgLatency*time.Duration(pm.Count-1) + duration) / time.Duration(pm.Count)
	}

	// 更新状态计数
	cc.metrics.TasksByStatus[task.Status]++
}

// collectMetrics 收集指标
func (cc *ConcurrencyController) collectMetrics() {
	// 创建定时器
	ticker := time.NewTicker(cc.option.MetricsInterval)
	defer ticker.Stop()

	// 循环收集指标
	for {
		select {
		case <-cc.stopChan:
			return
		case <-ticker.C:
			cc.mu.Lock()
			cc.metrics.QueueLength = int32(len(cc.taskQueue))
			cc.mu.Unlock()
		}
	}
}

// cleanupRoutine 清理例程
func (cc *ConcurrencyController) cleanupRoutine() {
	// 创建定时器
	ticker := time.NewTicker(cc.option.CleanupInterval)
	defer ticker.Stop()

	// 循环清理过期任务
	for {
		select {
		case <-cc.stopChan:
			return
		case <-ticker.C:
			cc.cleanup()
		}
	}
}

// cleanup 清理过期任务
func (cc *ConcurrencyController) cleanup() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 获取当前时间
	now := time.Now()
	// 清理已完成或失败的过期任务
	for taskID, task := range cc.tasks {
		if (task.Status == TaskStatusComplete || task.Status == TaskStatusFailed) &&
			now.Sub(task.EndTime) > cc.option.CleanupInterval {
			delete(cc.tasks, taskID)
		}
	}
}

// Stop 停止控制器
// 返回值:
//   - error: 停止错误
func (cc *ConcurrencyController) Stop() error {
	// 关闭停止信号通道
	close(cc.stopChan)

	// 创建超时定时器
	timeout := time.NewTimer(cc.option.ShutdownTimeout)
	defer timeout.Stop()

	// 创建完成通道
	done := make(chan struct{})
	go func() {
		cc.wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-timeout.C:
		return NewError(CtrlErrSystemBusy,
			fmt.Sprintf("shutdown timeout, active tasks: %d", atomic.LoadInt32(&cc.active)),
			nil)
	case <-done:
		close(cc.taskQueue)
		close(cc.sem)
		return nil
	}
}

// GetMetrics 获取指标
// 返回值:
//   - *ExtendedMetrics: 指标统计信息
//   - error: 获取错误
func (cc *ConcurrencyController) GetMetrics() (*ExtendedMetrics, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	metrics := *cc.metrics
	return &metrics, nil
}

// newExtendedMetrics 创建新的扩展指标实例
// 返回值:
//   - *ExtendedMetrics: 新创建的指标实例
func newExtendedMetrics() *ExtendedMetrics {
	return &ExtendedMetrics{
		WorkerMetrics: WorkerMetrics{
			MinProcessTime: time.Duration(1<<63 - 1),
		},
		PriorityMetrics: make(map[TaskPriority]struct {
			Count      int64
			AvgLatency time.Duration
		}),
		ErrorCounts:   make(map[string]int64),
		TasksByStatus: make(map[TaskStatus]int64),
	}
}

// addTask 添加任务
// 参数:
//   - task: 任务信息
//
// 返回值:
//   - error: 添加错误
func (cc *ConcurrencyController) addTask(task *TaskInfo) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 检查任务是否已存在
	if _, exists := cc.tasks[task.TaskID]; exists {
		return NewError(CtrlErrInvalidConfig,
			fmt.Sprintf("task %d already exists", task.TaskID),
			nil)
	}

	// 添加任务
	cc.tasks[task.TaskID] = task
	return nil
}

// removeTask 移除任务
// 参数:
//   - taskID: 任务ID
func (cc *ConcurrencyController) removeTask(taskID int64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 从任务映射表中删除任务
	delete(cc.tasks, taskID)
}

// getTask 获取任务
// 参数:
//   - taskID: 任务ID
//
// 返回值:
//   - *TaskInfo: 任务信息
//   - error: 获取错误
func (cc *ConcurrencyController) getTask(taskID int64) (*TaskInfo, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	// 查找任务
	task, exists := cc.tasks[taskID]
	if !exists {
		return nil, NewError(CtrlErrTaskNotFound,
			fmt.Sprintf("task %d not found", taskID),
			nil)
	}

	return task, nil
}
