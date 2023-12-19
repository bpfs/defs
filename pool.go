package defs

import (
	"fmt"
	"sync"

	"go.uber.org/fx"
)

// MemoryPool 定义了文件上传和下载的内存池
type MemoryPool struct {
	UploadTasks   map[string]*UploadTask   // 上传任务池
	DownloadTasks map[string]*DownloadTask // 下载任务池
	Mu            sync.RWMutex             // 读写互斥锁
}

// UploadTask 表示单个文件资产的上传状态
type UploadTask struct {
	TotalPieces int                         // 文件总片数
	Progress    BitSet                      // 文件上传进度
	PieceInfo   map[string]*UploadPieceInfo // 每个文件片段的详细信息
	Mu          sync.RWMutex                // 控制对Progress的并发访问
	RetryCounts map[int]int                 // 记录失败重试次数的映射
	Paused      bool                        // 是否暂停上传
}

// UploadPieceInfo 表示单个文件片段的信息
type UploadPieceInfo struct {
	Index  int      // 文件片段的序列号
	PeerID []string // 节点的host地址
}

// DownloadTask 表示单个文件资产的下载状态
type DownloadTask struct {
	FileHash    string                     // 文件内容的哈希值(内部标识)
	Name        string                     // 文件的基本名称
	Size        int64                      // 常规文件的长度(以字节为单位)
	TotalPieces int                        // 文件总片数（数据片段和纠删码片段的总数）
	DataPieces  int                        // 数据片段的数量
	Progress    BitSet                     // 文件下载进度
	PieceInfo   map[int]*DownloadPieceInfo // 存储每个文件片段的哈希和对应节点ID
	Mu          sync.RWMutex               // 控制对Progress的并发访问
	Paused      bool                       // 是否暂停上传
}

// DownloadPieceInfo 表示单个文件片段的信息和对应的节点ID
type DownloadPieceInfo struct {
	Hash    string   // 文件片段的哈希值
	PeerID  []string // 该片段对应的节点ID
	RSCodes bool     // 是否为纠删码
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

// NewMemoryPool 初始化一个新的内存池
// func NewMemoryPool() *MemoryPool {
// 	return &MemoryPool{
// 		UploadTasks:   make(map[string]*UploadTask),
// 		DownloadTasks: make(map[string]*DownloadTask),
// 	}
// }

// AddUploadTask 添加一个新的上传任务
func (pool *MemoryPool) AddUploadTask(assetID string, totalPieces int) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	if _, exists := pool.UploadTasks[assetID]; exists {
		return fmt.Errorf("upload task for assetID '%s' already exists", assetID)
	}

	pool.UploadTasks[assetID] = &UploadTask{
		TotalPieces: totalPieces,
		Progress:    *NewBitSet(totalPieces),
		PieceInfo:   make(map[string]*UploadPieceInfo),
		RetryCounts: make(map[int]int),
		Paused:      false,
	}
	return nil
}

// UpdateUploadPieceInfo 更新上传任务中特定片段的信息
func (pool *MemoryPool) UpdateUploadPieceInfo(assetID string, pieceHash string, pieceInfo *UploadPieceInfo) {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return
	}

	task.Mu.Lock()
	defer task.Mu.Unlock()
	task.PieceInfo[pieceHash] = pieceInfo
}

// MarkUploadPieceComplete 标记上传任务中的一个片段为完成，并返回是否所有片段都已上传
func (pool *MemoryPool) MarkUploadPieceComplete(assetID string, pieceIndex int) bool {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
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

// IsUploadComplete 检查指定文件资产的上传是否完成
func (pool *MemoryPool) IsUploadComplete(assetID string) bool {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
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
func (pool *MemoryPool) GetIncompleteUploadPieces(assetID string) []string {
	var incompletePieces []string

	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
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
func (pool *MemoryPool) PauseUploadTask(assetID string) error {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("upload task not found: %s", assetID)
	}

	task.Mu.Lock()
	task.Paused = true
	task.Mu.Unlock()

	return nil
}

// ResumeUploadTask 恢复指定的上传任务
func (pool *MemoryPool) ResumeUploadTask(assetID string) error {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("upload task not found: %s", assetID)
	}

	task.Mu.Lock()
	task.Paused = false
	task.Mu.Unlock()

	return nil
}

// IsUploadTaskPaused 检查指定的上传任务是否已暂停
func (pool *MemoryPool) IsUploadTaskPaused(assetID string) (bool, error) {
	pool.Mu.RLock()
	task, exists := pool.UploadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("upload task not found: %s", assetID)
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()
	return task.Paused, nil
}

// DeleteUploadTask 删除指定资产的上传任务
func (pool *MemoryPool) DeleteUploadTask(assetID string) {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()
	delete(pool.UploadTasks, assetID)
}

// AddDownloadTask 添加一个新的下载任务。如果任务已存在，返回错误。
// 可选参数 fileHash 用于指定文件内容的哈希值。
func (pool *MemoryPool) AddDownloadTask(assetID string, fileHash ...string) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	if _, exists := pool.DownloadTasks[assetID]; exists {
		return fmt.Errorf("download task for assetID '%s' already exists", assetID)
	}

	newTask := &DownloadTask{
		TotalPieces: 0, // 初始时未知
		DataPieces:  0, // 初始时未知
		Progress:    *NewBitSet(0),
		PieceInfo:   make(map[int]*DownloadPieceInfo),
		Paused:      false,
	}

	// 如果提供了文件哈希值，设置它
	if len(fileHash) > 0 {
		newTask.FileHash = fileHash[0]
	}

	pool.DownloadTasks[assetID] = newTask
	return nil
}

