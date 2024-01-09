package segment

import (
	"fmt"
	"os"
)

const maxSegmentTypeLen = 100

// WriteFileSegment 创建新文件并将数据写入
func WriteFileSegment(filePath string, data map[string][]byte) error {
	// 检查文件是否存在，如果存在则删除
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}

	// 创建新文件
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 创建一个新的 FileXref 对象，并初始化 xref 表
	xref := NewFileXref()

	// 批量将段写入文件
	if err := WriteSegmentsToFile(file, data, xref); err != nil {
		return err
	}

	// 保存 xref 表并关闭文件
	return SaveAndClose(file, xref)
}

// ReadFileSegments 从指定文件中读取一个或多个段
func ReadFileSegments(file *os.File, segmentTypes []string, fileXref ...*FileXref) (map[string]*SegmentReadResult, *FileXref, error) {
	startXrefPos, err := findStartXref(file)
	if err != nil {
		return nil, nil, fmt.Errorf("finding startxref failed: %w", err)
	}

	var xref *FileXref
	if len(fileXref) > 0 {
		xref = fileXref[0]
	} else {
		xref, err = parseXref(file, startXrefPos)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing Xref table failed: %w", err)
		}
	}

	segmentResults, err := readSegment(file, segmentTypes, xref)
	if err != nil {
		return nil, nil, err
	}

	results := make(map[string]*SegmentReadResult)
	for segmentType, result := range segmentResults {
		results[segmentType] = result
	}

	return results, xref, nil
}

// ReadFileSegment 从指定文件中读取一个指定的段
func ReadFileSegment(file *os.File, segmentType string) ([]byte, *FileXref, error) {
	// 从指定文件中读取一个或多个段
	results, xref, err := ReadFileSegments(file, []string{segmentType})
	if err != nil {
		return nil, nil, err
	}

	result, found := results[segmentType]
	if !found || result.Error != nil {
		if result.Error != nil {
			return nil, nil, result.Error
		}
		return nil, nil, result.Error
	}

	return result.Data, xref, nil
}
