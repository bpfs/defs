package segment

import (
	"fmt"
	"io"
	"os"
	"testing"
)

// go test -v -bench=. -benchtime=10m ./... -run TestReadSegmentToFile
// 测试从文件中读取指定的单个段
func TestReadSegmentToFile(t *testing.T) {
	// 重新打开文件以读取
	file, err := os.Open("/Users/wesign006/Downloads/QmNLtAhaakwTMNZ5f9vo1GHn3mABRse8BPA6JXDcs5uM4U/903c5f54c40e4509be94498c32ffc0d85f9b058d40fb30fc6883b21186bde7f0/55d26baa3f823c52e0fb22973c27b1a69b324bf95c603467622df0db55a02532")
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	xref, err := LoadXref(file)
	if err != nil {
		fmt.Println("从文件加载 xref 表失败:", err)
		return
	}

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	fmt.Printf("xref: %v\tStartXref: %v\n", xref.XrefTable, xref.StartXref)

	// 读取数据
	readData, err := readSegmentInternal(file, []string{"SHARED"}, xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}
	result, _ := readData["SHARED"]
	// 创建类型解码器
	codec := NewTypeCodec()
	// 解码签名
	sharedDecode, err := codec.Decode(result.Data)
	if err != nil {
		logger.Errorf("解码签名失败: %v", err)

	}
	fmt.Println("======", sharedDecode)
	gotType := fmt.Sprintf("%T", sharedDecode)
	fmt.Println("======", gotType)

}
