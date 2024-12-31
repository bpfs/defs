/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2015, Klaus Post
 * Copyright 2015, Backblaze, Inc.
 */

package reedsolomon

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// StreamEncoder 是一个接口,用于对数据进行Reed-Solomon奇偶校验编码。
// 它提供了完全流式的接口,并以最大4MB的块处理数据。
//
// 对于10MB及以下的小分片大小,建议使用内存接口,
// 因为流式接口有启动开销。
//
// 对于所有操作,读取器和写入器不应假定任何单个读/写的顺序/大小。
//
// 使用示例请参见examples文件夹中的"stream-encoder.go"和"streamdecoder.go"。
type StreamEncoder interface {
	// Encode 为一组数据分片编码奇偶校验分片。
	//
	// 参数:
	//   - data: 包含数据分片的读取器切片
	//   - parity: 用于写入奇偶校验分片的写入器切片
	//
	// 返回值:
	//   - error: 如果编码过程中出现错误则返回，否则返回nil
	//
	// 注意:
	//   - 分片数量必须与NewStream()中给定的数量匹配
	//   - 每个读取器必须提供相同数量的字节
	//   - 如果数据流返回错误,将返回StreamReadError类型的错误
	//   - 如果奇偶校验写入器返回错误,将返回StreamWriteError
	Encode(data []io.Reader, parity []io.Writer) error

	// Verify 验证奇偶校验分片是否包含正确的数据。
	//
	// 参数:
	//   - shards: 包含所有数据和奇偶校验分片的读取器切片
	//
	// 返回值:
	//   - bool: 如果奇偶校验正确则返回true，否则返回false
	//   - error: 如果验证过程中出现错误则返回，否则返回nil
	//
	// 注意:
	//   - 分片数量必须与NewStream()中给定的总数据+奇偶校验分片数量匹配
	//   - 每个读取器必须提供相同数量的字节
	//   - 如果分片流返回错误,将返回StreamReadError类型的错误
	Verify(shards []io.Reader) (bool, error)

	// Reconstruct 尝试重建丢失的分片。
	//
	// 参数:
	//   - valid: 有效分片的读取器切片，丢失的分片用nil表示
	//   - fill: 用于写入重建分片的写入器切片，不需要重建的分片用nil表示
	//
	// 返回值:
	//   - error: 如果重建过程中出现错误则返回，否则返回nil
	//
	// 注意:
	//   - 如果分片太少而无法重建丢失的分片,将返回ErrTooFewShards
	//   - 重建的分片集是完整的,但未验证完整性
	//   - 使用Verify函数检查数据集是否正常
	Reconstruct(valid []io.Reader, fill []io.Writer) error

	// Split 将输入流分割为给定编码器的分片数。
	//
	// 参数:
	//   - data: 输入数据流
	//   - dst: 用于写入分割后分片的写入器切片
	//   - size: 输入数据的总大小
	//
	// 返回值:
	//   - error: 如果分割过程中出现错误则返回，否则返回nil
	//
	// 注意:
	//   - 如果数据大小不能被分片数整除,最后一个分片将包含额外的零
	//   - 如果无法检索指定的字节数,将返回'ErrShortData'
	Split(data io.Reader, dst []io.Writer, size int64) (err error)

	// Join 将分片合并并将数据段写入dst。
	//
	// 参数:
	//   - dst: 用于写入合并后数据的写入器
	//   - shards: 包含所有分片的读取器切片
	//   - outSize: 期望的输出数据大小
	//
	// 返回值:
	//   - error: 如果合并过程中出现错误则返回，否则返回nil
	//
	// 注意:
	//   - 只考虑数据分片
	//   - 如果给定的分片太少,将返回ErrTooFewShards
	//   - 如果总数据大小小于outSize,将返回ErrShortData
	Join(dst io.Writer, shards []io.Reader, outSize int64) error
}

