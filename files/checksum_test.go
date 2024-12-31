package files

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestSHA256Consistency(t *testing.T) {
	// 测试数据
	testData := []byte("Hello, World!")

	// 1. 直接计算字节切片的哈希
	directHash := GetBytesSHA256(testData)

	// 2. 写入临时文件并计算文件的哈希
	tmpFile, err := os.CreateTemp("", "sha256test")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // 清理临时文件

	// 写入测试数据
	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatalf("写入测试数据失败: %v", err)
	}

	// 重置文件指针到开始位置
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("重置文件指针失败: %v", err)
	}

	// 计算文件哈希
	fileHash, err := GetFileSHA256(tmpFile)
	if err != nil {
		t.Fatalf("计算文件哈希失败: %v", err)
	}

	// 比较两种方式得到的哈希值
	if !bytes.Equal(directHash, fileHash) {
		t.Errorf("哈希值不匹配:\n直接计算: %x\n文件计算: %x", directHash, fileHash)
	}
}
