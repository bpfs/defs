package defs

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// WriteSegment 将段写入文件
func WriteSegment(file *os.File, segmentType string, data []byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return fmt.Errorf("segmentType cannot be empty")
	}

	// 获取当前文件偏移量，这将是此段的起始位置
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// 计算CRC32校验和
	checksum := crc32.ChecksumIEEE(data)

	// 写入段类型的长度

	if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
		return err
	}

	// 写入段类型
	if _, err := file.WriteString(segmentType); err != nil {
		return err
	}

	// 写入数据长度和CRC32校验和
	if err := binary.Write(file, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	if err := binary.Write(file, binary.BigEndian, checksum); err != nil {
		return err
	}

	// 写入数据
	if _, err := file.Write(data); err != nil {
		return err
	}

	// 更新 xref 表
	xref.XrefTable[segmentType] = XrefEntry{
		Offset: offset,
		Length: uint32(len(data)),
	}

	return nil
}

// ReadSegment 从文件读取段
func ReadSegment(file *os.File, segmentType string, xref *FileXref) ([]byte, error) {
	xref.mu.RLock()
	defer xref.mu.RUnlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return nil, fmt.Errorf("segmentType cannot be empty")
	}

	// 查找 xref 表以获取 segment 的信息
	entry, ok := xref.XrefTable[segmentType]
	if !ok {
		return nil, fmt.Errorf("segment not found in xref table")
	}

	// 跳到文件中 segment 的位置
	if _, err := file.Seek(entry.Offset, io.SeekStart); err != nil {
		return nil, err
	}

	// 读取 segmentType 的长度
	var segmentTypeLength uint32
	if err := binary.Read(file, binary.BigEndian, &segmentTypeLength); err != nil {
		return nil, err
	}

	// 读取 segmentType
	readSegmentType := make([]byte, segmentTypeLength)
	if _, err := io.ReadFull(file, readSegmentType); err != nil {
		return nil, err
	}

	// 读取数据长度和CRC32校验和
	var length uint32
	var checksum uint32
	if err := binary.Read(file, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if err := binary.Read(file, binary.BigEndian, &checksum); err != nil {
		return nil, err
	}

	// 读取实际数据
	data := make([]byte, length)
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, err
	}

	// 校验数据
	if checksum != crc32.ChecksumIEEE(data) {
		return nil, fmt.Errorf("data corruption detected")
	}

	return data, nil
}
