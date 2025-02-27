/**
 * 基于8位值的Reed-Solomon编码。
 *
 * 版权所有 2015, Klaus Post
 * 版权所有 2015, Backblaze, Inc.
 */

package reedsolomon

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// StreamEncoder 是一个用于对数据进行Reed-Solomon奇偶校验编码的接口。
// 它提供了完整的流式接口,并以最大4MB的块处理数据。
//
// 对于10MB及以下的小分片大小,建议使用内存接口,因为流式接口有启动开销。
//
// 对于所有操作,读取器和写入器不应假定任何单个读/写的顺序/大小。
//
// 使用示例请参见examples文件夹中的"stream-encoder.go"和"streamdecoder.go"。
type StreamEncoder interface {
	// Encode 为一组数据分片生成奇偶校验分片。
	//
	// 输入'shards'包含数据分片的读取器,后跟奇偶校验分片的io.Writer。
	//
	// 分片数量必须与传给NewStream()的数量匹配。
	//
	// 每个读取器必须提供相同数量的字节。
	//
	// 奇偶校验分片将写入写入器。
	// 写入的字节数将与输入大小匹配。
	//
	// 如果数据流返回错误,将返回StreamReadError类型错误。如果奇偶校验写入器返回错误,将返回StreamWriteError。
	Encode(inputs []io.Reader, outputs []io.Writer) error

	// Verify 如果奇偶校验分片包含正确的数据则返回true。
	//
	// 分片数量必须与传给NewStream()的数据+奇偶校验分片总数匹配。
	//
	// 每个读取器必须提供相同数量的字节。
	// 如果分片流返回错误,将返回StreamReadError类型错误。
	Verify(shards []io.Reader) (bool, error)

	// Reconstruct 将在可能的情况下重建丢失的分片。
	//
	// 给定有效分片列表(用于读取)和无效分片列表(用于写入)
	//
	// 通过在'valid'切片中将其设置为nil并同时在"fill"中设置非nil写入器来指示分片丢失。
	// 一个索引不能同时包含非nil的'valid'和'fill'条目。
	// 如果两者都提供了,将返回'ErrReconstructMismatch'。
	//
	// 如果分片太少而无法重建丢失的分片,将返回ErrTooFewShards。
	//
	// 重建的分片集是完整的,但未验证完整性。
	// 使用Verify函数检查数据集是否正常。
	Reconstruct(inputs []io.Reader, outputs []io.Writer) error

	// Split 将输入流分割成给定给编码器的分片数。
	//
	// 数据将被分割成大小相等的分片。
	// 如果数据大小不能被分片数整除,最后一个分片将包含额外的零。
	//
	// 您必须提供输入的总大小。
	// 如果无法检索指定的字节数,将返回'ErrShortData'。
	Split(data io.Reader, dst []io.Writer, size int64) error

	// Join 将分片连接起来并将数据段写入dst。
	//
	// 只考虑数据分片。
	//
	// 您必须提供想要的确切输出大小。
	// 如果给定的分片太少,将返回ErrTooFewShards。
	// 如果总数据大小小于outSize,将返回ErrShortData。
	Join(dst io.Writer, shards []io.Reader, outSize int64) error
}

// StreamReadError 在遇到与提供的流相关的读取错误时返回。
// 这将让您知道哪个读取器失败了。
type StreamReadError struct {
	Err    error // 错误
	Stream int   // 发生错误的流编号
}

// Error 以字符串形式返回错误
//
// 返回:
// - string: 错误字符串
func (s StreamReadError) Error() string {
	return fmt.Sprintf("读取流 %d 时出错: %s", s.Stream, s.Err)
}

// String 以字符串形式返回错误
//
// 返回:
// - string: 错误字符串
func (s StreamReadError) String() string {
	return s.Error()
}

// StreamWriteError 在遇到与提供的流相关的写入错误时返回。
// 这将让您知道哪个读取器失败了。
type StreamWriteError struct {
	Err    error // 错误
	Stream int   // 发生错误的流编号
}

