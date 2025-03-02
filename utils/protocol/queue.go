package protocol

import (
	"context"
	"sync/atomic"
	"time"
)

// QueuePolicy 队列策略
type QueuePolicy int

const (
	PolicyBlock     QueuePolicy = iota // 阻塞
	PolicyDrop                         // 丢弃
	PolicyOverwrite                    // 覆盖
)

// MessageQueue 消息队列
type MessageQueue struct {
	pending chan *QueueItem // 待处理消息
	maxSize int             // 最大队列长度
	policy  QueuePolicy     // 溢出策略

	metrics *QueueMetrics // 队列指标
}

// QueueItem 队列项
type QueueItem struct {
	Message  Message
	Deadline time.Time
	Priority int
	Context  context.Context
}

// QueueMetrics 队列指标
type QueueMetrics struct {
	Length    atomic.Int64
	Dropped   atomic.Int64
	Processed atomic.Int64
	WaitTime  AtomicDuration
}

// NewMessageQueue 创建消息队列
func NewMessageQueue(size int, policy QueuePolicy) *MessageQueue {
	return &MessageQueue{
		pending: make(chan *QueueItem, size),
		maxSize: size,
		policy:  policy,
		metrics: &QueueMetrics{},
	}
}

// Put 放入消息
func (q *MessageQueue) Put(ctx context.Context, msg Message, priority int) error {
	item := &QueueItem{
		Message:  msg,
		Deadline: time.Now().Add(time.Second * 30),
		Priority: priority,
		Context:  ctx,
	}

	select {
	case q.pending <- item:
		q.metrics.Length.Add(1)
		return nil
	default:
		switch q.policy {
		case PolicyBlock:
			// 阻塞等待
			select {
			case q.pending <- item:
				q.metrics.Length.Add(1)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		case PolicyDrop:
			// 丢弃消息
			q.metrics.Dropped.Add(1)
			return &ProtocolError{
				Code:    ErrCodeQueueFull,
				Message: "队列已满",
			}
		case PolicyOverwrite:
			// 覆盖最老的消息
			select {
			case <-q.pending:
				q.metrics.Length.Add(-1)
			default:
			}
			q.pending <- item
			q.metrics.Length.Add(1)
			return nil
		}
	}
	return nil
}

// Get 获取消息
func (q *MessageQueue) Get(ctx context.Context) (*QueueItem, error) {
	select {
	case item := <-q.pending:
		q.metrics.Length.Add(-1)
		q.metrics.Processed.Add(1)
		return item, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close 关闭队列
func (q *MessageQueue) Close() {
	close(q.pending)
}

// Len 获取当前队列长度
func (q *MessageQueue) Len() int {
	return int(q.metrics.Length.Load())
}

// IsFull 检查队列是否已满
func (q *MessageQueue) IsFull() bool {
	return q.Len() >= q.maxSize
}

// Clear 清空队列
func (q *MessageQueue) Clear() {
	for {
		select {
		case <-q.pending:
			q.metrics.Length.Add(-1)
		default:
			return
		}
	}
}

// PutWithTimeout 带超时的消息放入
func (q *MessageQueue) PutWithTimeout(ctx context.Context, msg Message, priority int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return q.Put(ctx, msg, priority)
}

// GetWithTimeout 带超时的消息获取
func (q *MessageQueue) GetWithTimeout(ctx context.Context, timeout time.Duration) (*QueueItem, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return q.Get(ctx)
}

// Metrics 获取队列指标
func (q *MessageQueue) Metrics() QueueMetrics {
	return QueueMetrics{
		Length:    atomic.Int64{},
		Dropped:   atomic.Int64{},
		Processed: atomic.Int64{},
		WaitTime:  AtomicDuration{},
	}
}
