// Package uploads 提供文件上传相关的功能实现
package uploads

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/crypto/gcm"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/reedsolomon"

	"bufio"

	"runtime/debug"

	"strings"

	"github.com/bpfs/defs/v2/files/tempfile"

	"hash/crc32"
	"path/filepath"
	"time"
)

const (
	// 上传临时文件前缀
	ProcessSegmentPrefix = "segment_process_" // 处理过程中的分片临时文件前缀
	SegmentDataPrefix    = "segment_data_"    // 分片数据临时文件前缀
)

// SafeHashTableMap 线程安全的哈希表映射
type SafeHashTableMap struct {
	sync.RWMutex
	data map[int64]*pb.HashTable
}

// NewSafeHashTableMap 创建新的线程安全的哈希表映射
// 返回值:
//   - *SafeHashTableMap: 新的哈希表映射
func NewSafeHashTableMap() *SafeHashTableMap {
	return &SafeHashTableMap{
		data: make(map[int64]*pb.HashTable),
	}
}

// Set 设置哈希表映射中的值
// 参数:
//   - index: 索引
//   - table: 哈希表
func (m *SafeHashTableMap) Set(index int64, table *pb.HashTable) {
	m.Lock()
	defer m.Unlock()
	m.data[index] = table
}

// Get 获取哈希表映射中的值
// 参数:
//   - index: 索引
//
// 返回值:
//   - *pb.HashTable: 哈希表
//   - bool: 是否存在
func (m *SafeHashTableMap) Get(index int64) (*pb.HashTable, bool) {
	m.RLock()
	defer m.RUnlock()
	table, ok := m.data[index]
	return table, ok
}

// ToMap 将哈希表映射转换为map
// 返回值:
//   - map[int64]*pb.HashTable: 哈希表映射
func (m *SafeHashTableMap) ToMap() map[int64]*pb.HashTable {
	m.RLock()
	defer m.RUnlock()
	result := make(map[int64]*pb.HashTable, len(m.data))
	for k, v := range m.data {
		result[k] = v
	}
	return result
}

// 临时文件类型常量
const (
	TempFileData     = "data"     // 数据分片临时文件
	TempFileParity   = "parity"   // 校验分片临时文件
	TempFileProcess  = "process"  // 处理过程临时文件
	TempFileCompress = "compress" // 压缩临时文件
)

// TempFileManager 临时文件管理器
type TempFileManager struct {
	files        map[string]*os.File
	mu           sync.RWMutex
	delayCleanup bool   // 是否延迟清理
	taskID       string // 任务ID，用于隔离不同上传任务的临时文件
}

// NewTempFileManager 创建临时文件管理器
// 参数:
//   - delayCleanup: 是否延迟清理
//   - taskID: 任务ID，用于隔离不同上传任务的临时文件
//
// 返回值:
//   - *TempFileManager: 临时文件管理器
func NewTempFileManager(delayCleanup bool, taskID string) *TempFileManager {
	return &TempFileManager{
		files:        make(map[string]*os.File),
		delayCleanup: delayCleanup,
		taskID:       taskID,
	}
}

// CreateTempFile 创建临时文件
// 参数:
//   - fileType: 文件类型
//   - index: 索引
//
// 返回值:
//   - *os.File: 临时文件
//   - error: 创建失败错误
func (tm *TempFileManager) CreateTempFile(fileType string, index int64) (*os.File, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := fmt.Sprintf("%s_%d", fileType, index)

	// 确保上传临时目录存在
	if err := CreateUploadsTempDir(); err != nil {
		return nil, err
	}

	// 生成临时文件路径，添加taskID作为隔离标识
	tempDir := GetUploadsTempDir()
	randNum := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s_%d", key, time.Now().UnixNano())))
	filename := filepath.Join(tempDir, fmt.Sprintf("segment_%s_%s_%d", tm.taskID, key, randNum))

	// 创建文件
	file, err := os.Create(filename)
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return nil, err
	}

	// 保存到映射
	tm.files[key] = file

	return file, nil
}

// GetTaskTempDir 获取任务专用的临时目录
// 返回值:
//   - string: 任务专用的临时目录
func (tm *TempFileManager) GetTaskTempDir() string {
	return filepath.Join(GetUploadsTempDir(), tm.taskID)
}

