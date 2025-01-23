/**
 * Unit tests for ReedSolomon Streaming API
 *
 * Copyright 2015, Klaus Post
 */

package reedsolomon

import (
	"bytes"
	"io"
	"math/rand"
	"testing"
)

// TestStreamEncoding 测试流式编码功能
func TestStreamEncoding(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	// 创建一个新的流式编码器，10个数据分片，3个校验分片
	r, err := NewStream(10, 3, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)
	par := emptyBuffers(3)

	// 对数据进行编码
	err = r.Encode(toReaders(data), toWriters(par))
	if err != nil {
		t.Fatal(err)
	}
	// 重置数据
	data = toBuffers(input)

	all := append(toReaders(data), toReaders(par)...)
	// 验证编码是否正确
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}

	// 测试错误情况：分片数量不足
	err = r.Encode(toReaders(emptyBuffers(1)), toWriters(emptyBuffers(1)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	err = r.Encode(toReaders(emptyBuffers(10)), toWriters(emptyBuffers(1)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	err = r.Encode(toReaders(emptyBuffers(10)), toWriters(emptyBuffers(3)))
	if err != ErrShardNoData {
		t.Errorf("expected %v, got %v", ErrShardNoData, err)
	}

	// 测试错误情况：分片大小不一致
	badShards := emptyBuffers(10)
	badShards[0] = randomBuffer(123)
	err = r.Encode(toReaders(badShards), toWriters(emptyBuffers(3)))
	if err != ErrShardSize {
		t.Errorf("expected %v, got %v", ErrShardSize, err)
	}
}

// TestStreamEncodingConcurrent 测试并发流式编码功能
func TestStreamEncodingConcurrent(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	// 创建一个新的并发流式编码器，10个数据分片，3个校验分片
	r, err := NewStreamC(10, 3, true, true, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)
	par := emptyBuffers(3)

	// 对数据进行编码
	err = r.Encode(toReaders(data), toWriters(par))
	if err != nil {
		t.Fatal(err)
	}
	// 重置数据
	data = toBuffers(input)

	all := append(toReaders(data), toReaders(par)...)
	// 验证编码是否正确
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}

	// 测试错误情况：分片数量不足
	err = r.Encode(toReaders(emptyBuffers(1)), toWriters(emptyBuffers(1)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	err = r.Encode(toReaders(emptyBuffers(10)), toWriters(emptyBuffers(1)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	err = r.Encode(toReaders(emptyBuffers(10)), toWriters(emptyBuffers(3)))
	if err != ErrShardNoData {
		t.Errorf("expected %v, got %v", ErrShardNoData, err)
	}

	// 测试错误情况：分片大小不一致
	badShards := emptyBuffers(10)
	badShards[0] = randomBuffer(123)
	badShards[1] = randomBuffer(123)
	err = r.Encode(toReaders(badShards), toWriters(emptyBuffers(3)))
	if err != ErrShardSize {
		t.Errorf("expected %v, got %v", ErrShardSize, err)
	}
}

// TestStreamZeroParity 测试零校验分片的情况
func TestStreamZeroParity(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	// 创建一个新的流式编码器，10个数据分片，0个校验分片
	r, err := NewStream(10, 0, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)

	// 对数据进行编码
	err = r.Encode(toReaders(data), []io.Writer{})
	if err != nil {
		t.Fatal(err)
	}
	// 重置数据
	data = toBuffers(input)

	all := toReaders(data)
	// 验证编码是否正确
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}
	// 重置数据
	data = toBuffers(input)

	// 检查重建操作是否无效
	all = toReaders(data)
	err = r.Reconstruct(all, nilWriters(10))
	if err != nil {
		t.Fatal(err)
	}
}

// TestStreamZeroParityConcurrent 测试并发零校验分片的情况
func TestStreamZeroParityConcurrent(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	// 创建一个新的并发流式编码器，10个数据分片，0个校验分片
	r, err := NewStreamC(10, 0, true, true, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)

	// 对数据进行编码
	err = r.Encode(toReaders(data), []io.Writer{})
	if err != nil {
		t.Fatal(err)
	}
	// 重置数据
	data = toBuffers(input)

	all := toReaders(data)
	// 验证编码是否正确
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}
	// 重置数据
	data = toBuffers(input)

	// 检查重建操作是否无效
	all = toReaders(data)
	err = r.Reconstruct(all, nilWriters(10))
	if err != nil {
		t.Fatal(err)
	}
}

// randomBuffer 生成指定长度的随机字节缓冲区
// 参数：
//   - length: 缓冲区长度
//
// 返回值：
//   - *bytes.Buffer: 随机字节缓冲区
func randomBuffer(length int) *bytes.Buffer {
	b := make([]byte, length)
	fillRandom(b)
	return bytes.NewBuffer(b)
}

// randomBytes 生成指定数量和长度的随机字节切片
// 参数：
//   - n: 切片数量
//   - length: 每个切片的长度
//
// 返回值：
//   - [][]byte: 随机字节切片数组
func randomBytes(n, length int) [][]byte {
	bufs := make([][]byte, n)
	for j := range bufs {
		bufs[j] = make([]byte, length)
		fillRandom(bufs[j])
	}
	return bufs
}

// toBuffers 将字节切片数组转换为字节缓冲区数组
// 参数：
//   - in: 输入的字节切片数组
//
// 返回值：
//   - []*bytes.Buffer: 字节缓冲区数组
func toBuffers(in [][]byte) []*bytes.Buffer {
	out := make([]*bytes.Buffer, len(in))
	for i := range in {
		out[i] = bytes.NewBuffer(in[i])
	}
	return out
}

// toReaders 将字节缓冲区数组转换为io.Reader接口数组
// 参数：
//   - in: 输入的字节缓冲区数组
//
// 返回值：
//   - []io.Reader: io.Reader接口数组
func toReaders(in []*bytes.Buffer) []io.Reader {
	out := make([]io.Reader, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// toWriters 将字节缓冲区数组转换为io.Writer接口数组
// 参数：
//   - in: 输入的字节缓冲区数组
//
// 返回值：
//   - []io.Writer: io.Writer接口数组
func toWriters(in []*bytes.Buffer) []io.Writer {
	out := make([]io.Writer, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// nilWriters 创建指定数量的空io.Writer接口数组
// 参数：
//   - n: 数组长度
//
// 返回值：
//   - []io.Writer: 空io.Writer接口数组
func nilWriters(n int) []io.Writer {
	out := make([]io.Writer, n)
	for i := range out {
		out[i] = nil
	}
	return out
}

// emptyBuffers 创建指定数量的空字节缓冲区数组
// 参数：
//   - n: 数组长度
//
// 返回值：
//   - []*bytes.Buffer: 空字节缓冲区数组
func emptyBuffers(n int) []*bytes.Buffer {
	b := make([]*bytes.Buffer, n)
	for i := range b {
		b[i] = &bytes.Buffer{}
	}
	return b
}

// toBytes 将字节缓冲区数组转换为字节切片数组
// 参数：
//   - in: 输入的字节缓冲区数组
//
// 返回值：
//   - [][]byte: 字节切片数组
func toBytes(in []*bytes.Buffer) [][]byte {
	b := make([][]byte, len(in))
	for i := range in {
		b[i] = in[i].Bytes()
	}
	return b
}
