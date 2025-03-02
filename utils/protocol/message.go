package protocol

import (
	"encoding/binary"
	"hash/crc32"
	"sync"
	"time"
)

// MessageWrapper 消息包装器
type MessageWrapper struct {
	Version   uint16 // 协议版本
	Checksum  uint32 // 校验和
	Timestamp int64  // 时间戳
	Sequence  uint64 // 序列号
	Payload   []byte // 消息内容
}

// Marshal 序列化消息包装器
func (w *MessageWrapper) Marshal() ([]byte, error) {
	size := 2 + 4 + 8 + 8 + len(w.Payload)
	buf := make([]byte, size)

	// 1. 写入版本号
	binary.BigEndian.PutUint16(buf[0:], w.Version)

	// 2. 写入时间戳和序列号
	binary.BigEndian.PutUint64(buf[6:], uint64(w.Timestamp))
	binary.BigEndian.PutUint64(buf[14:], w.Sequence)

	// 3. 写入负载数据
	copy(buf[22:], w.Payload)

	// 4. 计算校验和 (对[6:]部分: 时间戳+序列号+负载)
	checksumData := buf[6:]
	w.Checksum = crc32.ChecksumIEEE(checksumData)

	// 5. 写入校验和
	binary.BigEndian.PutUint32(buf[2:], w.Checksum)

	// 添加调试日志
	logger.Infof("消息封装: version=%d, checksum=%d, timestamp=%d, sequence=%d, payload_size=%d, checksum_data_size=%d",
		w.Version, w.Checksum, w.Timestamp, w.Sequence, len(w.Payload), len(checksumData))

	return buf, nil
}

// Unmarshal 反序列化消息包装器
func (w *MessageWrapper) Unmarshal(data []byte) error {
	if len(data) < 22 { // 最小长度检查
		return &ProtocolError{
			Code:    ErrCodeInvalidLength,
			Message: "消息长度不足",
		}
	}

	// 1. 读取版本号和校验和
	w.Version = binary.BigEndian.Uint16(data[0:])
	w.Checksum = binary.BigEndian.Uint32(data[2:])

	// 2. 计算并验证校验和
	checksumData := data[6:]
	expectedChecksum := crc32.ChecksumIEEE(checksumData)

	if w.Checksum != expectedChecksum {
		// 添加详细的调试日志
		logger.Errorf("校验和不匹配: 收到=%d, 期望=%d, 数据长度=%d, 校验和数据长度=%d",
			w.Checksum, expectedChecksum, len(data), len(checksumData))
		logger.Errorf("校验和范围数据前50字节: %x", checksumData[:min(50, uint32(len(checksumData)))])
		return &ProtocolError{
			Code:    ErrCodeChecksum,
			Message: "校验和错误",
		}
	}

	// 3. 读取其他字段
	w.Timestamp = int64(binary.BigEndian.Uint64(data[6:14]))
	w.Sequence = binary.BigEndian.Uint64(data[14:22])
	w.Payload = make([]byte, len(data)-22)
	copy(w.Payload, data[22:])

	// 添加调试日志
	logger.Infof("消息解析: version=%d, checksum=%d, timestamp=%d, sequence=%d, payload_size=%d, checksum_data_size=%d",
		w.Version, w.Checksum, w.Timestamp, w.Sequence, len(w.Payload), len(checksumData))

	return nil
}

// Validate 验证消息
func (w *MessageWrapper) Validate() error {
	// 检查版本
	if w.Version > CurrentVersion {
		return &ProtocolError{
			Code:    ErrCodeVersion,
			Message: "不支持的协议版本",
		}
	}

	// 检查时间戳
	msgTime := time.Unix(0, w.Timestamp)
	if time.Since(msgTime) > MaxMessageAge {
		return &ProtocolError{
			Code:    ErrCodeTimeout,
			Message: "消息已过期",
		}
	}

	return nil
}

// 添加消息序列号和去重处理
type MessageTracker struct {
	seen     map[uint64]struct{}
	maxSize  int
	mu       sync.RWMutex
	lastSeen uint64
}

// NewMessageTracker 创建消息跟踪器
func NewMessageTracker(maxSize int) *MessageTracker {
	return &MessageTracker{
		seen:    make(map[uint64]struct{}, maxSize),
		maxSize: maxSize,
	}
}

// Track 跟踪消息序列号
func (t *MessageTracker) Track(seq uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 更新最后见到的序列号
	if seq > t.lastSeen {
		t.lastSeen = seq
	}

	// 添加到已见集合
	t.seen[seq] = struct{}{}

	// 清理旧序列号
	if len(t.seen) > t.maxSize {
		for k := range t.seen {
			if k <= t.lastSeen-uint64(t.maxSize) {
				delete(t.seen, k)
			}
		}
	}
}

// IsOrdered 检查消息是否有序
func (t *MessageTracker) IsOrdered(seq uint64) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return seq > t.lastSeen
}

// IsDuplicate 检查消息是否重复
func (t *MessageTracker) IsDuplicate(seq uint64) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.seen[seq]
	return exists
}
