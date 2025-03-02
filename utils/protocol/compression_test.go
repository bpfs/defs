package protocol

import (
	"bytes"
	"testing"
)

func TestGzipCompression(t *testing.T) {
	comp := NewGzipCompressor(-1) // 使用默认压缩级别

	// 测试数据
	original := make([]byte, 1024)
	for i := 0; i < len(original); i++ {
		original[i] = byte(i % 256)
	}
	// 重复数据以提高压缩率
	original = bytes.Repeat(original, 10)

	// 压缩
	compressed, err := comp.Compress(original)
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	// 验证压缩是否有效
	if len(compressed) >= len(original) {
		t.Errorf("compression did not reduce data size: original=%d, compressed=%d",
			len(original), len(compressed))
	}

	// 解压
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompression failed: %v", err)
	}

	// 验证数据完整性
	if !bytes.Equal(original, decompressed) {
		t.Error("decompressed data does not match original")
	}

	// 输出压缩率
	ratio := float64(len(compressed)) / float64(len(original)) * 100
	t.Logf("Compression ratio: %.2f%%", ratio)
}

func TestCompressedMessage(t *testing.T) {
	msg := &CompressedMessage{
		Type:       CompressGzip,
		Original:   100,
		Compressed: []byte("compressed data"),
	}

	// 序列化
	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// 反序列化
	newMsg := &CompressedMessage{}
	if err := newMsg.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// 验证字段
	if newMsg.Type != CompressGzip {
		t.Error("compression type mismatch")
	}
	if newMsg.Original != 100 {
		t.Error("original size mismatch")
	}
	if !bytes.Equal(newMsg.Compressed, []byte("compressed data")) {
		t.Error("compressed data mismatch")
	}
}
