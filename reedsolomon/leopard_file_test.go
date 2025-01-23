package reedsolomon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLeopardFileEncoder(t *testing.T) {
	t.Log(">>> 开始Reed-Solomon文件编码测试")

	// 固定的测试文件路径
	testFilePath := "/Users/qinglong/Downloads/归档2.24GB.zip"
	// testFilePath := "/Users/qinglong/Downloads/归档1g2.zip"
	// testFilePath := "/Users/qinglong/Downloads/BPFS 白皮书.pdf"
	t.Log("测试文件路径:", testFilePath)

	// 设置参数
	dataShards := 1000
	parShards := 300
	t.Log("编码参数: 数据分片=", dataShards, " 校验分片=", parShards)

	// 计算文件哈希
	f, err := os.Open(testFilePath)
	if err != nil {
		t.Fatalf("打开输入文件失败: %v", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		t.Fatalf("计算文件哈希失败: %v", err)
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))
	t.Log("原始文件哈希:", fileHash)

	// 重置文件指针到开始位置
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("重置文件指针失败: %v", err)
	}
	defer f.Close()

	// 创建临时目录用于存储分片文件
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("leopard_test_%s_*", fileHash[:8]))
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建编码器
	enc, err := NewFile(dataShards, parShards)
	if err != nil {
		t.Fatalf("创建编码器失败: %v", err)
	}

	// 分割文件
	t.Log(">>> 开始分割文件")
	shards, err := enc.SplitFile(f)
	if err != nil {
		t.Fatalf("分割文件失败: %v", err)
	}
	t.Log("<<< 文件分割完成, 生成分片数:", len(shards))

	// 编码分片
	t.Log(">>> 开始编码分片")
	err = enc.EncodeFile(shards)
	if err != nil {
		t.Fatalf("编码分片失败: %v", err)
	}
	t.Log("<<< 分片编码完成")

	// 验证分片
	t.Log(">>> 开始验证分片")
	ok, err := enc.VerifyFile(shards)
	if err != nil {
		t.Fatalf("验证分片失败: %v", err)
	}
	if !ok {
		t.Fatal("分片验证未通过")
	}
	t.Log("<<< 分片验证通过")

	// 模拟丢失分片
	t.Log(">>> 模拟丢失分片: [2, 4]")
	shards[2].Close()
	os.Remove(shards[2].Name())
	shards[2] = nil
	shards[4].Close()
	os.Remove(shards[4].Name())
	shards[4] = nil

	// 重建丢失的分片
	t.Log(">>> 开始重建丢失的分片")
	err = enc.ReconstructFile(shards)
	if err != nil {
		t.Fatalf("重建分片失败: %v", err)
	}
	t.Log("<<< 分片重建完成")

	// 再次验证分片
	t.Log(">>> 开始验证重建后的分片")
	ok, err = enc.VerifyFile(shards)
	if err != nil {
		t.Fatalf("验证重建后的分片失败: %v", err)
	}
	if !ok {
		t.Fatal("重建后的分片验证未通过")
	}
	t.Log("<<< 重建后的分片验证通过")

	// 获取原始文件名
	origName := filepath.Base(testFilePath) // "BPFS 白皮书.pdf"
	// 在扩展名前添加标识
	ext := filepath.Ext(origName)                           // ".pdf"
	baseName := origName[:len(origName)-len(ext)]           // "BPFS 白皮书"
	newName := fmt.Sprintf("%s_recovered%s", baseName, ext) // "BPFS 白皮书_recovered.pdf"

	// 创建输出文件
	outPath := filepath.Join(os.Getenv("HOME"), "Downloads", newName)
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("创建输出文件失败: %v", err)
	}
	t.Logf("输出文件保存到: %s", outPath)
	defer outFile.Close()

	// 获取原始文件大小
	originalSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		t.Fatalf("获取文件大小失败: %v", err)
	}

	// 合并分片
	t.Log(">>> 开始合并分片")
	err = enc.JoinFile(outFile, shards[:dataShards], int(originalSize))
	if err != nil {
		t.Fatalf("合并分片失败: %v", err)
	}
	t.Log("<<< 分片合并完成")

	// 验证重建文件
	t.Log(">>> 开始验证重建文件")
	outFile.Seek(0, 0)
	hasher = sha256.New()
	if _, err := io.Copy(hasher, outFile); err != nil {
		t.Fatalf("计算重建文件哈希失败: %v", err)
	}
	reconstructedHash := hex.EncodeToString(hasher.Sum(nil))
	t.Log("重建文件哈希:", reconstructedHash)

	if fileHash != reconstructedHash {
		t.Fatal("重建文件的哈希值与原始文件不匹配")
	}
	t.Log("<<< 重建文件验证通过")

	t.Log(">>> Reed-Solomon文件编码测试完成")
}

// 添加基准测试
func BenchmarkLeopardFileEncoder(b *testing.B) {
	// 创建测试数据
	dataSize := 10 * 1024 * 1024 // 10MB
	data := make([]byte, dataSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "benchmark_*.defs")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 创建编码器
		enc, err := NewFile(10, 4)
		if err != nil {
			b.Fatal(err)
		}

		// 分割并编码
		tmpFile.Seek(0, 0)
		shards, err := enc.SplitFile(tmpFile)
		if err != nil {
			b.Fatal(err)
		}

		err = enc.EncodeFile(shards)
		if err != nil {
			b.Fatal(err)
		}

		// 清理临时文件
		for _, shard := range shards {
			if shard != nil {
				shard.Close()
				os.Remove(shard.Name())
			}
		}
	}
}
