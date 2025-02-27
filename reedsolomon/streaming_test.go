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

// 测试流式编码
func TestStreamEncoding(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	r, err := NewStream(10, 3, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)
	par := emptyBuffers(3)

	err = r.Encode(toReaders(data), toWriters(par))
	if err != nil {
		t.Fatal(err)
	}
	// Reset Data
	data = toBuffers(input)

	all := append(toReaders(data), toReaders(par)...)
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}

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

	badShards := emptyBuffers(10)
	badShards[0] = randomBuffer(123)
	err = r.Encode(toReaders(badShards), toWriters(emptyBuffers(3)))
	if err != ErrShardSize {
		t.Errorf("expected %v, got %v", ErrShardSize, err)
	}
}

// 测试流式编码并发
func TestStreamEncodingConcurrent(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	r, err := NewStreamC(10, 3, true, true, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)
	par := emptyBuffers(3)

	err = r.Encode(toReaders(data), toWriters(par))
	if err != nil {
		t.Fatal(err)
	}
	// Reset Data
	data = toBuffers(input)

	all := append(toReaders(data), toReaders(par)...)
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}

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

	badShards := emptyBuffers(10)
	badShards[0] = randomBuffer(123)
	badShards[1] = randomBuffer(123)
	err = r.Encode(toReaders(badShards), toWriters(emptyBuffers(3)))
	if err != ErrShardSize {
		t.Errorf("expected %v, got %v", ErrShardSize, err)
	}
}

// 测试流式编码零奇偶校验
func TestStreamZeroParity(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	r, err := NewStream(10, 0, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)

	err = r.Encode(toReaders(data), []io.Writer{})
	if err != nil {
		t.Fatal(err)
	}
	// Reset Data
	data = toBuffers(input)

	all := toReaders(data)
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}
	// Reset Data
	data = toBuffers(input)

	// Check that Reconstruct does nothing
	all = toReaders(data)
	err = r.Reconstruct(all, nilWriters(10))
	if err != nil {
		t.Fatal(err)
	}
}

// 测试流式编码零奇偶校验并发
func TestStreamZeroParityConcurrent(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	r, err := NewStreamC(10, 0, true, true, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	input := randomBytes(10, perShard)
	data := toBuffers(input)

	err = r.Encode(toReaders(data), []io.Writer{})
	if err != nil {
		t.Fatal(err)
	}
	// Reset Data
	data = toBuffers(input)

	all := toReaders(data)
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}
	// Reset Data
	data = toBuffers(input)

	// Check that Reconstruct does nothing
	all = toReaders(data)
	err = r.Reconstruct(all, nilWriters(10))
	if err != nil {
		t.Fatal(err)
	}
}

// 随机缓冲区
func randomBuffer(length int) *bytes.Buffer {
	b := make([]byte, length)
	fillRandom(b)
	return bytes.NewBuffer(b)
}

// 随机字节
func randomBytes(n, length int) [][]byte {
	bufs := make([][]byte, n)
	for j := range bufs {
		bufs[j] = make([]byte, length)
		fillRandom(bufs[j])
	}
	return bufs
}

// 转换为缓冲区
func toBuffers(in [][]byte) []*bytes.Buffer {
	out := make([]*bytes.Buffer, len(in))
	for i := range in {
		out[i] = bytes.NewBuffer(in[i])
	}
	return out
}

