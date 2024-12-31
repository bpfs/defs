package reedsolomon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// 测试数据
const testData = "Hello, this is a longer string used for testing Reed-Solomon encoding. It contains various characters and symbols to ensure a comprehensive test of the encoding process. The quick brown fox jumps over the lazy dog. 1234567890 !@#$%^&*()"
const dataShards = 3
const parityShards = 2

// TestStreamEncodeWithFiles 测试使用临时文件进行编码
func TestStreamEncodeWithFiles(t *testing.T) {
	// 创建输入读取器
	r := bytes.NewReader([]byte(testData))

	// 执行编码
	dataFiles, parityFiles, err := StreamEncodeWithFiles(r, dataShards, parityShards, int64(len(testData)))
	if err != nil {
		t.Fatalf("StreamEncodeWithFiles 失败: %v", err)
	}

	// 延迟清理临时文件
	defer func() {
		cleanupFiles(dataFiles)
		cleanupFiles(parityFiles)
	}()

	// 验证数据分片
	for i, f := range dataFiles {
		content, err := os.ReadFile(f.Name())
		if err != nil {
			t.Errorf("读取数据文件 %d 失败: %v", i, err)
		} else {
			t.Logf("数据分片 %d: %s", i, string(content))
		}
	}

	// 验证奇偶校验分片
	for i, f := range parityFiles {
		content, err := os.ReadFile(f.Name())
		if err != nil {
			t.Errorf("读取奇偶校验文件 %d 失败: %v", i, err)
		} else {
			t.Logf("奇偶校验分片 %d: %s", i, string(content))
		}
	}
}

// TestStreamEncodeWithFilesFromFile 测试从实际文件进行编码并保存到指定文件夹
func TestStreamEncodeWithFilesFromFile(t *testing.T) {
	// 设置测试文件路径
	testFilePath := "/Users/qinglong/go/src/chaincodes/BPFS/DeFS 2.0/examples/BPFS 白皮书.pdf"

	// 检查文件是否存在
	_, err := os.Stat(testFilePath)
	if os.IsNotExist(err) {
		t.Fatalf("测试文件不存在: %v", err)
	}

	// 打开输入文件
	inputFile, err := os.Open(testFilePath)
	if err != nil {
		t.Fatalf("打开输入文件失败: %v", err)
	}
	defer inputFile.Close()

	// 获取文件大小
	fileInfo, err := inputFile.Stat()
	if err != nil {
		t.Fatalf("获取文件信息失败: %v", err)
	}
	fileSize := fileInfo.Size()

	// 使用 StreamEncodeWithFiles 进行编码
	dataFiles, parityFiles, err := StreamEncodeWithFiles(inputFile, dataShards, parityShards, fileSize)
	if err != nil {
		t.Fatalf("StreamEncodeWithFiles 失败: %v", err)
	}

	// 创建输出目录
	dir, file := filepath.Split(testFilePath)
	outputDir := filepath.Join(dir, file+"_shards")
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		t.Fatalf("创建输出目录失败: %v", err)
	}

	// 移动临时文件到输出目录并比较哈希值
	allFiles := append(dataFiles, parityFiles...)
	for i, f := range allFiles {
		destPath := filepath.Join(outputDir, fmt.Sprintf("%s.%d", file, i))

		// 计算移动前的哈希值
		hashBefore, err := calculateHash(f)
		if err != nil {
			t.Fatalf("计算移动前文件 %d 的哈希值失败: %v", i, err)
		}

		// 移动文件
		err = moveFile(f.Name(), destPath)
		if err != nil {
			t.Fatalf("移动文件失败: %v", err)
		}
		f.Close() // 关闭临时文件

		// 计算移动后的哈希值
		movedFile, err := os.Open(destPath)
		if err != nil {
			t.Fatalf("打开移动后的文件 %d 失败: %v", i, err)
		}
		hashAfter, err := calculateHash(movedFile)
		if err != nil {
			t.Fatalf("计算移动后文件 %d 的哈希值失败: %v", i, err)
		}
		movedFile.Close()

		// 比较哈希值
		if hashBefore != hashAfter {
			t.Errorf("分片 %d 的哈希值在移动前后不一致。移动前: %s, 移动后: %s", i, hashBefore, hashAfter)
		} else {
			t.Logf("分片 %d 的哈希值在移动前后一致: %s", i, hashBefore)
		}
	}

	// 验证输出文件
	for i := 0; i < dataShards+parityShards; i++ {
		shardPath := filepath.Join(outputDir, fmt.Sprintf("%s.%d", file, i))
		_, err := os.Stat(shardPath)
		if err != nil {
			t.Errorf("分片文件 %s 不存在: %v", shardPath, err)
		}
	}

	t.Logf("成功将文件 %s 编码为 %d 个数据分片和 %d 个奇偶校验分片，保存在 %s 目录中", testFilePath, dataShards, parityShards, outputDir)
}

// moveFile 将文件从源路径移动到目标路径
func moveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	outputFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return err
	}

	return os.Remove(sourcePath)
}
func calculateHash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