// CleanupFiles 清理所有临时文件
func (tm *TempFileManager) CleanupFiles() {
	if tm.delayCleanup {
		return // 如果是延迟清理，这里不执行清理
	}
	tm.doCleanup()
}

// ForceCleanup 强制清理所有临时文件
func (tm *TempFileManager) ForceCleanup() {
	tm.doCleanup()
}

// doCleanup 执行实际的清理操作
func (tm *TempFileManager) doCleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, file := range tm.files {
		if file != nil {
			file.Close()
			_ = os.Remove(file.Name())
		}
	}
	tm.files = make(map[string]*os.File)
}

// CleanupFilesByType 根据文件类型清理临时文件
// 参数:
//   - fileType: 文件类型
//
// 返回值:
//   - error: 清理失败错误
func (tm *TempFileManager) CleanupFilesByType(fileType string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for key, file := range tm.files {
		if strings.HasPrefix(key, fileType+"_") {
			if file != nil {
				file.Close()
				_ = os.Remove(file.Name())
				delete(tm.files, key)
			}
		}
	}
}

// CleanupTaskSegmentFiles 清理特定任务的所有临时文件
// 参数:
//   - taskID: 任务ID
//
// 返回值:
//   - error: 清理失败错误
func CleanupTaskSegmentFiles(taskID string) error {
	// 尝试清理该任务的临时文件
	logger.Infof("开始清理任务[%s]的临时文件", taskID)

	// 清理该任务在临时文件管理器中的文件
	tempManager := NewTempFileManager(false, taskID)
	tempManager.ForceCleanup()

	// 在文件系统中查找并清理该任务的其他临时文件
	uploadsDir := GetUploadsTempDir()
	pattern := filepath.Join(uploadsDir, fmt.Sprintf("segment_%s_*", taskID))

	files, err := filepath.Glob(pattern)
	if err != nil {
		logger.Warnf("查找任务临时文件失败: taskID=%s pattern=%s err=%v",
			taskID, pattern, err)
		return err
	}

	// 删除匹配的文件
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			logger.Warnf("删除任务临时文件失败: taskID=%s file=%s err=%v",
				taskID, file, err)
		} else {
			logger.Infof("成功删除任务临时文件: taskID=%s file=%s", taskID, file)
		}
	}

	logger.Infof("任务[%s]临时文件清理完成，共清理%d个文件", taskID, len(files))
	return nil
}

// CleanupSegmentTempFiles 清理所有临时文件（慎用！）
// 返回值:
//   - error: 清理失败错误
func CleanupSegmentTempFiles() error {
	logger.Warn("清理所有上传临时文件，这可能会影响正在进行的上传任务！")

	// 清理整个上传临时目录
	return CleanupUploadsTempDir()
}

// NewFileSegment 创建并初始化一个新的 FileSegment 实例
// 参数:
//   - db: 数据库实例
//   - taskID: 任务ID
//   - fileID: 文件ID
//   - file: 原始文件
//   - pk: 公钥
//   - dataShards: 数据分片数
//   - parityShards: 奇偶校验分片数
//
// 返回值:
//   - error: 如果处理过程中发生错误，返回错误信息
func NewFileSegment(db *badgerhold.Store, taskID string, fileID string, file *os.File, pk []byte, dataShards, parityShards int64) error {
	// 创建不延迟清理的临时文件管理器
	tempManager := NewTempFileManager(false, taskID)
	defer func() {
		tempManager.CleanupFiles() // 确保清理

		// 强制进行内存回收
		runtime.GC()
		debug.FreeOSMemory() // 强制将内存归还给操作系统

		// 清理资源池
		Global().ResetPools()
	}()

	// 1. 初始化和状态更新
	if err := initializeFileSegment(db, taskID); err != nil {
		file.Close()
		logger.Errorf("初始化文件分片失败: %v", err)
		return err
	}

	// 获取文件大小
	fileInfo, err := file.Stat()
	if err != nil {
		file.Close()
		logger.Errorf("获取文件大小失败: %v", err)
		return err
	}
	fileSize := fileInfo.Size()

	// 2. 准备Reed-Solomon编码
	enc, tempFiles, err := prepareReedSolomon(dataShards, parityShards)
	if err != nil {
		file.Close()
		logger.Errorf("准备Reed-Solomon编码失败: %v", err)
		return err
	}
	// defer cleanupTempFiles(tempFiles)
	// 在processShardContent中处理所有分片(压缩、加密、存储)清理

	// 3. 分割文件到数据分片
	if err := splitFileToShards(file, enc, tempFiles, dataShards, fileSize, tempManager); err != nil {
		file.Close()
		logger.Errorf("分割文件到数据分片失败: %v", err)
		return err
	}
	file.Close()         // 分片完成后立即关闭原始文件
	runtime.GC()         // 原始文件处理完毕后立即回收
	debug.FreeOSMemory() // 强制将内存归还给操作系统

	// 4. 生成奇偶校验分片
	if err := generateParityShards(enc, tempFiles, dataShards, parityShards, tempManager); err != nil {
		logger.Errorf("生成奇偶校验分片失败: %v", err)
		return err
	}

	// 5. 处理所有分片(压缩、加密、存储)
	hashTableMap, err := processShards(db, taskID, fileID, pk, tempFiles, dataShards)
	if err != nil {
		logger.Errorf("处理所有分片失败: %v", err)
		return err
	}

	// 6. 更新文件的HashTable
	if err := UpdateUploadFileHashTable(db, taskID, hashTableMap); err != nil {
		logger.Errorf("更新文件的HashTable失败: %v", err)
		return err
	}

	return nil
}

