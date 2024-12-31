package reedsolomon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bpfs/defs/reedsolomon"
)

func TestStreamEncoder(t *testing.T) {
	// 固定的测试文件路径
	testFilePath := "/Users/qinglong/Downloads/BPFS 白皮书.pdf"

	// 设置参数
	dataShards := 7
	parShards := 3

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

	// 重置文件指针到开始位置
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("重置文件指针失败: %v", err)
	}
	defer f.Close()

	// 使用文件哈希创建输出目录
	dir, _ := filepath.Split(testFilePath)
	outDir := filepath.Join(dir, fileHash)
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		t.Fatalf("创建输出目录失败: %v", err)
	}

	// 步骤1：创建Reed-Solomon编码器
	enc, err := reedsolomon.NewStream(dataShards, parShards)
	if err != nil {
		t.Fatalf("创建Reed-Solomon编码器失败: %v", err)
	}

	// 步骤2：打开输入文件
	f, err = os.Open(testFilePath)
	if err != nil {
		t.Fatalf("打开输入文件失败: %v", err)
	}
	defer f.Close()

	// 步骤3：获取输入文件的状态信息
	instat, err := f.Stat()
	if err != nil {
		t.Fatalf("获取文件状态失败: %v", err)
	}

	// 步骤4：计算总分片数并创建输出文件切片
	shards := dataShards + parShards
	out := make([]*os.File, shards)

	// 步骤5：创建结果文件
	for i := range out {
		outfn := fmt.Sprintf("%d", i)
		out[i], err = os.Create(filepath.Join(outDir, outfn))
		if err != nil {
			t.Fatalf("创建输出文件 %s 失败: %v", outfn, err)
		}
		defer out[i].Close()
	}

	// 步骤6：准备数据分片的写入器
	data := make([]io.Writer, dataShards)
	for i := range data {
		data[i] = out[i]
	}

	// 步骤7：执行分片操作
	err = enc.Split(f, data, instat.Size())
	if err != nil {
		t.Fatalf("分片操作失败: %v", err)
	}

	// 关闭并重新打开文件
	input := make([]io.Reader, dataShards)
	for i := range data {
		out[i].Close()
		f, err := os.Open(out[i].Name())
		if err != nil {
			t.Fatalf("重新打开文件 %d 失败: %v", i, err)
		}
		input[i] = f
		defer f.Close()
	}

	// 步骤9：创建奇偶校验输出写入器
	parity := make([]io.Writer, parShards)
	for i := range parity {
		parity[i] = out[dataShards+i]
	}

	// 步骤10：编码奇偶校验
	err = enc.Encode(input, parity)
	if err != nil {
		t.Fatalf("编码奇偶校验失败: %v", err)
	}

	// 验证生成的文件
	for i := 0; i < shards; i++ {
		outPath := filepath.Join(outDir, fmt.Sprintf("%d", i))
		_, err := os.Stat(outPath)
		if err != nil {
			t.Errorf("生成的文件 %s 不存在: %v", outPath, err)
		}
	}

	t.Logf("文件成功分割为 %d 个数据分片 + %d 个奇偶校验分片。\n文件哈希: %s", dataShards, parShards, fileHash)
}
