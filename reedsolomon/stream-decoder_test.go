package reedsolomon

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestStreamDecodeFile(t *testing.T) {
	// 使用与编码器测试相同的参数
	testFilePath := "/Users/qinglong/go/src/chaincodes/BPFS/DeFS 2.0/examples/BPFS 白皮书.pdf"
	dataShards := 4
	parShards := 2

	// 首先运行编码器测试以生成分片（假设已经运行过）
	// TestStreamEncoder(t)

	// 执行解码
	decodedFilePath, err := StreamDecodeFile(testFilePath, dataShards, parShards)
	if err != nil {
		t.Fatalf("StreamDecodeFile 失败: %v", err)
	}

	// 比较原始文件和解码后的文件
	if err := compareFiles(testFilePath, decodedFilePath); err != nil {
		t.Fatalf("解码后的文件与原始文件不匹配: %v", err)
	}

	t.Logf("解码完成，输出文件: %s", decodedFilePath)
	t.Log("解码后的文件与原始文件匹配")

	// 清理解码后的文件
	// os.Remove(decodedFilePath)
}

func compareFiles(file1, file2 string) error {
	f1, err := os.Open(file1)
	if err != nil {
		return fmt.Errorf("打开文件 %s 失败: %v", file1, err)
	}
	defer f1.Close()

	f2, err := os.Open(file2)
	if err != nil {
		return fmt.Errorf("打开文件 %s 失败: %v", file2, err)
	}
	defer f2.Close()

	const chunkSize = 64 * 1024
	for {
		b1 := make([]byte, chunkSize)
		_, err1 := f1.Read(b1)

		b2 := make([]byte, chunkSize)
		_, err2 := f2.Read(b2)

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return nil // 文件读取完毕，内容匹配
			}
			return fmt.Errorf("读取文件时出错: %v / %v", err1, err2)
		}

		if string(b1) != string(b2) {
			return fmt.Errorf("文件内容不匹配")
		}
	}
}
