package segment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// SegmentReadResult 用于表示单个段的读取结果
type SegmentReadResult struct {
	Data  []byte // 读取的数据
	Error error  // 读取过程中发生的错误
}

// readSegmentInternal 从 reader 中读取多个段。它尝试寻址到每个段的起始位置并读取其内容。
// 如果段不存在或读取过程中遇到错误，则会在结果中记录错误信息。
// 参数:
//   - reader: io.Reader 输入流
//   - segmentTypes: []string 要读取的段类型列表
//   - xref: *FileXref 文件交叉引用表
//
// 返回值:
//   - map[string]*SegmentReadResult 读取结果映射
//   - error 可能的错误
func readSegmentInternal(reader io.Reader, segmentTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	// 检查 reader 是否支持寻址
	seeker, ok := reader.(io.Seeker)
	if !ok {
		return nil, fmt.Errorf("读取器不支持寻址")
	}

	// 初始化结果映射
	results := make(map[string]*SegmentReadResult)
	for _, segmentType := range segmentTypes {
		// 读取交叉引用表中的段信息
		xref.mu.RLock()
		entry, ok := xref.XrefTable[segmentType]
		xref.mu.RUnlock()

		// 初始化段读取结果
		result := new(SegmentReadResult)
		if !ok {
			result.Error = fmt.Errorf("ErrNoSuchField") // 段不存在
		} else {
			// 尝试寻址到段的起始位置
			if _, err := seeker.Seek(entry.Offset, io.SeekStart); err != nil {
				result.Error = fmt.Errorf("寻址失败: %v", err)
				continue
			}

			// 读取 segmentType 的长度
			var segmentTypeLength uint32
			if err := binary.Read(reader, binary.BigEndian, &segmentTypeLength); err != nil {
				result.Error = fmt.Errorf("读取段类型长度失败: %v", err)
				continue
			}

			// 读取 segmentType
			readSegmentType := make([]byte, segmentTypeLength)
			if _, err := io.ReadFull(reader, readSegmentType); err != nil {
				result.Error = fmt.Errorf("读取段类型失败: %v", err)
				continue
			}

			// 读取数据长度和 CRC32 校验和
			var length uint32
			var checksum uint32
			if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
				result.Error = fmt.Errorf("读取数据长度失败: %v", err)
				continue
			}
			if err := binary.Read(reader, binary.BigEndian, &checksum); err != nil {
				result.Error = fmt.Errorf("读取校验和失败: %v", err)
				continue
			}

			// 判断数据长度是否为零
			if length == 0 {
				// 段内容为空
				result.Data = []byte{}
			} else {
				// 读取实际数据
				data := make([]byte, length)
				if _, err := io.ReadFull(reader, data); err != nil {
					result.Error = fmt.Errorf("读取数据失败: %v", err)
					continue
				}

				// 校验数据
				if checksum != crc32.ChecksumIEEE(data) {
					result.Error = fmt.Errorf("段 '%s' 数据损坏", segmentType)
				} else {
					result.Data = data
				}
			}
		}

		// 将读取结果加入结果映射
		results[segmentType] = result
	}

	return results, nil
}

// ReadSegmentToBuffer 从缓冲区中读取指定的单个段
// 参数:
//   - buffer: *bytes.Buffer 输入缓冲区
//   - segmentType: string 要读取的段类型
//   - xref: *FileXref 文件交叉引用表
//
// 返回值:
//   - []byte 读取的段数据
//   - error 可能的错误
func ReadSegmentToBuffer(buffer *bytes.Buffer, segmentType string, xref *FileXref) ([]byte, error) {
	// 创建字节读取器
	bytesReader := bytes.NewReader(buffer.Bytes())
	// 调用内部读取函数
	segmentResults, err := readSegmentInternal(bytesReader, []string{segmentType}, xref)
	if err != nil {
		return nil, err
	}

	// 获取读取结果
	result, found := segmentResults[segmentType]
	if !found || result.Error != nil {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, result.Error
	}

	return result.Data, nil
}

