package defs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

const maxSegmentTypeLen = 100

// EOF 是当没有更多输入可用时 Read 返回的错误。
// 函数应该仅返回 EOF 以表示输入正常结束。
var ErrFoo = fmt.Errorf("EOF")

// XrefEntry 结构体用于保存每个段的偏移量和长度
type XrefEntry struct {
	Offset int64  // 偏移量
	Length uint32 // 长度
}

// FileXref 结构体用于跟踪单个文件的 xref 表和 startxref 的位置
type FileXref struct {
	mu        sync.RWMutex         // 线程安全
	XrefTable map[string]XrefEntry // xref 表
	StartXref int64                // startxref 的位置
}

// NewFileXref 创建一个新的 FileXref 对象，并初始化 xref 表
func NewFileXref() *FileXref {
	return &FileXref{
		XrefTable: make(map[string]XrefEntry),
	}
}

// SaveAndClose 保存 xref 表和关闭文件
func SaveAndClose(file *os.File, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 定位到文件的末尾，以便写入 xref 表
	// Seek 将文件上的下一个读取或写入的偏移量设置为偏移量，根据来源进行解释：0 表示相对于文件的原点，1 表示相对于当前偏移量，2 表示相对于结尾。
	// 它返回新的偏移量和错误（如果有）。
	// 未指定使用 O_APPEND 打开的文件上的 Seek 行为。
	xrefStartPosition, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	// 写入 xref 表
	for segmentType, entry := range xref.XrefTable {
		// 写入段类型的长度
		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
			return err
		}
		// 写入段类型
		if _, err := file.WriteString(segmentType); err != nil {
			return err
		}
		// 写入 xref 入口
		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
			return err
		}
	}

	// 在文件末尾写入 "startxref" 标记
	if _, err := file.WriteString("startxref"); err != nil {
		return err
	}

	// 写入 xref 表的起始位置
	if err := binary.Write(file, binary.BigEndian, xrefStartPosition); err != nil {
		return err
	}

	// 关闭文件
	return file.Close()
}

// LoadXref 从文件加载 xref 表
func LoadXref(file *os.File) (*FileXref, error) {
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, fi.Size())

	// 定位到文件尾部
	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}

	// 定义一个函数来逐步向后搜索 "startxref"
	findStartXref := func(startPos int64, buffer []byte) (int64, error) {
		for {
			// 更新开始位置
			startPos -= int64(len(buffer))
			if startPos < 0 {
				// return 0, fmt.Errorf("startxref not found")
				startPos = 0
			}

			// 读取缓冲区
			// ReadAt 从文件中从字节偏移量 off 处开始读取 len(b) 个字节。
			_, err := file.ReadAt(buffer, startPos)
			if err != nil {
				return 0, err
			}

			// 寻找 "startxref"
			// LastIndex 返回 s 中最后一个 sep 实例的索引，如果 s 中不存在 sep，则返回 -1。
			index := bytes.LastIndex(buffer, []byte("startxref"))
			if index != -1 {
				return startPos + int64(index), nil
			}
		}
	}

	// 从文件尾部开始查找 "startxref"
	startXrefPos, err := findStartXref(pos, buffer)
	if err != nil {
		return nil, err
	}

	// 读取并解析 xref 表的实际偏移量
	_, err = file.Seek(startXrefPos+int64(len("startxref")), io.SeekStart)
	if err != nil {
		return nil, err
	}

	var xrefOffset int64
	if err := binary.Read(file, binary.BigEndian, &xrefOffset); err != nil {
		return nil, err
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

	return xref, nil
}

// 内部函数，用于抽取共同的逻辑
func writeSegmentInternal(writer io.Writer, segmentType string, data []byte, xref *FileXref, offset int64) error {
	if len(data) == 0 {
		return fmt.Errorf("data cannot be empty")
	}

	// 计算CRC32校验和
	checksum := crc32.ChecksumIEEE(data)

	// 写入段类型的长度
	if err := binary.Write(writer, binary.BigEndian, uint32(len(segmentType))); err != nil {
		return err
	}

	// 写入段类型
	if _, err := io.WriteString(writer, segmentType); err != nil {
		return err
	}

	// 写入数据长度和CRC32校验和
	if err := binary.Write(writer, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.BigEndian, checksum); err != nil {
		return err
	}

	// 写入数据
	if _, err := writer.Write(data); err != nil {
		return err
	}

	// 更新 xref 表
	xref.XrefTable[segmentType] = XrefEntry{
		Offset: offset,
		Length: uint32(len(data)),
	}

	return nil
}

