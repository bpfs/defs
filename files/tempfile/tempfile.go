package tempfile

import (
	"bufio"
	"bytes"
	"fmt"

	"github.com/bpfs/defs/v2/files"
	logging "github.com/dep2p/log"

	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var logger = logging.Logger("tempfile")

const (
	defaultBufferSize = 32 * 1024 // 32KB
	streamBufferSize  = 1 << 20   // 1MB 流处理缓冲区
)

// WriteShards 将多个分片写入临时文件
// 参数:
//   - fileID: string 文件ID
//   - shards: []io.Writer 分片写入器切片
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func WriteShards(fileID string, shards []io.Writer) error {
	// 遍历所有分片
	for i, shard := range shards {
		// 生成分片ID
		segmentID, err := files.GenerateSegmentID(fileID, int64(i))
		if err != nil {
			logger.Errorf("生成分片ID失败: %v", err)
			return err
		}

		// 生成临时文件名
		filename, err := generateTempFilename()
		if err != nil {
			logger.Errorf("生成临时文件名失败: %v", err)
			return err
		}
		// 获取文件所在目录
		dir := filepath.Dir(filename)
		// 创建目录
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Errorf("创建目录失败: %v", err)
			return err
		}

		// 创建临时文件
		file, err := os.Create(filename)
		if err != nil {
			logger.Errorf("创建临时文件失败: %v", err)
			return err
		}
		defer file.Close()

		// 写入分片数据
		if bufWriter, ok := shard.(*bytes.Buffer); ok {
			_, err = bufWriter.WriteTo(file)
		} else {
			_, err = io.Copy(file, shard.(io.Reader))
		}
		if err != nil {
			logger.Errorf("写入分片数据失败: %v", err)
			return err
		}

		// 将分片ID与文件名关联
		addKeyToFileMapping(segmentID, filename)
	}
	return nil
}

// Write 将值写入临时文件，并将文件名与键关联
// 参数:
//   - key: string 用于关联临时文件的唯一键
//   - value: []byte 要写入临时文件的数据
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func Write(key string, value []byte) error {
	// 生成临时文件名
	filename, err := generateTempFilename()
	if err != nil {
		logger.Errorf("生成临时文件名失败: %v", err)
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("创建目录时失败: %v", err)
		return err
	}

	// 将数据写入临时文件
	err = os.WriteFile(filename, value, 0666)
	if err != nil {
		logger.Errorf("写入临时文件失败: %v", err)
		return err
	}

	// 将键与文件名关联
	addKeyToFileMapping(key, filename)
	return nil
}

// WriteBatch 批量写入多个临时文件
// 参数:
//   - segments: map[string][]byte 键为分片ID，值为分片数据的映射
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func WriteBatch(segments map[string][]byte) error {
	// 预分配足够大的缓冲区
	totalSize := int64(0)
	for _, data := range segments {
		totalSize += int64(len(data))
	}

	pool := NewBufferPool()
	buf := pool.Get(totalSize)
	defer pool.Put(buf)

	// 批量写入到缓冲区
	for segmentID, data := range segments {
		if _, err := buf.Write(data); err != nil {
			logger.Errorf("写入缓冲区失败: %v", err)
			return err
		}
		// 记录偏移量用于后续分割
		addKeyToFileMapping(segmentID, fmt.Sprintf("%d:%d", buf.Len()-len(data), len(data)))
	}

	// 一次性写入文件
	filename, err := generateTempFilename()
	if err != nil {
		logger.Errorf("生成临时文件名失败: %v", err)
		return err
	}

	return os.WriteFile(filename, buf.Bytes(), 0666)
}

// WriteBatchStream 批量写入多个临时文件，使用流式处理
// 参数:
//   - segments: map[string]io.Reader 键为分片ID，值为分片数据的读取器
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func WriteBatchStream(segments map[string]io.Reader) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))

	// 并发写入每个分片
	for segmentID, reader := range segments {
		wg.Add(1)
		go func(id string, r io.Reader) {
			defer wg.Done()
			if err := WriteStream(id, r); err != nil {
				logger.Errorf("写入分片 %s 失败: %v", id, err)
				errChan <- errors.Wrapf(err, "写入分片 %s 失败", id)
			}
		}(segmentID, reader)
	}

	// 等待所有goroutine完成并关闭错误通道
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// 检查是否有错误发生
	for err := range errChan {
		if err != nil {
			logger.Errorf("批量写入分片失败: %v", err)
			return err
		}
	}

	return nil
}