// UpdateDownloadPieceInfo 用于更新下载任务中特定片段的节点信息。
// assetID 是文件资产的唯一标识。
// sliceTable 是文件片段的哈希表，其中 key 是文件片段的序号，value 是文件片段的哈希。
// peerID 是存储文件片段的节点ID。
// fileHash 是文件内容的哈希值，如果提供则更新。
func (pool *MemoryPool) UpdateDownloadPieceInfo(peerID string, assetID, name string, size int64, sliceTable map[int]HashTable, pieceHashes map[int]string, fileHash ...string) {
	pool.Mu.RLock()
	downloadTask, exists := pool.DownloadTasks[assetID]
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
			if !hashTable.RsCodes {
				dataPieceCount++
			}
		}
		downloadTask.DataPieces = dataPieceCount

		for index, hashTable := range sliceTable {
			downloadTask.PieceInfo[index] = &DownloadPieceInfo{
				Hash:    hashTable.Hash,
				RSCodes: hashTable.RsCodes,
			}
		}

		// 如果提供了文件哈希值且当前任务尚未设置文件哈希值，则设置它
		if downloadTask.FileHash == "" && len(fileHash) > 0 {
			downloadTask.FileHash = fileHash[0]
		}

		downloadTask.Name = name // 文件的基本名称
		downloadTask.Size = size // 常规文件的长度(以字节为单位)
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
func (pool *MemoryPool) MarkDownloadPieceComplete(assetID string, pieceIndex int) bool {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
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
func (pool *MemoryPool) MarkDownloadPieceCompleteByHash(assetID, pieceHash string) bool {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
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

// IsDownloadComplete 检查指定文件资产的下载是否完成
func (pool *MemoryPool) IsDownloadComplete(assetID string) bool {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
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
func (pool *MemoryPool) GetIncompleteDownloadPieces(assetID string) []string {
	var incompletePieceHashes []string

	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
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
		if !pieceInfo.RSCodes && !task.Progress.IsSet(index) {
			incompletePieceHashes = append(incompletePieceHashes, pieceInfo.Hash)
		}
	}

	// 如果需要，还可以返回尚未下载的纠删码片段的哈希值
	for index, pieceInfo := range task.PieceInfo {
		if pieceInfo.RSCodes && !task.Progress.IsSet(index) {
			incompletePieceHashes = append(incompletePieceHashes, pieceInfo.Hash)
		}
	}

	return incompletePieceHashes
}

// ResetDownloadTask 清除下载任务的所有进度
func (pool *MemoryPool) ResetDownloadTask(assetID string) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	if _, exists := pool.DownloadTasks[assetID]; !exists {
		return fmt.Errorf("download task not found: %s", assetID)
	}

	// 重置下载任务
	pool.DownloadTasks[assetID] = &DownloadTask{
		TotalPieces: 0,
		Progress:    *NewBitSet(0),
		PieceInfo:   make(map[int]*DownloadPieceInfo),
		Paused:      false,
	}
	return nil
}

// RevertDownloadPieceProgress 回退单个文件片段的下载进度
func (pool *MemoryPool) RevertDownloadPieceProgress(assetID, pieceHash string) error {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()

	task, exists := pool.DownloadTasks[assetID]
	if !exists {
		return fmt.Errorf("download task not found: %s", assetID)
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
func (pool *MemoryPool) PauseDownloadTask(assetID string) error {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("download task not found: %s", assetID)
	}

	task.Mu.Lock()
	task.Paused = true
	task.Mu.Unlock()

	return nil
}

// ResumeDownloadTask 恢复指定的下载任务
func (pool *MemoryPool) ResumeDownloadTask(assetID string) error {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("download task not found: %s", assetID)
	}

	task.Mu.Lock()
	task.Paused = false
	task.Mu.Unlock()

	return nil
}

// IsDownloadTaskPaused 检查指定的下载任务是否已暂停
func (pool *MemoryPool) IsDownloadTaskPaused(assetID string) (bool, error) {
	pool.Mu.RLock()
	task, exists := pool.DownloadTasks[assetID]
	pool.Mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("download task not found: %s", assetID)
	}

	task.Mu.RLock()
	defer task.Mu.RUnlock()
	return task.Paused, nil
}

// DeleteDownloadTask 删除指定资产的下载任务
func (pool *MemoryPool) DeleteDownloadTask(assetID string) {
	pool.Mu.Lock()
	defer pool.Mu.Unlock()
	delete(pool.DownloadTasks, assetID)
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
