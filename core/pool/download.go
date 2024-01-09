package pool

import (
	"fmt"
	"sync"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/reedsolomon"
)

// DownloadTask 表示单个文件的下载状态
type DownloadTask struct {
	FileID       string                     // 文件的唯一标识
	FileKey      string                     // 文件的密钥
	FileHash     []byte                     // 文件的哈希值(用于文件防篡改)
	Name         string                     // 文件的名称
	Size         int64                      // 文件的长度(以字节为单位)
	TotalPieces  int                        // 文件总片数（数据片段和纠删码片段的总数）
	DataPieces   int                        // 数据片段的数量
	Progress     BitSet                     // 文件下载进度
	PieceInfo    map[int]*DownloadPieceInfo // 存储每个文件片段的哈希和对应节点ID
	ENC          reedsolomon.Encoder        // 纠删码编码器
	Mu           sync.RWMutex               // 控制对Progress的并发访问
	Paused       bool                       // 是否暂停上传
	IsMerged     bool                       // 标识文件是否已经合并
	MergeCounter int                        // 用于跟踪文件合并操作的计数器
}

// DownloadPieceInfo 表示单个文件片段的信息和对应的节点ID
type DownloadPieceInfo struct {
	Hash      string   // 文件片段的哈希值
	PeerID    []string // 该片段对应的节点ID
	IsRSCodes bool     // 是否为纠删码
}

// AddDownloadTask 添加一个新的下载任务。如果任务已存在，返回错误。
func (pool *MemoryPool) AddDownloadTask(fileID, fileKey string) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	if _, exists := pool.DownloadTasks[fileID]; exists {
		return fmt.Errorf("download task for fileID '%s' already exists", fileID)
	}

	newTask := &DownloadTask{
		FileID:      fileID,
		FileKey:     fileKey,
		TotalPieces: 0, // 初始时未知
		DataPieces:  0, // 初始时未知
		Progress:    *NewBitSet(0),
		PieceInfo:   make(map[int]*DownloadPieceInfo),
		Paused:      false,
	}

	pool.DownloadTasks[fileID] = newTask
	return nil
}

// UpdateDownloadPieceInfo 用于更新下载任务中特定片段的节点信息。
// fileID 是文件的唯一标识。
// sliceTable 是文件片段的哈希表，其中 key 是文件片段的序号，value 是文件片段的哈希。
// peerID 是存储文件片段的节点ID。
// fileKey 是文件的密钥，如果提供则更新。
func (pool *MemoryPool) UpdateDownloadPieceInfo(peerID string, fileID, name string, size int64, sliceTable map[int]core.HashTable, pieceHashes map[int]string, fileKey ...string) {
	pool.Mu.RLock()
	downloadTask, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return
	}

	downloadTask.Mu.Lock()
	defer downloadTask.Mu.Unlock()

	// 初始化片段信息
	if downloadTask.TotalPieces == 0 {
		downloadTask.TotalPieces = len(sliceTable) // 文件总片数
		downloadTask.Progress = *NewBitSet(downloadTask.TotalPieces)

		// 计算数据片段的数量，不是纠删码的数据片段
		var dataPieceCount int
		for _, hashTable := range sliceTable {
			if !hashTable.IsRsCodes {
				dataPieceCount++
			}
		}
		downloadTask.DataPieces = dataPieceCount

		for index, hashTable := range sliceTable {
			downloadTask.PieceInfo[index] = &DownloadPieceInfo{
				Hash:      hashTable.Hash,
				IsRSCodes: hashTable.IsRsCodes,
			}
		}

		// 如果提供了文件哈希值且当前任务尚未设置文件哈希值，则设置它
		if downloadTask.FileKey == "" && len(fileKey) > 0 {
			downloadTask.FileKey = fileKey[0]
		}

		downloadTask.Name = name // 文件的基本名称
		downloadTask.Size = size // 文件的长度(以字节为单位)

		// 初始化纠删码编码器
		downloadTask.ENC, _ = reedsolomon.New(downloadTask.DataPieces, downloadTask.TotalPieces-downloadTask.DataPieces)
	}

	// 更新每个文件片段的节点信息
	for _, hash := range pieceHashes {
		for index, piece := range downloadTask.PieceInfo {
			if piece.Hash == hash {
				downloadTask.PieceInfo[index].PeerID = append(downloadTask.PieceInfo[index].PeerID, peerID)
			}
		}
	}
}

// MarkDownloadPieceComplete 标记下载任务中的一个片段为完成，并返回是否所有片段都已下载
func (pool *MemoryPool) MarkDownloadPieceComplete(fileID string, pieceIndex int) bool {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false
	}

	task.Mu.Lock()
	defer task.Mu.Unlock()

	task.Progress.Set(pieceIndex)

	// 计算已下载的片段数量
	downloadedPieces := 0
	for i := 0; i < task.TotalPieces; i++ {
		if task.Progress.IsSet(i) {
			downloadedPieces++
		}
	}
	return downloadedPieces >= task.DataPieces // 检查是否达到数据片段的数量
}