// WriteStream 将 io.Reader 的内容写入临时文件，并将文件名与键关联
// 参数:
//   - key: string 用于关联临时文件的唯一键
//   - reader: io.Reader 要写入临时文件的数据读取器
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func WriteStream(key string, reader io.Reader) error {
	// 生成临时文件名
	filename, err := generateTempFilename()
	if err != nil {
		logger.Errorf("生成临时文件名失败: %v", err)
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("创建目录时失败: %v", err)
		return err
	}

	// 创建临时文件
	file, err := os.Create(filename)
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return err
	}
	defer file.Close()

	// 创建带缓冲的写入器
	bufWriter := bufio.NewWriter(file)
	// 将reader的内容写入文件
	written, err := io.Copy(bufWriter, reader)
	if err != nil {
		logger.Errorf("写入临时文件失败: %v", err)
		return err
	}

	// 刷新缓冲区
	if err := bufWriter.Flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	logger.Infof("写入临时文件成功: key=%s, filename=%s, size=%d", key, filename, written)

	// 检查写入的文件大小是否为0
	if written == 0 {
		logger.Warnf("写入的文件大小为0: key=%s, filename=%s", key, filename)
	}

	// 将键与文件名关联
	addKeyToFileMapping(key, filename)
	return nil
}

// WriteStreamOptimized 优化的流式写入
func WriteStreamOptimized(key string, reader io.Reader) error {
	const chunkSize = 32 * 1024 // 32KB chunks

	pool := NewBufferPool()
	buf := pool.Get(chunkSize)
	defer pool.Put(buf)

	file, err := generateTempFilename()
	if err != nil {
		logger.Errorf("生成临时文件名失败: %v", err)
		return err
	}

	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		logger.Errorf("打开临时文件失败: %v", err)
		return err
	}
	defer f.Close()

	// 使用bufio提升性能
	w := bufio.NewWriterSize(f, chunkSize)

	for {
		buf.Reset()
		_, err := io.CopyN(buf, reader, chunkSize)
		if err == io.EOF {
			break
		}
		if err != nil && err != io.EOF {
			logger.Errorf("写入临时文件失败: %v", err)
			return err
		}

		if _, err := w.Write(buf.Bytes()); err != nil {
			logger.Errorf("写入临时文件失败: %v", err)
			return err
		}
	}

	if err := w.Flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	addKeyToFileMapping(key, file)
	return nil
}

// Read 根据键读取临时文件的内容，并在读取成功后删除文件
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - []byte: 读取的文件内容
//   - error: 如果读取过程中发生错误，返回相应的错误信息
func Read(key string) ([]byte, error) {
	// 获取与键关联的文件名
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return nil, fmt.Errorf("未找到与键关联的文件")
	}

	// 读取文件内容
	content, err := os.ReadFile(filename)
	if err != nil {
		logger.Errorf("读取临时文件失败: %v", err)
		return nil, err
	}

	// 删除临时文件
	if err := os.Remove(filename); err != nil {
		logger.Errorf("删除临时文件失败: %v", err)
		return nil, err
	}

	// 删除键与文件名的关联
	deleteKeyToFileMapping(key)
	return content, nil
}

// OnlyRead 根据键读取临时文件的内容，并在读取成功后删除文件
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - []byte: 读取的文件内容
//   - error: 如果读取过程中发生错误，返回相应的错误信息
func OnlyRead(key string) ([]byte, error) {
	// 获取与键关联的文件名
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return nil, fmt.Errorf("未找到与键关联的文件")
	}

	// 读取文件内容
	content, err := os.ReadFile(filename)
	if err != nil {
		logger.Errorf("读取临时文件失败: %v", err)
		return nil, err
	}

	return content, nil
}

// GetFile 获取与键关联的文件句柄
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - *os.File: 文件句柄
//   - error: 如果获取过程中发生错误，返回相应的错误信息
func GetFile(key string) (*os.File, error) {
	// 获取与键关联的文件名
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return nil, fmt.Errorf("未找到与键关联的文件")
	}

	// 打开文件
	file, err := os.Open(filename)
	if err != nil {
		logger.Errorf("打开临时文件失败: %v", err)
		return nil, err
	}

	return file, nil
}

