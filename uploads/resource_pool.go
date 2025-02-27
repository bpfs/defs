package uploads

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"runtime"
	"sync"
)

// ResourcePool 资源池管理器
type ResourcePool struct {
	// 大缓冲区池 (用于文件分片)
	largeBufferPool sync.Pool

	// 流处理缓冲区池
	streamBufferPool sync.Pool

	// IO读写器池
	readerPool sync.Pool
	writerPool sync.Pool

	// 压缩相关池
	gzipWriterPool     sync.Pool
	compressBufferPool sync.Pool
}

var (
	// 全局资源池实例
	globalPool = NewResourcePool()
)

// NewResourcePool 创建新的资源池
// 返回值:
//   - *ResourcePool: 资源池实例
func NewResourcePool() *ResourcePool {
	rp := &ResourcePool{}

	// 初始化大缓冲区池
	rp.largeBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, largeBufferSize)
		},
	}

	// 初始化流处理缓冲区池
	rp.streamBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, processChunkSize)
		},
	}

	// 初始化IO读写器池
	rp.readerPool = sync.Pool{
		New: func() interface{} {
			return bufio.NewReaderSize(nil, defaultBufferSize)
		},
	}
	rp.writerPool = sync.Pool{
		New: func() interface{} {
			return bufio.NewWriterSize(nil, defaultBufferSize)
		},
	}

	// 初始化压缩相关池
	rp.gzipWriterPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(nil)
		},
	}
	rp.compressBufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, processChunkSize))
		},
	}

	return rp
}

// GetLargeBuffer 获取大缓冲区
// 返回值:
//   - []byte: 大缓冲区
func (rp *ResourcePool) GetLargeBuffer() []byte {
	return rp.largeBufferPool.Get().([]byte)
}

// PutLargeBuffer 归还大缓冲区
// 参数:
//   - buf: []byte 大缓冲区
func (rp *ResourcePool) PutLargeBuffer(buf []byte) {
	rp.largeBufferPool.Put(buf)
}

// GetStreamBuffer 获取流处理缓冲区
// 返回值:
//   - []byte: 流处理缓冲区
func (rp *ResourcePool) GetStreamBuffer() []byte {
	return rp.streamBufferPool.Get().([]byte)
}

// PutStreamBuffer 归还流处理缓冲区
// 参数:
//   - buf: []byte 流处理缓冲区
func (rp *ResourcePool) PutStreamBuffer(buf []byte) {
	rp.streamBufferPool.Put(buf)
}

// GetReader 获取带缓冲的Reader
// 参数:
//   - r: io.Reader 读取器
//
// 返回值:
//   - *bufio.Reader: 带缓冲的读取器
func (rp *ResourcePool) GetReader(r io.Reader) *bufio.Reader {
	reader := rp.readerPool.Get().(*bufio.Reader)
	reader.Reset(r)
	return reader
}

// PutReader 归还Reader
// 参数:
//   - r: *bufio.Reader 带缓冲的读取器
func (rp *ResourcePool) PutReader(r *bufio.Reader) {
	r.Reset(nil)
	rp.readerPool.Put(r)
}

// GetWriter 获取带缓冲的Writer
// 参数:
//   - w: io.Writer 写入器
//
// 返回值:
//   - *bufio.Writer: 带缓冲的写入器
func (rp *ResourcePool) GetWriter(w io.Writer) *bufio.Writer {
	writer := rp.writerPool.Get().(*bufio.Writer)
	writer.Reset(w)
	return writer
}

// PutWriter 归还Writer
// 参数:
//   - w: *bufio.Writer 带缓冲的写入器
func (rp *ResourcePool) PutWriter(w *bufio.Writer) {
	w.Reset(nil)
	rp.writerPool.Put(w)
}

// GetCompressContext 获取压缩上下文
// 返回值:
//   - *CompressContext: 压缩上下文
func (rp *ResourcePool) GetCompressContext() *CompressContext {
	buf := rp.compressBufferPool.Get().(*bytes.Buffer)
	gw := rp.gzipWriterPool.Get().(*gzip.Writer)

	buf.Reset()
	gw.Reset(buf)

	return &CompressContext{
		buffer: buf,
		writer: gw,
	}
}

// PutCompressContext 归还压缩上下文
// 参数:
//   - ctx: *CompressContext 压缩上下文
func (rp *ResourcePool) PutCompressContext(ctx *CompressContext) {
	ctx.writer.Close()
	rp.gzipWriterPool.Put(ctx.writer)
	rp.compressBufferPool.Put(ctx.buffer)
}

// GetCompressBuffer 获取压缩缓冲区
// 返回值:
//   - *bytes.Buffer: 压缩缓冲区
func (rp *ResourcePool) GetCompressBuffer() *bytes.Buffer {
	return rp.compressBufferPool.Get().(*bytes.Buffer)
}

// PutCompressBuffer 归还压缩缓冲区
// 参数:
//   - buf: *bytes.Buffer 压缩缓冲区
func (rp *ResourcePool) PutCompressBuffer(buf *bytes.Buffer) {
	buf.Reset()
	rp.compressBufferPool.Put(buf)
}

// Global 获取全局资源池实例
// 返回值:
//   - *ResourcePool: 全局资源池实例
func Global() *ResourcePool {
	return globalPool
}

// ResetPools 重置所有资源池
func (p *ResourcePool) ResetPools() {
	// 创建新的池替换旧的
	p.largeBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, largeBufferSize)
		},
	}

	p.streamBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, processChunkSize)
		},
	}

	p.readerPool = sync.Pool{
		New: func() interface{} {
			return bufio.NewReaderSize(nil, defaultBufferSize)
		},
	}

	p.writerPool = sync.Pool{
		New: func() interface{} {
			return bufio.NewWriterSize(nil, defaultBufferSize)
		},
	}

	p.gzipWriterPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(nil)
		},
	}

	p.compressBufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, processChunkSize))
		},
	}

	// 强制GC回收旧的池对象
	runtime.GC()
}

// initPools 初始化所有资源池
// func (p *ResourcePool) initPools() {
// 	// 原有的初始化代码...
// }