// WriteSegment 将段写入文件
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
		return err
	}

	return writeSegmentInternal(file, segmentType, data, xref, offset)
}

// WriteSegmentToBuffer 将段写入缓冲区
func WriteSegmentToBuffer(buffer *bytes.Buffer, segmentType string, data []byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return fmt.Errorf("segmentType cannot be empty")
	}

	// 获取当前缓冲区的长度，这将是此段的起始位置
	offset := int64(buffer.Len())

	return writeSegmentInternal(buffer, segmentType, data, xref, offset)
}

// WriteSegmentsToFile 批量将段写入文件
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
		if err := writeSegmentInternal(file, segmentType, data, xref, offset); err != nil {
			return err
		}

		// 更新偏移量
		offset += int64(4 + len(segmentType) + 4 + 4 + len(data)) // 更新偏移量的计算基于 segmentType 长度、数据长度和 CRC32 校验和的字节数
	}

	return nil
}

// WriteSegmentsToBuffer 批量将段写入缓冲区
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
		if err := writeSegmentInternal(buffer, segmentType, data, xref, offset); err != nil {
			return err
		}

		// 更新偏移量
		offset += int64(4 + len(segmentType) + 4 + 4 + len(data)) // 更新偏移量的计算基于 segmentType 长度、数据长度和 CRC32 校验和的字节数
	}

	return nil
}

// 内部函数，用于抽取共同的读取逻辑
func readSegmentInternal(reader io.Reader, segmentType string, xref *FileXref) ([]byte, error) {
	// 尝试将 reader 转换为 io.Seeker 以启用搜索
	seeker, ok := reader.(io.Seeker)
	if !ok {
		return nil, fmt.Errorf("reader does not support seeking")
	}

	// 检查 segmentType 是否存在于 xref 表中
	xref.mu.RLock()
	entry, ok := xref.XrefTable[segmentType]
	xref.mu.RUnlock()
	if !ok {
		return nil, ErrFoo
	}

	// 跳转到数据段的位置
	if _, err := seeker.Seek(entry.Offset, io.SeekStart); err != nil {
		return nil, err
	}

	// 读取 segmentType 的长度
	var segmentTypeLength uint32
	if err := binary.Read(reader, binary.BigEndian, &segmentTypeLength); err != nil {
		return nil, err
	}

	// 读取 segmentType
	readSegmentType := make([]byte, segmentTypeLength)
	if _, err := io.ReadFull(reader, readSegmentType); err != nil {
		return nil, err
	}

	// 读取数据长度和CRC32校验和
	var length uint32
	var checksum uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if err := binary.Read(reader, binary.BigEndian, &checksum); err != nil {
		return nil, err
	}

	// 读取实际数据
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	// 校验数据
	if checksum != crc32.ChecksumIEEE(data) {
		return nil, fmt.Errorf("data corruption detected")
	}

	return data, nil
}

// ReadSegment 从文件读取段
func ReadSegmentToFile(file *os.File, segmentType string, xref *FileXref) ([]byte, error) {
	xref.mu.RLock()
	defer xref.mu.RUnlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return nil, fmt.Errorf("segmentType cannot be empty")
	}

	return readSegmentInternal(file, segmentType, xref)
}

// ReadSegmentFromBuffer 从缓冲区读取段
func ReadSegmentFromBuffer(buffer *bytes.Buffer, segmentType string, xref *FileXref) ([]byte, error) {
	xref.mu.RLock()
	defer xref.mu.RUnlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return nil, fmt.Errorf("segmentType cannot be empty")
	}

	// NewReader 返回从 b 读取的新 Reader。
	bytesReader := bytes.NewReader(buffer.Bytes())
	return readSegmentInternal(bytesReader, segmentType, xref)
}

