package tempfile

import (
	"bytes"
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
