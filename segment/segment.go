package segment

import (
	"os"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

const maxSegmentTypeLen = 100 // 段类型名称的最大长度

// WriteFileSegment 创建新文件并将数据写入
// 参数:
//   - filePath: string 要创建和写入的文件路径
//   - data: map[string][]byte 要写入的段数据，键为段类型，值为段内容
//
// 返回值:
//   - error 可能出现的错误
func WriteFileSegment(filePath string, data map[string][]byte) error {
	// 检查文件是否存在，如果存在则删除
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}
	}

	// 创建新文件
	file, err := os.Create(filePath)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}
	defer file.Close()

	// 创建一个新的 FileXref 对象，并初始化 xref 表
	xref := NewFileXref()

	// 批量将段写入文件
	if err := WriteSegmentsToFile(file, data, xref); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 保存 xref 表并关闭文件
	return SaveAndClose(file, xref)
}

// ReadFileSegments 从指定文件中读取一个或多个段
// 参数:
//   - file: *os.File 待读取的文件对象
//   - segmentTypes: []string 要读取的段类型列表
//   - fileXref: ...*FileXref 可选参数，已解析的 Xref 表
//
// 返回值:
//   - map[string]*SegmentReadResult 读取结果映射
//   - *FileXref 解析得到的 Xref 表
//   - error 可能出现的错误
func ReadFileSegments(file *os.File, segmentTypes []string, fileXref ...*FileXref) (map[string]*SegmentReadResult, *FileXref, error) {
	// 查找文件中的 "startxref" 位置
	startXrefPos, err := findStartXref(file)
	if err != nil {
		logrus.Errorf("[%s]查找 startxref 失败: %v", debug.WhereAmI(), err)
		return nil, nil, err
	}

	var xref *FileXref
	// 如果提供了 Xref 表，则使用提供的，否则解析文件中的 Xref 表
	if len(fileXref) > 0 {
		xref = fileXref[0]
	} else {
		xref, err = parseXref(file, startXrefPos)
		if err != nil {
			logrus.Errorf("[%s]解析 Xref 表失败: %v", debug.WhereAmI(), err)
			return nil, nil, err
		}
	}

	// 读取指定的段
	segmentResults, err := readSegment(file, segmentTypes, xref)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, nil, err
	}

	results := make(map[string]*SegmentReadResult)
	for segmentType, result := range segmentResults {
		results[segmentType] = result
	}

	return results, xref, nil
}

// ReadFileSegment 从指定文件中读取一个指定的段
// 参数:
//   - file: *os.File 待读取的文件对象
//   - segmentType: string 要读取的段类型
//
// 返回值:
//   - []byte 读取的段内容
//   - *FileXref 解析得到的 Xref 表
//   - error 可能出现的错误
func ReadFileSegment(file *os.File, segmentType string) ([]byte, *FileXref, error) {
	// 从指定文件中读取一个或多个段
	results, xref, err := ReadFileSegments(file, []string{segmentType})
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, nil, err
	}

	// 获取指定段的读取结果
	result, found := results[segmentType]
	if !found || result.Error != nil {
		if result.Error != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), result.Error)
			return nil, nil, result.Error
		}
		return nil, nil, result.Error
	}

	return result.Data, xref, nil
}
