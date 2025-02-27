package uploads

import (
	"bytes"
	"compress/gzip"
	"sync"

	"github.com/bpfs/defs/v2/pb"
)

// ShardProcessor 分片处理器
type ShardProcessor struct {
	sync.RWMutex                         // 读写锁
	hashTables   map[int64]*pb.HashTable // 分片哈希表
	chunkPool    *sync.Pool              // 处理块池
	compressPool *sync.Pool              // 压缩上下文池
}

// NewShardProcessor 创建分片处理器
// 返回值:
//   - *ShardProcessor: 分片处理器实例
func NewShardProcessor() *ShardProcessor {
	return &ShardProcessor{
		hashTables: make(map[int64]*pb.HashTable),
		chunkPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 1<<20) // 1MB chunks
			},
		},
		compressPool: &sync.Pool{
			New: func() interface{} {
				return &CompressContext{
					buffer: bytes.NewBuffer(make([]byte, 0, 1<<20)),
					writer: gzip.NewWriter(nil),
				}
			},
		},
	}
}

// CompressContext 压缩上下文
type CompressContext struct {
	writer *gzip.Writer  // 压缩写入器
	buffer *bytes.Buffer // 压缩缓冲区
}

// Reset 重置压缩上下文
func (cc *CompressContext) Reset() {
	cc.buffer.Reset()
	cc.writer.Reset(cc.buffer)
}

// UpdateHashTable 更新分片哈希表
// 参数:
//   - index: int64 分片索引
//   - table: *pb.HashTable 分片哈希表
func (sp *ShardProcessor) UpdateHashTable(index int64, table *pb.HashTable) {
	sp.Lock()
	defer sp.Unlock()
	sp.hashTables[index] = table
}

// GetHashTables 获取所有分片哈希表
// 返回值:
//   - map[int64]*pb.HashTable: 分片哈希表
func (sp *ShardProcessor) GetHashTables() map[int64]*pb.HashTable {
	sp.RLock()
	defer sp.RUnlock()

	// 创建副本避免并发访问
	result := make(map[int64]*pb.HashTable, len(sp.hashTables))
	for k, v := range sp.hashTables {
		result[k] = v
	}
	return result
}

// GetChunk 获取处理块
// 返回值:
//   - []byte: 处理块
func (sp *ShardProcessor) GetChunk() []byte {
	return sp.chunkPool.Get().([]byte)
}

// PutChunk 归还处理块
// 参数:
//   - chunk: []byte 处理块
func (sp *ShardProcessor) PutChunk(chunk []byte) {
	sp.chunkPool.Put(chunk)
}

// GetCompressContext 获取压缩上下文
// 返回值:
//   - *CompressContext: 压缩上下文
func (sp *ShardProcessor) GetCompressContext() *CompressContext {
	ctx := sp.compressPool.Get().(*CompressContext)
	ctx.Reset()
	return ctx
}

// PutCompressContext 归还压缩上下文
// 参数:
//   - ctx: *CompressContext 压缩上下文
func (sp *ShardProcessor) PutCompressContext(ctx *CompressContext) {
	sp.compressPool.Put(ctx)
}
