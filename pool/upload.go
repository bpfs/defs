package pool

import (
	"fmt"
	"sync"

	"go.uber.org/fx"
)

// UploadTask 表示单个文件的上传状态
type UploadTask struct {
	TotalPieces int                         // 文件总片数
	Progress    BitSet                      // 文件上传进度
	PieceInfo   map[string]*UploadPieceInfo // 每个文件片段的详细信息
	Mu          sync.RWMutex                // 控制对Progress的并发访问
	RetryCounts map[int]int                 // 记录失败重试次数的映射
	Paused      bool                        // 是否暂停下载
}

// UploadPieceInfo 表示单个文件片段的信息
type UploadPieceInfo struct {
	Index  int      // 文件片段的序列号
	PeerID []string // 节点的host地址
}

type NewMemoryPoolOutput struct {
	fx.Out
	Pool *MemoryPool // 文件上传内存池
}

// NewMemoryPool 初始化一个新的文件上传内存池
func NewMemoryPool(lc fx.Lifecycle) (out NewMemoryPoolOutput, err error) {
	out.Pool = &MemoryPool{
		UploadTasks:   make(map[string]*UploadTask),
		DownloadTasks: make(map[string]*DownloadTask),
	}

	return out, nil
}

// AddUploadTask 添加一个新的上传任务
func (pool *MemoryPool) AddUploadTask(fileID string, totalPieces int) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	if _, exists := pool.UploadTasks[fileID]; exists {
		return fmt.Errorf("upload task for fileID '%s' already exists", fileID)
	}

	pool.UploadTasks[fileID] = &UploadTask{
		TotalPieces: totalPieces,
		Progress:    *NewBitSet(totalPieces),
		PieceInfo:   make(map[string]*UploadPieceInfo),
		RetryCounts: make(map[int]int),
		Paused:      false,
	}
	return nil
}

// UpdateUploadPieceInfo 更新上传任务中特定片段的信息
func (pool *MemoryPool) UpdateUploadPieceInfo(fileID string, pieceHash string, pieceInfo *UploadPieceInfo) {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return
	}

	task.Mu.Lock()
	defer task.Mu.Unlock()
	task.PieceInfo[pieceHash] = pieceInfo
}

// MarkUploadPieceComplete 标记上传任务中的一个片段为完成，并返回是否所有片段都已上传
func (pool *MemoryPool) MarkUploadPieceComplete(fileID string, pieceIndex int) bool {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false
	}

	task.Mu.Lock()
	defer task.Mu.Unlock()

	task.Progress.Set(pieceIndex)

	// 检查所有片段是否已上传
	for i := 0; i < task.TotalPieces; i++ {
		if !task.Progress.IsSet(i) {
			return false // 如果有任何片段未上传，则返回 false
		}
	}
	return true // 所有片段已上传
}

// IsUploadComplete 检查指定文件的上传是否完成
func (pool *MemoryPool) IsUploadComplete(fileID string) bool {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()

	for i := 0; i < task.TotalPieces; i++ {
		if !task.Progress.IsSet(i) {
			return false
		}
	}
	return true
}

// GetIncompleteUploadPieces 获取未完成的上传片段
func (pool *MemoryPool) GetIncompleteUploadPieces(fileID string) []string {
	var incompletePieces []string

	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return incompletePieces
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()

	for hash, pieceInfo := range task.PieceInfo {
		if !task.Progress.IsSet(pieceInfo.Index) {
			incompletePieces = append(incompletePieces, hash)
		}
	}
	return incompletePieces
}

// PauseUploadTask 暂停指定的上传任务
func (pool *MemoryPool) PauseUploadTask(fileID string) error {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("upload task not found: %s", fileID)
	}

	task.Mu.Lock()
	task.Paused = true
	task.Mu.Unlock()

	return nil
}

// ResumeUploadTask 恢复指定的上传任务
func (pool *MemoryPool) ResumeUploadTask(fileID string) error {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("upload task not found: %s", fileID)
	}

	task.Mu.Lock()
	task.Paused = false
	task.Mu.Unlock()

	return nil
}

// IsUploadTaskPaused 检查指定的上传任务是否已暂停
func (pool *MemoryPool) IsUploadTaskPaused(fileID string) (bool, error) {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("upload task not found: %s", fileID)
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()
	return task.Paused, nil
}

// DeleteUploadTask 删除指定文件的上传任务
func (pool *MemoryPool) DeleteUploadTask(fileID string) {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()
	delete(pool.UploadTasks, fileID)
}