// initializeFileSegment 初始化文件分片处理,更新文件状态为编码中
// 参数:
//   - db: 数据库实例
//   - taskID: 任务ID
//
// 返回值:
//   - error: 如果状态更新失败，返回错误信息
func initializeFileSegment(db *badgerhold.Store, taskID string) error {
	if err := UpdateUploadFileStatus(db, taskID, pb.UploadStatus_UPLOAD_STATUS_ENCODING); err != nil {
		logger.Errorf("更新文件状态为编码中失败: taskID=%s %v", taskID, err)
		return err
	}
	return nil
}

// prepareReedSolomon 准备Reed-Solomon编码器和临时文件数组
// 参数:
//   - dataShards: 数据分片数
//   - parityShards: 奇偶校验分片数
//
// 返回值:
//   - reedsolomon.StreamEncoder16: Reed-Solomon编码器
//   - []*os.File: 临时文件数组
//   - error: 如果创建失败，返回错误信息
func prepareReedSolomon(dataShards, parityShards int64) (reedsolomon.StreamEncoder16, []*os.File, error) {
	// 创建编码器
	enc, err := reedsolomon.NewStream16(int(dataShards), int(parityShards))
	if err != nil {
		logger.Errorf("创建Reed-Solomon编码器失败: dataShards=%d parityShards=%d %v",
			dataShards, parityShards, err)
		return nil, nil, err
	}

	// 创建临时文件数组
	tempFiles := make([]*os.File, int(dataShards+parityShards))
	return enc, tempFiles, nil
}

// splitFileToShards 将文件分割成数据分片
func splitFileToShards(file *os.File, enc reedsolomon.StreamEncoder16, tempFiles []*os.File, dataShards int64, fileSize int64, tempManager *TempFileManager) error {
	// 使用已有的缓冲区方法
	buffer := Global().GetLargeBuffer()
	defer Global().PutLargeBuffer(buffer)

	// 创建输出writers
	outputs := make([]io.Writer, dataShards)
	writers := make([]*bufio.Writer, dataShards)
	defer func() {
		for _, w := range writers {
			if w != nil {
				w.Flush() // 确保数据写入
				Global().PutWriter(w)
			}
		}
	}()

	// 创建临时文件和writers
	for i := int64(0); i < dataShards; i++ {
		file, err := tempManager.CreateTempFile(TempFileData, i)
		if err != nil {
			return err
		}
		tempFiles[i] = file
		writers[i] = Global().GetWriter(file)
		outputs[i] = writers[i]
	}

	// 使用Reed-Solomon编码器的Split方法进行流式分片
	if err := enc.Split(file, outputs, fileSize); err != nil {
		logger.Errorf("Reed-Solomon分片失败: %v", err)
		return err
	}

	// 确保数据写入并验证
	for i, f := range tempFiles[:dataShards] {
		if err := writers[i].Flush(); err != nil {
			logger.Errorf("刷新缓冲区失败: index=%d %v", i, err)
			return err
		}
		if err := f.Sync(); err != nil {
			logger.Errorf("同步文件失败: index=%d %v", i, err)
			return err
		}

		// 验证文件大小
		info, err := f.Stat()
		if err != nil {
			logger.Errorf("获取文件信息失败: index=%d %v", i, err)
			return err
		}
		if info.Size() == 0 {
			logger.Errorf("分片文件为空: index=%d", i)
			return err
		}

		// 重置文件指针
		if _, err := f.Seek(0, 0); err != nil {
			logger.Errorf("重置文件指针失败: index=%d %v", i, err)
			return err
		}
	}

	return nil
}