// StreamReadError 表示在读取流时遇到的错误。
// 它可以帮助定位哪个读取器失败了。
//
// 字段:
//   - Err: 具体的错误信息
//   - Stream: 发生错误的流序号
type StreamReadError struct {
	Err    error // 具体的错误信息
	Stream int   // 发生错误的流序号
}

// Error 返回格式化的错误字符串。
//
// 返回值:
//   - string: 包含流序号和错误信息的格式化字符串
func (s StreamReadError) Error() string {
	// 使用fmt.Sprintf格式化错误信息,包含流序号和具体错误
	return fmt.Sprintf("error reading stream %d: %s", s.Stream, s.Err)
}

// String 返回错误的字符串表示。
//
// 返回值:
//   - string: 与Error()方法返回相同的错误字符串
func (s StreamReadError) String() string {
	// 直接调用Error()方法返回错误字符串
	return s.Error()
}

// StreamWriteError 表示在写入流时遇到的错误。
// 它可以帮助定位哪个写入器失败了。
//
// 字段:
//   - Err: 具体的错误信息
//   - Stream: 发生错误的流序号
type StreamWriteError struct {
	Err    error // 具体的错误信息
	Stream int   // 发生错误的流序号
}

// Error 返回格式化的错误字符串。
//
// 返回值:
//   - string: 包含流序号和错误信息的格式化字符串
func (s StreamWriteError) Error() string {
	// 使用fmt.Sprintf格式化错误信息,包含流序号和具体错误
	return fmt.Sprintf("error writing stream %d: %s", s.Stream, s.Err)
}

// String 返回错误的字符串表示。
//
// 返回值:
//   - string: 与Error()方法返回相同的错误字符串
func (s StreamWriteError) String() string {
	// 直接调用Error()方法返回错误字符串
	return s.Error()
}

// rsStream 实现了基于Reed-Solomon码的流式编解码器
// 用于处理大文件的分片编码和解码
// 通过NewStream()函数构造实例
type rsStream struct {
	// r 是底层的Reed-Solomon编码器实例
	// 用于执行实际的编码和解码操作
	r *reedSolomon

	// o 包含编码器的配置选项
	// 如分片大小、并发数等
	o options

	// readShards 是分片读取函数
	// 参数:
	//   - dst: 用于存储读取数据的字节切片数组
	//   - in: 输入的Reader切片
	// 返回值:
	//   - error: 读取过程中的错误信息
	readShards func(dst [][]byte, in []io.Reader) error

	// writeShards 是分片写入函数
	// 参数:
	//   - out: 输出的Writer切片
	//   - in: 要写入的字节切片数组
	// 返回值:
	//   - error: 写入过程中的错误信息
	writeShards func(out []io.Writer, in [][]byte) error

	// blockPool 是字节切片数组的对象池
	// 用于重用内存,减少GC压力
	blockPool sync.Pool
}

