package files

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewRetryableOperation 测试创建重试操作实例
func TestNewRetryableOperation(t *testing.T) {
	tests := []struct {
		name    string
		opts    []func(*RetryConfig)
		wantErr bool
	}{
		{
			name:    "默认配置",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "自定义配置",
			opts: []func(*RetryConfig){
				WithMaxRetries(5),
				WithInitialBackoff(time.Second),
				WithMaxBackoff(10 * time.Second),
				WithMaxElapsed(time.Minute),
			},
			wantErr: false,
		},
		{
			name: "无效的重试次数",
			opts: []func(*RetryConfig){
				WithMaxRetries(-1),
			},
			wantErr: true,
		},
		{
			name: "无效的初始等待时间",
			opts: []func(*RetryConfig){
				WithInitialBackoff(-time.Second),
			},
			wantErr: true,
		},
		{
			name: "无效的最大等待时间",
			opts: []func(*RetryConfig){
				WithInitialBackoff(2 * time.Second),
				WithMaxBackoff(time.Second),
			},
			wantErr: true,
		},
		{
			name: "无效的最大总耗时",
			opts: []func(*RetryConfig){
				WithMaxElapsed(-time.Second),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, err := NewRetryableOperation(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRetryableOperation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && retry == nil {
				t.Error("NewRetryableOperation() returned nil without error")
			}
		})
	}
}

// TestRetryableOperation_Execute 测试执行重试操作
func TestRetryableOperation_Execute(t *testing.T) {
	// 创建一个模拟的临时错误
	tempErr := &temporaryError{msg: "临时错误"}

	tests := []struct {
		name       string
		maxRetries int
		op         func() error
		wantErr    bool
	}{
		{
			name:       "成功操作无需重试",
			maxRetries: 3,
			op: func() error {
				return nil
			},
			wantErr: false,
		},
		{
			name:       "永久错误不应重试",
			maxRetries: 3,
			op: func() error {
				return errors.New("永久错误")
			},
			wantErr: true,
		},
		{
			name:       "临时错误应该重试直到成功",
			maxRetries: 3,
			op: func() (err error) {
				if tempErr.attempts < 2 {
					tempErr.attempts++
					return tempErr
				}
				return nil
			},
			wantErr: false,
		},
		{
			name:       "超过最大重试次数",
			maxRetries: 2,
			op: func() error {
				return tempErr
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, err := NewRetryableOperation(
				WithMaxRetries(tt.maxRetries),
				WithInitialBackoff(10*time.Millisecond),
				WithMaxBackoff(50*time.Millisecond),
			)
			if err != nil {
				t.Fatalf("NewRetryableOperation() error = %v", err)
			}

			ctx := context.Background()
			err = retry.Execute(ctx, tt.op)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRetryableOperation_ExecuteWithFallback 测试带降级的重试操作
func TestRetryableOperation_ExecuteWithFallback(t *testing.T) {
	tests := []struct {
		name       string
		op         func() error
		fallback   func() error
		wantErr    bool
		wantResult string
	}{
		{
			name: "主操作成功",
			op: func() error {
				return nil
			},
			fallback: func() error {
				return errors.New("不应该调用降级")
			},
			wantErr: false,
		},
		{
			name: "主操作失败，降级成功",
			op: func() error {
				return errors.New("主操作失败")
			},
			fallback: func() error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "主操作和降级都失败",
			op: func() error {
				return errors.New("主操作失败")
			},
			fallback: func() error {
				return errors.New("降级也失败")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, err := NewRetryableOperation(
				WithMaxRetries(2),
				WithInitialBackoff(10*time.Millisecond),
			)
			if err != nil {
				t.Fatalf("NewRetryableOperation() error = %v", err)
			}

			ctx := context.Background()
			err = retry.ExecuteWithFallback(ctx, tt.op, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecuteWithFallback() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRetryableOperation_ContextCancellation 测试上下文取消
func TestRetryableOperation_ContextCancellation(t *testing.T) {
	retry, err := NewRetryableOperation(
		WithMaxRetries(5),
		WithInitialBackoff(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewRetryableOperation() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 在一段时间后取消上下文
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	err = retry.Execute(ctx, func() error {
		return errors.New("模拟错误")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Execute() error = %v, want context.Canceled", err)
	}
}

// TestRetryableOperation_MaxElapsed 测试最大总耗时
func TestRetryableOperation_MaxElapsed(t *testing.T) {
	retry, err := NewRetryableOperation(
		WithMaxRetries(5),
		WithInitialBackoff(100*time.Millisecond),
		WithMaxElapsed(250*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewRetryableOperation() error = %v", err)
	}

	startTime := time.Now()
	err = retry.Execute(context.Background(), func() error {
		return errors.New("模拟错误")
	})

	if time.Since(startTime) > 300*time.Millisecond {
		t.Error("Execute() took too long, should respect MaxElapsed")
	}
	if err == nil {
		t.Error("Execute() should return error when MaxElapsed is reached")
	}
}

// 用于测试的临时错误类型
type temporaryError struct {
	msg      string
	attempts int
}

func (e *temporaryError) Error() string {
	return e.msg
}

func (e *temporaryError) Temporary() bool {
	return true
}

// TestIsRetryable 测试错误重试判断
func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil错误",
			err:  nil,
			want: false,
		},
		{
			name: "普通错误",
			err:  errors.New("普通错误"),
			want: false,
		},
		{
			name: "临时错误",
			err:  &temporaryError{msg: "临时错误"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}