// generateParityShards 生成奇偶校验分片
// 参数:
//   - enc: Reed-Solomon编码器
//   - tempFiles: 临时文件数组
//   - dataShards: 数据分片数
//   - parityShards: 奇偶校验分片数
//
// 返回值:
//   - error: 如果生成过程失败，返回错误信息
func generateParityShards(enc reedsolomon.StreamEncoder16, tempFiles []*os.File, dataShards, parityShards int64, tempManager *TempFileManager) error {
	// 使用传入的tempManager，不再创建新的
	// 创建校验分片临时文件
	for i := int64(0); i < parityShards; i++ {
		file, err := tempManager.CreateTempFile(TempFileParity, i)
		if err != nil {
			logger.Errorf("创建校验分片临时文件失败: %v", err)
			return err
		}
		tempFiles[int(dataShards)+int(i)] = file
	}

	// 准备编码输入输出
	dataInputs := make([]io.Reader, dataShards)
	parityOutputs := make([]io.Writer, parityShards)

	// 使用资源池的reader和writer
	readers := make([]*bufio.Reader, dataShards)
	writers := make([]*bufio.Writer, parityShards)
	defer func() {
		// 归还资源
		for _, r := range readers {
			if r != nil {
				Global().PutReader(r)
			}
		}
		for _, w := range writers {
			if w != nil {
				Global().PutWriter(w)
			}
		}

		// 强制进行内存回收
		runtime.GC()
		debug.FreeOSMemory() // 强制将内存归还给操作系统
	}()

	for i := range dataInputs {
		if _, err := tempFiles[i].Seek(0, 0); err != nil {
			logger.Errorf("重置数据分片指针失败: index=%d %v", i, err)
			return err
		}
		readers[i] = Global().GetReader(tempFiles[i])
		dataInputs[i] = readers[i]
	}

	for i := range parityOutputs {
		writers[i] = Global().GetWriter(tempFiles[int(dataShards)+int(i)])
		parityOutputs[i] = writers[i]
	}

	// 生成奇偶校验分片
	if err := enc.Encode(dataInputs, parityOutputs); err != nil {
		logger.Errorf("生成奇偶校验分片失败: %v", err)
		return err
	}

	// 确保数据写入并验证
	for i := int64(0); i < parityShards; i++ {
		f := tempFiles[int(dataShards)+int(i)]

		// 刷新缓冲区
		if bw, ok := parityOutputs[i].(*bufio.Writer); ok {
			if err := bw.Flush(); err != nil {
				logger.Errorf("刷新校验分片缓冲区失败: index=%d %v", i, err)
				return err
			}
		}

		// 同步到磁盘
		if err := f.Sync(); err != nil {
			logger.Errorf("同步校验分片失败: index=%d %v", i, err)
			return err
		}

		// 验证文件大小
		info, err := f.Stat()
		if err != nil {
			logger.Errorf("获取校验分片信息失败: index=%d %v", i, err)
			return err
		}
		if info.Size() == 0 {
			logger.Errorf("校验分片文件为空: index=%d", i)
			return fmt.Errorf("校验分片文件为空: index=%d", i)
		}

		// 重置文件指针
		if _, err := f.Seek(0, 0); err != nil {
			logger.Errorf("重置校验分片指针失败: index=%d %v", i, err)
			return err
		}
	}

	return nil
}

