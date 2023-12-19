package defs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// 测试NewFileXref函数
func TestNewFileXref(t *testing.T) {
	xref := NewFileXref()
	assert.NotNil(t, xref.XrefTable)
}

// 测试WriteSegmentToFile和ReadSegmentToFile函数
func TestWriteAndReadSegmentToFile(t *testing.T) {
	xref := NewFileXref()
	file, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err)
	defer os.Remove(file.Name())
	defer file.Close()

	segmentType := "test_segment"
	data := []byte("This is some test data.")

	// 写入数据段
	err = WriteSegmentToFile(file, segmentType, data, xref)
	assert.NoError(t, err)

	// 读取数据段
	readData, err := ReadSegmentToFile(file, segmentType, xref)
	assert.NoError(t, err)
	assert.Equal(t, data, readData)
}

// 测试WriteSegmentToBuffer和ReadSegmentFromBuffer函数
func TestWriteAndReadSegmentToBuffer(t *testing.T) {
	xref := NewFileXref()
	var buffer bytes.Buffer

	segmentType := "test_segment"
	data := []byte("This is some test data.")

	// 写入数据段
	err := WriteSegmentToBuffer(&buffer, segmentType, data, xref)
	assert.NoError(t, err)

	// 读取数据段
	readData, err := ReadSegmentFromBuffer(&buffer, segmentType, xref)
	assert.NoError(t, err)
	assert.Equal(t, data, readData)
}

// 测试SaveAndClose函数
func TestSaveAndClose(t *testing.T) {
	xref := NewFileXref()
	file, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err)
	defer os.Remove(file.Name())

	// 先写入一些数据
	err = WriteSegmentToFile(file, "test1", []byte("data1"), xref)
	assert.NoError(t, err)
	err = WriteSegmentToFile(file, "test2", []byte("data2"), xref)
	assert.NoError(t, err)

	// 保存并关闭
	err = SaveAndClose(file, xref)
	assert.NoError(t, err)
}

// 测试LoadXref函数
func TestLoadXref(t *testing.T) {
	// 创建一个临时文件并写入一些数据
	file, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err)
	defer os.Remove(file.Name())

	xref := NewFileXref()
	segmentTypes := []string{"test1", "test2"}
	data := []byte("some data")

	for _, segmentType := range segmentTypes {
		err := WriteSegmentToFile(file, segmentType, data, xref)
		assert.NoError(t, err)
	}

	// 保存并关闭文件
	err = SaveAndClose(file, xref)
	assert.NoError(t, err)

	// 重新打开文件并加载xref
	file, err = os.Open(file.Name())
	assert.NoError(t, err)
	loadedXref, err := LoadXref(file)
	assert.NoError(t, err)
	assert.Equal(t, xref.XrefTable, loadedXref.XrefTable)
}

// 测试AppendSegmentToFile函数
func TestAppendSegmentToFile(t *testing.T) {
	xref := NewFileXref()
	file, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err)
	defer os.Remove(file.Name())

	// 写入初始数据段
	err = WriteSegmentToFile(file, "test1", []byte("data1"), xref)
	assert.NoError(t, err)

	// 追加数据段
	err = AppendSegmentToFile(file, "test2", []byte("data2"), xref)
	assert.NoError(t, err)

	// 读取并验证所有数据段
	for segmentType, entry := range xref.XrefTable {
		file.Seek(entry.Offset, io.SeekStart)
		data, err := ReadSegmentToFile(file, segmentType, xref)
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("data%s", strings.TrimPrefix(segmentType, "test")), string(data))
	}
}

////////////////////////////////////////////////////////////////////////

func TestSegment(t *testing.T) {
	// 创建一个新文件
	file, err := os.Create("example.bpdf")
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

	// 重新打开文件以读取
	file, err = os.Open("example.bpdf")
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	logrus.Printf("xref: %v\tStartXref: %v", xref.XrefTable, xref.StartXref)

	// 读取并验证数据
	readData1, err := ReadSegmentToFile(file, "NAME", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}
	if !bytes.Equal(readData1, data1) {
		t.Errorf("读取的数据不匹配. 期望 %v, 得到 %v", data1, readData1)
	}

	readData2, err := ReadSegmentToFile(file, "CONTENT", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}
	if !bytes.Equal(readData2, data2) {
		t.Errorf("读取的数据不匹配. 期望 %v, 得到 %v", data2, readData2)
	}

	logrus.Printf("NAME: %s\tCONTENT: %s", readData1, readData2)
}

// 测试批量写入
func TestSegments(t *testing.T) {
	// 创建一个新文件
	file, err := os.Create("example.bpdf")
	if err != nil {
		t.Fatalf("文件创建错误: %v", err)
	}

	xref := NewFileXref()

	data := map[string][]byte{
		"apple":  []byte("昨天！今天++++++++++明天（）"),
		"banana": []byte("DeP2P, bpfs.xyz"),
	}

	if err := WriteSegmentsToFile(file, data, xref); err != nil {
		t.Fatalf("写入段错误: %v", err)
	}

	// 保存 xref 表并关闭文件
	if err := SaveAndClose(file, xref); err != nil {
		t.Fatalf("保存 xref 表或关闭文件错误: %v", err)
	}

}
func TestUpdateSegment(t *testing.T) {
	// 创建一个新文件
	file, err := os.Create("example.bpdf")
	if err != nil {
		t.Fatalf("文件创建错误: %v", err)
	}

	xref := NewFileXref()

	// 写入一个示例段
	data1 := []byte("DeP2P, bpfs.xyz")
	if err := WriteSegmentToFile(file, "NAME", data1, xref); err != nil {
		t.Fatalf("写入段错误: %v", err)
	}

	data2 := []byte("昨天！今天～明天（）")
	if err := WriteSegmentToFile(file, "CONTENT", data2, xref); err != nil {
		t.Fatalf("写入段错误: %+v", err)
	}

	// 保存 xref 表并关闭文件
	if err := SaveAndClose(file, xref); err != nil {
		t.Fatalf("保存 xref 表或关闭文件错误: %v", err)
	}

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	logrus.Printf("xref: %v\tStartXref: %v", xref.XrefTable, xref.StartXref)
}

