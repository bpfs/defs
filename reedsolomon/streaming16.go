/**
 * Reed-Solomon Coding over 16-bit values.
 *
 * Copyright 2024
 */

package reedsolomon

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

// StreamEncoder16 是一个基于GF(2^16)的Reed-Solomon编码器接口
type StreamEncoder16 interface {
	// Encode 为一组数据分片生成奇偶校验分片
	Encode(inputs []io.Reader, outputs []io.Writer) error

	// Verify 验证奇偶校验分片的正确性
	Verify(shards []io.Reader) (bool, error)

	// Reconstruct 重建丢失的分片
	Reconstruct(inputs []io.Reader, outputs []io.Writer) error

	// Split 将输入流分割成多个分片
	Split(data io.Reader, dst []io.Writer, size int64) error
}

// rsStream16 实现了 StreamEncoder16 接口
type rsStream16 struct {
	r *leopardFF16 // 使用已有的 leopardFF16 实现

	dataShards   int
	parityShards int
	totalShards  int

	blockSize int // 处理块大小

	blockPool sync.Pool
	o         options

	// 并发控制
	concurrentReads  bool
	concurrentWrites bool
}

// NewStream16 创建一个新的GF(2^16) Reed-Solomon流式编码器
// 参数:
// - dataShards: 数据分片数量
// - parityShards: 奇偶校验分片数量
// - opts: 可选参数
// 返回:
// - StreamEncoder16: 新的流式编码器
// - error: 如果发生错误,返回错误信息
func NewStream16(dataShards, parityShards int, opts ...Option) (StreamEncoder16, error) {
	// 参数验证
	if dataShards <= 0 {
		return nil, ErrInvShardNum
	}
	if parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	// 创建流式编码器
	r := &rsStream16{
		dataShards:       dataShards,
		parityShards:     parityShards,
		totalShards:      dataShards + parityShards,
		blockSize:        4 * 1024 * 1024, // 4MB 块大小
		o:                defaultOptions,
		concurrentReads:  false,
		concurrentWrites: false,
	}

	// 确保块大小是16位对齐的
	if r.blockSize%2 != 0 {
		r.blockSize++
	}

	for _, opt := range opts {
		opt(&r.o)
	}

	// 设置块大小
	if r.o.streamBS > 0 {
		r.blockSize = r.o.streamBS
	}

	// 设置并发
	r.concurrentReads = r.o.concReads
	r.concurrentWrites = r.o.concWrites

	// 创建基础编码器
	enc, err := newFF16(dataShards, parityShards, r.o)
	if err != nil {
		logger.Errorf("创建流式编码器失败: %v", err)
		return nil, err
	}
	r.r = enc

	// 初始化内存池
	r.blockPool.New = func() interface{} {
		return r.createSlice()
	}

	return r, nil
}

// createSlice 创建一个新的分片缓冲区
// 返回:
// - [][]byte: 一个包含totalShards个分片缓冲区的切片,每个分片的大小为blockSize
func (r *rsStream16) createSlice() [][]byte {
	// 确保块大小是2字节对齐的
	if r.blockSize%2 != 0 {
		r.blockSize++
	}
	return AllocAligned(r.totalShards, r.blockSize)
}

