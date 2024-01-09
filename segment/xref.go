package segment

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// FileXref 结构体用于跟踪单个文件的 xref 表和 startxref 的位置
type FileXref struct {
	mu        sync.RWMutex         // 线程安全
	XrefTable map[string]XrefEntry // xref 表
	StartXref int64                // startxref 的位置
}

// XrefEntry 结构体用于保存每个段的偏移量和长度
type XrefEntry struct {
	Offset int64  // 偏移量
	Length uint32 // 长度
}

// NewFileXref 创建一个新的 FileXref 对象，并初始化 xref 表
func NewFileXref() *FileXref {
	return &FileXref{
		XrefTable: make(map[string]XrefEntry),
	}
}

// SaveAndClose 保存 xref 表并关闭文件
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

// LoadXrefFromFile 从文件加载 xref 表
func LoadXrefFromFile(file *os.File) (*FileXref, error) {
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
	xref.StartXref = xrefOffset

	return xref, nil
}

// LoadXrefFromBuffer 从缓冲区加载 xref 表
func LoadXrefFromBuffer(reader io.Reader) (*FileXref, error) {
	// 读取整个 reader 的内容到内存，这是必需的，因为我们需要多次扫描数据
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取数据失败: %v", err)
	}

	// 定义一个函数来逐步向后搜索 "startxref"
	findStartXref := func(data []byte) (int64, error) {
		index := bytes.LastIndex(data, []byte("startxref"))
		if index == -1 {
			return 0, fmt.Errorf("startxref not found")
		}
		return int64(index), nil
	}

	// 查找 "startxref"
	startXrefPos, err := findStartXref(data)
	if err != nil {
		return nil, err
	}

	// 创建一个用于读取数据的新 reader
	dataReader := bytes.NewReader(data)

	// 读取并解析 xref 表的实际偏移量
	if _, err := dataReader.Seek(startXrefPos+int64(len("startxref")), io.SeekStart); err != nil {
		return nil, err
	}

	var xrefOffset int64
	if err := binary.Read(dataReader, binary.BigEndian, &xrefOffset); err != nil {
		return nil, err
	}

	// 定位到 xref 表并读取它
	if _, err := dataReader.Seek(xrefOffset, io.SeekStart); err != nil {
		return nil, err
	}

	xref := NewFileXref()
	for {
		var segmentTypeLen uint32
		if err := binary.Read(dataReader, binary.BigEndian, &segmentTypeLen); err == io.EOF {
			break
		} else if segmentTypeLen > maxSegmentTypeLen {
			break
		} else if err != nil {
			return nil, err
		}

		segmentTypeBytes := make([]byte, segmentTypeLen)
		if _, err := io.ReadFull(dataReader, segmentTypeBytes); err != nil {
			return nil, err
		}

		segmentType := string(segmentTypeBytes)

		entry := XrefEntry{}
		if err := binary.Read(dataReader, binary.BigEndian, &entry); err != nil {
			return nil, err
		}

		xref.XrefTable[segmentType] = entry
	}
	xref.StartXref = xrefOffset

	return xref, nil
}

/////////////////

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
	xref.StartXref = xrefOffset

	return xref, nil
}