// NewStream 创建一个新的编码器并初始化它
// 参数:
//   - dataShards: int, 数据分片的数量
//   - parityShards: int, 奇偶校验分片的数量
//   - o: ...Option, 可选的配置选项
//
// 返回值:
//   - StreamEncoder: 创建的流编码器
//   - error: 如果创建过程中出现错误则返回，否则为 nil
//
// 注意:
//   - 数据分片的最大数量为 256
//   - 可以重复使用此编码器
func NewStream(dataShards, parityShards int, o ...Option) (StreamEncoder, error) {
	// 检查分片总数是否超过最大限制(256)
	if dataShards+parityShards > 256 {
		return nil, ErrMaxShardNum
	}

	// 创建rsStream实例并使用默认选项初始化
	r := rsStream{o: defaultOptions}

	// 应用用户提供的所有选项
	for _, opt := range o {
		opt(&r.o)
	}

	// 如果设置了分片大小,则覆盖流块大小
	if r.o.streamBS == 0 && r.o.shardSize > 0 {
		r.o.streamBS = r.o.shardSize
	}

	// 如果流块大小未设置,使用默认值4MB
	if r.o.streamBS <= 0 {
		r.o.streamBS = 4 << 20 // 4MB
	}

	// 如果未设置分片大小且使用默认goroutine数,则根据流块大小自动设置
	if r.o.shardSize == 0 && r.o.maxGoroutines == defaultOptions.maxGoroutines {
		o = append(o, WithAutoGoroutines(r.o.streamBS))
	}

	// 创建底层Reed-Solomon编码器
	enc, err := New(dataShards, parityShards, o...)
	if err != nil {
		return nil, err
	}
	r.r = enc.(*reedSolomon)

	// 初始化对象池的创建函数
	r.blockPool.New = func() interface{} {
		return AllocAligned(dataShards+parityShards, r.o.streamBS)
	}

	// 设置默认的读写分片函数
	r.readShards = readShards
	r.writeShards = writeShards

	// 如果启用并发读取,使用并发读取函数
	if r.o.concReads {
		r.readShards = cReadShards
	}

	// 如果启用并发写入,使用并发写入函数
	if r.o.concWrites {
		r.writeShards = cWriteShards
	}

	// 返回创建的rsStream实例
	return &r, err
}

// NewStreamC 创建一个新的流编码器并初始化数据分片和校验分片数量
//
// 参数:
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//   - conReads: 是否启用并发读取
//   - conWrites: 是否启用并发写入
//   - o: ...Option, 可选的配置选项
//
// 返回值:
//   - StreamEncoder: 创建的流编码器
//   - error: 如果创建过程中出现错误则返回，否则为 nil
//
// 说明:
//   - 此函数功能与 NewStream 相同，但允许启用并发读写
func NewStreamC(dataShards, parityShards int, conReads, conWrites bool, o ...Option) (StreamEncoder, error) {
	return NewStream(dataShards, parityShards, append(o, WithConcurrentStreamReads(conReads), WithConcurrentStreamWrites(conWrites))...)
}

// createSlice 创建一个新的字节切片数组用于存储数据
//
// 返回值:
//   - [][]byte: 创建的字节切片数组
//
// 说明:
//   - 从对象池中获取预分配的内存
//   - 根据流块大小调整每个切片的容量
func (r *rsStream) createSlice() [][]byte {
	// 从对象池获取预分配的切片数组
	out := r.blockPool.Get().([][]byte)
	// 调整每个切片的大小为流块大小
	for i := range out {
		out[i] = out[i][:r.o.streamBS]
	}
	return out
}

// Encode 对一组数据分片进行编码,生成校验分片
// 参数:
//   - data: 包含数据分片的Reader数组
//   - parity: 用于写入校验分片的Writer数组
//
// 返回值:
//   - error: 编码过程中的错误信息
//
// 说明:
// - data数组长度必须等于初始化时指定的数据分片数
// - parity数组长度必须等于初始化时指定的校验分片数
// - 每个Reader必须提供相同数量的字节
// - 写入的校验分片大小将与输入数据大小相同
// - 如果数据流返回错误,将返回StreamReadError类型错误
// - 如果校验分片写入失败,将返回StreamWriteError类型错误
func (r *rsStream) Encode(data []io.Reader, parity []io.Writer) error {
	// 验证数据分片数量是否正确
	if len(data) != r.r.dataShards {
		return ErrTooFewShards
	}

	// 验证校验分片数量是否正确
	if len(parity) != r.r.parityShards {
		return ErrTooFewShards
	}

	// 创建用于存储所有分片的缓冲区
	all := r.createSlice()
	defer r.blockPool.Put(all)  // 使用完毕后归还缓冲区
	in := all[:r.r.dataShards]  // 数据分片缓冲区
	out := all[r.r.dataShards:] // 校验分片缓冲区
	read := 0                   // 已读取的字节数

	// 循环处理所有数据
	for {
		// 读取数据分片
		err := r.readShards(in, data)
		switch err {
		case nil:
		case io.EOF:
			// 如果没有读取到任何数据则返回错误
			if read == 0 {
				return ErrShardNoData
			}
			return nil
		default:
			return err
		}

		// 调整校验分片缓冲区大小以匹配数据分片
		out = trimShards(out, shardSize(in))
		read += shardSize(in)

		// 对数据进行编码生成校验分片
		err = r.r.Encode(all)
		if err != nil {
			return err
		}

		// 写入校验分片
		err = r.writeShards(parity, out)
		if err != nil {
			return err
		}
	}
}

