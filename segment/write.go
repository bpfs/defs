package segment

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// 内部函数，用于抽取共同的逻辑
// func writeSegmentInternal(writer io.Writer, segmentType string, data []byte, xref *FileXref, offset int64) error {
// 	if len(data) == 0 {
// 		return fmt.Errorf("data cannot be empty")
// 	}

// 	// 计算CRC32校验和
// 	checksum := crc32.ChecksumIEEE(data)

// 	// 写入段类型的长度
// 	if err := binary.Write(writer, binary.BigEndian, uint32(len(segmentType))); err != nil {
// 		return err
// 	}

// 	// 写入段类型
// 	if _, err := io.WriteString(writer, segmentType); err != nil {
// 		return err
// 	}

// 	// 写入数据长度和CRC32校验和
// 	if err := binary.Write(writer, binary.BigEndian, uint32(len(data))); err != nil {
// 		return err
// 	}
// 	if err := binary.Write(writer, binary.BigEndian, checksum); err != nil {
// 		return err
// 	}

// 	// 写入数据
// 	if _, err := writer.Write(data); err != nil {
// 		return err
// 	}

// 	// 更新 xref 表
// 	xref.XrefTable[segmentType] = XrefEntry{
// 		Offset: offset,
// 		Length: uint32(len(data)),
// 	}

//		return nil
//	}
//

// writeSegmentInternal 是一个内部函数，用于抽取共同的逻辑
// 它现在返回写入的总字节数
// 参数:
//   - writer: io.Writer 数据写入的目标
//   - segmentType: string 段类型
//   - data: []byte 段的数据
//   - xref: *FileXref 文件的交叉引用表
//   - offset: int64 段在文件中的偏移量
//
// 返回值:
//   - int 写入的总字节数
//   - error 可能出现的错误
func writeSegmentInternal(writer io.Writer, segmentType string, data []byte, xref *FileXref, offset int64) (int, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("data cannot be empty")
	}

	totalWritten := 0

	// 计算CRC32校验和
	checksum := crc32.ChecksumIEEE(data)

	// 写入段类型的长度
	segmentTypeLength := uint32(len(segmentType))
	if err := binary.Write(writer, binary.BigEndian, segmentTypeLength); err != nil {
		logger.Errorf("写入段类型长度失败: %v", err)
		return totalWritten, err
	}
	totalWritten += 4 // uint32 的长度

	// 写入段类型
	n, err := io.WriteString(writer, segmentType)
	if err != nil {
		logger.Errorf("写入段类型失败: %v", err)
		return totalWritten, err
	}
	totalWritten += n

	// 写入数据长度
	dataLength := uint32(len(data))
	if err := binary.Write(writer, binary.BigEndian, dataLength); err != nil {
		logger.Errorf("写入数据长度失败: %v", err)
		return totalWritten, err
	}
	totalWritten += 4 // uint32 的长度

	// 写入CRC32校验和
	if err := binary.Write(writer, binary.BigEndian, checksum); err != nil {
		logger.Errorf("写入CRC32校验和失败: %v", err)
		return totalWritten, err
	}
	totalWritten += 4 // uint32 的长度

	// 写入数据
	n, err = writer.Write(data)
	if err != nil {
		logger.Errorf("写入数据失败: %v", err)
		return totalWritten, err
	}
	totalWritten += n

	// 更新 xref 表
	xref.XrefTable[segmentType] = XrefEntry{
		Offset: offset,
		Length: dataLength,
	}

	return totalWritten, nil
}

// WriteSegmentToFile 将段写入文件
// 参数:
//   - file: *os.File 要写入的文件对象
//   - segmentType: string 段类型
//   - data: []byte 段的数据
//   - xref: *FileXref 文件的交叉引用表
//
// 返回值:
//   - error 可能出现的错误
func WriteSegmentToFile(file *os.File, segmentType string, data []byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return fmt.Errorf("segmentType cannot be empty")
	}

	// 获取当前文件偏移量，这将是此段的起始位置
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		logger.Errorf("获取文件偏移量失败: %v", err)
		return err
	}

	// 将当前段写入文件
	_, err = writeSegmentInternal(file, segmentType, data, xref, offset)
	if err != nil {
		logger.Errorf("写入段到文件失败: %v", err)
		return err
	}

	return err
}

// WriteSegmentToBuffer 将段写入缓冲区
// 参数:
//   - buffer: *bytes.Buffer 要写入的缓冲区
//   - segmentType: string 段类型
//   - data: []byte 段的数据
//   - xref: *FileXref 文件的交叉引用表
//
// 返回值:
//   - error 可能出现的错误
func WriteSegmentToBuffer(buffer *bytes.Buffer, segmentType string, data []byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return fmt.Errorf("segmentType cannot be empty")
	}

	// 获取当前缓冲区的长度，这将是此段的起始位置
	offset := int64(buffer.Len())

	_, err := writeSegmentInternal(buffer, segmentType, data, xref, offset)
	if err != nil {
		logger.Errorf("写入段到缓冲区失败: %v", err)
		return err
	}

	return err
}

// WriteSegmentsToFile 批量将段写入文件
// 参数:
//   - file: *os.File 要写入的文件对象
//   - segments: map[string][]byte 要写入的段数据，键为段类型，值为段内容
//   - xref: *FileXref 文件的交叉引用表
//
// 返回值:
//   - error 可能出现的错误
func WriteSegmentsToFile(file *os.File, segments map[string][]byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 获取当前文件偏移量，这将是此段的起始位置
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// 遍历所有段并将它们写入文件
	for segmentType, data := range segments {
		// 检查 segmentType 是否为空
		if len(segmentType) == 0 {
			return fmt.Errorf("segmentType cannot be empty")
		}

		// 将当前段写入文件
		_, err := writeSegmentInternal(file, segmentType, data, xref, offset)
		if err != nil {
			logger.Errorf("批量写入段到文件失败: %v", err)
			return err
		}

		// 更新偏移量
		offset += int64(4 + len(segmentType) + 4 + 4 + len(data)) // 更新偏移量的计算基于 segmentType 长度、数据长度和 CRC32 校验和的字节数
	}

	return nil
}

// WriteSegmentsToBuffer 批量将段写入缓冲区
// 参数:
//   - buffer: *bytes.Buffer 要写入的缓冲区
//   - segments: map[string][]byte 要写入的段数据，键为段类型，值为段内容
//   - xref: *FileXref 文件的交叉引用表
//
// 返回值:
//   - error 可能出现的错误
func WriteSegmentsToBuffer(buffer *bytes.Buffer, segments map[string][]byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 获取当前缓冲区的长度，这将是此段的起始位置
	offset := int64(buffer.Len())

	// 遍历所有段并将它们写入缓冲区
	for segmentType, data := range segments {
		// 检查 segmentType 是否为空
		if len(segmentType) == 0 {
			return fmt.Errorf("segmentType cannot be empty")
		}

		// 将当前段写入缓冲区
		_, err := writeSegmentInternal(buffer, segmentType, data, xref, offset)
		if err != nil {
			logger.Errorf("批量写入段到缓冲区失败: %v", err)
			return err
		}

		// 更新偏移量
		offset += int64(4 + len(segmentType) + 4 + 4 + len(data)) // 更新偏移量的计算基于 segmentType 长度、数据长度和 CRC32 校验和的字节数
	}

	return nil
}
