// Package tempfile 提供临时文件的管理功能,包括文件的创建、读写、缓存和清理
package tempfile

import (
	"bytes"
	"sync"
)

// 缓冲区大小定义(单位:字节)
const (
	_4KB  = 4 << 10  // 4KB 适用于小数据块
	_32KB = 32 << 10 // 32KB 适用于中等数据块
	_1MB  = 1 << 20  // 1MB 适用于大数据块
	_4MB  = 4 << 20  // 4MB 适用于超大数据块
)

// BufferPool 提供多级缓冲区池,通过复用不同大小的缓冲区来减少内存分配
type BufferPool struct {
	tiny   sync.Pool // 4KB 适用于小数据
	small  sync.Pool // 32KB 适用于一般数据
	medium sync.Pool // 1MB 适用于大数据
	large  sync.Pool // 4MB 适用于超大数据
}

// NewBufferPool 创建并初始化一个新的缓冲区池
// 返回值:
//   - *BufferPool: 初始化完成的缓冲区池
func NewBufferPool() *BufferPool {
	return &BufferPool{
		tiny: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, _4KB))
			},
		},
		small: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, _32KB))
			},
		},
		medium: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, _1MB))
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, _4MB))
			},
		},
	}
}

// Get 根据数据大小获取合适的缓冲区
// 参数:
//   - size: 需要的缓冲区大小(字节)
//
// 返回值:
//   - *bytes.Buffer: 获取到的缓冲区,容量>=size
func (p *BufferPool) Get(size int64) *bytes.Buffer {
	switch {
	case size <= _4KB:
		return p.tiny.Get().(*bytes.Buffer)
	case size <= _32KB:
		return p.small.Get().(*bytes.Buffer)
	case size <= _1MB:
		return p.medium.Get().(*bytes.Buffer)
	default:
		return p.large.Get().(*bytes.Buffer)
	}
}

// Put 将使用完的缓冲区归还到池中
// 参数:
//   - buf: 要归还的缓冲区
func (p *BufferPool) Put(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	buf.Reset()

	switch buf.Cap() {
	case _4KB:
		p.tiny.Put(buf)
	case _32KB:
		p.small.Put(buf)
	case _1MB:
		p.medium.Put(buf)
	case _4MB:
		p.large.Put(buf)
	}
}
