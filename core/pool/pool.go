package pool

import (
	"sync"
)

// MemoryPool 定义了文件上传和下载的内存池
type MemoryPool struct {
	UploadTasks   map[string]*UploadTask   // 上传任务池
	DownloadTasks map[string]*DownloadTask // 下载任务池
	DeleteTasks   map[string]*DeleteTask   // 删除任务池
	Mu            sync.RWMutex             // 读写互斥锁
}

// DeleteTask 表示单个文件的删除状态
type DeleteTask struct {
	FileID    string                   // 文件的唯一标识
	PieceInfo map[int]*DeletePieceInfo // 存储每个文件片段的哈希和对应节点ID
	Mu        sync.RWMutex             // 控制对Progress的并发访问
}

// DeletePieceInfo 表示单个文件片段的信息
type DeletePieceInfo struct {
	Hash   string          // 文件片段的哈希值
	PeerID map[string]bool // 节点是否已经删除该片段对应
}

// BitSet 实现
type BitSet struct {
	bits []byte
}

func NewBitSet(size int) *BitSet {
	return &BitSet{bits: make([]byte, (size+7)/8)}
}

func (b *BitSet) Set(i int) {
	b.bits[i/8] |= 1 << (i % 8)
}

func (b *BitSet) Clear(i int) {
	b.bits[i/8] &^= 1 << (i % 8)
}

func (b *BitSet) IsSet(i int) bool {
	return (b.bits[i/8] & (1 << (i % 8))) != 0
}