// 转换为读取器
func toReaders(in []*bytes.Buffer) []io.Reader {
	out := make([]io.Reader, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// 转换为写入器
func toWriters(in []*bytes.Buffer) []io.Writer {
	out := make([]io.Writer, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// 空写入器
func nilWriters(n int) []io.Writer {
	out := make([]io.Writer, n)
	for i := range out {
		out[i] = nil
	}
	return out
}

// 空缓冲区
func emptyBuffers(n int) []*bytes.Buffer {
	b := make([]*bytes.Buffer, n)
	for i := range b {
		b[i] = &bytes.Buffer{}
	}
	return b
}

// 转换为字节
func toBytes(in []*bytes.Buffer) [][]byte {
	b := make([][]byte, len(in))
	for i := range in {
		b[i] = in[i].Bytes()
	}
	return b
}

// 测试流式重建
func TestStreamReconstruct(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	r, err := NewStream(10, 3, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	rand.Seed(0)
	shards := randomBytes(10, perShard)
	parb := emptyBuffers(3)

	err = r.Encode(toReaders(toBuffers(shards)), toWriters(parb))
	if err != nil {
		t.Fatal(err)
	}

	parity := toBytes(parb)

	all := append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)
	fill := make([]io.Writer, 13)

	// Reconstruct with all shards present, all fill nil
	err = r.Reconstruct(all, fill)
	if err != nil {
		t.Fatal(err)
	}

	all = append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)

	// Reconstruct with 10 shards present, asking for all shards to be reconstructed
	all[0] = nil
	fill[0] = emptyBuffers(1)[0]
	all[7] = nil
	fill[7] = emptyBuffers(1)[0]
	all[11] = nil
	fill[11] = emptyBuffers(1)[0]

	err = r.Reconstruct(all, fill)
	if err != nil {
		t.Fatal(err)
	}

	shards[0] = fill[0].(*bytes.Buffer).Bytes()
	shards[7] = fill[7].(*bytes.Buffer).Bytes()
	parity[1] = fill[11].(*bytes.Buffer).Bytes()

	all = append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)

	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}

	all = append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)

	// Reconstruct with 10 shards present, asking for just data shards to be reconstructed
	all[0] = nil
	fill[0] = emptyBuffers(1)[0]
	all[7] = nil
	fill[7] = emptyBuffers(1)[0]
	all[11] = nil
	fill[11] = nil

	err = r.Reconstruct(all, fill)
	if err != nil {
		t.Fatal(err)
	}

	if fill[11] != nil {
		t.Fatal("Unexpected parity block reconstructed")
	}

	all = append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)

	// Reconstruct with 9 shards present (should fail)
	all[0] = nil
	fill[0] = emptyBuffers(1)[0]
	all[4] = nil
	fill[4] = emptyBuffers(1)[0]
	all[7] = nil
	fill[7] = emptyBuffers(1)[0]
	all[11] = nil
	fill[11] = emptyBuffers(1)[0]

	err = r.Reconstruct(all, fill)
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}

	err = r.Reconstruct(toReaders(emptyBuffers(3)), toWriters(emptyBuffers(3)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	err = r.Reconstruct(toReaders(emptyBuffers(13)), toWriters(emptyBuffers(3)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	err = r.Reconstruct(toReaders(emptyBuffers(13)), toWriters(emptyBuffers(13)))
	if err != ErrReconstructMismatch {
		t.Errorf("expected %v, got %v", ErrReconstructMismatch, err)
	}
	err = r.Reconstruct(toReaders(emptyBuffers(13)), nilWriters(13))
	if err != ErrShardNoData {
		t.Errorf("expected %v, got %v", ErrShardNoData, err)
	}
}

// 测试流式验证
func TestStreamVerify(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}
	r, err := NewStream(10, 4, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	shards := randomBytes(10, perShard)
	parb := emptyBuffers(4)

	err = r.Encode(toReaders(toBuffers(shards)), toWriters(parb))
	if err != nil {
		t.Fatal(err)
	}
	parity := toBytes(parb)
	all := append(toReaders(toBuffers(shards)), toReaders(parb)...)
	ok, err := r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Verification failed")
	}

	// Flip bits in a random byte
	parity[0][len(parity[0])-20000] = parity[0][len(parity[0])-20000] ^ 0xff

	all = append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)
	ok, err = r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("Verification did not fail")
	}
	// Re-encode
	err = r.Encode(toReaders(toBuffers(shards)), toWriters(parb))
	if err != nil {
		t.Fatal(err)
	}
	// Fill a data segment with random data
	shards[0][len(shards[0])-30000] = shards[0][len(shards[0])-30000] ^ 0xff
	all = append(toReaders(toBuffers(shards)), toReaders(parb)...)
	ok, err = r.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("Verification did not fail")
	}

	_, err = r.Verify(toReaders(emptyBuffers(10)))
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}

	_, err = r.Verify(toReaders(emptyBuffers(14)))
	if err != ErrShardNoData {
		t.Errorf("expected %v, got %v", ErrShardNoData, err)
	}
}