// Encode 实现
// 参数:
// - inputs: 一个包含dataShards个输入流的切片
// - outputs: 一个包含parityShards个输出流的切片
// 返回:
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) Encode(inputs []io.Reader, outputs []io.Writer) error {
	if len(inputs) != r.dataShards {
		logger.Errorf("输入分片数量不正确: %d != %d", len(inputs), r.dataShards)
		return ErrTooFewShards
	}
	if len(outputs) != r.parityShards {
		logger.Errorf("输出分片数量不正确: %d != %d", len(outputs), r.parityShards)
		return ErrTooFewShards
	}

	// 获取缓冲区
	shards := r.blockPool.Get().([][]byte)
	defer r.blockPool.Put(shards)

	// 初始化所有分片
	for i := range shards {
		shards[i] = shards[i][:r.blockSize]
	}

	for {
		// 读取输入数据
		var size int
		var err error
		if r.concurrentReads {
			size, err = r.readInputsConcurrent(shards[:r.dataShards], inputs)
		} else {
			size, err = r.readInputs(inputs, shards[:r.dataShards])
		}

		if err == io.EOF {
			return nil
		}
		if err != nil {
			logger.Errorf("读取输入数据失败: %v", err)
			return err
		}

		// 验证是否有有效数据
		hasData := false
		for i := 0; i < r.dataShards; i++ {
			if len(shards[i]) > 0 {
				hasData = true
				break
			}
		}
		if !hasData {
			logger.Errorf("没有有效的分片数据")
			return ErrShardNoData
		}

		// 计算对齐大小并设置所有分片
		alignedSize := ((size + 63) / 64) * 64
		for i := range shards {
			if len(shards[i]) < alignedSize {
				newShard := make([]byte, alignedSize)
				copy(newShard, shards[i])
				shards[i] = newShard
			}
			shards[i] = shards[i][:alignedSize]
		}

		// 编码
		if err := r.r.Encode(shards); err != nil {
			logger.Errorf("编码失败: %v", err)
			return err
		}

		// 写入奇偶校验数据
		if r.concurrentWrites {
			err = r.writeOutputsConcurrent(outputs, shards[r.dataShards:], size)
		} else {
			err = r.writeOutputs(outputs, shards[r.dataShards:], size)
		}
		if err != nil {
			logger.Errorf("写入奇偶校验数据失败: %v", err)
			return err
		}
	}
}

// readInputs 从输入流读取数据
// 参数:
// - readers: 一个包含dataShards个输入流的切片
// - dst: 一个包含dataShards个分片缓冲区的切片
// 返回:
// - int: 读取的字节数
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) readInputs(readers []io.Reader, dst [][]byte) (int, error) {
	var size int
	for i, reader := range readers {
		if reader == nil {
			dst[i] = dst[i][:0]
			continue
		}

		// 限制读取大小不超过块大小
		n, err := io.ReadFull(reader, dst[i][:r.blockSize])
		switch err {
		case io.EOF, io.ErrUnexpectedEOF:
			if i == 0 {
				size = n
			} else if n != size {
				return 0, ErrShardSize
			}
			dst[i] = dst[i][:n]
		case nil:
			if i == 0 {
				size = n
			}
			if n != size {
				return 0, ErrShardSize
			}
			dst[i] = dst[i][:n]
		default:
			return 0, StreamReadError{Err: err, Stream: i}
		}
	}

	// 确保64字节对齐
	if size%64 != 0 {
		paddedSize := ((size + 63) / 64) * 64
		for i := range dst {
			if len(dst[i]) == size {
				// 扩展切片到对齐大小
				dst[i] = dst[i][:paddedSize]
				// 用0填充未对齐部分
				for j := size; j < paddedSize; j++ {
					dst[i][j] = 0
				}
			}
		}
		size = paddedSize
	}

	if size == 0 {
		return 0, io.EOF
	}
	return size, nil
}

// writeOutputs 写入输出流
// 参数:
// - writers: 一个包含parityShards个输出流的切片
// - src: 一个包含parityShards个分片缓冲区的切片
// - size: 写入的字节数
// 返回:
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) writeOutputs(writers []io.Writer, src [][]byte, size int) error {
	for i, writer := range writers {
		if writer == nil {
			continue
		}

		n, err := writer.Write(src[i][:size])
		if err != nil {
			logger.Errorf("写入奇偶校验数据失败: %v", err)
			return StreamWriteError{Err: err, Stream: i}
		}
		if n != size {
			logger.Errorf("写入奇偶校验数据失败: %v", io.ErrShortWrite)
			return StreamWriteError{Err: io.ErrShortWrite, Stream: i}
		}
	}
	return nil
}