// ReadSegmentsFromBuffer 从缓冲区中批量读取多个段
// 参数:
//   - buffer: *bytes.Buffer 输入缓冲区
//   - segmentTypes: []string 要读取的段类型列表
//   - xref: *FileXref 文件交叉引用表
//
// 返回值:
//   - map[string]*SegmentReadResult 读取结果映射
//   - error 可能的错误
func ReadSegmentsFromBuffer(buffer *bytes.Buffer, segmentTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	// 创建字节读取器
	bytesReader := bytes.NewReader(buffer.Bytes())
	// 调用内部读取函数
	segmentResults, err := readSegmentInternal(bytesReader, segmentTypes, xref)
	if err != nil {
		return nil, err
	}

	// 初始化结果映射
	results := make(map[string]*SegmentReadResult)
	for segmentType, result := range segmentResults {
		results[segmentType] = result
	}

	return results, nil
}

// ReadFieldFromBytes 从字节切片中读取指定的单个字段
// 参数:
//   - data: []byte 输入字节切片
//   - fieldType: string 要读取的字段类型
//   - xref: *FileXref 文件交叉引用表
//
// 返回值:
//   - []byte 读取的字段数据
//   - error 可能的错误
func ReadFieldFromBytes(data []byte, fieldType string, xref *FileXref) ([]byte, error) {
	// 创建字节读取器
	bytesReader := bytes.NewReader(data)
	// 调用内部读取函数
	fieldResults, err := readSegmentInternal(bytesReader, []string{fieldType}, xref)
	if err != nil {
		return nil, err
	}

	// 获取读取结果
	result, found := fieldResults[fieldType]
	if !found || result.Error != nil {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, errors.New("field not found")
	}

	return result.Data, nil
}

// ReadFieldsFromBytes 从字节切片中批量读取多个字段
// 参数:
//   - data: []byte 输入字节切片
//   - fieldTypes: []string 要读取的字段类型列表
//   - xref: *FileXref 文件交叉引用表
//
// 返回值:
//   - map[string]*SegmentReadResult 读取结果映射
//   - error 可能的错误
func ReadFieldsFromBytes(data []byte, fieldTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	// 创建字节读取器
	bytesReader := bytes.NewReader(data)
	// 调用内部读取函数
	fieldResults, err := readSegmentInternal(bytesReader, fieldTypes, xref)
	if err != nil {
		return nil, err
	}

	// 初始化结果映射
	results := make(map[string]*SegmentReadResult)
	for fieldType, result := range fieldResults {
		results[fieldType] = result
	}

	return results, nil
}

// //////////////////////////////////////////////////////////////////////
const blockSize = 1024 // 搜索块的大小

// findStartXref 查找文件中 "startxref" 关键词的位置。
// 它从文件的末尾开始向前搜索，以找到该关键词。
// 参数:
//   - file: *os.File 待搜索的文件对象。
//
// 返回值:
//   - int64 "startxref" 关键词的位置
//   - error 可能出现的错误
func findStartXref(file *os.File) (int64, error) {
	// 获取文件大小
	fileSize, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("failed to seek file end: %v", err)
	}

	// 从文件末尾向前搜索 "startxref"
	for startPos := fileSize; startPos > 0; startPos -= blockSize {
		endPos := startPos
		startPos = max(startPos-blockSize, 0)

		buffer := make([]byte, endPos-startPos)
		_, err := file.ReadAt(buffer, startPos)
		if err != nil && err != io.EOF {
			return 0, fmt.Errorf("failed to read file: %v", err)
		}

		index := bytes.LastIndex(buffer, []byte("startxref"))
		if index != -1 {
			return startPos + int64(index), nil
		}
	}

	return 0, errors.New("startxref not found")
}

