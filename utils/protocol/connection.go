package protocol

import (
	"bufio"
	"context"
	"net"
	"sync/atomic"
	"time"
)

// connect 重新建立连接
func (h *Handler) connect() error {
	// logger.Infof("开始建立连接: network=%s, address=%s", h.network, h.address)
	// 获取连接信号量
	if err := h.acquireConn(); err != nil {
		logger.Errorf("获取连接信号量失败: %v", err)
		return err
	}
	// logger.Infof("成功获取连接信号量")
	defer h.releaseConn()

	// 关闭现有连接
	h.cleanup()

	// 设置连接超时
	ctx, cancel := context.WithTimeout(context.Background(), h.dialTimeout)
	defer cancel()

	// 创建新连接
	dialer := &net.Dialer{Timeout: h.dialTimeout}
	conn, err := dialer.DialContext(ctx, h.network, h.address)
	if err != nil {
		h.metrics.Errors.Add(1)
		return &ProtocolError{
			Code:    ErrCodeConnection,
			Message: "建立连接失败",
			Err:     err,
		}
	}

	// 更新连接和读写器
	h.mu.Lock()
	h.conn = conn
	h.reader = bufio.NewReader(conn)
	h.writer = bufio.NewWriter(conn)
	h.lastActivity = time.Now()
	h.mu.Unlock()

	h.metrics.ActiveConnections.Add(1)

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetKeepAlive(true); err != nil {
			logger.Errorf("设置TCP连接保持活跃失败: %v", err)
			return err
		}
		if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
			logger.Errorf("设置TCP连接保持活跃周期失败: %v", err)
			return err
		}
	}

	return nil
}

// cleanup 清理连接资源
func (h *Handler) cleanup() {
	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
		h.metrics.ActiveConnections.Add(-1)
	}
	h.reader = nil
	h.writer = nil
}

// acquireConn 获取连接信号量
func (h *Handler) acquireConn() error {
	select {
	case h.semaphore <- struct{}{}:
		return nil
	case <-time.After(h.dialTimeout):
		return &ProtocolError{
			Code:    ErrCodeTimeout,
			Message: "获取连接超时",
		}
	}
}

// releaseConn 释放连接信号量
func (h *Handler) releaseConn() {
	select {
	case <-h.semaphore:
	default:
	}
}

// isConnected 检查连接状态
func (h *Handler) isConnected() bool {
	return atomic.LoadInt32(&h.status) == StatusConnected
}

// Reconnect 重新连接
func (h *Handler) Reconnect(ctx context.Context) error {
	if !h.reconnect {
		return &ProtocolError{
			Code:    ErrCodeConnection,
			Message: "未启用重连机制",
		}
	}

	atomic.StoreInt32(&h.status, StatusReconnecting)
	h.metrics.Retries.Add(1)

	var lastErr error
	for i := 0; i < h.maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := h.connect(); err != nil {
				lastErr = err
				time.Sleep(h.retryDelay)
				continue
			}
			atomic.StoreInt32(&h.status, StatusConnected)
			return nil
		}
	}

	atomic.StoreInt32(&h.status, StatusDisconnected)
	return lastErr
}

// StartHeartbeat 开始心跳
func (h *Handler) StartHeartbeat() {
	if h.heartbeat <= 0 {
		return
	}

	// 确保只初始化一次
	h.mu.Lock()
	if h.heartbeatStop != nil {
		h.mu.Unlock()
		return
	}
	h.heartbeatStop = make(chan struct{})
	h.mu.Unlock()

	go func() {
		ticker := time.NewTicker(h.heartbeat)
		defer ticker.Stop()

		for {
			select {
			case <-h.heartbeatStop:
				return
			case <-ticker.C:
				// 检查连接状态
				if !h.isConnected() {
					return
				}

				if err := h.sendHeartbeat(); err != nil {
					logger.Warnf("发送心跳失败: %v", err)
					h.metrics.Errors.Add(1)

					// 只在连接错误时触发断开回调
					if IsRecoverable(err) {
						if h.recovery != nil && h.recovery.OnDisconnect != nil {
							h.recovery.OnDisconnect(err)
						}
					}
				}
			}
		}
	}()
}

// StopHeartbeat 停止心跳
func (h *Handler) StopHeartbeat() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.heartbeatStop != nil {
		close(h.heartbeatStop)
		h.heartbeatStop = nil
	}
}

// sendHeartbeat 发送心跳包
func (h *Handler) sendHeartbeat() error {
	// 创建心跳消息
	heartbeat := &HeartbeatMessage{
		Timestamp: time.Now().UnixNano(),
	}

	// 发送心跳
	return h.SendMessage(heartbeat)
}

// HeartbeatMessage 心跳消息
type HeartbeatMessage struct {
	Timestamp int64
}

// Marshal 实现Message接口
func (m *HeartbeatMessage) Marshal() ([]byte, error) {
	return []byte{}, nil // 空心跳包
}

// Unmarshal 实现Message接口
func (m *HeartbeatMessage) Unmarshal(data []byte) error {
	return nil
}