// Verify 验证奇偶校验分片的正确性
// 参数:
// - shards: 一个包含totalShards个输入流的切片
// 返回:
// - bool: 如果验证成功,返回true,否则返回false
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) Verify(shards []io.Reader) (bool, error) {
	if len(shards) != r.totalShards {
		logger.Errorf("验证奇偶校验分片失败: 分片数量不正确: %d != %d", len(shards), r.totalShards)
		return false, ErrTooFewShards
	}

	all := r.blockPool.Get().([][]byte)
	defer r.blockPool.Put(all)

	read := 0
	for {
		// 读取所有分片数据
		size := 0
		for i, shard := range shards {
			if shard == nil {
				all[i] = all[i][:0]
				continue
			}

			// 限制读取大小不超过块大小
			n, err := io.ReadFull(shard, all[i][:r.blockSize])
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				if i == 0 {
					size = n
				} else {
					// 对于奇偶校验分片，允许大小为对齐后的大小
					alignedSize := ((size + 63) / 64) * 64
					if i >= r.dataShards {
						if n != alignedSize {
							return false, ErrShardSize
						}
					} else if n != size {
						return false, ErrShardSize
					}
				}
				all[i] = all[i][:n]
			case nil:
				if i == 0 {
					size = n
				} else {
					// 同上，处理非EOF情况
					alignedSize := ((size + 63) / 64) * 64
					if i >= r.dataShards {
						if n != alignedSize {
							return false, ErrShardSize
						}
					} else if n != size {
						return false, ErrShardSize
					}
				}
				all[i] = all[i][:n]
			default:
				return false, StreamReadError{Err: err, Stream: i}
			}
		}

		if size == 0 {
			if read == 0 {
				logger.Errorf("验证奇偶校验分片失败: 没有数据")
				return false, ErrShardNoData
			}
			return true, nil
		}

		// 确保数据分片也是64字节对齐的
		if size%64 != 0 {
			paddedSize := ((size + 63) / 64) * 64
			for i := 0; i < r.dataShards; i++ {
				if len(all[i]) == size {
					all[i] = all[i][:paddedSize]
					for j := size; j < paddedSize; j++ {
						all[i][j] = 0
					}
				}
			}
		}

		read += size
		ok, err := r.r.Verify(all)
		if !ok || err != nil {
			logger.Errorf("验证奇偶校验分片失败: %v", err)
			return ok, err
		}
	}
}

