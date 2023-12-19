package defs

import (
	"bytes"
	"compress/gzip"
	"io"
)

// 数据压缩
func compressData(data []byte) ([]byte, error) {
	// Buffer 是一个可变大小的字节缓冲区，具有 Read 和 Write 方法。
	var compressedData bytes.Buffer

	// 	NewWriter 返回一个新的 Writer。 对返回的 writer 的写入被压缩并写入 w。
	w := gzip.NewWriter(&compressedData)

	// Write 将 p 的压缩形式写入底层 io.Writer。 在 Writer 关闭之前，压缩字节不一定会被刷新。
	if _, err := w.Write(data); err != nil {
		return nil, err
	}

	// Close 通过将所有未写入的数据刷新到底层 io.Writer 并写入 GZIP 页脚来关闭 Writer。
	if err := w.Close(); err != nil {
		return nil, err
	}

	// Bytes 返回长度为 b.Len() 的切片，其中保存缓冲区的未读部分。
	return compressedData.Bytes(), nil
}

// 数据解压
func decompressData(data []byte) ([]byte, error) {
	// NewReader 创建一个新的 Reader 来读取给定的 reader。
	//
	// 	NewBuffer 使用 buf 作为初始内容创建并初始化一个新的 Buffer。
	r, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	// ReadAll 从 r 读取直到出现错误或 EOF，然后返回读取的数据。
	decompressedData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return decompressedData, nil
}