// max 返回两个 int64 类型值中的最大值。
// 参数:
//   - a: int64 第一个值
//   - b: int64 第二个值
//
// 返回值:
//   - int64 两个值中的最大值
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// parseXref 解析文件中的 Xref 表。
// 它根据提供的开始位置解析 Xref 表的内容。
// 参数:
//   - file: *os.File 待解析的文件对象。
//   - startXrefPos: int64 "startxref" 关键词的位置。
//
// 返回值:
//   - *FileXref 解析得到的 Xref 表
//   - error 可能出现的错误
func parseXref(file *os.File, startXrefPos int64) (*FileXref, error) {
	// 定位到 "startxref" 后面的位置
	_, err := file.Seek(startXrefPos+int64(len("startxref")), io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek xref position: %v", err)
	}

	var xrefOffset int64
	// 读取 xref 表的偏移位置
	if err := binary.Read(file, binary.BigEndian, &xrefOffset); err != nil {
		return nil, fmt.Errorf("failed to read xref offset: %v", err)
	}

	// 定位到 xref 表并读取它
	if _, err := file.Seek(xrefOffset, io.SeekStart); err != nil {
		return nil, err
	}

	xref := NewFileXref()
	// 逐个读取 xref 表中的条目
	for {
		var segmentTypeLen uint32
		// 读取段类型的长度
		if err := binary.Read(file, binary.BigEndian, &segmentTypeLen); err == io.EOF {
			break
		} else if segmentTypeLen > maxSegmentTypeLen {
			break
		} else if err != nil {
			return nil, err
		}

		segmentTypeBytes := make([]byte, segmentTypeLen)
		// 读取段类型
		if _, err := io.ReadFull(file, segmentTypeBytes); err != nil {
			return nil, err
		}

		segmentType := string(segmentTypeBytes)

		entry := XrefEntry{}
		// 读取 xref 表中的条目
		if err := binary.Read(file, binary.BigEndian, &entry); err != nil {
			return nil, err
		}

		xref.XrefTable[segmentType] = entry
	}
	xref.StartXref = xrefOffset

	return xref, nil
}

// readSegment 读取并返回文件中指定的段。
// 它根据 Xref 表中的条目读取特定类型的段。
// 参数:
//   - file: *os.File 待读取的文件对象。
//   - segmentTypes: []string 要读取的段类型列表
//   - xref: *FileXref 文件交叉引用表
//
// 返回值:
//   - map[string]*SegmentReadResult 读取结果映射
//   - error 可能出现的错误
func readSegment(file *os.File, segmentTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	results := make(map[string]*SegmentReadResult)
	// 遍历要读取的段类型列表
	for _, segmentType := range segmentTypes {
		xref.mu.RLock()
		entry, exists := xref.XrefTable[segmentType]
		xref.mu.RUnlock()

		result := new(SegmentReadResult)
		if !exists {
			result.Error = fmt.Errorf("ErrNoSuchField") // 段不存在
		} else {
			// 定位到段的起始位置
			if _, err := file.Seek(entry.Offset, io.SeekStart); err != nil {
				result.Error = fmt.Errorf("寻址失败: %v", err)
				continue
			}

			// 读取 segmentType 的长度
			var segmentTypeLength uint32
			if err := binary.Read(file, binary.BigEndian, &segmentTypeLength); err != nil {
				result.Error = fmt.Errorf("读取段类型长度失败: %v", err)
				continue
			}

			// 读取 segmentType
			readSegmentType := make([]byte, segmentTypeLength)
			if _, err := io.ReadFull(file, readSegmentType); err != nil {
				result.Error = fmt.Errorf("读取段类型失败: %v", err)
				continue
			}

			// 读取数据长度和 CRC32 校验和
			var length uint32
			var checksum uint32
			if err := binary.Read(file, binary.BigEndian, &length); err != nil {
				result.Error = fmt.Errorf("读取数据长度失败: %v", err)
				continue
			}
			if err := binary.Read(file, binary.BigEndian, &checksum); err != nil {
				result.Error = fmt.Errorf("读取校验和失败: %v", err)
				continue
			}

			// 判断数据长度是否为零
			if length == 0 {
				result.Data = []byte{} // 段内容为空
			} else {
				// 读取实际数据
				data := make([]byte, length)
				if _, err := io.ReadFull(file, data); err != nil {
					result.Error = fmt.Errorf("读取数据失败: %v", err)
					continue
				}

				// 校验数据
				if checksum != crc32.ChecksumIEEE(data) {
					result.Error = fmt.Errorf("段 '%s' 数据损坏", segmentType)
				} else {
					result.Data = data
				}
			}
		}
		results[segmentType] = result
	}

	return results, nil
}