// Reconstruct 重建丢失的分片
// 参数:
// - inputs: 一个包含totalShards个输入流的切片
// - outputs: 一个包含totalShards个输出流的切片
// 返回:
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) Reconstruct(inputs []io.Reader, outputs []io.Writer) error {
	if len(inputs) != r.totalShards {
		logger.Errorf("重建丢失分片失败: 输入分片数量不正确: %d != %d", len(inputs), r.totalShards)
		return ErrTooFewShards
	}
	if len(outputs) != r.totalShards {
		logger.Errorf("重建丢失分片失败: 输出分片数量不正确: %d != %d", len(outputs), r.totalShards)
		return ErrTooFewShards
	}

	all := r.blockPool.Get().([][]byte)
	defer r.blockPool.Put(all)

	// 检查是否有冲突的输入输出
	reconDataOnly := true
	for i := range inputs {
		if inputs[i] != nil && outputs[i] != nil {
			logger.Errorf("重建丢失分片失败: 输入输出冲突: %d", i)
			return ErrReconstructMismatch
		}
		if i >= r.dataShards && outputs[i] != nil {
			reconDataOnly = false
		}
	}

	read := 0
	for {
		// 读取所有分片数据
		size := 0
		for i, shard := range inputs {
			if shard == nil {
				all[i] = all[i][:0]
				continue
			}

			// 限制读取大小不超过块大小
			n, err := io.ReadFull(shard, all[i][:r.blockSize])
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				if size == 0 {
					size = n
				} else {
					// 对于奇偶校验分片，允许大小为对齐后的大小
					alignedSize := ((size + 63) / 64) * 64
					if i >= r.dataShards {
						if n != alignedSize {
							return ErrShardSize
						}
					} else if n != size {
						return ErrShardSize
					}
				}
				all[i] = all[i][:n]
			case nil:
				if size == 0 {
					size = n
				} else {
					// 同上，处理非EOF情况
					alignedSize := ((size + 63) / 64) * 64
					if i >= r.dataShards {
						if n != alignedSize {
							return ErrShardSize
						}
					} else if n != size {
						return ErrShardSize
					}
				}
				all[i] = all[i][:n]
			default:
				return StreamReadError{Err: err, Stream: i}
			}
		}

		if size == 0 {
			if read == 0 {
				logger.Errorf("重建丢失分片失败: 没有数据")
				return ErrShardNoData
			}
			return nil
		}

		// 确保所有分片都是64字节对齐的
		paddedSize := size
		if size%64 != 0 {
			paddedSize = ((size + 63) / 64) * 64
			// 对齐数据分片
			for i := 0; i < r.dataShards; i++ {
				if len(all[i]) == size {
					all[i] = all[i][:paddedSize]
					for j := size; j < paddedSize; j++ {
						all[i][j] = 0
					}
				}
			}
			// 对齐奇偶校验分片
			for i := r.dataShards; i < r.totalShards; i++ {
				if len(all[i]) > 0 {
					all[i] = all[i][:paddedSize]
				}
			}
		}

		read += size

		// 重建
		var err error
		if reconDataOnly {
			err = r.r.ReconstructData(all)
		} else {
			err = r.r.Reconstruct(all)
		}
		if err != nil {
			logger.Errorf("重建丢失分片失败: %v", err)
			return err
		}

		// 写入重建的数据
		for i := range outputs {
			if outputs[i] == nil {
				continue
			}

			writeSize := size
			if i >= r.dataShards {
				writeSize = paddedSize // 奇偶校验分片写入对齐后的大小
			}

			n, err := outputs[i].Write(all[i][:writeSize])
			if err != nil {
				return StreamWriteError{Err: err, Stream: i}
			}
			if n != writeSize {
				return StreamWriteError{Err: io.ErrShortWrite, Stream: i}
			}
		}
	}
}

// Split 将输入流分割成多个分片
// 参数:
// - data: 一个包含输入数据的io.Reader
// - dst: 一个包含dataShards个输出流的切片
// - size: 输入数据的总大小
// 返回:
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) Split(data io.Reader, dst []io.Writer, size int64) error {
	if len(dst) != r.dataShards {
		logger.Errorf("分割失败: 输出分片数量不正确: %d != %d", len(dst), r.dataShards)
		return ErrTooFewShards
	}

	// 检查输入大小
	if size == 0 {
		return ErrShortData
	}

	// 确保大小是64字节对齐的
	alignedSize := size
	if size%64 != 0 {
		alignedSize = ((size + 63) / 64) * 64
	}

	// 计算每个分片的大小
	perShard := (alignedSize + int64(r.dataShards) - 1) / int64(r.dataShards)

	// 确保分片大小是64字节对齐的
	if perShard%64 != 0 {
		perShard = ((perShard + 63) / 64) * 64
	}

	// 创建读取缓冲区
	buf := make([]byte, perShard)
	totalRead := int64(0)

	for shardNum := range dst {
		n, err := io.ReadFull(data, buf)
		if err == io.EOF {
			// 如果还没有读完所有分片就遇到EOF，说明数据不足
			if totalRead < size {
				return ErrShortData
			}
			// 用0填充剩余的分片
			for i := shardNum; i < len(dst); i++ {
				_, err = dst[i].Write(make([]byte, perShard))
				if err != nil {
					logger.Errorf("写入分片失败: %v", err)
					return err
				}
			}
			return nil
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			logger.Errorf("分割失败: %v", err)
			return err
		}

		totalRead += int64(n)

		// 如果读取的数据不足一个完整的分片，需要填充
		if n < int(perShard) {
			// 用0填充剩余部分
			for i := n; i < int(perShard); i++ {
				buf[i] = 0
			}
		}

		// 写入分片
		_, err = dst[shardNum].Write(buf[:perShard])
		if err != nil {
			logger.Errorf("写入分片失败: %v", err)
			return err
		}
	}

	// 检查是否还有剩余数据
	extra := make([]byte, 1)
	n, err := data.Read(extra)
	if n > 0 {
		logger.Errorf("分割失败: 数据大小超过预期")
		return ErrShardSize
	}
	if err != io.EOF {
		logger.Errorf("分割失败: 预期遇到EOF")
		return err
	}

	// 检查是否读取了足够的数据
	if totalRead < size {
		return ErrShortData
	}

	return nil
}