// trimShards 调整分片大小使其保持一致
//
// 参数:
//   - in: [][]byte, 输入分片数组
//   - size: int, 目标分片大小
//
// 返回值:
//   - [][]byte: 调整后的分片数组
//
// 说明:
//   - 如果分片长度不为0,则截取到指定大小
//   - 如果分片长度小于目标大小,则将其置空
func trimShards(in [][]byte, size int) [][]byte {
	for i := range in {
		// 如果分片不为空,则截取到指定大小
		if len(in[i]) != 0 {
			in[i] = in[i][0:size]
		}
		// 如果分片长度小于目标大小,则置空
		if len(in[i]) < size {
			in[i] = in[i][:0]
		}
	}
	return in
}

// readShards 从多个输入流读取分片数据
//
// 参数:
//   - dst: [][]byte, 目标字节数组,用于存储读取的分片数据
//   - in: []io.Reader, 输入流数组,用于读取分片数据
//
// 返回值:
//   - error: 读取过程中的错误信息
//
// 说明:
//   - 所有分片的大小必须相同,否则返回ErrShardSize错误
//   - 如果读取过程中发生错误,将返回StreamReadError类型的错误
func readShards(dst [][]byte, in []io.Reader) error {
	// 检查输入输出数组长度是否匹配
	if len(in) != len(dst) {
		panic("internal error: in and dst size do not match")
	}

	// 记录第一个非空分片的大小,用于校验所有分片大小是否一致
	size := -1

	// 遍历所有输入流
	for i := range in {
		// 如果输入流为nil,则将对应的目标数组置空
		if in[i] == nil {
			dst[i] = dst[i][:0]
			continue
		}

		// 读取数据到目标数组
		n, err := io.ReadFull(in[i], dst[i])

		// 处理读取结果
		switch err {
		case io.ErrUnexpectedEOF, io.EOF:
			// 记录第一个分片的大小
			if size < 0 {
				size = n
			} else if n != size {
				// 检查分片大小是否一致
				return ErrShardSize
			}
			// 调整目标数组大小为实际读取的字节数
			dst[i] = dst[i][0:n]
		case nil:
			continue
		default:
			// 返回读取错误
			return StreamReadError{Err: err, Stream: i}
		}
	}

	// 如果没有读取到任何数据则返回EOF
	if size == 0 {
		return io.EOF
	}
	return nil
}

// writeShards 将分片数据写入多个输出流
//
// 参数:
//   - out: []io.Writer, 输出流数组,用于写入分片数据
//   - in: [][]byte, 输入字节数组,包含要写入的分片数据
//
// 返回值:
//   - error: 写入过程中的错误信息
//
// 说明:
//   - 如果写入过程中发生错误,将返回StreamWriteError类型的错误
//   - 如果写入的字节数不等于输入数据长度,将返回io.ErrShortWrite错误
func writeShards(out []io.Writer, in [][]byte) error {
	// 检查输入输出数组长度是否匹配
	if len(out) != len(in) {
		panic("internal error: in and out size do not match")
	}

	// 遍历所有输出流
	for i := range in {
		// 如果输出流为nil则跳过
		if out[i] == nil {
			continue
		}

		// 写入数据
		n, err := out[i].Write(in[i])
		if err != nil {
			return StreamWriteError{Err: err, Stream: i}
		}

		// 检查是否完整写入
		if n != len(in[i]) {
			return StreamWriteError{Err: io.ErrShortWrite, Stream: i}
		}
	}
	return nil
}

