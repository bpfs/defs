package protocol

import (
	"compress/gzip"
)

// getDecompressor 获取解压缩器
func (h *Handler) getDecompressor(typ CompressionType) Compressor {
	switch typ {
	case CompressGzip:
		return NewGzipCompressor(gzip.DefaultCompression)
	default:
		return nil
	}
}