// MarkDownloadPieceCompleteByHash 根据文件片段的哈希值标记下载任务中的一个片段为完成，并返回是否所有片段都已下载
func (pool *MemoryPool) MarkDownloadPieceCompleteByHash(fileID, pieceHash string) bool {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false
	}

	task.Mu.Lock()
	defer task.Mu.Unlock()

	// 查找哈希值对应的片段索引
	found := false
	for index, pieceInfo := range task.PieceInfo {
		if pieceInfo.Hash == pieceHash {
			task.Progress.Set(index) // 更新进度
			found = true
			break
		}
	}

	if !found {
		return false // 如果没有找到对应的哈希值，则返回 false
	}

	// 计算已下载的片段数量
	downloadedPieces := 0
	for i := 0; i < task.TotalPieces; i++ {
		if task.Progress.IsSet(i) {
			downloadedPieces++
		}
	}
	return downloadedPieces >= task.DataPieces // 检查是否达到数据片段的数量
}

// IsDownloadComplete 检查指定文件的下载是否完成
func (pool *MemoryPool) IsDownloadComplete(fileID string) bool {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()

	// 计算已下载的片段数量
	downloadedPieces := 0
	for i := 0; i < task.TotalPieces; i++ {
		if task.Progress.IsSet(i) {
			downloadedPieces++
		}
	}
	return downloadedPieces >= task.DataPieces // 检查是否达到数据片段的数量
}

// GetIncompleteDownloadPieces 获取未完成的下载片段的哈希值
func (pool *MemoryPool) GetIncompleteDownloadPieces(fileID string) []string {
	var incompletePieceHashes []string

	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return incompletePieceHashes
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()

	downloadedPieces := 0
	for i := 0; i < task.TotalPieces; i++ {
		if task.Progress.IsSet(i) {
			downloadedPieces++
		}
	}

	// 如果已下载的片段数量已足够进行数据恢复，则返回空列表
	if downloadedPieces >= task.DataPieces {
		return incompletePieceHashes
	}

	// 返回尚未下载的数据片段的哈希值
	for index, pieceInfo := range task.PieceInfo {
		if !pieceInfo.IsRSCodes && !task.Progress.IsSet(index) {
			incompletePieceHashes = append(incompletePieceHashes, pieceInfo.Hash)
		}
	}

	// 如果需要，还可以返回尚未下载的纠删码片段的哈希值
	for index, pieceInfo := range task.PieceInfo {
		if pieceInfo.IsRSCodes && !task.Progress.IsSet(index) {
			incompletePieceHashes = append(incompletePieceHashes, pieceInfo.Hash)
		}
	}

	return incompletePieceHashes
}

// ResetDownloadTask 清除下载任务的所有进度
func (pool *MemoryPool) ResetDownloadTask(fileID string) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	if _, exists := pool.DownloadTasks[fileID]; !exists {
		return fmt.Errorf("download task not found: %s", fileID)
	}

	// 重置下载任务
	pool.DownloadTasks[fileID] = &DownloadTask{
		TotalPieces: 0,
		Progress:    *NewBitSet(0),
		PieceInfo:   make(map[int]*DownloadPieceInfo),
		Paused:      false,
	}
	return nil
}

// RevertDownloadPieceProgress 回退单个文件片段的下载进度
func (pool *MemoryPool) RevertDownloadPieceProgress(fileID, pieceHash string) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	task, exists := pool.DownloadTasks[fileID]
	if !exists {
		return fmt.Errorf("download task not found: %s", fileID)
	}

	for index, pieceInfo := range task.PieceInfo {
		if pieceInfo.Hash == pieceHash {
			task.Progress.Clear(index) // 清除该片段的进度
			return nil
		}
	}
	return fmt.Errorf("piece hash not found in download task: %s", pieceHash)
}

// PauseDownloadTask 暂停指定的下载任务
func (pool *MemoryPool) PauseDownloadTask(fileID string) error {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("download task not found: %s", fileID)
	}

	task.Mu.Lock()
	task.Paused = true
	task.Mu.Unlock()

	return nil
}

// ResumeDownloadTask 恢复指定的下载任务
func (pool *MemoryPool) ResumeDownloadTask(fileID string) error {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("download task not found: %s", fileID)
	}

	task.Mu.Lock()
	task.Paused = false
	task.Mu.Unlock()

	return nil
}

// IsDownloadTaskPaused 检查指定的下载任务是否已暂停
func (pool *MemoryPool) IsDownloadTaskPaused(fileID string) (bool, error) {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[fileID]
	pool.Mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("download task not found: %s", fileID)
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()
	return task.Paused, nil
}

// DeleteDownloadTask 删除指定文件的下载任务
func (pool *MemoryPool) DeleteDownloadTask(fileID string) {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()
	delete(pool.DownloadTasks, fileID)
}