// readResult 定义了读取结果的结构体
//
// 字段:
//   - n: int, 分片索引
//   - size: int, 读取的字节数
//   - err: error, 读取过程中的错误
type readResult struct {
	n    int
	size int
	err  error
}

// cReadShards 并发读取分片数据
//
// 参数:
//   - dst: [][]byte, 目标字节数组,用于存储读取的分片数据
//   - in: []io.Reader, 输入流数组,用于读取分片数据
//
// 返回值:
//   - error: 读取过程中的错误信息
//
// 说明:
//   - 通过goroutine并发读取每个分片数据
//   - 所有分片的大小必须相同,否则返回ErrShardSize错误
//   - 如果读取过程中发生错误,将返回StreamReadError类型的错误
func cReadShards(dst [][]byte, in []io.Reader) error {
	// 检查输入输出数组长度是否匹配
	if len(in) != len(dst) {
		panic("internal error: in and dst size do not match")
	}

	// 创建等待组和结果通道
	var wg sync.WaitGroup
	wg.Add(len(in))
	res := make(chan readResult, len(in))

	// 并发读取每个分片
	for i := range in {
		// 如果输入流为nil,则跳过读取
		if in[i] == nil {
			dst[i] = dst[i][:0]
			wg.Done()
			continue
		}
		go func(i int) {
			defer wg.Done()
			// 读取数据
			n, err := io.ReadFull(in[i], dst[i])
			// 将读取结果发送到通道
			res <- readResult{size: n, err: err, n: i}
		}(i)
	}

	// 等待所有读取操作完成并关闭通道
	wg.Wait()
	close(res)

	// 处理读取结果
	size := -1
	for r := range res {
		switch r.err {
		case io.ErrUnexpectedEOF, io.EOF:
			// 检查分片大小是否一致
			if size < 0 {
				size = r.size
			} else if r.size != size {
				return ErrShardSize
			}
			dst[r.n] = dst[r.n][0:r.size]
		case nil:
		default:
			return StreamReadError{Err: r.err, Stream: r.n}
		}
	}

	// 处理特殊情况
	if size == 0 {
		return io.EOF
	}
	return nil
}