// 测试流式编码一个
func TestStreamOneEncode(t *testing.T) {
	codec, err := NewStream(5, 5, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	shards := [][]byte{
		{0, 1},
		{4, 5},
		{2, 3},
		{6, 7},
		{8, 9},
	}
	parb := emptyBuffers(5)
	codec.Encode(toReaders(toBuffers(shards)), toWriters(parb))
	parity := toBytes(parb)
	if parity[0][0] != 12 || parity[0][1] != 13 {
		t.Fatal("shard 5 mismatch")
	}
	if parity[1][0] != 10 || parity[1][1] != 11 {
		t.Fatal("shard 6 mismatch")
	}
	if parity[2][0] != 14 || parity[2][1] != 15 {
		t.Fatal("shard 7 mismatch")
	}
	if parity[3][0] != 90 || parity[3][1] != 91 {
		t.Fatal("shard 8 mismatch")
	}
	if parity[4][0] != 94 || parity[4][1] != 95 {
		t.Fatal("shard 9 mismatch")
	}

	all := append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)
	ok, err := codec.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("did not verify")
	}
	shards[3][0]++
	all = append(toReaders(toBuffers(shards)), toReaders(toBuffers(parity))...)
	ok, err = codec.Verify(all)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("verify did not fail as expected")
	}

}

