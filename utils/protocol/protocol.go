package protocol

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	logging "github.com/dep2p/log"
	"golang.org/x/time/rate"
)

var logger = logging.Logger("utils/protocol")

const (
	// 协议常量定义
	MaxMessageSize = 100 * 1024 * 1024 // 最大消息大小(100MB)
	DefaultTimeout = 30 * time.Second  // 默认超时时间
	HeaderSize     = 4                 // 消息头大小(用于长度)
	MaxRetries     = 3                 // 最大重试次数

	// 连接状态
	StatusConnected = iota
	StatusDisconnected
	StatusReconnecting

	// 默认配置
	DefaultBufferSize = 2 * 1024 * 1024 // 增大缓冲区到2MB
	DefaultMaxConns   = 1000
	DefaultQueueSize  = 1000

	CurrentVersion = 1           // 当前协议版本
	MaxMessageAge  = time.Minute // 消息最大有效期

	// 添加功能位图常量
	FeatureCompression = 1 << iota // 支持压缩
	FeatureEncryption              // 支持加密
	FeatureHeartbeat               // 支持心跳
	FeatureReconnect               // 支持重连

	// 修改速率限制相关配置
	RateLimitBurst = 20 * 1024 * 1024 // 突发限制提高到20MB
	RateLimitRate  = 50 * 1024 * 1024 // 速率限制提高到50MB/s

	// 添加常量定义
	MaxLogDataLength = 200 // 日志中显示的最大数据长度
)

// 缓冲区池
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, DefaultBufferSize)
	},
}

// Message 定义协议消息接口
type Message interface {
	Marshal() ([]byte, error) // 序列化
	Unmarshal([]byte) error   // 反序列化
}

// Handler 处理协议通信
type Handler struct {
	reader   *bufio.Reader
	writer   *bufio.Writer
	conn     net.Conn
	recovery *RecoveryHandler
	network  string // 网络类型(tcp/udp)
	address  string // 目标地址

	// 连接状态
	status     int32         // 原子操作
	reconnect  bool          // 是否允许重连
	maxRetries int           // 最大重连次数
	retryDelay time.Duration // 重连延迟

	// 心跳相关
	heartbeat     time.Duration // 心跳间隔
	lastActivity  time.Time     // 最后活动时间
	heartbeatStop chan struct{} // 停止心跳

	// 超时控制
	dialTimeout    time.Duration
	writeTimeout   time.Duration
	readTimeout    time.Duration
	processTimeout time.Duration

	// 并发控制
	semaphore chan struct{}
	closeOnce sync.Once
	closed    int32

	// 消息序列号
	sequence uint64 // 原子操作的消息序列号计数器

	// 统计信息
	metrics *Metrics

	mu sync.RWMutex // 保护并发访问

	// 握手相关
	features uint32 // 协商的功能
	authData []byte // 认证数据

	// 流量控制
	flowController *FlowController

	// 消息队列
	sendQueue    *MessageQueue
	receiveQueue *MessageQueue

	// 压缩相关
	compressor Compressor

	// 添加背压控制
	backPressure BackPressure

	ctx        context.Context
	cancelFunc context.CancelFunc

	msgTracker *MessageTracker

	// 新增速率限制器
	rateLimiter *rate.Limiter
}

// Metrics 统计指标
type Metrics struct {
	ActiveConnections atomic.Int64
	MessagesSent      atomic.Int64
	MessagesReceived  atomic.Int64
	Errors            atomic.Int64
	Retries           atomic.Int64
	BufferUsage       atomic.Int64
	LastError         error
}

// Options 配置选项
type Options struct {
	Recovery       *RecoveryHandler
	Reconnect      bool
	MaxRetries     int
	RetryDelay     time.Duration
	HeartBeat      time.Duration
	DialTimeout    time.Duration
	WriteTimeout   time.Duration
	ReadTimeout    time.Duration
	ProcessTimeout time.Duration
	MaxConns       int
	Rate           int64
	Window         int64
	Threshold      int64
	QueueSize      int
	QueuePolicy    QueuePolicy
	RetryConfig    *RetryConfig
}

// RetryConfig 定义重试配置
type RetryConfig struct {
	MaxRetries int
	RetryDelay time.Duration
}