// 修改并发读取实现
func (r *rsStream16) readInputsConcurrent(dst [][]byte, readers []io.Reader) (int, error) {
	var wg sync.WaitGroup
	wg.Add(len(readers))
	res := make(chan readResult, len(readers))

	// 修改: 使用map来存储每个分片的读取长度
	shardSizes := make(map[int]int)
	var firstSize int32 = -1

	for i := range readers {
		go func(i int) {
			defer wg.Done()
			if readers[i] == nil {
				dst[i] = dst[i][:0]
				res <- readResult{size: 0, err: nil, n: i}
				return
			}

			// 确保目标切片有足够空间且初始化为非零长度
			if cap(dst[i]) < r.blockSize {
				dst[i] = make([]byte, r.blockSize)
			}
			dst[i] = dst[i][:r.blockSize] // 设置切片长度为blockSize

			// 读取数据
			n, err := io.ReadFull(readers[i], dst[i])
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				res <- readResult{size: 0, err: err, n: i}
				return
			}

			// 设置第一个有效大小
			if atomic.CompareAndSwapInt32(&firstSize, -1, int32(n)) {
				logger.Debugf("设置首个分片大小: %d", n)
			}

			res <- readResult{size: n, err: nil, n: i}
		}(i)
	}

	wg.Wait()
	close(res)

	// 收集所有分片的读取结果
	for result := range res {
		if result.err != nil {
			return 0, result.err
		}
		// 记录每个分片的实际读取长度
		shardSizes[result.n] = result.size
	}

	// 获取第一个非零的读取长度作为基准
	size := int(atomic.LoadInt32(&firstSize))
	if size == -1 {
		return 0, io.EOF
	}

	// 验证所有分片的读取长度是否一致
	for i := 0; i < r.dataShards; i++ {
		if n, ok := shardSizes[i]; ok {
			if n != size {
				return 0, ErrShardSize
			}
			// 确保分片长度正确设置
			dst[i] = dst[i][:n]
		} else {
			// 如果某个分片没有读取结果，返回错误
			return 0, ErrShardNoData
		}
	}

	// 添加调试日志
	logger.Debugf("读取的数据大小: %d", size)
	for i := range dst {
		logger.Debugf("分片 %d 大小: %d", i, len(dst[i]))
	}

	return size, nil
}

// 并发写入实现
// 参数:
// - writers: 一个包含parityShards个输出流的切片
// - src: 一个包含parityShards个分片缓冲区的切片
// - size: 写入的字节数
// 返回:
// - error: 如果发生错误,返回错误信息
func (r *rsStream16) writeOutputsConcurrent(writers []io.Writer, src [][]byte, size int) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(writers))

	// 确保所有分片使用相同的对齐大小
	alignedSize := ((size + 63) / 64) * 64

	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if writers[i] == nil {
				errs <- nil
				return
			}

			// 确保写入对齐后的大小
			if len(src[i]) < alignedSize {
				tmp := make([]byte, alignedSize)
				copy(tmp, src[i])
				src[i] = tmp
			}

			n, err := writers[i].Write(src[i][:alignedSize])
			if err != nil {
				errs <- StreamWriteError{Err: err, Stream: i}
				return
			}
			if n != alignedSize {
				errs <- StreamWriteError{Err: io.ErrShortWrite, Stream: i}
				return
			}
			errs <- nil
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			logger.Errorf("并发写入失败: %v", err)
			return err
		}
	}
	return nil
}

// 添加缓冲区池
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// 在并发操作中使用缓冲区池
func getBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}
