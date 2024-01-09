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
	Data  []byte
	Error error
}

// readSegmentInternal 从 reader 中读取多个段。它尝试寻址到每个段的起始位置并读取其内容。
// 如果段不存在或读取过程中遇到错误，则会在结果中记录错误信息。
// 内部函数，用于抽取共同的读取逻辑
func readSegmentInternal(reader io.Reader, segmentTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	seeker, ok := reader.(io.Seeker)
	if !ok {
		return nil, fmt.Errorf("读取器不支持寻址")
	}

	results := make(map[string]*SegmentReadResult)
	for _, segmentType := range segmentTypes {
		xref.mu.RLock()
		entry, ok := xref.XrefTable[segmentType]
		xref.mu.RUnlock()

		result := new(SegmentReadResult)
		if !ok {
			result.Error = fmt.Errorf("ErrNoSuchField") // 段不存在
		} else {
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

		results[segmentType] = result
		//	fmt.Printf("result=>:\t%s\t\t%v\n", segmentType, result)
	}

	return results, nil
}

// ReadSegmentToBuffer 从缓冲区中读取指定的单个段
func ReadSegmentToBuffer(buffer *bytes.Buffer, segmentType string, xref *FileXref) ([]byte, error) {
	bytesReader := bytes.NewReader(buffer.Bytes())
	segmentResults, err := readSegmentInternal(bytesReader, []string{segmentType}, xref)
	if err != nil {
		return nil, err
	}

	result, found := segmentResults[segmentType]
	if !found || result.Error != nil {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, result.Error
	}

	return result.Data, nil
}

// ReadSegmentFromBuffer 从缓冲区中批量读取多个段
func ReadSegmentsFromBuffer(buffer *bytes.Buffer, segmentTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	bytesReader := bytes.NewReader(buffer.Bytes())
	segmentResults, err := readSegmentInternal(bytesReader, segmentTypes, xref)
	if err != nil {
		return nil, err
	}

	results := make(map[string]*SegmentReadResult)
	for segmentType, result := range segmentResults {
		results[segmentType] = result
	}

	return results, nil
}

// ReadFieldFromBytes 从字节切片中读取指定的单个字段。
// data: 字节切片数据。
// fieldType: 要读取的字段类型。
// xref: 文件交叉引用表。
// 返回读取的字段数据和可能出现的错误。
func ReadFieldFromBytes(data []byte, fieldType string, xref *FileXref) ([]byte, error) {
	bytesReader := bytes.NewReader(data)
	fieldResults, err := readSegmentInternal(bytesReader, []string{fieldType}, xref)
	if err != nil {
		return nil, err
	}

	result, found := fieldResults[fieldType]
	if !found || result.Error != nil {
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, errors.New("field not found")
	}

	return result.Data, nil
}

// ReadFieldsFromBytes 从字节切片中批量读取多个字段。
// data: 字节切片数据。
// fieldTypes: 要读取的字段类型列表。
// xref: 文件交叉引用表。
// 返回读取的字段数据集合和可能出现的错误。
func ReadFieldsFromBytes(data []byte, fieldTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	bytesReader := bytes.NewReader(data)
	fieldResults, err := readSegmentInternal(bytesReader, fieldTypes, xref)
	if err != nil {
		return nil, err
	}

	// for k, v := range fieldResults {
	// 	fmt.Printf("%v\n", k)
	// 	fmt.Printf("Data:\t%s\n", v.Data)
	// 	fmt.Printf("Error:\t%v\n", v.Error)
	// 	fmt.Print("\n")
	// }

	results := make(map[string]*SegmentReadResult)
	for fieldType, result := range fieldResults {
		results[fieldType] = result
	}

	return results, nil
}

////////////////////////////////////////////////////////////////////////

const blockSize = 1024 // 搜索块的大小

// findStartXref 查找文件中 "startxref" 关键词的位置。
// 它从文件的末尾开始向前搜索，以找到该关键词。
// file: 待搜索的文件对象。
// 返回 "startxref" 关键词的位置以及可能出现的错误。
func findStartXref(file *os.File) (int64, error) {
	fileSize, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("failed to seek file end: %w", err)
	}

	for startPos := fileSize; startPos > 0; startPos -= blockSize {
		endPos := startPos
		startPos = max(startPos-blockSize, 0)

		buffer := make([]byte, endPos-startPos)
		_, err := file.ReadAt(buffer, startPos)
		if err != nil && err != io.EOF {
			return 0, fmt.Errorf("failed to read file: %w", err)
		}

		index := bytes.LastIndex(buffer, []byte("startxref"))
		if index != -1 {
			return startPos + int64(index), nil
		}
	}

	return 0, errors.New("startxref not found")
}

// max 返回两个 int64 类型值中的最大值。
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// parseXref 解析文件中的 Xref 表。
// 它根据提供的开始位置解析 Xref 表的内容。
// file: 待解析的文件对象。
// startXrefPos: "startxref" 关键词的位置。
// 返回解析得到的 Xref 表以及可能出现的错误。
func parseXref(file *os.File, startXrefPos int64) (*FileXref, error) {
	_, err := file.Seek(startXrefPos+int64(len("startxref")), io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek xref position: %w", err)
	}

	var xrefOffset int64
	if err := binary.Read(file, binary.BigEndian, &xrefOffset); err != nil {
		return nil, fmt.Errorf("failed to read xref offset: %w", err)
	}

	// 定位到 xref 表并读取它
	if _, err := file.Seek(xrefOffset, io.SeekStart); err != nil {
		return nil, err
	}

	xref := NewFileXref()
	for {
		var segmentTypeLen uint32
		// Read 将结构化二进制数据从 r 读取到 data 中。
		if err := binary.Read(file, binary.BigEndian, &segmentTypeLen); err == io.EOF {
			break
		} else if segmentTypeLen > maxSegmentTypeLen {
			break
		} else if err != nil {
			return nil, err
		}

		segmentTypeBytes := make([]byte, segmentTypeLen)
		// ReadFull 将 r 中的 len(buf) 个字节准确读取到 buf 中。
		if _, err := io.ReadFull(file, segmentTypeBytes); err != nil {
			return nil, err
		}

		segmentType := string(segmentTypeBytes)

		entry := XrefEntry{}
		// Read 将结构化二进制数据从 r 读取到 data 中。
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
// file: 待读取的文件对象。
// entry: 段在 Xref 表中的条目。
// 返回读取的段内容以及可能出现的错误。
func readSegment(file *os.File, segmentTypes []string, xref *FileXref) (map[string]*SegmentReadResult, error) {
	results := make(map[string]*SegmentReadResult)
	for _, segmentType := range segmentTypes {
		xref.mu.RLock()
		entry, exists := xref.XrefTable[segmentType]
		xref.mu.RUnlock()

		result := new(SegmentReadResult)
		if !exists {
			result.Error = fmt.Errorf("ErrNoSuchField") // 段不存在
		} else {
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