// Error 以字符串形式返回错误
//
// 返回:
// - string: 错误字符串
func (s StreamWriteError) Error() string {
	return fmt.Sprintf("写入流 %d 时出错: %s", s.Stream, s.Err)
}

// String 以字符串形式返回错误
//
// 返回:
// - string: 错误字符串
func (s StreamWriteError) String() string {
	return s.Error()
}

// rsStream 包含用于特定数据分片和奇偶校验分片分布的矩阵。
// 使用NewStream()构造
type rsStream struct {
	r *reedSolomon
	o options

	// 分片读取器
	readShards func(dst [][]byte, in []io.Reader) error
	// 分片写入器
	writeShards func(out []io.Writer, in [][]byte) error

	blockPool sync.Pool
}

// NewStream 创建一个新的编码器并将其初始化为您想要使用的数据分片和奇偶校验分片的数量。您可以重用此编码器。
// 注意数据分片的最大数量是256。
// 参数:
// - dataShards: int 数据分片数量
// - parityShards: int 奇偶校验分片数量
// - o: 可选参数,可以传递WithConcurrentStreamReads(true)和WithConcurrentStreamWrites(true)来启用并发读取和写入。
// 返回:
// - StreamEncoder: 流式编码器
// - error: 错误
func NewStream(dataShards, parityShards int, o ...Option) (StreamEncoder, error) {
	if dataShards+parityShards > 256 {
		return nil, ErrMaxShardNum
	}

	r := rsStream{o: defaultOptions}
	for _, opt := range o {
		opt(&r.o)
	}
	// 如果设置了分片大小,则覆盖块大小。
	if r.o.streamBS == 0 && r.o.shardSize > 0 {
		r.o.streamBS = r.o.shardSize
	}
	if r.o.streamBS <= 0 {
		r.o.streamBS = 4 << 20
	}
	if r.o.shardSize == 0 && r.o.maxGoroutines == defaultOptions.maxGoroutines {
		o = append(o, WithAutoGoroutines(r.o.streamBS))
	}

	enc, err := New(dataShards, parityShards, o...)
	if err != nil {
		return nil, err
	}
	r.r = enc.(*reedSolomon)

	r.blockPool.New = func() interface{} {
		return AllocAligned(dataShards+parityShards, r.o.streamBS)
	}
	r.readShards = readShards
	r.writeShards = writeShards
	if r.o.concReads {
		r.readShards = cReadShards
	}
	if r.o.concWrites {
		r.writeShards = cWriteShards
	}

	return &r, err
}

// NewStreamC 创建一个新的编码器并将其初始化为给定的数据分片和奇偶校验分片数量。
//
// 此函数与'NewStream'功能相同,但允许您启用并发读取和写入。
//
// 参数:
// - dataShards: int 数据分片数量
// - parityShards: int 奇偶校验分片数量
// - conReads: bool 是否启用并发读取
// - conWrites: bool 是否启用并发写入
// - o: 可选参数,可以传递WithConcurrentStreamReads(true)和WithConcurrentStreamWrites(true)来启用并发读取和写入。
func NewStreamC(dataShards, parityShards int, conReads, conWrites bool, o ...Option) (StreamEncoder, error) {
	return NewStream(dataShards, parityShards, append(o, WithConcurrentStreamReads(conReads), WithConcurrentStreamWrites(conWrites))...)
}

// createSlice 创建一个分片切片
// 返回:
// - [][]byte: 分片切片
func (r *rsStream) createSlice() [][]byte {
	out := r.blockPool.Get().([][]byte)
	for i := range out {
		out[i] = out[i][:r.o.streamBS]
	}
	return out
}

