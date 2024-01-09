package segment

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

func TestWriteSegmentsToFile(t *testing.T) {
	// 创建一个新文件
	file, err := os.Create("writeSegmentsToFile.bpdf")
	if err != nil {
		t.Fatalf("文件创建错误: %v", err)
	}

	xref := NewFileXref()

	// 写入一个示例段
	data1 := []byte("DeP2P, bpfs.xyz")
	if err := WriteSegmentToFile(file, "NAME", data1, xref); err != nil {
		t.Fatalf("写入段错误: %v", err)
	}

	data2 := []byte("昨天！今天++++++++++明天（）")
	if err := WriteSegmentToFile(file, "CONTENT", data2, xref); err != nil {
		t.Fatalf("写入段错误: %+v", err)
	}

	// 保存 xref 表并关闭文件
	if err := SaveAndClose(file, xref); err != nil {
		t.Fatalf("保存 xref 表或关闭文件错误: %v", err)
	}

	fmt.Printf("xref.XrefTable:\t%v\n", xref.XrefTable)
	fmt.Printf("xref.StartXref:\t%v\n", xref.StartXref)
}

func TestLoadXrefFromFile(t *testing.T) {
	// 重新打开文件以读取
	file, err := os.Open("writeSegmentsToFile.bpdf")
	if err != nil {
		t.Fatalf("文件创建错误: %v", err)
	}

	xref, err := LoadXrefFromFile(file)
	if err != nil {
		t.Fatalf("xref错误: %v", err)
	}
	fmt.Printf("xref.XrefTable:\t%v\n", xref.XrefTable)
	fmt.Printf("xref.StartXref:\t%v\n", xref.StartXref)
}

// 测试从字节切片中读取指定的单个段
func TestReadFieldFromBytes(t *testing.T) {
	// 重新打开文件以读取
	fileData, err := os.ReadFile("writeSegmentsToFile.bpdf")
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}

	fmt.Printf("大小:\t%d\n", len(fileData))

	bytesReader := bytes.NewReader(fileData)
	// file, err := os.Open("writeSegmentsToFile.bpdf")
	// if err != nil {
	// 	t.Fatalf("文件创建错误: %v", err)
	// }

	xref, err := LoadXrefFromBuffer(bytesReader)
	// xref, err := LoadXrefFromBuffer(file)
	if err != nil {
		t.Fatalf("读取xref表失败: %v", err)
	}

	fmt.Printf("xref.XrefTable:\t%v\n", xref.XrefTable)
	fmt.Printf("xref.StartXref:\t%v\n", xref.StartXref)

	// 读取数据
	// readData, err := ReadFieldFromBytes(fileData, "[标题-1]", xref)
	// if err != nil {
	// 	t.Fatalf("读取段失败: %v", err)
	// }
	// fmt.Printf("NAME: %s\n", readData)
}

// 测试批量写入
func TestWriteFileSegment(t *testing.T) {
	// 创建一个新文件
	file, err := os.Create("writeFileSegment.bpdf")
	if err != nil {
		t.Fatalf("文件创建错误: %v", err)
	}

	xref := NewFileXref()

	data := map[string][]byte{
		"[标题-1]": []byte("昨天！今天++++++++++明天（）"),
		"[标题-2]": []byte("DeP2P, bpfs.xyz"),
	}

	if err := WriteSegmentsToFile(file, data, xref); err != nil {
		t.Fatalf("写入段错误: %v", err)
	}

	// 保存 xref 表并关闭文件
	if err := SaveAndClose(file, xref); err != nil {
		t.Fatalf("保存 xref 表或关闭文件错误: %v", err)
	}
}

// 测试从文件中批量读取多个段
func TestReadSegmentsFromFile(t *testing.T) {
	// 重新打开文件以读取
	file, err := os.Open("writeFileSegment.bpdf")
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer file.Close()

	segmentTypes := []string{
		`[标题-1]`,
		`[标题-2]`,
		`[标题-10]`,
	}

	// 读取数据
	readData, _, err := ReadFileSegments(file, segmentTypes)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}
	for k, v := range readData {
		fmt.Printf("%v\n", k)
		fmt.Printf("Data:\t%s\n", v.Data)
		fmt.Printf("Error:\t%v\n", v.Error)
		fmt.Print("\n")
	}
}

// go test -v -bench=. -benchtime=10m ./... -run TestRead
// 测试从文件中读取指定的单个段
func TestReadFileSegment(t *testing.T) {
	// 重新打开文件以读取
	file, err := os.Open("writeFileSegment.bpdf")
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 读取数据
	readData1, _, err := ReadFileSegment(file, "[标题-1]")
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}
	fmt.Printf("NAME: %s\n", readData1)
}

// 测试从字节切片中读取指定的单个段
// func TestReadFieldsFromBytes(t *testing.T) {
// 	// 重新打开文件以读取
// 	fileData, err := os.ReadFile("writeFileSegment.bpdf")
// 	if err != nil {
// 		t.Fatalf("读取文件失败: %v", err)
// 	}

// 	xref, err := LoadXrefFromBuffer(fileData)
// 	if err != nil {
// 		t.Fatalf("读取xref表失败: %v", err)
// 	}
// 	fmt.Printf("xref:\t%v\n", xref)

// 	segmentTypes := []string{
// 		`[标题-1]`,
// 		`[标题-2]`,
// 		`[标题-10]`,
// 	}

// 	// 读取数据
// 	readData, err := ReadFieldsFromBytes(fileData, segmentTypes, xref)
// 	if err != nil {
// 		t.Fatalf("读取段失败: %v", err)
// 	}
// 	for k, v := range readData {
// 		fmt.Printf("%v\n", k)
// 		fmt.Printf("Data:\t%s\n", v.Data)
// 		fmt.Printf("Error:\t%v\n", v.Error)
// 		fmt.Print("\n")
// 	}
// }
