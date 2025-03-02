package protocol

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

// mockConn 模拟网络连接
type mockConn struct {
	*bytes.Buffer
	closed bool
}

func (m *mockConn) Close() error                       { m.closed = true; return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// newMockHandler 创建测试用的Handler
func newMockHandler() (*Handler, *mockConn) {
	conn := &mockConn{Buffer: bytes.NewBuffer(nil)}
	opts := &Options{
		MaxRetries:     3,
		RetryDelay:     time.Millisecond * 100,
		HeartBeat:      time.Second,
		DialTimeout:    time.Second,
		WriteTimeout:   time.Second,
		ReadTimeout:    time.Second,
		ProcessTimeout: time.Second,
		MaxConns:       10,
		Rate:           1000,
		Window:         1024,
		Threshold:      512,
		QueueSize:      100,
		QueuePolicy:    PolicyBlock,
	}
	return NewHandler(conn, opts), conn
}

// TestHandlerBasic 基础功能测试
func TestHandlerBasic(t *testing.T) {
	h, conn := newMockHandler()

	// 确保初始状态正确
	if h.conn == nil {
		t.Fatal("connection should be initialized")
	}

	// 测试连接状态
	if !h.isConnected() {
		t.Error("handler should be connected")
	}

	// 测试特性设置
	h.features = FeatureCompression | FeatureHeartbeat
	if !h.IsFeatureEnabled(FeatureCompression) {
		t.Error("compression should be enabled")
	}

	// 确保心跳通道已初始化
	if h.heartbeatStop == nil {
		t.Fatal("heartbeat channel should be initialized")
	}

	// 执行关闭
	if err := h.GracefulClose(time.Second); err != nil {
		t.Errorf("failed to close handler: %v", err)
	}

	// 测试关闭状态
	if !conn.closed {
		t.Error("connection should be closed")
	}

	// 验证资源已清理
	if h.conn != nil || h.reader != nil || h.writer != nil {
		t.Error("resources should be cleaned up")
	}
}

// TestMessageSendReceive 消息收发测试
func TestMessageSendReceive(t *testing.T) {
	h, _ := newMockHandler()
	defer func() {
		if err := h.GracefulClose(time.Second); err != nil {
			t.Errorf("failed to close handler: %v", err)
		}
	}()

	ctx := context.Background()
	msg := &testMessage{data: []byte("test message")}

	// 启动队列处理器
	h.StartQueueProcessor(ctx)

	// 添加等待消息处理的机制
	done := make(chan struct{})
	go func() {
		for {
			if h.GetMetrics().MessagesSent.Load() > 0 {
				close(done)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// 发送消息
	if err := h.sendQueue.Put(ctx, msg, 0); err != nil {
		t.Errorf("failed to put message: %v", err)
	}

	// 等待消息发送完成或超时
	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Error("timeout waiting for message to be sent")
	}

	// 检查指标
	metrics := h.GetMetrics()
	if metrics.MessagesSent.Load() != 1 {
		t.Error("message sent count should be 1")
	}
}

// testMessage 用于测试的消息类型
type testMessage struct {
	data []byte
}

func (m *testMessage) Marshal() ([]byte, error) {
	return m.data, nil
}

func (m *testMessage) Unmarshal(data []byte) error {
	m.data = make([]byte, len(data))
	copy(m.data, data)
	return nil
}