// cWriteShards 并发写入分片数据
//
// 参数:
//   - out: []io.Writer, 输出流数组,用于写入分片数据
//   - in: [][]byte, 输入数据数组,包含要写入的分片数据
//
// 返回值:
//   - error: 写入过程中的错误信息
//
// 说明:
//   - 通过goroutine并发写入每个分片数据
//   - 如果写入过程中发生错误,将返回StreamWriteError类型的错误
//   - 如果写入的字节数不等于输入数据长度,将返回io.ErrShortWrite错误
func cWriteShards(out []io.Writer, in [][]byte) error {
	// 检查输入输出数组长度是否匹配
	if len(out) != len(in) {
		panic("internal error: in and out size do not match")
	}

	// 创建错误通道和等待组
	var errs = make(chan error, len(out))
	var wg sync.WaitGroup
	wg.Add(len(out))

	// 并发写入每个分片
	for i := range in {
		go func(i int) {
			defer wg.Done()
			// 如果输出流为nil,则跳过写入
			if out[i] == nil {
				errs <- nil
				return
			}
			// 写入数据
			n, err := out[i].Write(in[i])
			if err != nil {
				errs <- StreamWriteError{Err: err, Stream: i}
				return
			}
			// 检查写入的字节数是否正确
			if n != len(in[i]) {
				errs <- StreamWriteError{Err: io.ErrShortWrite, Stream: i}
			}
		}(i)
	}

	// 等待所有写入操作完成
	wg.Wait()
	close(errs)

	// 检查是否有错误发生
	for err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

// Verify 验证校验分片是否包含正确的数据
//
// 参数:
//   - shards: []io.Reader, 分片数据的Reader数组
//
// 返回值:
//   - bool: 验证结果,true表示验证通过
//   - error: 验证过程中的错误信息
//
// 说明:
//   - 分片数量必须与初始化时指定的总分片数(数据分片+校验分片)相匹配
//   - 所有分片流必须提供相同数量的字节
//   - 如果分片流返回错误,将返回StreamReadError类型的错误
func (r *rsStream) Verify(shards []io.Reader) (bool, error) {
	// 检查分片数量是否正确
	if len(shards) != r.r.totalShards {
		return false, ErrTooFewShards
	}

	// 初始化读取计数和分片缓冲区
	read := 0
	all := r.createSlice()
	defer r.blockPool.Put(all)

	// 循环读取并验证分片数据
	for {
		// 读取所有分片数据
		err := r.readShards(all, shards)
		if err == io.EOF {
			if read == 0 {
				return false, ErrShardNoData
			}
			return true, nil
		}
		if err != nil {
			return false, err
		}

		// 更新读取计数并验证数据
		read += shardSize(all)
		ok, err := r.r.Verify(all)
		if !ok || err != nil {
			return ok, err
		}
	}
}

// ErrReconstructMismatch 当在同一个索引位置同时提供了"valid"和"fill"流时返回此错误
// 这种情况下无法判断是将该分片视为有效还是需要重建
var ErrReconstructMismatch = errors.New("valid shards and fill shards are mutually exclusive")

// Reconstruct 重建丢失的分片(如果可能的话)
//
// 参数:
//   - valid: []io.Reader, 有效分片的Reader数组,用于读取现有数据
//   - fill: []io.Writer, 无效分片的Writer数组,用于写入重建的数据
//
// 返回值:
//   - error: 重建过程中的错误信息
//
// 说明:
//   - 通过在valid切片中设置nil并同时在fill中设置非nil的writer来标记缺失的分片
//   - 同一个索引位置不能同时包含非nil的valid和fill项
//   - 如果可用分片数量不足以重建丢失的分片,将返回ErrTooFewShards错误
//   - 只有在明确请求所有缺失分片时,重建的分片集才是完整的
//   - 重建后的数据完整性不会自动验证,需要使用Verify函数进行检查
func (r *rsStream) Reconstruct(valid []io.Reader, fill []io.Writer) error {
	// 检查输入参数的长度是否符合要求
	if len(valid) != r.r.totalShards {
		return ErrTooFewShards
	}
	if len(fill) != r.r.totalShards {
		return ErrTooFewShards
	}

	// 创建用于存储分片数据的缓冲区
	all := r.createSlice()
	defer r.blockPool.Put(all)

	// 判断是否只需要重建数据分片
	reconDataOnly := true
	for i := range valid {
		// 检查是否存在同时设置valid和fill的情况
		if valid[i] != nil && fill[i] != nil {
			return ErrReconstructMismatch
		}
		// 如果需要重建校验分片,则设置reconDataOnly为false
		if i >= r.r.dataShards && fill[i] != nil {
			reconDataOnly = false
		}
	}

	// 读取并重建分片数据
	read := 0
	for {
		// 从有效分片中读取数据
		err := r.readShards(all, valid)
		if err == io.EOF {
			if read == 0 {
				return ErrShardNoData
			}
			return nil
		}
		if err != nil {
			return err
		}

		// 更新已读取的数据量并调整分片大小
		read += shardSize(all)
		all = trimShards(all, shardSize(all))

		// 根据需求选择重建方式
		if reconDataOnly {
			// 仅重建缺失的数据分片
			err = r.r.ReconstructData(all)
		} else {
			// 重建所有缺失的分片(包括数据分片和校验分片)
			err = r.r.Reconstruct(all)
		}
		if err != nil {
			return err
		}

		// 将重建的数据写入目标Writer
		err = r.writeShards(fill, all)
		if err != nil {
			return err
		}
	}
}

// Join 将多个数据分片合并并写入目标Writer
//
// 参数:
//   - dst: io.Writer, 用于写入合并后数据的目标Writer
//   - shards: []io.Reader, 包含数据分片的Reader数组
//   - outSize: int64, 期望输出的数据大小
//
// 返回值:
//   - error: 合并过程中的错误信息
//
// 说明:
//   - 只处理数据分片,不包含校验分片
//   - 必须提供准确的输出大小
//   - 如果提供的分片数量不足,将返回ErrTooFewShards错误
//   - 如果实际数据大小小于outSize,将返回ErrShortData错误
func (r *rsStream) Join(dst io.Writer, shards []io.Reader, outSize int64) error {
	// 检查是否有足够的数据分片
	if len(shards) < r.r.dataShards {
		return ErrTooFewShards
	}

	// 截取数据分片,去除可能存在的校验分片
	shards = shards[:r.r.dataShards]
	// 检查每个分片是否有效
	for i := range shards {
		if shards[i] == nil {
			return StreamReadError{Err: ErrShardNoData, Stream: i}
		}
	}
	// 将所有分片合并为一个Reader
	src := io.MultiReader(shards...)

	// 将合并后的数据复制到目标Writer
	n, err := io.CopyN(dst, src, outSize)
	if err == io.EOF {
		return ErrShortData
	}
	if err != nil {
		return err
	}
	// 检查复制的数据大小是否符合预期
	if n != outSize {
		return ErrShortData
	}
	return nil
}

// Split 将输入流分割成编码器指定数量的分片
//
// 参数:
//   - data: io.Reader, 输入数据流
//   - dst: []io.Writer, 用于写入分片数据的Writer数组
//   - size: int64, 输入数据的总大小
//
// 返回值:
//   - error: 分割过程中的错误信息
//
// 说明:
//   - 数据会被均分成大小相等的分片
//   - 如果数据大小不能被分片数整除,最后一个分片会用0填充
//   - 必须提供输入数据的总大小
//   - 如果无法获取指定大小的数据,将返回ErrShortData错误
func (r *rsStream) Split(data io.Reader, dst []io.Writer, size int64) error {
	// 检查输入大小是否为0
	if size == 0 {
		return ErrShortData
	}

	// 检查目标Writer数量是否等于数据分片数
	if len(dst) != r.r.dataShards {
		return ErrInvShardNum
	}

	// 检查每个Writer是否有效
	for i := range dst {
		if dst[i] == nil {
			return StreamWriteError{Err: ErrShardNoData, Stream: i}
		}
	}

	// 计算每个分片的字节数
	// 向上取整以确保能容纳所有数据
	perShard := (size + int64(r.r.dataShards) - 1) / int64(r.r.dataShards)

	// 计算需要填充的字节数
	// 使总大小达到 r.Shards*perShard
	paddingSize := (int64(r.r.totalShards) * perShard) - size
	// 将原始数据和填充数据组合成一个Reader
	data = io.MultiReader(data, io.LimitReader(zeroPaddingReader{}, paddingSize))

	// 将数据分割成等长分片并复制
	for i := range dst {
		n, err := io.CopyN(dst[i], data, perShard)
		if err != io.EOF && err != nil {
			return err
		}
		if n != perShard {
			return ErrShortData
		}
	}

	return nil
}

// zeroPaddingReader 实现了一个只返回0字节的Reader接口
type zeroPaddingReader struct{}

// 确保zeroPaddingReader实现了io.Reader接口
var _ io.Reader = &zeroPaddingReader{}

// Read 实现io.Reader接口的Read方法
// 参数:
//   - p: []byte, 用于存储读取数据的字节切片
//
// 返回值:
//   - n: int, 读取的字节数
//   - err: error, 读取过程中的错误,始终为nil
func (t zeroPaddingReader) Read(p []byte) (n int, err error) {
	n = len(p)
	// 将切片填充为0
	for i := 0; i < n; i++ {
		p[i] = 0
	}
	return n, nil
}
