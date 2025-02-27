package uploads

import (
	"bytes"
	"io"
	"os"
)

// ShardWriter 提供分片数据的流式写入功能
type ShardWriter struct {
	file     *os.File      // 目标文件
	buffer   *bytes.Buffer // 写入缓冲区
	index    int           // 分片索引
	position int64         // 当前位置
}

// NewShardWriter 创建新的分片写入器
// 参数:
//   - file: 目标文件
//   - index: 分片索引
//
// 返回值:
//   - *ShardWriter: 分片写入器
func NewShardWriter(file *os.File, index int) *ShardWriter {
	return &ShardWriter{
		file:   file,
		buffer: Global().GetCompressBuffer(), // 使用资源池
		index:  index,
	}
}

// Write 实现io.Writer接口,提供流式写入
// 参数:
//   - p: 要写入的数据
//
// 返回值:
//   - n: 写入的字节数
//   - err: 写入错误
func (sw *ShardWriter) Write(p []byte) (n int, err error) {
	n, err = sw.buffer.Write(p)
	sw.position += int64(n)

	// 使用统一的缓冲区大小常量
	if sw.buffer.Len() >= processChunkSize {
		if err := sw.flush(); err != nil {
			logger.Errorf("刷新分片缓冲区失败: index=%d position=%d err=%v",
				sw.index, sw.position, err)
			return n, err
		}
	}
	return n, err
}

// Close 关闭写入器
// 返回值:
//   - error: 关闭错误
func (sw *ShardWriter) Close() error {
	err := sw.flush()
	Global().PutCompressBuffer(sw.buffer) // 归还到资源池
	return err
}

// ProcessFunc 定义数据处理函数类型
type ProcessFunc func([]byte) ([]byte, error)

// StreamProcessor 提供流式数据处理
type StreamProcessor struct {
	reader      io.Reader
	writer      io.Writer
	buffer      []byte
	processors  []ProcessFunc
	compressCtx *CompressContext
}

// NewStreamProcessor 创建新的流式处理器
// 参数:
//   - reader: 输入读取器
//   - writer: 输出写入器
//   - processors: 数据处理函数列表
//
// 返回值:
//   - *StreamProcessor: 流式处理器
func NewStreamProcessor(reader io.Reader, writer io.Writer, processors []ProcessFunc) *StreamProcessor {
	sp := &StreamProcessor{
		reader:      reader,
		writer:      writer,
		buffer:      Global().GetStreamBuffer(),
		processors:  processors,
		compressCtx: Global().GetCompressContext(),
	}
	return sp
}

// Process 执行流式处理
// 返回值:
//   - error: 处理错误
func (sp *StreamProcessor) Process() error {
	defer func() {
		Global().PutStreamBuffer(sp.buffer)
		Global().PutCompressContext(sp.compressCtx)
	}()

	for {
		// 读取固定大小的块
		n, err := sp.reader.Read(sp.buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorf("读取数据失败: %v", err)
			return err
		}

		// 处理这个块
		chunk := sp.buffer[:n]
		for _, proc := range sp.processors {
			chunk, err = proc(chunk)
			if err != nil {
				logger.Errorf("处理数据失败: %v", err)
				return err
			}
		}

		// 写入处理后的数据
		if _, err := sp.writer.Write(chunk); err != nil {
			logger.Errorf("写入数据失败: %v", err)
			return err
		}
	}
	return nil
}

// flush 将缓冲区数据写入文件
// 返回值:
//   - error: 写入错误
func (sw *ShardWriter) flush() error {
	if sw.buffer.Len() > 0 {
		_, err := sw.buffer.WriteTo(sw.file)
		return err
	}
	return nil
}
