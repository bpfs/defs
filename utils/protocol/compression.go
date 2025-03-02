package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
)

// CompressionType 压缩类型
type CompressionType uint8

const (
	CompressNone CompressionType = iota
	CompressGzip
	CompressSnappy
	CompressLZ4
)

// Compressor 压缩器接口
type Compressor interface {
	Compress([]byte) ([]byte, error)
	Decompress([]byte) ([]byte, error)
	Type() CompressionType
}

// GzipCompressor Gzip压缩实现
type GzipCompressor struct {
	level int
}

// NewGzipCompressor 创建Gzip压缩器
func NewGzipCompressor(level int) *GzipCompressor {
	if level < gzip.DefaultCompression || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}
	return &GzipCompressor{level: level}
}

// Compress 压缩数据
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, c.level)
	if err != nil {
		logger.Errorf("创建Gzip压缩器失败: %v", err)
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		logger.Errorf("写入数据失败: %v", err)
		return nil, err
	}

	if err := w.Close(); err != nil {
		logger.Errorf("关闭Gzip压缩器失败: %v", err)
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decompress 解压数据
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		logger.Errorf("创建Gzip解压器失败: %v", err)
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// Type 返回压缩类型
func (c *GzipCompressor) Type() CompressionType {
	return CompressGzip
}

// CompressedMessage 压缩消息包装
type CompressedMessage struct {
	Type       CompressionType
	Original   uint32
	Compressed []byte
}

// Marshal 实现Message接口
func (m *CompressedMessage) Marshal() ([]byte, error) {
	buf := make([]byte, 5+len(m.Compressed))
	buf[0] = byte(m.Type)
	binary.BigEndian.PutUint32(buf[1:], m.Original)
	copy(buf[5:], m.Compressed)
	return buf, nil
}

// Unmarshal 实现Message接口
func (m *CompressedMessage) Unmarshal(data []byte) error {
	if len(data) < 5 {
		return &ProtocolError{
			Code:    ErrCodeInvalidLength,
			Message: "压缩消息长度不足",
		}
	}

	m.Type = CompressionType(data[0])
	m.Original = binary.BigEndian.Uint32(data[1:])
	m.Compressed = make([]byte, len(data)-5)
	copy(m.Compressed, data[5:])
	return nil
}