// Delete 根据键删除临时文件
// 参数:
//   - key: string 用于检索要删除的临时文件的唯一键
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func Delete(key string) error {
	// 获取与键关联的文件名
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return fmt.Errorf("未找到与键关联的文件")
	}

	// 删除临时文件
	err := os.Remove(filename)
	if err != nil {
		logger.Errorf("删除临时文件失败: %v", err)
		return err
	}

	// 删除键与文件名的关联
	deleteKeyToFileMapping(key)
	return nil
}

// CheckFileExistsAndInfo 检查与给定键关联的临时文件是否存在并返回其文件信息
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - fs.FileInfo: 文件信息（如果存在）
//   - bool: 文件是否存在
//   - error: 如果检查过程中发生错误，返回相应的错误信息
func CheckFileExistsAndInfo(key string) (fs.FileInfo, bool, error) {
	// 获取与键关联的文件名
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return nil, false, nil
	}

	// 获取文件信息
	fileInfo, err := os.Stat(filename)
	if err != nil {

		if os.IsNotExist(err) {
			return nil, false, nil
		}
		logger.Errorf("获取文件信息失败: %v", err)
		return nil, false, err
	}

	// 文件存在，返回 文件信息, true, nil
	return fileInfo, true, nil
}

// init 初始化临时文件
// 参数:
//   - config: Config 配置
//
// 返回值:
//   - error: 如果初始化过程中发生错误，返回相应的错误信息
func (tf *TempFile) init(config Config) error {
	// 生成临时文件路径
	path, err := generateTempFilename()
	if err != nil {
		return &TempFileError{Op: "init", Err: err}
	}

	// 打开文件
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return &TempFileError{Op: "init", Path: path, Err: err}
	}

	tf.path = path
	tf.file = file
	tf.size = 0
	tf.maxSize = config.MaxFileSize
	tf.bufferSize = _32KB // 使用固定的32KB缓冲区大小
	tf.lastAccess = time.Now()
	tf.buffer.Reset()

	return nil
}

// Write 优化写入逻辑
// 参数:
//   - p: []byte 要写入的数据
//
// 返回值:
//   - int: 写入的字节数
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func (tf *TempFile) Write(p []byte) (n int, err error) {
	tf.lastAccess = time.Now()

	// 检查大小限制
	if tf.size+int64(len(p)) > tf.maxSize {
		return 0, &TempFileError{Op: "write", Path: tf.path, Err: errors.New("超出文件大小限制")}
	}

	// 大数据块直接写入文件
	if len(p) >= tf.bufferSize {
		if tf.buffer.Len() > 0 {
			// 先刷新缓冲区
			if err := tf.flush(); err != nil {
				logger.Errorf("刷新缓冲区失败: %v", err)
				return 0, err
			}
		}
		n, err = tf.file.Write(p)
		tf.size += int64(n)
		return n, err
	}

	// 小数据块写入缓冲区
	n, err = tf.buffer.Write(p)
	if err != nil {
		return n, &TempFileError{Op: "write", Path: tf.path, Err: err}
	}

	// 缓冲区满时刷新
	if tf.buffer.Len() >= tf.bufferSize {
		if err := tf.flush(); err != nil {
			logger.Errorf("刷新缓冲区失败: %v", err)
			return n, err
		}
	}

	tf.size += int64(n)
	return n, nil
}

// Read 优化读取逻辑
// 参数:
//   - p: []byte 要读取的数据
//
// 返回值:
//   - int: 读取的字节数
//   - error: 如果读取过程中发生错误，返回相应的错误信息
func (tf *TempFile) Read(p []byte) (n int, err error) {
	tf.lastAccess = time.Now()

	// 大块读取直接从文件读
	if tf.buffer.Len() == 0 && len(p) >= tf.bufferSize {
		return tf.file.Read(p)
	}

	// 缓冲区为空时预读取
	if tf.buffer.Len() == 0 {
		buf := make([]byte, tf.bufferSize)
		n, err := tf.file.Read(buf)
		if err != nil && err != io.EOF {
			logger.Errorf("读取临时文件失败: %v", err)
			return 0, err
		}
		if n > 0 {
			tf.buffer.Write(buf[:n])
		}
	}

	// 从缓冲区读取
	return tf.buffer.Read(p)
}

