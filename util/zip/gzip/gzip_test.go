package gzip

import (
	"bytes"
	"testing"
)

// 测试数据压缩和解压
func TestCompressDecompress(t *testing.T) {
	// 待压缩的原始数据
	originalData := []byte("这是一段用于测试的数据。")

	// 打印原始数据的大小
	t.Logf("原始数据大小: %d bytes", len(originalData))

	// 数据压缩
	compressedData, err := CompressData(originalData)
	if err != nil {
		t.Error("数据压缩失败：", err)
		return
	}

	// 打印压缩后的数据大小
	t.Logf("压缩后的数据大小: %d bytes", len(compressedData))

	// 数据解压
	decompressedData, err := DecompressData(compressedData)
	if err != nil {
		t.Error("数据解压失败：", err)
		return
	}

	// 打印解压后的数据大小
	t.Logf("解压后的数据大小: %d bytes", len(decompressedData))

	// 检查解压后的数据是否与原始数据相同
	if !bytes.Equal(originalData, decompressedData) {
		t.Error("解压后的数据与原始数据不匹配。")
		return
	}

	t.Log("数据压缩和解压测试通过。")
}
