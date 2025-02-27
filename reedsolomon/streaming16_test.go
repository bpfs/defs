/**
 * Unit tests for Reed-Solomon Streaming GF(2^16) API
 *
 * Copyright 2024
 */

package reedsolomon

import (
	"bytes"
	"io"
	"testing"
)

// 测试流式编码
func TestStream16Encoding(t *testing.T) {
	// 使用非对齐的随机大小
	perShard := (10 << 20) + 1 // 10MB + 1 byte per shard
	if testing.Short() {
		perShard = 50001 // 使用非对齐大小
	}
	t.Logf("开始测试,每个分片大小: %d", perShard)

	r, err := NewStream16(10, 3)
	if err != nil {
		t.Fatal(err)
	}

	// 创建非对齐的输入数据
	inputData := make([][]byte, 10)
	for i := range inputData {
		inputData[i] = make([]byte, perShard)
		fillRandom(inputData[i])
	}

	// 添加调试信息
	t.Logf("输入数据大小:")
	for i, data := range inputData {
		t.Logf("分片 %d 大小: %d", i, len(data))
	}

	// 创建输入readers并确保位置正确
	inputs := make([]io.Reader, len(inputData))
	for i := range inputs {
		// 先创建reader
		br := bytes.NewReader(inputData[i])
		// 确保位置在开始处
		br.Seek(0, 0)
		inputs[i] = br
		// 打印状态
		t.Logf("输入reader %d: 大小=%d, 位置=%d", i, br.Size(), br.Len())
	}

	// 创建输出writers
	outputs := make([]io.Writer, 3)
	parity := make([]*bytes.Buffer, 3)
	for i := range outputs {
		parity[i] = &bytes.Buffer{}
		outputs[i] = parity[i]
	}

	// 添加调试信息
	t.Logf("奇偶校验大小:")
	for i, p := range parity {
		t.Logf("奇偶校验 %d 大小: %d", i, p.Len())
	}

	// 编码前再次确认所有reader位置
	for i := range inputs {
		if br, ok := inputs[i].(*bytes.Reader); ok {
			if br.Len() != int(br.Size()) {
				br.Seek(0, 0)
				t.Logf("重置reader %d 位置", i)
			}
		}
	}

	err = r.Encode(inputs, outputs)
	if err != nil {
		t.Fatal(err)
	}

	// 验证
	allShards := make([]io.Reader, 13)
	for i := range inputData {
		allShards[i] = bytes.NewReader(inputData[i])
	}
	for i := range parity {
		allShards[i+10] = bytes.NewReader(parity[i].Bytes())
	}

	// 打印所有分片大小
	t.Logf("验证时所有分片大小:")
	for i, shard := range allShards {
		if br, ok := shard.(*bytes.Reader); ok {
			t.Logf("分片 %d 大小: %d", i, br.Size())
		}
	}

	verifyReaders := make([]io.Reader, len(inputData)+len(parity))
	for i := range inputData {
		verifyReaders[i] = bytes.NewReader(inputData[i])
	}
	for i := range parity {
		verifyReaders[i+len(inputData)] = bytes.NewReader(parity[i].Bytes())
	}

	ok, err := r.Verify(verifyReaders)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("验证失败")
	}
}

