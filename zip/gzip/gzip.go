package gzip

import (
	"bytes"
	"compress/gzip"
	"io"

	logging "github.com/dep2p/log"
)

var logger = logging.Logger("gzip")

// 定义压缩级别常量
const (
	// NoCompression 表示不进行压缩
	NoCompression = gzip.NoCompression
	// BestSpeed 表示最快的压缩速度，但压缩率较低
	BestSpeed = gzip.BestSpeed
	// BestCompression 表示最高的压缩率，但压缩速度较慢
	BestCompression = gzip.BestCompression
	// DefaultCompression 表示默认的压缩级别，平衡了速度和压缩率
	DefaultCompression = gzip.DefaultCompression
	// HuffmanOnly 表示仅使用哈夫曼编码进行压缩
	HuffmanOnly = gzip.HuffmanOnly
)

// CompressData 压缩数据
// 参数:
//   - data: []byte 需要压缩的原始数据
//
// 返回值:
//   - []byte: 压缩后的数据
//   - error: 如果压缩过程中发生错误，返回相应的错误信息
func CompressData(data []byte) ([]byte, error) {
	// Buffer 是一个可变大小的字节缓冲区，具有 Read 和 Write 方法。
	var compressedData bytes.Buffer

	// 	NewWriter 返回一个新的 Writer。 对返回的 writer 的写入被压缩并写入 w。
	w := gzip.NewWriter(&compressedData)

	// Write 将 p 的压缩形式写入底层 io.Writer。 在 Writer 关闭之前，压缩字节不一定会被刷新。
	if _, err := w.Write(data); err != nil {
		logger.Error("写入压缩数据失败:", err)
		return nil, err
	}

	// Close 通过将所有未写入的数据刷新到底层 io.Writer 并写入 GZIP 页脚来关闭 Writer。
	if err := w.Close(); err != nil {
		logger.Error("关闭压缩写入器失败:", err)
		return nil, err
	}

	// Bytes 返回长度为 b.Len() 的切片，其中保存缓冲区的未读部分。
	return compressedData.Bytes(), nil
}

// DecompressData 解压数据
// 参数:
//   - data: []byte 需要解压的压缩数据
//
// 返回值:
//   - []byte: 解压后的原始数据
//   - error: 如果解压过程中发生错误，返回相应的错误信息
func DecompressData(data []byte) ([]byte, error) {
	// NewReader 创建一个新的 Reader 来读取给定的 reader。
	//
	// 	NewBuffer 使用 buf 作为初始内容创建并初始化一个新的 Buffer。
	r, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		logger.Error("创建解压缩读取器失败:", err)
		return nil, err
	}

	// ReadAll 从 r 读取直到出现错误或 EOF，然后返回读取的数据。
	decompressedData, err := io.ReadAll(r)
	if err != nil {
		logger.Error("读取解压缩数据失败:", err)
		return nil, err
	}

	return decompressedData, nil
}

// NewWriterLevel 返回一个新的带有指定压缩级别的 gzip.Writer
// 参数:
//   - w: io.Writer 底层的写入器
//   - level: int 压缩级别
//
// 返回值:
//   - *gzip.Writer: 新创建的 gzip.Writer
//   - error: 如果创建过程中发生错误，返回相应的错误信息
func NewWriterLevel(w io.Writer, level int) (*gzip.Writer, error) {
	// 创建指定压缩级别的 gzip.Writer
	gw, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		logger.Error("创建指定压缩级别的 gzip.Writer 失败:", err)
		return nil, err
	}
	// 返回创建的 gzip.Writer
	return gw, nil
}