// processShards 处理所有分片(压缩、加密、存储)
// 参数:
//   - db: 数据库实例
//   - taskID: 任务ID
//   - fileID: 文件ID
//   - pk: 公钥
//   - tempFiles: 临时文件数组
//   - dataShards: 数据分片数
//
// 返回值:
//   - map[int64]*pb.HashTable: 分片哈希表
//   - error: 如果处理失败，返回错误信息
func processShards(db *badgerhold.Store, taskID, fileID string, pk []byte, tempFiles []*os.File, dataShards int64) (map[int64]*pb.HashTable, error) {
	hashTables := NewSafeHashTableMap()

	// 限制最大并发数
	maxWorkers := runtime.NumCPU()
	if maxWorkers > 4 {
		maxWorkers = 4 // 最多4个并发
	}

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 每处理4个分片执行一次GC
	gcCounter := 0
	for i := range tempFiles {
		if tempFiles[i] == nil {
			continue // 跳过空文件
		}

		gcCounter++
		if gcCounter >= 4 {
			runtime.GC()
			gcCounter = 0
		}
		wg.Add(1)
		sem <- struct{}{} // 获取信号量

		go func(index int64, f *os.File) {
			defer func() {
				<-sem // 释放信号量
				wg.Done()

				tempFiles[int(index)] = nil
			}()

			// 处理单个分片
			if err := processShardContent(db, taskID, fileID, f, pk, index, dataShards, hashTables); err != nil {
				logger.Errorf("处理分片失败: index=%d %v", index, err)
			}
		}(int64(i), tempFiles[i])

		// 每处理一批次分片就尝试一次内存回收
		if i > 0 && i%maxWorkers == 0 {
			// 等待一小段时间让goroutines完成工作
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
		}
	}

	wg.Wait()

	// 处理完成后执行完整的内存回收
	runtime.GC()
	debug.FreeOSMemory() // 强制将内存归还给操作系统

	return hashTables.ToMap(), nil
}