// 测试流式重建
func TestStream16Reconstruction(t *testing.T) {
	perShard := 50001 // 使用非对齐的大小

	r, err := NewStream16(8, 4)
	if err != nil {
		t.Fatal(err)
	}

	// 创建输入数据，不需要预先对齐
	inputData := make([][]byte, 8)
	for i := range inputData {
		inputData[i] = make([]byte, perShard)
		fillRandom(inputData[i])
	}

	// 创建输入readers和输出writers
	inputs := make([]io.Reader, len(inputData))
	for i := range inputs {
		inputs[i] = bytes.NewReader(inputData[i])
	}

	outputs := make([]io.Writer, 4)
	parity := make([]*bytes.Buffer, 4)
	for i := range outputs {
		parity[i] = &bytes.Buffer{}
		outputs[i] = parity[i]
	}

	// 编码
	err = r.Encode(inputs, outputs)
	if err != nil {
		t.Fatal(err)
	}

	// 验证所有组合的重建
	for i := 0; i < len(inputData); i++ {
		// 保存原始数据
		origData := make([]byte, perShard)
		copy(origData, inputData[i])

		allShards := make([]io.Reader, 12)
		for j := range inputData {
			if j == i {
				allShards[j] = nil
			} else {
				allShards[j] = bytes.NewReader(inputData[j])
			}
		}
		for j := range parity {
			allShards[j+8] = bytes.NewReader(parity[j].Bytes())
		}

		// 重建
		reconstructed := &bytes.Buffer{}
		reconstructOutputs := make([]io.Writer, 12)
		reconstructOutputs[i] = reconstructed

		err = r.Reconstruct(allShards, reconstructOutputs)
		if err != nil {
			t.Fatal(err)
		}

		// 验证重建的数据
		if !bytes.Equal(reconstructed.Bytes(), origData) {
			t.Fatal("重建的数据不匹配")
		}
	}
}

