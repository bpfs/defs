package tempfile

import (
	"bytes"
	"math/rand"
	"testing"
)

// TestWriteAndRead 测试写入和读取功能
func TestWriteAndRead(t *testing.T) {
	// 准备测试数据
	key := "test_key"
	expectedData := []byte("这是一些测试数据")

	// 测试写入
	err := Write(key, expectedData)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 测试读取
	actualData, err := Read(key)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	// 打印读取的内容
	t.Logf("读取的内容: %s", string(actualData))

	// 比较写入和读取的数据
	if !bytes.Equal(expectedData, actualData) {
		t.Errorf("读取的数据与写入的数据不匹配。\n期望: %v\n实际: %v", expectedData, actualData)
	}

	// 再次尝试读取，应该返回错误
	_, err = Read(key)
	if err == nil {
		t.Error("期望在第二次读取时返回错误，但没有")
	}
}

// TestDelete 测试删除功能
func TestDelete(t *testing.T) {
	// 准备测试数据
	key := "test_delete_key"
	data := []byte("要删除的测试数据")

	// 写入数据
	err := Write(key, data)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 删除数据
	err = Delete(key)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 尝试读取已删除的数据，应该返回错误
	_, err = Read(key)
	if err == nil {
		t.Error("期望读取已删除的数据时返回错误，但没有")
	}
}

// TestExists 测试文件存在性检查功能
func TestExists(t *testing.T) {
	// 准备测试数据
	key := "test_exists_key"
	data := []byte("测试数据")

	// 写入数据
	err := Write(key, data)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 检查文件是否存在
	exists, err := Exists(key)
	if err != nil {
		t.Fatalf("检查文件存在性失败: %v", err)
	}
	if !exists {
		t.Error("期望文件存在，但返回不存在")
	}

	// 删除文件
	err = Delete(key)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 再次检查文件是否存在
	exists, err = Exists(key)
	if err != nil {
		t.Fatalf("检查文件存在性失败: %v", err)
	}
	if exists {
		t.Error("期望文件不存在，但返回存在")
	}
}

// TestSize 测试获取文件大小功能
func TestSize(t *testing.T) {
	// 准备测试数据
	key := "test_size_key"
	data := []byte("测试数据大小")

	// 写入数据
	err := Write(key, data)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 获取文件大小
	size, err := Size(key)
	if err != nil {
		t.Fatalf("获取文件大小失败: %v", err)
	}

	// 检查文件大小是否正确
	expectedSize := int64(len(data))
	if size != expectedSize {
		t.Errorf("文件大小不匹配。期望: %d, 实际: %d", expectedSize, size)
	}

	// 清理
	err = Delete(key)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
}

func TestWriteOptimized(t *testing.T) {
	tests := []struct {
		name     string
		dataSize int
		wantErr  bool
	}{
		{"SmallWrite", _4KB, false},
		{"MediumWrite", _32KB, false},
		{"LargeWrite", _1MB, false},
		{"HugeWrite", _4MB, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.dataSize)
			// 填充随机数据
			fillRandom(data)

			key := "test_" + tt.name
			err := Write(key, data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// 验证写入
			got, err := Read(key)
			if err != nil {
				t.Errorf("Read() error = %v", err)
				return
			}
			if !bytes.Equal(got, data) {
				t.Errorf("Read() got = %v, want %v", got, data)
			}
		})
	}
}

func TestWriteBatchOptimized(t *testing.T) {
	segments := map[string][]byte{
		"small1": make([]byte, _4KB),
		"small2": make([]byte, _4KB),
		"large1": make([]byte, _1MB),
		"large2": make([]byte, _1MB),
	}

	// 填充随机数据
	for _, data := range segments {
		fillRandom(data)
	}

	err := WriteBatchOptimized(segments)
	if err != nil {
		t.Errorf("WriteBatchOptimized() error = %v", err)
		return
	}

	// 验证写入
	for id, want := range segments {
		got, err := Read(id)
		if err != nil {
			t.Errorf("Read() error = %v", err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Read() got = %v, want %v", got, want)
		}
	}
}

// fillRandom 用随机数据填充字节切片
func fillRandom(p []byte) {
	for i := 0; i < len(p); i += 7 {
		val := rand.Int63()
		for j := 0; i+j < len(p) && j < 7; j++ {
			p[i+j] = byte(val)
			val >>= 8
		}
	}
}