// 基准流式编码
func benchmarkStreamEncode(b *testing.B, dataShards, parityShards, shardSize int) {
	r, err := NewStream(dataShards, parityShards, testOptions(WithAutoGoroutines(shardSize))...)
	if err != nil {
		b.Fatal(err)
	}
	shards := make([][]byte, dataShards)
	for s := range shards {
		shards[s] = make([]byte, shardSize)
	}

	rand.Seed(0)
	for s := 0; s < dataShards; s++ {
		fillRandom(shards[s])
	}

	b.SetBytes(int64(shardSize * dataShards))
	b.ResetTimer()
	out := make([]io.Writer, parityShards)
	for i := range out {
		out[i] = io.Discard
	}
	for i := 0; i < b.N; i++ {
		err = r.Encode(toReaders(toBuffers(shards)), out)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// 基准流式编码10个数据分片和2个奇偶校验分片，每个分片10000字节
func BenchmarkStreamEncode10x2x10000(b *testing.B) {
	benchmarkStreamEncode(b, 10, 2, 10000)
}

// 基准流式编码100个数据分片和20个奇偶校验分片，每个分片10000字节
func BenchmarkStreamEncode100x20x10000(b *testing.B) {
	benchmarkStreamEncode(b, 100, 20, 10000)
}

// 基准流式编码17个数据分片和3个奇偶校验分片，每个分片1MB
func BenchmarkStreamEncode17x3x1M(b *testing.B) {
	benchmarkStreamEncode(b, 17, 3, 1024*1024)
}

// 基准流式编码10个数据分片和4个奇偶校验分片，每个分片16MB
func BenchmarkStreamEncode10x4x16M(b *testing.B) {
	benchmarkStreamEncode(b, 10, 4, 16*1024*1024)
}

// 基准流式编码5个数据分片和2个奇偶校验分片，每个分片1MB
func BenchmarkStreamEncode5x2x1M(b *testing.B) {
	benchmarkStreamEncode(b, 5, 2, 1024*1024)
}

// 基准流式编码1个数据分片和2个奇偶校验分片，每个分片1MB
func BenchmarkStreamEncode10x2x1M(b *testing.B) {
	benchmarkStreamEncode(b, 10, 2, 1024*1024)
}

// 基准流式编码10个数据分片和4个奇偶校验分片，每个分片1MB
func BenchmarkStreamEncode10x4x1M(b *testing.B) {
	benchmarkStreamEncode(b, 10, 4, 1024*1024)
}

// 基准流式编码50个数据分片和20个奇偶校验分片，每个分片1MB
func BenchmarkStreamEncode50x20x1M(b *testing.B) {
	benchmarkStreamEncode(b, 50, 20, 1024*1024)
}

// 基准流式编码17个数据分片和3个奇偶校验分片，每个分片16MB
func BenchmarkStreamEncode17x3x16M(b *testing.B) {
	benchmarkStreamEncode(b, 17, 3, 16*1024*1024)
}

// 基准流式验证
func benchmarkStreamVerify(b *testing.B, dataShards, parityShards, shardSize int) {
	r, err := NewStream(dataShards, parityShards, testOptions(WithAutoGoroutines(shardSize))...)
	if err != nil {
		b.Fatal(err)
	}
	shards := make([][]byte, parityShards+dataShards)
	for s := range shards {
		shards[s] = make([]byte, shardSize)
	}

	rand.Seed(0)
	for s := 0; s < dataShards; s++ {
		fillRandom(shards[s])
	}
	err = r.Encode(toReaders(toBuffers(shards[:dataShards])), toWriters(toBuffers(shards[dataShards:])))
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(shardSize * dataShards))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = r.Verify(toReaders(toBuffers(shards)))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// 基准流式验证10个数据分片和2个奇偶校验分片，每个分片10000字节
func BenchmarkStreamVerify10x2x10000(b *testing.B) {
	benchmarkStreamVerify(b, 10, 2, 10000)
}

// 基准流式验证50个数据分片和5个奇偶校验分片，每个分片100000字节
func BenchmarkStreamVerify50x5x50000(b *testing.B) {
	benchmarkStreamVerify(b, 50, 5, 100000)
}

// 基准流式验证10个数据分片和2个奇偶校验分片，每个分片1MB
func BenchmarkStreamVerify10x2x1M(b *testing.B) {
	benchmarkStreamVerify(b, 10, 2, 1024*1024)
}

// 基准流式验证5个数据分片和2个奇偶校验分片，每个分片1MB
func BenchmarkStreamVerify5x2x1M(b *testing.B) {
	benchmarkStreamVerify(b, 5, 2, 1024*1024)
}

// 基准流式验证10个数据分片和4个奇偶校验分片，每个分片1MB
func BenchmarkStreamVerify10x4x1M(b *testing.B) {
	benchmarkStreamVerify(b, 10, 4, 1024*1024)
}

// 基准流式验证50个数据分片和20个奇偶校验分片，每个分片1MB
func BenchmarkStreamVerify50x20x1M(b *testing.B) {
	benchmarkStreamVerify(b, 50, 20, 1024*1024)
}

// 基准流式验证10个数据分片和4个奇偶校验分片，每个分片16MB
func BenchmarkStreamVerify10x4x16M(b *testing.B) {
	benchmarkStreamVerify(b, 10, 4, 16*1024*1024)
}

// 测试流式分割和连接
func TestStreamSplitJoin(t *testing.T) {
	var data = make([]byte, 250000)
	rand.Seed(0)
	fillRandom(data)

	enc, _ := NewStream(5, 3, testOptions()...)
	split := emptyBuffers(5)
	err := enc.Split(bytes.NewBuffer(data), toWriters(split), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	splits := toBytes(split)
	expect := len(data) / 5
	// Beware, if changing data size
	if split[0].Len() != expect {
		t.Errorf("unexpected size. expected %d, got %d", expect, split[0].Len())
	}

	err = enc.Split(bytes.NewBuffer([]byte{}), toWriters(emptyBuffers(3)), 0)
	if err != ErrShortData {
		t.Errorf("expected %v, got %v", ErrShortData, err)
	}

	buf := new(bytes.Buffer)
	err = enc.Join(buf, toReaders(toBuffers(splits)), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	joined := buf.Bytes()
	if !bytes.Equal(joined, data) {
		t.Fatal("recovered data does match original", joined[:8], data[:8], "... lengths:", len(joined), len(data))
	}

	err = enc.Join(buf, toReaders(emptyBuffers(2)), 0)
	if err != ErrTooFewShards {
		t.Errorf("expected %v, got %v", ErrTooFewShards, err)
	}
	bufs := toReaders(emptyBuffers(5))
	bufs[2] = nil
	err = enc.Join(buf, bufs, 0)
	if se, ok := err.(StreamReadError); ok {
		if se.Err != ErrShardNoData {
			t.Errorf("expected %v, got %v", ErrShardNoData, se.Err)
		}
		if se.Stream != 2 {
			t.Errorf("Expected error on stream 2, got %d", se.Stream)
		}
	} else {
		t.Errorf("expected error type %T, got %T", StreamReadError{}, err)
	}

	err = enc.Join(buf, toReaders(toBuffers(splits)), int64(len(data)+1))
	if err != ErrShortData {
		t.Errorf("expected %v, got %v", ErrShortData, err)
	}
}

// 测试NewStream
func TestNewStream(t *testing.T) {
	tests := []struct {
		data, parity int
		err          error
	}{
		{127, 127, nil},
		{1, 0, nil},
		{256, 256, ErrMaxShardNum},

		{0, 1, ErrInvShardNum},
		{1, -1, ErrInvShardNum},
		{257, 1, ErrMaxShardNum},

		// overflow causes r.Shards to be negative
		{256, int(^uint(0) >> 1), errInvalidRowSize},
	}
	for _, test := range tests {
		_, err := NewStream(test.data, test.parity, testOptions()...)
		if err != test.err {
			t.Errorf("New(%v, %v): expected %v, got %v", test.data, test.parity, test.err, err)
		}
	}
}
