// Package reedsolomon 实现了Reed-Solomon编码算法
package reedsolomon

import (
	"io"
	"os"
)

// StreamEncodeWithFiles 对输入流进行Reed-Solomon编码，并将结果写入临时文件
// 参数:
//   - r: 输入数据的读取器
//   - dataShards: 数据分片的数量
//   - parityShards: 奇偶校验分片的数量
//   - size: 输入数据的总大小
//
// 返回值:
//   - dataFiles: 数据分片的临时文件切片
//   - parityFiles: 奇偶校验分片的临时文件切片
//   - err: 编码过程中的错误，如果成功则为nil
func StreamEncodeWithFiles(r io.Reader, dataShards, parityShards int, size int64) (dataFiles, parityFiles []*os.File, err error) {
	// 创建Reed-Solomon编码器
	enc, err := NewStream(dataShards, parityShards)
	if err != nil {
		return nil, nil, err // 如果创建编码器失败，返回错误
	}

	// 创建临时数据分片文件
	dataFiles = make([]*os.File, dataShards)
	for i := range dataFiles {
		dataFiles[i], err = os.CreateTemp("", "rs-data-*") // 创建临时文件用于存储数据分片
		if err != nil {
			cleanupFiles(dataFiles) // 如果创建失败，清理已创建的文件
			return nil, nil, err    // 返回错误
		}
	}

	// 创建临时奇偶校验分片文件
	parityFiles = make([]*os.File, parityShards)
	for i := range parityFiles {
		parityFiles[i], err = os.CreateTemp("", "rs-parity-*") // 创建临时文件用于存储奇偶校验分片
		if err != nil {
			cleanupFiles(dataFiles)   // 如果创建失败，清理已创建的数据文件
			cleanupFiles(parityFiles) // 清理已创建的奇偶校验文件
			return nil, nil, err      // 返回错误
		}
	}

	// 执行数据分片操作
	err = enc.Split(r, asWriters(dataFiles), size)
	if err != nil {
		cleanupFiles(dataFiles)   // 如果分片失败，清理数据文件
		cleanupFiles(parityFiles) // 清理奇偶校验文件
		return nil, nil, err      // 返回错误
	}

	// 重置所有数据文件的读取位置到开头
	for _, f := range dataFiles {
		_, err = f.Seek(0, 0) // 将文件指针移动到文件开头
		if err != nil {
			cleanupFiles(dataFiles)   // 如果重置失败，清理数据文件
			cleanupFiles(parityFiles) // 清理奇偶校验文件
			return nil, nil, err      // 返回错误
		}
	}

	// 执行奇偶校验编码
	err = enc.Encode(asReaders(dataFiles), asWriters(parityFiles))
	if err != nil {
		cleanupFiles(dataFiles)   // 如果编码失败，清理数据文件
		cleanupFiles(parityFiles) // 清理奇偶校验文件
		return nil, nil, err      // 返回错误
	}

	// 重置所有文件的读取位置到开头
	for _, f := range append(dataFiles, parityFiles...) {
		_, err = f.Seek(0, 0) // 将所有文件的指针移动到文件开头
		if err != nil {
			cleanupFiles(dataFiles)   // 如果重置失败，清理数据文件
			cleanupFiles(parityFiles) // 清理奇偶校验文件
			return nil, nil, err      // 返回错误
		}
	}

	return dataFiles, parityFiles, nil // 返回数据文件和奇偶校验文件的切片，以及nil错误
}

// 辅助函数

// cleanupFiles 关闭并删除给定的文件切片
// 参数:
//   - files: 要清理的文件切片
func cleanupFiles(files []*os.File) {
	for _, f := range files {
		if f != nil {
			f.Close()           // 关闭文件
			os.Remove(f.Name()) // 删除文件
		}
	}
}

// asWriters 将给定的接口转换为io.Writer切片
// 参数:
//   - rws: 要转换的接口，可以是[]io.ReadWriter或[]*os.File
//
// 返回值:
//   - []io.Writer: 转换后的写入器切片
func asWriters(rws interface{}) []io.Writer {
	switch v := rws.(type) {
	case []io.ReadWriter:
		writers := make([]io.Writer, len(v))
		for i, rw := range v {
			writers[i] = rw // 将每个ReadWriter转换为Writer
		}
		return writers
	case []*os.File:
		writers := make([]io.Writer, len(v))
		for i, f := range v {
			writers[i] = f // 将每个*os.File转换为Writer
		}
		return writers
	default:
		return nil // 如果类型不匹配，返回nil
	}
}

// asReaders 将给定的接口转换为io.Reader切片
// 参数:
//   - rws: 要转换的接口，可以是[]io.ReadWriter或[]*os.File
//
// 返回值:
//   - []io.Reader: 转换后的读取器切片
func asReaders(rws interface{}) []io.Reader {
	switch v := rws.(type) {
	case []io.ReadWriter:
		readers := make([]io.Reader, len(v))
		for i, rw := range v {
			readers[i] = rw // 将每个ReadWriter转换为Reader
		}
		return readers
	case []*os.File:
		readers := make([]io.Reader, len(v))
		for i, f := range v {
			readers[i] = f // 将每个*os.File转换为Reader
		}
		return readers
	default:
		return nil // 如果类型不匹配，返回nil
	}
}
