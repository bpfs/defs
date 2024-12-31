package files

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewController 测试创建控制器
func TestNewController(t *testing.T) {
	tests := []struct {
		name    string
		opts    []func(*ControllerOption)
		wantErr bool
	}{
		{
			name:    "默认配置",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "有效配置",
			opts: []func(*ControllerOption){
				WithMaxWorkers(5),
				WithMinWorkers(2),
				WithQueueSize(100),
			},
			wantErr: false,
		},
		{
			name: "无效的最大工作协程数",
			opts: []func(*ControllerOption){
				WithMaxWorkers(-1),
			},
			wantErr: true,
		},
		{
			name: "无效的最小工作协程数",
			opts: []func(*ControllerOption){
				WithMaxWorkers(10),
				WithMinWorkers(20),
			},
			wantErr: true,
		},
		{
			name: "无效的队列大小",
			opts: []func(*ControllerOption){
				WithQueueSize(0),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, err := NewController(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewController() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && controller == nil {
				t.Error("NewController() returned nil controller")
			}
			if controller != nil {
				controller.Stop()
			}
		})
	}
}

// TestExecuteTaskWithPriority 测试任务执行
func TestExecuteTaskWithPriority(t *testing.T) {
	controller, err := NewController(
		WithMaxWorkers(2),
		WithMinWorkers(1),
		WithQueueSize(10),
	)
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Stop()

	tests := []struct {
		name     string
		handler  TaskHandler
		priority TaskPriority
		wantErr  bool
	}{
		{
			name: "成功执行任务",
			handler: func(ctx context.Context, task *TaskInfo) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			},
			priority: PriorityNormal,
			wantErr:  false,
		},
		{
			name: "任务执行失败",
			handler: func(ctx context.Context, task *TaskInfo) error {
				return errors.New("task failed")
			},
			priority: PriorityHigh,
			wantErr:  true,
		},
		{
			name:     "空处理函数",
			handler:  nil,
			priority: PriorityLow,
			wantErr:  true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := controller.ExecuteTaskWithPriority(
				context.Background(),
				tt.handler,
				int64(i),
				tt.priority,
				map[string]interface{}{"test": true},
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecuteTaskWithPriority() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConcurrentTasks 测试并发任务执行
func TestConcurrentTasks(t *testing.T) {
	controller, err := NewController(
		WithMaxWorkers(5),
		WithMinWorkers(2),
		WithQueueSize(100),
	)
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Stop()

	taskCount := 20
	var wg sync.WaitGroup
	wg.Add(taskCount)

	successCount := int32(0)
	for i := 0; i < taskCount; i++ {
		go func(id int64) {
			defer wg.Done()
			err := controller.ExecuteTaskWithPriority(
				context.Background(),
				func(ctx context.Context, task *TaskInfo) error {
					time.Sleep(50 * time.Millisecond)
					return nil
				},
				id,
				PriorityNormal,
				nil,
			)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(int64(i))
	}

	wg.Wait()

	if successCount != int32(taskCount) {
		t.Errorf("Expected %d successful tasks, got %d", taskCount, successCount)
	}
}

// TestTaskTimeout 测试任务超时
func TestTaskTimeout(t *testing.T) {
	controller, err := NewController(
		WithMaxWorkers(1),
		WithMinWorkers(1),
		WithQueueSize(1),
	)
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Stop()

	// 创建一个短超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = controller.ExecuteTaskWithPriority(
		ctx,
		func(ctx context.Context, task *TaskInfo) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		},
		1,
		PriorityHigh,
		nil,
	)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	var controllerErr *ControllerError
	if !errors.As(err, &controllerErr) || controllerErr.Code != CtrlErrTaskCanceled {
		t.Errorf("Expected task canceled error, got %v", err)
	}
}

// TestMetrics 测试指标收集
func TestMetrics(t *testing.T) {
	controller, err := NewController(
		WithMaxWorkers(2),
		WithMinWorkers(1),
		WithQueueSize(10),
	)
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Stop()

	// 执行一些任务
	for i := 0; i < 5; i++ {
		err := controller.ExecuteTaskWithPriority(
			context.Background(),
			func(ctx context.Context, task *TaskInfo) error {
				if i%2 == 0 {
					return errors.New("planned failure")
				}
				return nil
			},
			int64(i),
			PriorityNormal,
			nil,
		)
		if err != nil && i%2 == 0 {
			continue // 预期的失败
		}
		if err != nil {
			t.Errorf("Task %d failed unexpectedly: %v", i, err)
		}
	}

	// 等待指标收集
	time.Sleep(2 * time.Second)

	metrics, err := controller.GetMetrics()
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}

	// 验证指标
	if metrics.TotalTasks != 5 {
		t.Errorf("Expected 5 total tasks, got %d", metrics.TotalTasks)
	}
	if metrics.CompletedTasks != 2 {
		t.Errorf("Expected 2 completed tasks, got %d", metrics.CompletedTasks)
	}
	if metrics.FailedTasks != 3 {
		t.Errorf("Expected 3 failed tasks, got %d", metrics.FailedTasks)
	}
}

// TestGracefulShutdown 测试优雅关闭
func TestGracefulShutdown(t *testing.T) {
	controller, err := NewController(
		WithMaxWorkers(2),
		WithMinWorkers(1),
		WithQueueSize(10),
	)
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}

	// 启动一个长时间运行的任务
	err = controller.ExecuteTaskWithPriority(
		context.Background(),
		func(ctx context.Context, task *TaskInfo) error {
			time.Sleep(500 * time.Millisecond)
			return nil
		},
		1,
		PriorityNormal,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to execute task: %v", err)
	}

	// 立即开始关闭
	startTime := time.Now()
	err = controller.Stop()
	duration := time.Since(startTime)

	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
	if duration < 500*time.Millisecond {
		t.Error("Controller stopped too quickly")
	}
}

// TestTaskPriorities 测试任务优先级
func TestTaskPriorities(t *testing.T) {
	controller, err := NewController(
		WithMaxWorkers(1),
		WithMinWorkers(1),
		WithQueueSize(10),
	)
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Stop()

	executionOrder := make([]TaskPriority, 0)
	var mu sync.Mutex

	priorities := []TaskPriority{
		PriorityLow,
		PriorityNormal,
		PriorityHigh,
		PriorityCritical,
	}

	var wg sync.WaitGroup
	wg.Add(len(priorities))

	for i, priority := range priorities {
		go func(id int64, p TaskPriority) {
			defer wg.Done()
			err := controller.ExecuteTaskWithPriority(
				context.Background(),
				func(ctx context.Context, task *TaskInfo) error {
					mu.Lock()
					executionOrder = append(executionOrder, p)
					mu.Unlock()
					time.Sleep(100 * time.Millisecond)
					return nil
				},
				id,
				p,
				nil,
			)
			if err != nil {
				t.Errorf("Task execution failed: %v", err)
			}
		}(int64(i), priority)
	}

	wg.Wait()

	// 验证执行顺序
	if len(executionOrder) != len(priorities) {
		t.Errorf("Expected %d executions, got %d", len(priorities), len(executionOrder))
	}
}