// NewHandler 创建新的协议处理器
func NewHandler(conn net.Conn, opts *Options) *Handler {
	if opts == nil {
		opts = &Options{
			MaxRetries:     MaxRetries,
			RetryDelay:     time.Second,
			HeartBeat:      time.Second * 30,
			DialTimeout:    DefaultTimeout,
			WriteTimeout:   DefaultTimeout,
			ReadTimeout:    DefaultTimeout,
			ProcessTimeout: DefaultTimeout,
			MaxConns:       DefaultMaxConns,
			Rate:           1000,
			Window:         1024,
			Threshold:      512,
			QueueSize:      DefaultQueueSize,
			QueuePolicy:    PolicyBlock,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	h := &Handler{
		conn:           conn,
		reader:         bufio.NewReader(conn),
		writer:         bufio.NewWriter(conn),
		recovery:       opts.Recovery,
		reconnect:      opts.Reconnect,
		maxRetries:     opts.MaxRetries,
		retryDelay:     opts.RetryDelay,
		heartbeat:      opts.HeartBeat,
		dialTimeout:    opts.DialTimeout,
		writeTimeout:   opts.WriteTimeout,
		readTimeout:    opts.ReadTimeout,
		processTimeout: opts.ProcessTimeout,
		semaphore:      make(chan struct{}, opts.MaxConns),
		metrics:        &Metrics{},
		flowController: NewFlowController(opts.Rate, opts.Window, opts.Threshold),
		sendQueue:      NewMessageQueue(opts.QueueSize, opts.QueuePolicy),
		receiveQueue:   NewMessageQueue(opts.QueueSize, opts.QueuePolicy),
		backPressure: BackPressure{
			highWaterMark: 10000,
			lowWaterMark:  1000,
		},
		ctx:           ctx,
		cancelFunc:    cancel,
		msgTracker:    NewMessageTracker(10000), // 跟踪最近的10000个消息
		heartbeatStop: make(chan struct{}),
	}

	atomic.StoreInt32(&h.status, StatusConnected)
	h.initRateLimiter()
	return h
}

// Close 关闭连接
func (h *Handler) Close() error {
	return h.conn.Close()
}

// SetDeadline 设置超时
func (h *Handler) SetDeadline(t time.Time) error {
	return h.conn.SetDeadline(t)
}

// Handshake 执行握手
func (h *Handler) Handshake(authData []byte) error {
	// 设置握手超时
	if err := h.conn.SetDeadline(time.Now().Add(HandshakeTimeout)); err != nil {
		logger.Errorf("设置握手超时失败: %v", err)
		return err
	}
	defer h.conn.SetDeadline(time.Time{}) // 清除超时

	// 准备请求
	req := &HandshakeRequest{
		Version:   CurrentVersion,
		Timestamp: time.Now().UnixNano(),
		Features:  FeatureCompression | FeatureHeartbeat | FeatureReconnect,
		AuthData:  authData,
	}

	// 发送请求
	if err := h.SendMessage(req); err != nil {
		logger.Errorf("发送握手请求失败: %v", err)
		return err
	}

	// 接收响应
	resp := &HandshakeResponse{}
	if err := h.ReceiveMessage(resp); err != nil {
		logger.Errorf("接收握手响应失败: %v", err)
		return err
	}

	// 检查响应
	if resp.Status != HandshakeStatusOK {
		logger.Errorf("握手响应失败: %v", resp.Message)
		return &ProtocolError{
			Code:    ErrCodeHandshake,
			Message: resp.Message,
		}
	}

	// 保存协商结果
	h.features = resp.Features & req.Features
	h.authData = authData

	return nil
}

// StartQueueProcessor 启动队列处理器
func (h *Handler) StartQueueProcessor(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				item, err := h.sendQueue.Get(ctx)
				if err != nil {
					if err != context.Canceled {
						logger.Errorf("获取发送队列消息失败: %v", err)
					}
					continue
				}

				// 处理消息
				if err := h.processSendItem(item); err != nil {
					logger.Errorf("处理发送队列消息失败: %v", err)
				}
			}
		}
	}()
}

// processSendItem 处理发送队列项
func (h *Handler) processSendItem(item *QueueItem) error {
	// 检查并更新背压状态
	h.checkBackPressure()

	// 检查截止时间
	if !item.Deadline.IsZero() && time.Now().After(item.Deadline) {
		return &ProtocolError{
			Code:    ErrCodeTimeout,
			Message: "消息已过期",
		}
	}

	// 检查上下文
	if err := item.Context.Err(); err != nil {
		return err
	}

	// 发送消息
	return h.SendMessage(item.Message)
}

// SendMessage 发送消息
func (h *Handler) SendMessage(msg Message) error {
	return h.SafeHandle(func() error {
		// 序列化消息
		data, err := msg.Marshal()
		if err != nil {
			return err
		}

		// 创建消息包装
		wrapper := &MessageWrapper{
			Version:   CurrentVersion,
			Sequence:  atomic.AddUint64(&h.sequence, 1),
			Timestamp: time.Now().UnixNano(),
			Payload:   data,
		}

		// 序列化包装后的消息
		msgData, err := wrapper.Marshal()
		if err != nil {
			return err
		}

		// 检查消息大小
		if len(msgData) > MaxMessageSize {
			return &ProtocolError{
				Code:    ErrCodeSize,
				Message: "消息超过最大限制",
			}
		}

		// 设置写入超时
		if err := h.conn.SetWriteDeadline(time.Now().Add(h.writeTimeout)); err != nil {
			return err
		}

		// 写入长度头
		lenBuf := make([]byte, HeaderSize)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(msgData)))
		if _, err := h.writer.Write(lenBuf); err != nil {
			return err
		}

		// 使用流量控制器进行分块传输
		chunkSize := 64 * 1024 // 64KB chunks
		for i := 0; i < len(msgData); i += chunkSize {
			end := i + chunkSize
			if end > len(msgData) {
				end = len(msgData)
			}

			// 使用流量控制器
			if err := h.flowController.Acquire(int64(end - i)); err != nil {
				return err
			}

			// 写入数据块
			if _, err := h.writer.Write(msgData[i:end]); err != nil {
				return err
			}

			// 释放流量控制
			h.flowController.Release(int64(end - i))
		}

		return h.writer.Flush()
	})
}