// go test -v -bench=. -benchtime=10m ./... -run TestRead
func TestRead(t *testing.T) {
	// 重新打开文件以读取
	file, err := os.Open("example.bpdf")
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

	logrus.Printf("xref: %v\tStartXref: %v", xref.XrefTable, xref.StartXref)

	// 读取数据
	readData1, err := ReadSegmentToFile(file, "NAME", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}
	logrus.Printf("NAME: %s", readData1)

	readData2, err := ReadSegmentToFile(file, "CONTENT", xref)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("读取段失败: %v", err)
	}
	logrus.Printf("CONTENT: %s", readData2)

	// logrus.Printf("NAME: %s\tCONTENT: %s", readData1, readData2)

}

func TestReadSegment(t *testing.T) {
	// 重新打开文件以读取
	file, err := os.Open("example.bpdf")
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

	logrus.Printf("xref: %v\tStartXref: %v", xref.XrefTable, xref.StartXref)

	// 读取数据

	readData2, err := ReadSegment(file, "CONTENT", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}

	logrus.Printf("CONTENT: \t%s", readData2)

	readData1, err := ReadSegmentToFile(file, "NAME", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}

	readData3, err := ReadSegment(file, "NAME", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}

	logrus.Printf("NAME: \t%s\tNAME: \t%s", readData1, readData3)

	readData4, err := ReadSegmentToFile(file, "XXXYYY", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}

	logrus.Printf("XXXYYY: \t%s", readData4)

}

func TestAppendSegmentToFile2(t *testing.T) {
	file, err := os.OpenFile("example.bpdf", os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	xref, err := LoadXref(file)
	if err != nil {
		t.Fatalf("从文件加载 xref 表失败: %v", err)
	}

	data1 := []byte("新增标签内容")
	if err := AppendSegmentToFile(file, "XXXYYY3", data1, xref); err != nil {
		t.Fatalf("写入段错误: %v", err)
	}

	readData1, err := ReadSegmentToFile(file, "XXXYYY3", xref)
	if err != nil {
		t.Fatalf("读取段失败: %v", err)
	}

	logrus.Printf("XXXYYY3: \t%s", readData1)

}

// func TestXref(t *testing.T) {
// 	// 重新打开文件以读取
// 	file, err := os.Open("example.bpdf")
// 	if err != nil {
// 		fmt.Println("打开文件失败:", err)
// 		return
// 	}
// 	defer file.Close()
// 	// 重置文件指针
// 	file.Seek(0, io.SeekStart)

// 	xref, err := LoadXref(file)
// 	if err != nil {
// 		fmt.Println("从文件加载 xref 表失败:", err)
// 		return
// 	}

// 	// 重置文件指针
// 	file.Seek(0, io.SeekStart)

// 	// 读取数据
// 	// readData1, err := ReadSegmentToFile(file, "NAME", xref)
// 	// if err != nil {
// 	// 	t.Fatalf("读取段失败: %v", err)
// 	// }

// 	readData2, err := ReadSegmentToFile(file, "CONTENT", xref)
// 	if err != nil && err.Error() != "EOF" {
// 		t.Fatalf("读取段失败: %v", err)
// 	}

// 	logrus.Printf("CONTENT: %s", readData2)

// 	file.Seek(0, io.SeekStart)

// 	if err := ReplaceSegmentInFile(file, "CONTENT", []byte("昨天！今天==========明天（）")); err != nil {
// 		fmt.Println("替换文件失败:", err)
// 		return
// 	}

// 	data, err := ReadSegmentToFile(file, "NAME", xref)
// 	if err != nil {
// 		fmt.Println("读取段失败:", err)
// 		return
// 	}

// 	fmt.Println("读取的数据:", string(data))
// }

// func TestReplaceSegmentInFile(t *testing.T) {
// 	file, err := os.OpenFile("example.bpdf", os.O_RDWR, 0644)
// 	if err != nil {
// 		t.Fatalf("打开文件失败: %v", err)
// 	}
// 	defer file.Close()

// 	// 重置文件指针
// 	file.Seek(0, io.SeekStart)

// 	xref, err := LoadXref(file)
// 	if err != nil {
// 		t.Fatalf("从文件加载 xref 表失败: %v", err)
// 	}

// 	data1, err := ReadSegmentToFile(file, "CONTENT", xref)
// 	if err != nil && err != io.EOF {
// 		t.Fatalf("读取段失败: %v", err)
// 	}

// 	fmt.Printf("CONTENT before: %s\n", string(data1))

// 	if err := ReplaceSegmentAndUpdateFile(file, "CONTENT", []byte("昨天！今天==========明天（）"), xref); err != nil {
// 		t.Fatalf("替换文件失败: %v", err)
// 	}

// 	data2, err := ReadSegmentToFile(file, "CONTENT", xref)
// 	if err != nil {
// 		t.Fatalf("读取段失败: %v", err)
// 	}

// 	fmt.Printf("CONTENT after: %s\n", string(data2))
// }
