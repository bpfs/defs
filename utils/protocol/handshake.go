package protocol

import (
	"encoding/binary"
	"time"
)

const (
	// 握手状态码
	HandshakeStatusOK           = 0
	HandshakeStatusVersionError = 1
	HandshakeStatusAuthError    = 2
	HandshakeStatusError        = 3

	// 握手超时时间
	HandshakeTimeout = 10 * time.Second
)

// HandshakeRequest 握手请求
type HandshakeRequest struct {
	Version   uint16 // 协议版本
	Timestamp int64  // 请求时间戳
	Features  uint32 // 功能位图
	AuthData  []byte // 认证数据
}

// HandshakeResponse 握手响应
type HandshakeResponse struct {
	Status   uint16 // 状态码
	Version  uint16 // 协议版本
	Features uint32 // 支持的功能
	Message  string // 响应消息
}

// 实现 Message 接口
func (r *HandshakeRequest) Marshal() ([]byte, error) {
	size := 2 + 8 + 4 + len(r.AuthData)
	buf := make([]byte, size)

	binary.BigEndian.PutUint16(buf[0:], r.Version)
	binary.BigEndian.PutUint64(buf[2:], uint64(r.Timestamp))
	binary.BigEndian.PutUint32(buf[10:], r.Features)
	copy(buf[14:], r.AuthData)

	return buf, nil
}

func (r *HandshakeRequest) Unmarshal(data []byte) error {
	if len(data) < 14 {
		return &ProtocolError{
			Code:    ErrCodeInvalidLength,
			Message: "握手请求数据长度不足",
		}
	}

	r.Version = binary.BigEndian.Uint16(data[0:])
	r.Timestamp = int64(binary.BigEndian.Uint64(data[2:]))
	r.Features = binary.BigEndian.Uint32(data[10:])
	r.AuthData = make([]byte, len(data)-14)
	copy(r.AuthData, data[14:])

	return nil
}

// 实现 Message 接口
func (r *HandshakeResponse) Marshal() ([]byte, error) {
	msgBytes := []byte(r.Message)
	size := 2 + 2 + 4 + len(msgBytes)
	buf := make([]byte, size)

	binary.BigEndian.PutUint16(buf[0:], r.Status)
	binary.BigEndian.PutUint16(buf[2:], r.Version)
	binary.BigEndian.PutUint32(buf[4:], r.Features)
	copy(buf[8:], msgBytes)

	return buf, nil
}

func (r *HandshakeResponse) Unmarshal(data []byte) error {
	if len(data) < 8 {
		return &ProtocolError{
			Code:    ErrCodeInvalidLength,
			Message: "握手响应数据长度不足",
		}
	}

	r.Status = binary.BigEndian.Uint16(data[0:])
	r.Version = binary.BigEndian.Uint16(data[2:])
	r.Features = binary.BigEndian.Uint32(data[4:])
	r.Message = string(data[8:])

	return nil
}