// processShardContent 处理分片内容
// 参数:
//   - db: 数据库实例
//   - taskID: 任务ID
//   - fileID: 文件ID
//   - file: 文件
//   - secret: 公钥
//   - index: 分片索引
//   - dataShards: 数据分片数
//   - hashTables: 分片哈希表
//
// 返回值:
//   - error: 如果处理失败，返回错误信息
func processShardContent(db *badgerhold.Store, taskID, fileID string, file *os.File, secret []byte, index int64, dataShards int64, hashTables *SafeHashTableMap) error {
	// 创建临时文件管理器
	procTempManager := NewTempFileManager(false, taskID)
	defer procTempManager.CleanupFiles()

	// 创建处理文件 - 用于压缩内容
	compressedFile, err := procTempManager.CreateTempFile(TempFileCompress, index)
	if err != nil {
		logger.Errorf("创建压缩文件失败: %v", err)
		return err
	}
	defer compressedFile.Close()

	// 创建处理文件 - 用于加密结果
	processedFile, err := procTempManager.CreateTempFile(TempFileProcess, index)
	if err != nil {
		logger.Errorf("创建处理文件失败: %v", err)
		return err
	}
	defer processedFile.Close()

	// 获取原始文件大小
	fileInfo, err := file.Stat()
	if err != nil {
		logger.Errorf("获取文件信息失败: %v", err)
		return err
	}
	originalSize := fileInfo.Size()
	logger.Infof("处理分片[%d] - 原始大小: %d bytes", index, originalSize)

	// 重置源文件指针
	if _, err := file.Seek(0, 0); err != nil {
		logger.Errorf("重置源文件指针失败: %v", err)
		return err
	}

	// 先压缩 - 从原始文件读取数据
	compressPipe := &ProcessPipeline{
		reader: file,
		writer: compressedFile,
		processors: []ProcessFunc{
			compressChunk(),
		},
	}

	// 流式处理
	if err := compressPipe.Process(); err != nil {
		logger.Errorf("流式处理失败: %v", err)
		return err
	}

	file.Close()           // 压缩完成后立即关闭原始文件
	os.Remove(file.Name()) // 删除原始文件

	// 获取压缩后的文件大小
	compressedInfo, err := compressedFile.Stat()
	if err != nil {
		logger.Errorf("获取压缩文件信息失败: %v", err)
		return err
	}
	compressedSize := compressedInfo.Size()

	// 重置文件指针用于计算校验和
	if _, err := compressedFile.Seek(0, 0); err != nil {
		logger.Errorf("重置压缩文件指针失败: %v", err)
		return err
	}

	// 计算校验和 - 流式计算
	checksumHash := crc32.NewIEEE()
	if _, err := io.Copy(checksumHash, compressedFile); err != nil {
		logger.Errorf("计算校验和失败: %v", err)
		return err
	}
	checksum := checksumHash.Sum32()

	logger.Infof("处理分片[%d] - 压缩后大小: %d bytes (压缩率: %.2f%%), 校验和: %d",
		index, compressedSize, float64(compressedSize)/float64(originalSize)*100, checksum)

	// 重置压缩文件指针用于加密
	if _, err := compressedFile.Seek(0, 0); err != nil {
		logger.Errorf("重置压缩文件指针失败: %v", err)
		return err
	}

	// 再加密
	pipe := &ProcessPipeline{
		reader: compressedFile,
		writer: processedFile,
		processors: []ProcessFunc{
			encryptChunk(secret),
		},
	}

	// 流式处理
	if err := pipe.Process(); err != nil {
		logger.Errorf("流式处理失败: %v", err)
		return err
	}

	// 获取加密后文件大小
	encryptedInfo, err := processedFile.Stat()
	if err != nil {
		logger.Errorf("获取加密文件信息失败: %v", err)
		return err
	}
	encryptedSize := encryptedInfo.Size()

	logger.Infof("处理分片[%d] - 加密后大小: %d bytes (膨胀率: %.2f%%)",
		index, encryptedSize, float64(encryptedSize)/float64(compressedSize)*100)

	// 验证加密数据的基本格式
	minGCMSize := 12 + 16 // Nonce(12字节) + 最小AuthTag(16字节)
	if encryptedSize < int64(minGCMSize) {
		logger.Errorf("处理分片[%d] - 加密数据大小异常: %d bytes, 最小需要: %d bytes",
			index, encryptedSize, minGCMSize)
		return fmt.Errorf("invalid encrypted data size: too small")
	}

	// 验证加密后数据大小关系
	if encryptedSize <= compressedSize {
		logger.Errorf("处理分片[%d] - 加密数据大小异常: 加密后(%d bytes) <= 压缩后(%d bytes)",
			index, encryptedSize, compressedSize)
		return fmt.Errorf("invalid encrypted data size: smaller than compressed data")
	}

	// 重置文件指针用于后续操作
	if _, err := processedFile.Seek(0, 0); err != nil {
		logger.Errorf("重置处理文件指针失败: %v", err)
		return err
	}

	// 根据文件ID和分片索引生成唯一的分片ID
	segmentID, err := files.GenerateSegmentID(fileID, index)
	if err != nil {
		logger.Errorf("生成分片ID失败: %v", err)
		return err
	}

	// 写入临时存储
	isRsCodes := index >= dataShards
	readKey, err := tempfile.WriteEncryptedSegment(segmentID, processedFile, isRsCodes, taskID)
	if err != nil {
		logger.Errorf("写入加密分片失败: %v", err)
		return err
	}

	// 创建分片记录
	if err := CreateUploadSegmentRecord(
		db,           // 数据库实例
		taskID,       // 任务ID
		segmentID,    // 分片ID
		index,        // 分片索引
		originalSize, // 原始数据大小
		checksum,     // 校验和
		readKey,      // 临时文件读取标识
		isRsCodes,    // 是否为纠删码分片
		pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_PENDING, // 分片状态
	); err != nil {
		logger.Errorf("创建分片记录失败: %v", err)
		return err
	}

	// 线程安全更新HashTable
	hashTables.Set(index, &pb.HashTable{
		SegmentId:     segmentID,
		SegmentIndex:  index,
		Crc32Checksum: checksum,
		IsRsCodes:     isRsCodes,
	})

	logger.Infof("处理分片[%d] - 处理完成: 原始大小=%d bytes -> 压缩后=%d bytes -> 加密后=%d bytes",
		index, originalSize, compressedSize, encryptedSize)

	return nil
}

// ProcessPipeline 处理管道
type ProcessPipeline struct {
	reader     io.Reader
	writer     io.Writer
	processors []ProcessFunc
	// 移除 chunkSize，因为我们会处理整个shard
}