// 测试流式分割
func TestStream16SplitAndAlign(t *testing.T) {
	var (
		dataShards   = 5
		parityShards = 3
		perShard     = 50000
	)

	// 创建编码器
	enc, err := NewStream16(dataShards, parityShards)
	if err != nil {
		t.Fatal(err)
	}

	// 创建测试数据
	data := make([]byte, perShard*dataShards)
	fillRandom(data)

	// 确保16位对齐
	if len(data)%2 != 0 {
		data = append(data, 0)
	}

	// 分片输出
	outputs := make([]io.Writer, dataShards)
	for i := range outputs {
		outputs[i] = &bytes.Buffer{}
	}

	// 分割数据
	err = enc.Split(bytes.NewReader(data), outputs, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	// 验证分片大小
	for i, w := range outputs {
		buf := w.(*bytes.Buffer)
		if buf.Len()%2 != 0 {
			t.Errorf("分片 %d 不是16位对齐的", i)
		}
	}

	// 测试错误情况
	// 1. 测试空数据
	err = enc.Split(bytes.NewBuffer([]byte{}), outputs, 0)
	if err != ErrShortData {
		t.Errorf("期望错误 %v, 得到 %v", ErrShortData, err)
	}

	// 2. 测试分片数量不足
	err = enc.Split(bytes.NewReader(data), outputs[:2], int64(len(data)))
	if err != ErrTooFewShards {
		t.Errorf("期望错误 %v, 得到 %v", ErrTooFewShards, err)
	}

	// 3. 测试数据大小不匹配
	err = enc.Split(bytes.NewReader(data), outputs, int64(len(data)+1))
	if err != ErrShortData {
		t.Errorf("期望错误 %v, 得到 %v", ErrShortData, err)
	}
}

// 测试流式并发
func TestStream16Concurrent(t *testing.T) {
	perShard := 10 << 20
	if testing.Short() {
		perShard = 50000
	}

	// 使用 testOptions() 并添加并发选项
	opts := append([]Option{WithConcurrentStreamReads(true), WithConcurrentStreamWrites(true)}, testOptions()...)
	r, err := NewStream16(10, 3, opts...)
	if err != nil {
		t.Fatal(err)
	}

	// 创建输入数据
	input := make([][]byte, 10)
	for i := range input {
		input[i] = make([]byte, perShard)
		// 确保数据不全为零
		fillRandom(input[i])
		// 确保数据是2字节对齐的
		if len(input[i])%2 != 0 {
			input[i] = append(input[i], 0)
		}
		t.Logf("输入数据 %d 大小: %d", i, len(input[i]))
	}

	// 创建输入readers并确保位置正确
	readers := make([]io.Reader, len(input))
	for i := range input {
		// 先创建reader
		br := bytes.NewReader(input[i])
		// 确保位置在开始处
		br.Seek(0, 0)
		readers[i] = br
		// 打印状态
		t.Logf("输入reader %d: 大小=%d, 位置=%d", i, br.Size(), br.Len())
	}

	// 在编码前打印输入数据的状态
	t.Log("编码前状态检查:")
	for i, reader := range readers {
		if br, ok := reader.(*bytes.Reader); ok {
			br.Seek(0, 0)
			t.Logf("输入reader %d: 大小=%d, 位置=%d", i, br.Size(), br.Len())
		}
	}

	// 创建输出缓冲区
	par := make([]*bytes.Buffer, 3)
	writers := make([]io.Writer, 3)
	for i := range par {
		par[i] = &bytes.Buffer{}
		writers[i] = par[i]
	}

	// 编码前再次确认所有reader位置
	for i := range readers {
		if br, ok := readers[i].(*bytes.Reader); ok {
			if br.Len() != int(br.Size()) {
				br.Seek(0, 0)
				t.Logf("重置reader %d 位置", i)
			}
		}
	}

	// 编码
	err = r.Encode(readers, writers)
	if err != nil {
		t.Logf("编码失败: %v", err)
		t.Fatal(err)
	}

	// 编码后立即检查输出
	t.Log("编码后状态检查:")
	for i, buf := range par {
		t.Logf("奇偶校验分片 %d: 大小=%d", i, buf.Len())
	}

	// 首先创建verifyReaders
	verifyReaders := make([]io.Reader, len(input)+len(par))
	for i := range input {
		verifyReaders[i] = bytes.NewReader(input[i])
	}
	for i := range par {
		verifyReaders[i+len(input)] = bytes.NewReader(par[i].Bytes())
	}

	// 然后再进行验证前检查
	t.Log("验证前状态检查:")
	for i, reader := range verifyReaders {
		if br, ok := reader.(*bytes.Reader); ok {
			t.Logf("验证reader %d: 大小=%d, 位置=%d", i, br.Size(), br.Len())
		}
	}

	// 最后进行验证
	ok, err := r.Verify(verifyReaders)
	if err != nil {
		t.Logf("验证失败: %v", err)
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("并发编码验证失败")
	}

	// 在验证完成后，添加重建测试的准备代码
	// 模拟丢失两个数据分片和一个校验分片
	rebuiltInput := make([][]byte, len(input))
	copy(rebuiltInput, input)
	rebuiltInput[0] = nil // 删除第一个数据分片
	rebuiltInput[4] = nil // 删除第五个数据分片

	rebuiltPar := make([]*bytes.Buffer, len(par))
	for i := range par {
		rebuiltPar[i] = bytes.NewBuffer(par[i].Bytes())
	}
	rebuiltPar[1] = nil // 删除第二个校验分片

	// 准备重建
	allShards := make([]io.Reader, 13)
	for i := range rebuiltInput {
		if rebuiltInput[i] != nil {
			allShards[i] = bytes.NewReader(rebuiltInput[i])
		}
	}
	for i := range rebuiltPar {
		if rebuiltPar[i] != nil {
			allShards[i+10] = bytes.NewReader(rebuiltPar[i].Bytes())
		}
	}

	// 重建前检查
	t.Log("重建前状态检查:")
	for i, shard := range allShards {
		if shard == nil {
			t.Logf("分片 %d: nil", i)
		} else if br, ok := shard.(*bytes.Reader); ok {
			t.Logf("分片 %d: 大小=%d, 位置=%d", i, br.Size(), br.Len())
		}
	}

	// 重建
	reconstructed := make([]io.Writer, 13)
	reconstructed[0] = &bytes.Buffer{}  // 重建第一个数据分片
	reconstructed[4] = &bytes.Buffer{}  // 重建第五个数据分片
	reconstructed[11] = &bytes.Buffer{} // 重建第二个校验分片

	// 重建
	err = r.Reconstruct(allShards, reconstructed)
	if err != nil {
		t.Logf("重建失败: %v", err)
		t.Fatal(err)
	}

	// 重建后检查
	t.Log("重建后状态检查:")
	for i, writer := range reconstructed {
		if writer == nil {
			continue
		}
		if buf, ok := writer.(*bytes.Buffer); ok {
			t.Logf("重建分片 %d: 大小=%d", i, buf.Len())
		}
	}

	// 验证重建的数据
	if !bytes.Equal(reconstructed[0].(*bytes.Buffer).Bytes(), input[0]) {
		t.Log("第一个分片重建结果:")
		t.Logf("期望大小: %d, 实际大小: %d", len(input[0]), reconstructed[0].(*bytes.Buffer).Len())
		t.Error("重建的第一个分片与原始数据不匹配")
	}
	if !bytes.Equal(reconstructed[4].(*bytes.Buffer).Bytes(), input[4]) {
		t.Log("第五个分片重建结果:")
		t.Logf("期望大小: %d, 实际大小: %d", len(input[4]), reconstructed[4].(*bytes.Buffer).Len())
		t.Error("重建的第五个分片与原始数据不匹配")
	}

	// 测试错误情况
	// 1. 测试分片数量不足
	err = r.Encode(toReaders(emptyBuffers(1)), toWriters(emptyBuffers(1)))
	if err != ErrTooFewShards {
		t.Errorf("期望错误 %v, 得到 %v", ErrTooFewShards, err)
	}

	// 2. 测试分片大小不一致
	badShards := make([]*bytes.Buffer, 10)
	for i := range badShards {
		badShards[i] = &bytes.Buffer{}
		if i == 0 {
			badShards[i].Write(make([]byte, 100))
		} else {
			badShards[i].Write(make([]byte, 200))
		}
	}
	err = r.Encode(toReaders(badShards), toWriters(emptyBuffers(3)))
	if err != ErrShardSize {
		t.Errorf("期望错误 %v, 得到 %v", ErrShardSize, err)
	}
}

// 测试流式无效输入
func TestStream16InvalidInput(t *testing.T) {
	tests := []struct {
		data, parity int
		err          error
	}{
		{127, 127, nil},
		{1, 0, ErrInvShardNum},
		{0, 1, ErrInvShardNum},
		{1, -1, ErrInvShardNum},
	}

	for i, test := range tests {
		_, err := NewStream16(test.data, test.parity)
		if err != test.err {
			t.Errorf("测试 %d: 期望错误 %v, 得到 %v", i+1, test.err, err)
		}
	}
}

// 测试流式编码最小奇偶校验
func TestStream16ZeroParity(t *testing.T) {
	perShard := (10 << 20) + 3 // 使用非对齐大小
	if testing.Short() {
		perShard = 50003
	}
	// 修改为1个奇偶校验分片
	r, err := NewStream16(10, 1)
	if err != nil {
		t.Fatal(err)
	}

	// 创建非对齐的输入数据
	inputData := make([][]byte, 10)
	for i := range inputData {
		inputData[i] = make([]byte, perShard)
		fillRandom(inputData[i])
	}

	// 创建1个奇偶校验分片的输出
	outputs := make([]io.Writer, 1)
	outputs[0] = &bytes.Buffer{}

	// 编码
	err = r.Encode(toReaders(toBuffers(inputData)), outputs)
	if err != nil {
		t.Fatal(err)
	}

	// 验证
	verifyReaders := make([]io.Reader, len(inputData)+1) // +1 for parity
	for i := range inputData {
		verifyReaders[i] = bytes.NewReader(inputData[i])
	}
	// 添加奇偶校验分片
	verifyReaders[len(inputData)] = bytes.NewReader(outputs[0].(*bytes.Buffer).Bytes())

	ok, err := r.Verify(verifyReaders)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("验证失败")
	}
}

// 测试流式编码错误情况
func TestStream16EncodingErrors(t *testing.T) {
	r, err := NewStream16(10, 3)
	if err != nil {
		t.Fatal(err)
	}

	// 测试分片数量不足
	err = r.Encode(toReaders(emptyBuffers(1)), toWriters(emptyBuffers(1)))
	if err != ErrTooFewShards {
		t.Errorf("期望错误 %v, 得到 %v", ErrTooFewShards, err)
	}

	// 测试分片大小不一致
	badShards := emptyBuffers(10)
	badShards[0] = bytes.NewBuffer(make([]byte, 100))
	badShards[1] = bytes.NewBuffer(make([]byte, 200))
	err = r.Encode(toReaders(badShards), toWriters(emptyBuffers(3)))
	if err != ErrShardSize {
		t.Errorf("期望错误 %v, 得到 %v", ErrShardSize, err)
	}
}