// Encode 为一组数据分片生成奇偶校验分片。
//
// 输入'shards'包含数据分片的读取器,后跟奇偶校验分片的io.Writer。
//
// 分片数量必须与传给NewStream()的数量匹配。
//
// 每个读取器必须提供相同数量的字节。
//
// 奇偶校验分片将写入写入器。
// 写入的字节数将与输入大小匹配。
//
// 如果数据流返回错误,将返回StreamReadError类型错误。如果奇偶校验写入器返回错误,将返回StreamWriteError。
//
// 参数:
// - data: []io.Reader 数据分片的读取器
// - parity: []io.Writer 奇偶校验分片的写入器
// 返回:
// - error: 错误
func (r *rsStream) Encode(inputs []io.Reader, outputs []io.Writer) error {
	if len(inputs) != r.r.dataShards {
		return ErrTooFewShards
	}

	if len(outputs) != r.r.parityShards {
		return ErrTooFewShards
	}

	all := r.createSlice()
	defer r.blockPool.Put(all)
	in := all[:r.r.dataShards]
	out := all[r.r.dataShards:]
	read := 0

	for {
		err := r.readShards(in, inputs)
		switch err {
		case nil:
		case io.EOF:
			if read == 0 {
				return ErrShardNoData
			}
			return nil
		default:
			return err
		}
		out = trimShards(out, shardSize(in))
		read += shardSize(in)
		err = r.r.Encode(all)
		if err != nil {
			return err
		}
		err = r.writeShards(outputs, out)
		if err != nil {
			return err
		}
	}
}

// 修剪分片使它们都具有相同的大小
//
// 参数:
// - in: [][]byte 分片切片
// - size: int 分片大小
// 返回:
// - [][]byte: 修剪后的分片切片
func trimShards(in [][]byte, size int) [][]byte {
	for i := range in {
		if len(in[i]) != 0 {
			in[i] = in[i][0:size]
		}
		if len(in[i]) < size {
			in[i] = in[i][:0]
		}
	}
	return in
}

// readShards 读取分片
//
// 参数:
// - dst: [][]byte 分片切片
// - in: []io.Reader 分片读取器
// 返回:
// - error: 错误
func readShards(dst [][]byte, in []io.Reader) error {
	if len(in) != len(dst) {
		panic("内部错误:in和dst大小不匹配")
	}
	size := -1
	for i := range in {
		if in[i] == nil {
			dst[i] = dst[i][:0]
			continue
		}
		n, err := io.ReadFull(in[i], dst[i])
		// 只有在未读取任何字节时错误才是EOF。
		// 如果在读取一些但不是所有字节后发生EOF,ReadFull返回ErrUnexpectedEOF。
		switch err {
		case io.ErrUnexpectedEOF, io.EOF:
			if size < 0 {
				size = n
			} else if n != size {
				// 分片大小必须匹配。
				return ErrShardSize
			}
			dst[i] = dst[i][0:n]
		case nil:
			continue
		default:
			return StreamReadError{Err: err, Stream: i}
		}
	}
	if size == 0 {
		return io.EOF
	}
	return nil
}

// writeShards 写入分片
//
// 参数:
// - out: []io.Writer 分片写入器
// - in: [][]byte 分片切片
// 返回:
// - error: 错误
func writeShards(out []io.Writer, in [][]byte) error {
	if len(out) != len(in) {
		panic("内部错误:in和out大小不匹配")
	}
	for i := range in {
		if out[i] == nil {
			continue
		}
		n, err := out[i].Write(in[i])
		if err != nil {
			return StreamWriteError{Err: err, Stream: i}
		}
		//
		if n != len(in[i]) {
			return StreamWriteError{Err: io.ErrShortWrite, Stream: i}
		}
	}
	return nil
}

type readResult struct {
	n    int
	size int
	err  error
}