// flush 将缓冲区数据写入文件
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func (tf *TempFile) flush() error {
	if tf.buffer.Len() == 0 {
		return nil
	}

	_, err := tf.buffer.WriteTo(tf.file)
	if err != nil {
		return &TempFileError{Op: "flush", Path: tf.path, Err: err}
	}

	tf.buffer.Reset()
	return nil
}

// reset 重置文件状态
// 返回值:
//   - error: 如果重置过程中发生错误，返回相应的错误信息
func (tf *TempFile) reset() error {
	if err := tf.flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	if err := tf.file.Truncate(0); err != nil {
		return &TempFileError{Op: "reset", Path: tf.path, Err: err}
	}

	if _, err := tf.file.Seek(0, 0); err != nil {
		return &TempFileError{Op: "reset", Path: tf.path, Err: err}
	}

	tf.size = 0
	tf.buffer.Reset()
	return nil
}

// close 关闭并删除文件
// 返回值:
//   - error: 如果关闭过程中发生错误，返回相应的错误信息
func (tf *TempFile) close() error {
	if tf.file != nil {
		tf.file.Close()
		os.Remove(tf.path)
		tf.file = nil
	}
	return nil
}

// WriteBatchOptimized 优化批量写入
// 参数:
//   - segments: map[string][]byte 键为分片ID，值为分片数据的映射
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func WriteBatchOptimized(segments map[string][]byte) error {
	// 按大小分组
	smallSegs := make(map[string][]byte)
	largeSegs := make(map[string][]byte)

	for id, data := range segments {
		if len(data) <= _32KB {
			smallSegs[id] = data
		} else {
			largeSegs[id] = data
		}
	}

	// 小文件合并写入
	if len(smallSegs) > 0 {
		if err := writeBatchSmall(smallSegs); err != nil {
			logger.Errorf("写入小文件失败: %v", err)
			return err
		}
	}

	// 大文件并发写入
	if len(largeSegs) > 0 {
		if err := writeBatchLarge(largeSegs); err != nil {
			logger.Errorf("写入大文件失败: %v", err)
			return err
		}
	}

	return nil
}

// writeBatchSmall 合并写入小文件
// 参数:
//   - segments: map[string][]byte 键为分片ID，值为分片数据的映射
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func writeBatchSmall(segments map[string][]byte) error {
	// 预分配缓冲区
	totalSize := int64(0)
	for _, data := range segments {
		totalSize += int64(len(data))
	}

	pool := NewBufferPool()
	buf := pool.Get(totalSize)
	defer pool.Put(buf)

	// 创建临时文件
	filename, err := generateTempFilename()
	if err != nil {
		logger.Errorf("生成临时文件名失败: %v", err)
		return err
	}

	// 写入所有数据并记录偏移量
	offset := 0
	for id, data := range segments {
		if _, err := buf.Write(data); err != nil {
			logger.Errorf("写入缓冲区失败: %v", err)
			return err
		}
		addKeyToFileMapping(id, fmt.Sprintf("%s:%d:%d", filename, offset, len(data)))
		offset += len(data)
	}

	// 一次性写入文件
	return os.WriteFile(filename, buf.Bytes(), 0666)
}

// writeBatchLarge 并发写入大文件
// 参数:
//   - segments: map[string][]byte 键为分片ID，值为分片数据的映射
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func writeBatchLarge(segments map[string][]byte) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))

	for id, data := range segments {
		wg.Add(1)
		go func(id string, data []byte) {
			defer wg.Done()
			if err := Write(id, data); err != nil {
				errChan <- err
			}
		}(id, data)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			logger.Errorf("写入大文件失败: %v", err)
			return err
		}
	}
	return nil
}

// GetUploadsDir 返回上传文件的临时目录
// 返回值:
//   - string: 上传文件的临时目录路径
func GetUploadsDir() string {
	uploadsDir := filepath.Join(os.TempDir(), "bpfs_uploads")
	return uploadsDir
}