// AppendSegmentToFile 打开现有文件并添加一个新的段（segmentType 和 data），同时更新 xref 表
func AppendSegmentToFile(file *os.File, segmentType string, data []byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 检查 segmentType 是否为空
	if len(segmentType) == 0 {
		return fmt.Errorf("segmentType cannot be empty")
	}

	// 定位到文件的末尾
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	// 写入新的 segment
	if err := writeSegmentInternal(file, segmentType, data, xref, offset); err != nil {
		return err
	}

	// 获取新的 xref 表的起始位置
	newXrefStart, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	// 写入新的 xref 表
	for segmentType, entry := range xref.XrefTable {
		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
			return err
		}
		if _, err := file.WriteString(segmentType); err != nil {
			return err
		}
		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
			return err
		}
	}

	// 在文件末尾写入新的 "startxref" 标记
	if _, err := file.WriteString("startxref"); err != nil {
		return err
	}

	// 写入新的 xref 表的起始位置
	if err := binary.Write(file, binary.BigEndian, newXrefStart); err != nil {
		return err
	}

	// 更新内存中的 startxref 位置
	xref.StartXref = newXrefStart

	return nil
}

// AppendSegmentsToFile 打开现有文件并批量添加新的段，同时更新 xref 表
func AppendSegmentsToFile(file *os.File, segments map[string][]byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 定位到文件的末尾
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	for segmentType, data := range segments {
		// 检查 segmentType 是否为空
		if len(segmentType) == 0 {
			return fmt.Errorf("segmentType cannot be empty")
		}

		// 写入新的 segment
		if err := writeSegmentInternal(file, segmentType, data, xref, offset); err != nil {
			return err
		}

		// 获取新的偏移量，作为下一个段的起始位置
		offset, err = file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
	}

	// 获取新的 xref 表的起始位置
	newXrefStart, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	// 写入新的 xref 表
	for segmentType, entry := range xref.XrefTable {
		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
			return err
		}
		if _, err := file.WriteString(segmentType); err != nil {
			return err
		}
		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
			return err
		}
	}

	// 在文件末尾写入新的 "startxref" 标记
	if _, err := file.WriteString("startxref"); err != nil {
		return err
	}

	// 写入新的 xref 表的起始位置
	if err := binary.Write(file, binary.BigEndian, newXrefStart); err != nil {
		return err
	}

	// 更新内存中的 startxref 位置
	xref.StartXref = newXrefStart

	return nil
}

///////////////////////////////

// ReplaceSegmentInFile 方法用于替换文件中的指定段。
// 该方法自动处理不同长度的新旧数据。
// func ReplaceSegmentInFile(file *os.File, segmentType string, newData []byte) error {
// 	// TODO: 根据您的具体需求获取 xref
// 	// xref := NewFileXref() // 这里是一个占位符
// 	// xref.mu.Lock()
// 	// defer xref.mu.Unlock()

// 	xref, err := LoadXref(file)
// 	if err != nil {
// 		return err
// 	}

// 	// 重置文件指针
// 	file.Seek(0, io.SeekStart)

// 	// 从 xref 表中获取该段类型的信息
// 	entry, ok := xref.XrefTable[segmentType]
// 	if !ok {
// 		logrus.Printf("报错：%v", segmentType)
// 		// return fmt.Errorf("segment not found in xref table")
// 		return ErrFoo
// 	}
// 	logrus.Printf("==>\t%v", entry)

// 	// 获取新数据和旧数据的长度
// 	newLength := uint32(len(newData))
// 	oldLength := entry.Length

// 	// 情况 1: 新数据和旧数据的长度相等，直接覆盖旧数据
// 	if newLength == oldLength {
// 		logrus.Printf("newLength:\t%v\toldLength:\t%v\tnewData:\t%v\tOffset:\t%v", newLength, oldLength, len(newData), entry.Offset)
// 		if _, err := file.WriteAt(newData, entry.Offset); err != nil {
// 			logrus.Printf("报错2:\t%v", err)
// 			return err
// 		}
// 		return nil
// 	}

// 	// 情况 2: 新数据长度小于旧数据长度
// 	if newLength < oldLength {
// 		logrus.Printf("newLength:\t%v\toldLength:\t%v\tnewData:\t%v\tOffset:\t%v", newLength, oldLength, len(newData), entry.Offset)
// 		// WriteAt 将 len(b) 个字节写入文件，从字节偏移量 off 开始。
// 		// 它返回写入的字节数和错误（如果有）。
// 		// 当 n != len(b) 时，WriteAt 返回非零错误。
// 		// 如果文件是使用 O_APPEND 标志打开的，则 WriteAt 返回错误。
// 		if _, err := file.WriteAt(newData, entry.Offset); err != nil {
// 			logrus.Printf("报错3:\t%v", err)
// 			return err
// 		}