// Process 流式处理
func (p *ProcessPipeline) Process() error {
	// 读取整个shard的数据
	data, err := io.ReadAll(p.reader)
	if err != nil {
		logger.Errorf("读取数据失败: %v", err)
		return err
	}

	// 对整个shard进行处理
	var processed []byte = data
	for _, proc := range p.processors {
		processed, err = proc(processed)
		if err != nil {
			logger.Errorf("处理数据失败: %v", err)
			return err
		}
	}

	// 写入处理后的数据
	if _, err := p.writer.Write(processed); err != nil {
		logger.Errorf("写入数据失败: %v", err)
		return err
	}

	return nil
}

// compressChunk 压缩块
func compressChunk() ProcessFunc {
	return func(data []byte) ([]byte, error) {
		// 每次处理获取新的上下文
		ctx := Global().GetCompressContext()
		defer Global().PutCompressContext(ctx)

		// 重置缓冲区
		ctx.buffer.Reset()
		ctx.writer.Reset(ctx.buffer)

		// 写入数据并压缩
		if _, err := ctx.writer.Write(data); err != nil {
			logger.Errorf("写入数据失败: %v", err)
			return nil, err
		}
		if err := ctx.writer.Close(); err != nil {
			logger.Errorf("关闭压缩上下文失败: %v", err)
			return nil, err
		}

		// 返回压缩后的数据
		return ctx.buffer.Bytes(), nil
	}
}

// encryptChunk 加密块
func encryptChunk(key []byte) ProcessFunc {
	return func(data []byte) ([]byte, error) {
		// 添加输入数据的详细日志
		logger.Infof("开始加密数据块: 大小=%d bytes", len(data))
		logger.Infof("使用的加密密钥: %s", hex.EncodeToString(key))

		// 计算AES密钥
		aesKey := md5.Sum(key)
		logger.Infof("计算得到的AES密钥: %s", hex.EncodeToString(aesKey[:]))

		// 加密数据
		encryptedData, err := gcm.EncryptData(data, aesKey[:])
		if err != nil {
			logger.Errorf("加密数据失败: %v", err)
			return nil, err
		}

		// 添加加密结果的日志
		logger.Infof("数据加密完成: 原始大小=%d bytes, 加密后大小=%d bytes",
			len(data), len(encryptedData))

		return encryptedData, nil
	}
}

// GetUploadsTempDir 返回上传文件的临时目录
// 返回值:
//   - string: 上传文件的临时目录路径
func GetUploadsTempDir() string {
	return filepath.Join(os.TempDir(), "bpfs_uploads")
}

// CreateUploadsTempDir 创建上传文件的临时目录
// 返回值:
//   - error: 如果创建失败则返回错误
func CreateUploadsTempDir() error {
	dir := GetUploadsTempDir()
	return os.MkdirAll(dir, 0755)
}

// CleanupUploadsTempDir 清理上传文件的临时目录
// 返回值:
//   - error: 如果清理失败则返回错误
func CleanupUploadsTempDir() error {
	return tempfile.CleanupTempFiles(GetUploadsTempDir())
}

// CreateProcessTempFile 创建处理分片的临时文件
// 参数:
//   - segmentID: 分片ID
//
// 返回值:
//   - string: 临时文件路径
//   - error: 如果创建失败则返回错误
func CreateProcessTempFile(segmentID string) (string, error) {
	// 确保上传临时目录存在
	if err := CreateUploadsTempDir(); err != nil {
		return "", err
	}

	// 生成随机数
	randNum := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s_%d", segmentID, time.Now().UnixNano())))

	// 创建临时文件路径
	tempDir := GetUploadsTempDir()
	filename := filepath.Join(tempDir, fmt.Sprintf("%s%s_%d", ProcessSegmentPrefix, segmentID, randNum))

	// 创建文件
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	return filename, nil
}

// CreateSegmentDataTempFile 创建分片数据的临时文件
// 参数:
//   - segmentID: 分片ID
//
// 返回值:
//   - string: 临时文件路径
//   - error: 如果创建失败则返回错误
func CreateSegmentDataTempFile(segmentID string) (string, error) {
	// 确保上传临时目录存在
	if err := CreateUploadsTempDir(); err != nil {
		return "", err
	}

	// 创建临时文件路径
	tempDir := GetUploadsTempDir()
	filename := filepath.Join(tempDir, fmt.Sprintf("%s%s", SegmentDataPrefix, segmentID))

	// 创建文件
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	return filename, nil
}