// WriteEncryptedSegment 写入加密的分片数据
// 参数:
//   - segmentID: string 分片ID
//   - reader: io.Reader 要写入的数据读取器
//   - isRsCodes: bool 是否是RsCodes
//   - taskID: string (可选) 任务ID，用于隔离不同上传任务的临时文件
//
// 返回值:
//   - string: 读取标识
func WriteEncryptedSegment(segmentID string, reader io.Reader, isRsCodes bool, taskID ...string) (string, error) {
	// 生成唯一的读取标识
	readKey := fmt.Sprintf("segment_%s_%v", segmentID, isRsCodes)

	// 确保上传临时目录存在
	uploadsDir := GetUploadsDir()
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		logger.Errorf("创建上传临时目录失败: %v", err)
		return "", err
	}

	// 处理任务ID
	filePrefix := "segment_"
	if len(taskID) > 0 && taskID[0] != "" {
		filePrefix = fmt.Sprintf("segment_%s_", taskID[0])
	}

	// 在上传临时目录中创建临时文件
	tempFile, err := os.CreateTemp(uploadsDir, filePrefix+segmentID+"_*")
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return "", err
	}
	defer tempFile.Close()

	filename := tempFile.Name()

	// 使用缓冲写入
	bufWriter := bufio.NewWriterSize(tempFile, streamBufferSize)

	// 复制数据
	written, err := io.Copy(bufWriter, reader)
	if err != nil {
		os.Remove(filename)
		logger.Errorf("写入数据失败: %v", err)
		return "", err
	}

	// 刷新缓冲区
	if err := bufWriter.Flush(); err != nil {
		os.Remove(filename)
		logger.Errorf("刷新缓冲区失败: %v", err)
		return "", err
	}

	// 同步到磁盘
	if err := tempFile.Sync(); err != nil {
		os.Remove(filename)
		logger.Errorf("同步文件失败: %v", err)
		return "", err
	}

	// 检查写入大小
	if written == 0 {
		os.Remove(filename)
		logger.Errorf("写入数据大小为0")
		return "", errors.New("写入数据大小为0")
	}

	// 记录映射关系
	addKeyToFileMapping(readKey, filename)

	logger.Infof("加密分片数据写入成功: key=%s, file=%s, size=%d", readKey, filename, written)

	return readKey, nil
}

// ReadEncryptedSegment 读取加密的分片数据
// 参数:
//   - readKey: WriteEncryptedSegment返回的读取标识
//
// 返回值:
//   - []byte: 加密的数据
//   - error: 读取失败时返回错误
func ReadEncryptedSegment(readKey string) ([]byte, error) {
	return OnlyRead(readKey)
}

// CleanupTempFiles 清理所有临时文件
// 参数:
//   - dirPath: string (可选) 指定要清理的临时目录，如果为空则清理系统临时目录中的defs_tempfile_*文件
//
// 返回值:
//   - error: 如果清理过程中发生错误，返回相应的错误信息
func CleanupTempFiles(dirPath ...string) error {
	var patterns []string

	// 处理特定目录清理
	if len(dirPath) > 0 && dirPath[0] != "" {
		// 清理指定目录下的所有文件
		if err := os.RemoveAll(dirPath[0]); err != nil {
			logger.Errorf("清理指定临时目录失败: path=%s err=%v", dirPath[0], err)
			return err
		}
		// 重新创建目录以保证存在
		if err := os.MkdirAll(dirPath[0], 0755); err != nil {
			logger.Errorf("重新创建临时目录失败: path=%s err=%v", dirPath[0], err)
			return err
		}
		logger.Infof("成功清理临时目录: %s", dirPath[0])
		return nil
	}

	// 默认清理系统临时目录中的defs_tempfile_*文件
	tempDir := os.TempDir()
	patterns = append(patterns, filepath.Join(tempDir, "defs_tempfile_*"))

	// 清理所有匹配的文件
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			logger.Errorf("查找临时文件失败: pattern=%s err=%v", pattern, err)
			continue
		}

		// 删除文件
		for _, file := range files {
			if err := os.Remove(file); err != nil {
				logger.Warnf("删除临时文件失败: file=%s err=%v", file, err)
			} else {
				logger.Debugf("成功删除临时文件: %s", file)
			}
		}

		logger.Infof("临时文件清理完成: 模式=%s 文件数=%d", pattern, len(files))
	}

	return nil
}