// cReadShards 并发读取分片
//
// 参数:
// - dst: [][]byte 分片切片
// - in: []io.Reader 分片读取器
// 返回:
// - error: 错误
func cReadShards(dst [][]byte, in []io.Reader) error {
	if len(in) != len(dst) {
		panic("内部错误:in和dst大小不匹配")
	}
	var wg sync.WaitGroup
	wg.Add(len(in))
	res := make(chan readResult, len(in))
	for i := range in {
		if in[i] == nil {
			dst[i] = dst[i][:0]
			wg.Done()
			continue
		}
		go func(i int) {
			defer wg.Done()
			n, err := io.ReadFull(in[i], dst[i])
			// 只有在未读取任何字节时错误才是EOF。
			// 如果在读取一些但不是所有字节后发生EOF,ReadFull返回ErrUnexpectedEOF。
			res <- readResult{size: n, err: err, n: i}

		}(i)
	}
	wg.Wait()
	close(res)
	size := -1
	for r := range res {
		switch r.err {
		case io.ErrUnexpectedEOF, io.EOF:
			if size < 0 {
				size = r.size
			} else if r.size != size {
				// 分片大小必须匹配。
				return ErrShardSize
			}
			dst[r.n] = dst[r.n][0:r.size]
		case nil:
		default:
			return StreamReadError{Err: r.err, Stream: r.n}
		}
	}
	if size == 0 {
		return io.EOF
	}
	return nil
}