// 		// 移动剩余的数据以覆盖旧数据的剩余部分
// 		remainingData := make([]byte, oldLength-newLength)
// 		if _, err := file.ReadAt(remainingData, entry.Offset+int64(oldLength)); err != nil {
// 			logrus.Printf("报错4:\t%v", err)
// 			return err
// 		}
// 		if _, err := file.WriteAt(remainingData, entry.Offset+int64(newLength)); err != nil {
// 			logrus.Printf("报错5:\t%v", err)
// 			return err
// 		}
// 		return nil
// 	}

// 	// 情况 3: 新数据长度大于旧数据长度
// 	// 先获取文件的总长度
// 	fileSize, err := file.Seek(0, io.SeekEnd)
// 	if err != nil {
// 		logrus.Printf("报错6:\t%v", err)
// 		return err
// 	}

// 	// 计算剩余数据的长度并读取它
// 	remainingDataSize := fileSize - (entry.Offset + int64(oldLength))
// 	remainingData := make([]byte, remainingDataSize)
// 	if _, err := file.ReadAt(remainingData, entry.Offset+int64(oldLength)); err != nil {
// 		logrus.Printf("报错7:\t%v", err)
// 		return err
// 	}

// 	// 将剩余数据向后移动以适应新数据
// 	if _, err := file.WriteAt(remainingData, entry.Offset+int64(newLength)); err != nil {
// 		logrus.Printf("报错8:\t%v", err)
// 		return err
// 	}

// 	// 最后，写入新数据
// 	if _, err := file.WriteAt(newData, entry.Offset); err != nil {
// 		logrus.Printf("报错9:\t%v", err)
// 		return err
// 	}

// 	// 更新 xref 表中的该段信息
// 	xref.XrefTable[segmentType] = XrefEntry{
// 		Offset: entry.Offset,
// 		Length: newLength,
// 	}

//		return nil
//	}

// func ReplaceSegmentInBuffer(buffer *bytes.Buffer, segmentType string, newData []byte, xref *FileXref) error {
// 	// 获取该段类型的信息
// 	entry, ok := xref.XrefTable[segmentType]
// 	if !ok {
// 		return ErrFoo
// 	}

// 	// 获取新数据和旧数据的长度
// 	newLength := uint32(len(newData))
// 	oldLength := entry.Length

// 	// 计算长度差异
// 	diffLength := int64(newLength) - int64(oldLength)

// 	// 将 buffer 转换为一个 byte 切片
// 	oldData := buffer.Bytes()

// 	// 新的 byte 切片，用于存储更改后的数据
// 	var newDataSlice []byte

// 	// 将旧数据的开始部分（到要替换的段的开始位置）添加到 newDataSlice
// 	newDataSlice = append(newDataSlice, oldData[:entry.Offset]...)

// 	// 添加新数据到 newDataSlice
// 	newDataSlice = append(newDataSlice, newData...)

// 	// 添加旧数据的剩余部分到 newDataSlice
// 	newDataSlice = append(newDataSlice, oldData[entry.Offset+int64(oldLength):]...)

// 	// 用 newDataSlice 更新 buffer
// 	buffer.Reset()
// 	buffer.Write(newDataSlice)

// 	// 更新 xref 表中的该段信息
// 	entry.Length = newLength
// 	xref.XrefTable[segmentType] = entry

// 	// 更新此段后面所有段的xref条目
// 	for key, e := range xref.XrefTable {
// 		if e.Offset > entry.Offset {
// 			e.Offset += diffLength
// 			xref.XrefTable[key] = e
// 		}
// 	}

// 	return nil
// }

// func ReplaceSegmentAndUpdateFile(file *os.File, segmentType string, newData []byte, xref *FileXref) error {
// 	// 从文件中读取全部内容到缓冲区
// 	file.Seek(0, io.SeekStart) // 重置文件指针到开始位置
// 	buffer := new(bytes.Buffer)
// 	if _, err := io.Copy(buffer, file); err != nil {
// 		return err
// 	}

// 	// 使用ReplaceSegmentInBuffer函数替换缓冲区中的数据
// 	if err := ReplaceSegmentInBuffer(buffer, segmentType, newData, xref); err != nil {
// 		return err
// 	}

// 	// 重置文件指针并调整文件大小
// 	if err := file.Truncate(int64(buffer.Len())); err != nil {
// 		return err
// 	}

// 	file.Seek(0, io.SeekStart) // 重置文件指针到开始位置
// 	if _, err := io.Copy(file, buffer); err != nil {
// 		return err
// 	}

// 	return nil
// }
