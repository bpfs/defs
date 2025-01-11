package reedsolomon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bpfs/defs/v2/reedsolomon"
)

func TestStreamDecoder(t *testing.T) {
	// 基础目录和文件哈希
	hashDir := "/Users/qinglong/Downloads/59e660ba8e7c4ff11e4fc63888e0755622ff1f3abc82f40202accffe1d640595"
	// hashDir := "/Users/qinglong/Downloads/df5d236ba9e28b91c4fcb34d2790ca835e629b114718ad2142ab2c9fcd96a89e"
	// 指定要恢复到的目标文件路径
	targetFile := "/Users/qinglong/Downloads/42.pdf"

	dataShards := 7
	parShards := 3
	// dataShards := 6
	// parShards := 2

	// 创建编码矩阵
	enc, err := reedsolomon.NewStream(dataShards, parShards)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}

	// 打开输入文件
	shards, _, err := openInput(dataShards, parShards, hashDir)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}

	// 验证分片
	ok, _ := enc.Verify(shards)
	if ok {
		t.Log("无需重建")
	} else {
		t.Log("验证失败。正在重建数据")
		// 重新打开输入文件
		shards, _, err = openInput(dataShards, parShards, hashDir)
		if err != nil {
			t.Fatalf("Error: %s", err.Error())
		}
		// 创建输出目标写入器
		out := make([]io.Writer, len(shards))
		for i := range out {
			if shards[i] == nil {
				outfn := filepath.Join(hashDir, fmt.Sprintf("%d", i))
				t.Logf("Creating %s", outfn)
				out[i], err = os.Create(outfn)
				if err != nil {
					t.Fatalf("Error: %s", err.Error())
				}
			}
		}
		// 重建数据
		err = enc.Reconstruct(shards, out)
		if err != nil {
			t.Fatalf("重建失败 - %v", err)
		}
		// 关闭输出文件
		for i := range out {
			if out[i] != nil {
				err := out[i].(*os.File).Close()
				if err != nil {
					t.Fatalf("Error: %s", err.Error())
				}
			}
		}
		// 重新打开输入文件并验证
		shards, _, err = openInput(dataShards, parShards, hashDir)
		if err != nil {
			t.Fatalf("Error: %s", err.Error())
		}
		ok, err = enc.Verify(shards)
		if !ok {
			t.Fatalf("重建后验证失败，数据可能已损坏: %v", err)
		}
		if err != nil {
			t.Fatalf("Error: %s", err.Error())
		}
	}

	// 创建目标文件的目录（如果不存在）
	targetDir := filepath.Dir(targetFile)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}

	// 创建目标文件
	t.Logf("正在将数据写入 %s", targetFile)
	f, err := os.Create(targetFile)
	if err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}
	defer f.Close()

	// 重新打开输入文件
	shards, size, err := openInput(dataShards, parShards, hashDir)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}

	// 合并分片并写入目标文件
	err = enc.Join(f, shards, int64(dataShards)*size)
	if err != nil {
		t.Fatalf("Error: %s", err.Error())
	}

	// 验证恢复的文件
	restoredStat, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("无法获取恢复后文件信息: %v", err)
	}

	if restoredStat.Size() == 0 {
		t.Fatalf("恢复后的文件大小为0")
	}

	t.Logf("文件恢复成功，恢复后的文件大小: %d bytes", restoredStat.Size())
}

func openInput(dataShards, parShards int, hashDir string) (r []io.Reader, size int64, err error) {
	shards := make([]io.Reader, dataShards+parShards)
	for i := range shards {
		infn := filepath.Join(hashDir, fmt.Sprintf("%d", i))
		fmt.Println("Opening", infn)
		f, err := os.Open(infn)
		if err != nil {
			fmt.Println("Error reading file", err)
			shards[i] = nil
			continue
		} else {
			shards[i] = f
		}
		stat, err := f.Stat()
		if err != nil {
			return nil, 0, err
		}
		if stat.Size() > 0 {
			if size == 0 {
				size = stat.Size()
			} else if size != stat.Size() {
				return nil, 0, fmt.Errorf("分片大小不一致: %d != %d", size, stat.Size())
			}
		} else {
			shards[i] = nil
		}
	}
	return shards, size, nil
}
