package tempfile

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/bpfs/defs/files"
	"github.com/bpfs/defs/utils/logger"
	"github.com/pkg/errors"
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
			return fmt.Errorf("生成分片ID失败: %v", err)
		}

		// 生成临时文件名
		filename := generateTempFilename()
		// 获取文件所在目录
		dir := filepath.Dir(filename)
		// 创建目录
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %v", err)
		}

		// 创建临时文件
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建临时文件失败: %v", err)
		}
		defer file.Close()

		// 写入分片数据
		if bufWriter, ok := shard.(*bytes.Buffer); ok {
			_, err = bufWriter.WriteTo(file)
		} else {
			_, err = io.Copy(file, shard.(io.Reader))
		}
		if err != nil {
			return fmt.Errorf("写入分片数据失败: %v", err)
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
	filename := generateTempFilename()

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("创建目录时失败: %v", err)
		return err
	}

	// 将数据写入临时文件
	err := os.WriteFile(filename, value, 0666)
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
	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))

	// 并发写入每个分片
	for segmentID, data := range segments {
		wg.Add(1)
		go func(id string, value []byte) {
			defer wg.Done()
			if err := Write(id, value); err != nil {
				errChan <- errors.Wrapf(err, "写入分片 %s 失败", id)
			}
		}(segmentID, data)
	}

	// 等待所有goroutine完成并关闭错误通道
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// 检查是否有错误发生
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
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
	filename := generateTempFilename()

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
		return nil, false, err
	}

	// 文件存在，返回 文件信息, true, nil
	return fileInfo, true, nil
}

// 废弃：syscall.Mmap不兼容问题
// ReadMmap 使用内存映射来读取临时文件的内容
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - []byte: 内存映射的文件内容
//   - func(): 用于解除内存映射和清理资源的函数
//   - error: 果过程中发生错误，返回相应的错误信息
// func ReadMmap(key string) ([]byte, func(), error) {
// 	// 获取与键关联的文件名
// 	filename, ok := getKeyToFileMapping(key)
// 	if !ok {
// 		return nil, nil, fmt.Errorf("未找到与键关联的文件")
// 	}

// 	// 打开文件
// 	file, err := os.Open(filename)
// 	if err != nil {
// 		logger.Errorf("打开临时文件失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 获取文件信息
// 	fileInfo, err := file.Stat()
// 	if err != nil {
// 		file.Close()
// 		logger.Errorf("获取文件信息失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 创建内存映射
// 	mmap, err := syscall.Mmap(int(file.Fd()), 0, int(fileInfo.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
// 	if err != nil {
// 		file.Close()
// 		logger.Errorf("创建内存映射失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 定义清理函数
// 	cleanup := func() {
// 		syscall.Munmap(mmap)
// 		file.Close()
// 		os.Remove(filename)
// 		deleteKeyToFileMapping(key)
// 	}

// 	return mmap, cleanup, nil
// }

// 废弃：syscall.Mmap不兼容问题
// ReadMmapReader 使用内存映射创建一个读取器
// 参数:
//   - key: string 文件的唯一标识符
//
// 返回值:
//   - io.Reader: 用于读取文件内容的读取器
//   - func(): 清理函数，用于释放资源
//   - error: 如果在过程中发生错误，返回错误信息
// func ReadMmapReader(key string) (io.Reader, func(), error) {
// 	// 获取文件名
// 	filename, ok := getKeyToFileMapping(key)
// 	if !ok {
// 		return nil, nil, fmt.Errorf("未找到与键关联的文件")
// 	}

// 	// 打开文件
// 	file, err := os.Open(filename)
// 	if err != nil {
// 		// 如果打开文件失败，记录错误并返回
// 		logger.Errorf("打开文件失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 获取文件信息
// 	fileInfo, err := file.Stat()
// 	if err != nil {
// 		// 如果获取文件信息失败，关闭文件并返回错误
// 		file.Close()
// 		logger.Errorf("获取文件信息失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 获取文件大小
// 	size := fileInfo.Size()

// 	// 创建内存映射
// 	mmap, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
// 	if err != nil {
// 		// 如果创建内存映射失败，关闭文件并返回错误
// 		file.Close()
// 		logger.Errorf("创建内存映射失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 创建字节切片读取器
// 	reader := bytes.NewReader(mmap)

// 	// 定义清理函数
// 	cleanup := func() {
// 		// 解除内存映射
// 		syscall.Munmap(mmap)
// 		// 关闭文件
// 		file.Close()
// 		// 删除文件名到键的映射
// 		deleteKeyToFileMapping(key)
// 	}

// 	// 返回读取器和清理函数
// 	return reader, cleanup, nil
// }