// 添加一些辅助方法

// SetCompressor 设置压缩器
func (h *Handler) SetCompressor(c Compressor) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.compressor = c
}

// GetFeatures 获取当前功能位图
func (h *Handler) GetFeatures() uint32 {
	return atomic.LoadUint32(&h.features)
}

// IsFeatureEnabled 检查功能是否启用
func (h *Handler) IsFeatureEnabled(feature uint32) bool {
	return h.GetFeatures()&feature != 0
}

// GetMetrics 获取指标统计
func (h *Handler) GetMetrics() *Metrics {
	return h.metrics
}

// GetQueueMetrics 获取队列指标
func (h *Handler) GetQueueMetrics() (send, receive QueueMetrics) {
	return h.sendQueue.Metrics(), h.receiveQueue.Metrics()
}

// GracefulClose 优雅关闭
func (h *Handler) GracefulClose(timeout time.Duration) error {
	var err error
	h.closeOnce.Do(func() {
		// 设置关闭标志
		atomic.StoreInt32(&h.closed, 1)

		// 获取锁保护资源清理
		h.mu.Lock()
		defer h.mu.Unlock()

		// 取消上下文
		h.cancelFunc()

		// 停止心跳
		h.StopHeartbeat()

		// 关闭队列
		h.sendQueue.Close()
		h.receiveQueue.Close()

		// 关闭连接
		h.cleanup()
	})
	return err
}

// 添加背压控制
type BackPressure struct {
	highWaterMark int64
	lowWaterMark  int64
	paused        atomic.Bool
}

func (h *Handler) checkBackPressure() {
	if int64(h.sendQueue.Len()) > h.backPressure.highWaterMark {
		h.backPressure.paused.Store(true)
	} else if int64(h.sendQueue.Len()) < h.backPressure.lowWaterMark {
		h.backPressure.paused.Store(false)
	}
}

// 添加辅助函数用于截断日志数据
func truncateForLog(data []byte) string {
	if len(data) <= MaxLogDataLength {
		return fmt.Sprintf("%x", data)
	}
	return fmt.Sprintf("%x...(%d bytes total)", data[:MaxLogDataLength], len(data))
}

// ReceiveMessage 接收消息
func (h *Handler) ReceiveMessage(msg Message) error {
	return h.SafeHandle(func() error {
		// logger.Infof("开始接收消息: type=%T", msg)

		// 读取长度头
		buf := bufferPool.Get().([]byte)
		defer bufferPool.Put(buf)

		// 读取4字节的长度头
		if _, err := io.ReadFull(h.reader, buf[:HeaderSize]); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(buf[:HeaderSize])

		// 检查消息大小
		if length > MaxMessageSize {
			return &ProtocolError{
				Code:    ErrCodeSize,
				Message: "消息长度超过最大限制",
			}
		}

		// 对大消息使用更长的超时时间
		timeout := h.readTimeout
		if length > DefaultBufferSize {
			timeout = h.readTimeout * 10
		}

		// 设置读取超时
		if err := h.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}

		// 确保缓冲区足够大
		if int(length) > len(buf) {
			return &ProtocolError{
				Code:    ErrCodeSize,
				Message: "缓冲区大小不足",
			}
		}

		// 使用 io.ReadFull 确保读取完整的消息
		if _, err := io.ReadFull(h.reader, buf[:length]); err != nil {
			return err
		}

		// 创建新的缓冲区保存数据
		data := make([]byte, length)
		copy(data, buf[:length])

		// 修改日志记录
		// logger.Infof("接收到的原始数据: %s", truncateForLog(data))

		// 反序列化消息
		if err := msg.Unmarshal(data); err != nil {
			logger.Errorf("反序列化消息失败: %v, 数据前200字节: %s",
				err, truncateForLog(data))
			return err
		}

		h.metrics.MessagesReceived.Add(1)
		h.metrics.BufferUsage.Add(int64(length + HeaderSize))

		// logger.Infof("接收消息: size=%d", length)

		return nil
	})
}

// 辅助函数
func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// 修改速率限制器的初始化
func (h *Handler) initRateLimiter() {
	// 使用新的配置
	h.rateLimiter = rate.NewLimiter(rate.Limit(RateLimitRate), RateLimitBurst)
}