// cWriteShards 并发写入分片
//
// 参数:
// - out: []io.Writer 分片写入器
// - in: [][]byte 分片切片
// 返回:
// - error: 错误
func cWriteShards(out []io.Writer, in [][]byte) error {
	if len(out) != len(in) {
		panic("内部错误:in和out大小不匹配")
	}
	var errs = make(chan error, len(out))
	var wg sync.WaitGroup
	wg.Add(len(out))
	for i := range in {
		go func(i int) {
			defer wg.Done()
			if out[i] == nil {
				errs <- nil
				return
			}
			n, err := out[i].Write(in[i])
			if err != nil {
				errs <- StreamWriteError{Err: err, Stream: i}
				return
			}
			if n != len(in[i]) {
				errs <- StreamWriteError{Err: io.ErrShortWrite, Stream: i}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

// Verify 如果奇偶校验分片包含正确的数据则返回true。
//
// 分片数量必须与传给NewStream()的数据+奇偶校验分片总数匹配。
//
// 每个读取器必须提供相同数量的字节。
// 如果分片流返回错误,将返回StreamReadError类型错误。
//
// 参数:
// - shards: []io.Reader 分片读取器
// 返回:
// - bool: 是否包含正确的数据
// - error: 错误
func (r *rsStream) Verify(shards []io.Reader) (bool, error) {
	if len(shards) != r.r.totalShards {
		return false, ErrTooFewShards
	}

	read := 0
	all := r.createSlice()
	defer r.blockPool.Put(all)
	for {
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
		read += shardSize(all)
		ok, err := r.r.Verify(all)
		if !ok || err != nil {
			return ok, err
		}
	}
}

// ErrReconstructMismatch 在StreamEncoder中返回,如果您在同一索引上提供了"valid"和"fill"流。
// 因此无法判断您是否认为该分片有效或希望重建它。
var ErrReconstructMismatch = errors.New("有效分片和填充分片是互斥的")

// Reconstruct 将在可能的情况下重建丢失的分片。
//
// 给定有效分片列表(用于读取)和无效分片列表(用于写入)
//
// 通过在'valid'切片中将其设置为nil并同时在"fill"中设置非nil写入器来指示分片丢失。
// 一个索引不能同时包含非nil的'valid'和'fill'条目。
//
// 如果分片太少而无法重建丢失的分片,将返回ErrTooFewShards。
//
// 当明确要求所有丢失的分片时,重建的分片集是完整的。
// 但是其完整性不会自动验证。
// 如果数据集完整,请使用Verify函数进行检查。
//
// 参数:
// - valid: []io.Reader 有效分片列表
// - fill: []io.Writer 填充分片列表
// 返回:
// - error: 错误
func (r *rsStream) Reconstruct(inputs []io.Reader, outputs []io.Writer) error {
	if len(inputs) != r.r.totalShards {
		return ErrTooFewShards
	}
	if len(outputs) != r.r.totalShards {
		return ErrTooFewShards
	}

	all := r.createSlice()
	defer r.blockPool.Put(all)
	reconDataOnly := true
	for i := range inputs {
		if inputs[i] != nil && outputs[i] != nil {
			return ErrReconstructMismatch
		}
		if i >= r.r.dataShards && outputs[i] != nil {
			reconDataOnly = false
		}
	}

	read := 0
	for {
		err := r.readShards(all, inputs)
		if err == io.EOF {
			if read == 0 {
				return ErrShardNoData
			}
			return nil
		}
		if err != nil {
			return err
		}
		read += shardSize(all)
		all = trimShards(all, shardSize(all))

		if reconDataOnly {
			err = r.r.ReconstructData(all) // 仅重建丢失的数据分片
		} else {
			err = r.r.Reconstruct(all) // 重建所有丢失的分片
		}
		if err != nil {
			return err
		}
		err = r.writeShards(outputs, all)
		if err != nil {
			return err
		}
	}
}

// Join 将分片连接起来并将数据段写入dst。
//
// 只考虑数据分片。
//
// 您必须提供想要的确切输出大小。
// 如果给定的分片太少,将返回ErrTooFewShards。
// 如果总数据大小小于outSize,将返回ErrShortData。
//
// 参数:
// - dst: io.Writer 数据写入器
// - shards: []io.Reader 分片读取器
// - outSize: int64 输出大小
// 返回:
// - error: 错误
func (r *rsStream) Join(dst io.Writer, shards []io.Reader, outSize int64) error {
	// 我们有足够的分片吗?
	if len(shards) < r.r.dataShards {
		return ErrTooFewShards
	}

	// 如果有的话,修剪掉奇偶校验分片
	shards = shards[:r.r.dataShards]
	for i := range shards {
		if shards[i] == nil {
			return StreamReadError{Err: ErrShardNoData, Stream: i}
		}
	}
	// 连接所有分片
	src := io.MultiReader(shards...)

	// 将数据复制到dst
	n, err := io.CopyN(dst, src, outSize)
	if err == io.EOF {
		return ErrShortData
	}
	if err != nil {
		return err
	}
	if n != outSize {
		return ErrShortData
	}
	return nil
}

// Split 将输入流分割成给定给编码器的分片数。
//
// 数据将被分割成大小相等的分片。
// 如果数据大小不能被分片数整除,最后一个分片将包含额外的零。
//
// 您必须提供输入的总大小。
// 如果无法检索指定的字节数,将返回'ErrShortData'。
//
// 参数:
// - data: io.Reader 数据读取器
// - dst: []io.Writer 分片写入器
// - size: int64 输入大小
// 返回:
// - error: 错误
func (r *rsStream) Split(data io.Reader, dst []io.Writer, size int64) error {
	if size == 0 {
		return ErrShortData
	}
	if len(dst) != r.r.dataShards {
		return ErrInvShardNum
	}

	for i := range dst {
		if dst[i] == nil {
			return StreamWriteError{Err: ErrShardNoData, Stream: i}
		}
	}

	// 计算每个分片的字节数。
	perShard := (size + int64(r.r.dataShards) - 1) / int64(r.r.dataShards)

	// 将数据填充到r.Shards*perShard。
	paddingSize := (int64(r.r.totalShards) * perShard) - size
	data = io.MultiReader(data, io.LimitReader(zeroPaddingReader{}, paddingSize))

	// 分割成等长分片并复制。
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

type zeroPaddingReader struct{}

var _ io.Reader = &zeroPaddingReader{}

// Read 读取零填充的读者
//
// 参数:
// - p: []byte 数据缓冲区
// 返回:
// - n: int 读取的字节数
// - err: 错误
func (t zeroPaddingReader) Read(p []byte) (n int, err error) {
	n = len(p)
	for i := 0; i < n; i++ {
		p[i] = 0
	}
	return n, nil
}
