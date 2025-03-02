package protocol

import "fmt"

// ProtocolError 定义协议错误类型
type ProtocolError struct {
	Code    int
	Message string
	Err     error
}

// Error 实现error接口
func (e *ProtocolError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("协议错误(代码=%d): %s - %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("协议错误(代码=%d): %s", e.Code, e.Message)
}

// 定义错误码
const (
	ErrCodeInvalidLength = 1001 // 无效的消息长度
	ErrCodeTimeout       = 1002 // 超时错误
	ErrCodeSerialize     = 1003 // 序列化错误
	ErrCodeDeserialize   = 1004 // 反序列化错误
	ErrCodeRead          = 1005 // 读取错误
	ErrCodeWrite         = 1006 // 写入错误
	ErrCodeConnection    = 1007 // 连接错误
	ErrCodePanic         = 1008 // Panic错误
	ErrCodeHeartbeat     = 1009 // 心跳错误
	ErrCodeReconnect     = 1010 // 重连错误
	ErrCodeChecksum      = 1011 // 校验和错误
	ErrCodeVersion       = 1012 // 版本错误
	ErrCodeSequence      = 1013 // 序列号错误
	ErrCodeMessageAge    = 1014 // 消息过期
	ErrCodeHandshake     = 1015 // 握手错误
	ErrCodeFlowControl   = 1016 // 流量控制错误
	ErrCodeQueueFull     = 1017 // 队列已满
	ErrCodeCompression   = 1018 // 压缩错误
	ErrCodeSize          = 1019 // 消息大小错误
	// ... 其他错误码
)

// IsRecoverable 判断错误是否可恢复
func IsRecoverable(err error) bool {
	if pe, ok := err.(*ProtocolError); ok {
		switch pe.Code {
		case ErrCodeTimeout, ErrCodeRead, ErrCodeWrite, ErrCodeHeartbeat:
			return true
		}
	}
	return false
}
