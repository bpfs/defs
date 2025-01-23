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
// 返回值：*FileXref 一个新的 FileXref 对象
func NewFileXref() *FileXref {
	return &FileXref{
		XrefTable: make(map[string]XrefEntry),
	}
}

// SaveAndClose 保存 xref 表并关闭文件
// 参数：
//   - file: *os.File 要操作的文件对象
//   - xref: *FileXref 文件的交叉引用表
//
// 返回值：error 可能出现的错误
func SaveAndClose(file *os.File, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 定位到文件的末尾，以便写入 xref 表
	xrefStartPosition, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		logger.Errorf("定位文件末尾失败: %v", err)
		return err
	}

	// 写入 xref 表
	for segmentType, entry := range xref.XrefTable {
		// 写入段类型的长度
		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
			logger.Errorf("写入段类型长度失败: %v", err)
			return err
		}
		// 写入段类型
		if _, err := file.WriteString(segmentType); err != nil {
			logger.Errorf("写入段类型失败: %v", err)
			return err
		}
		// 写入 xref 入口
		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
			logger.Errorf("写入xref入口失败: %v", err)
			return err
		}
	}

	// 在文件末尾写入 "startxref" 标记
	if _, err := file.WriteString("startxref"); err != nil {
		logger.Errorf("写入startxref标记失败: %v", err)
		return err
	}

	// 写入 xref 表的起始位置
	if err := binary.Write(file, binary.BigEndian, xrefStartPosition); err != nil {
		logger.Errorf("写入xref表起始位置失败: %v", err)
		return err
	}

	// 关闭文件
	return file.Close()
}

// LoadXrefFromFile 从文件加载 xref 表
// 参数：
//   - file: *os.File 要读取的文件对象
//
// 返回值：
//   - *FileXref 解析后的 xref 表
//   - error 可能出现的错误
func LoadXrefFromFile(file *os.File) (*FileXref, error) {
	fi, err := file.Stat()
	if err != nil {
		logger.Errorf("获取文件状态失败: %v", err)
		return nil, err
	}

	buffer := make([]byte, fi.Size())

	// 定位到文件尾部
	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		logger.Errorf("定位文件尾部失败: %v", err)
		return nil, err
	}

	// 定义一个函数来逐步向后搜索 "startxref"
	findStartXref := func(startPos int64, buffer []byte) (int64, error) {
		for {
			// 更新开始位置
			startPos -= int64(len(buffer))
			if startPos < 0 {
				startPos = 0
			}

			// 读取缓冲区
			_, err := file.ReadAt(buffer, startPos)
			if err != nil {
				logger.Errorf("读取缓冲区失败: %v", err)
				return 0, err
			}

			// 寻找 "startxref"
			index := bytes.LastIndex(buffer, []byte("startxref"))
			if index != -1 {
				return startPos + int64(index), nil
			}
		}
	}

	// 从文件尾部开始查找 "startxref"
	startXrefPos, err := findStartXref(pos, buffer)
	if err != nil {
		logger.Errorf("查找startxref位置失败: %v", err)
		return nil, err
	}

	// 读取并解析 xref 表的实际偏移量
	_, err = file.Seek(startXrefPos+int64(len("startxref")), io.SeekStart)
	if err != nil {
		logger.Errorf("定位xref表偏移量失败: %v", err)
		return nil, err
	}

	var xrefOffset int64
	if err := binary.Read(file, binary.BigEndian, &xrefOffset); err != nil {
		logger.Errorf("读取xref表偏移量失败: %v", err)
		return nil, err
	}

	// 定位到 xref 表并读取它
	if _, err := file.Seek(xrefOffset, io.SeekStart); err != nil {
		logger.Errorf("定位xref表失败: %v", err)
		return nil, err
	}

	xref := NewFileXref()
	for {
		var segmentTypeLen uint32
		if err := binary.Read(file, binary.BigEndian, &segmentTypeLen); err == io.EOF {
			break
		} else if segmentTypeLen > maxSegmentTypeLen {
			break
		} else if err != nil {
			logger.Errorf("读取段类型长度失败: %v", err)
			return nil, err
		}

		segmentTypeBytes := make([]byte, segmentTypeLen)
		if _, err := io.ReadFull(file, segmentTypeBytes); err != nil {
			logger.Errorf("读取段类型失败: %v", err)
			return nil, err
		}

		segmentType := string(segmentTypeBytes)

		entry := XrefEntry{}
		if err := binary.Read(file, binary.BigEndian, &entry); err != nil {
			logger.Errorf("读取xref入口失败: %v", err)
			return nil, err
		}

		xref.XrefTable[segmentType] = entry
	}
	xref.StartXref = xrefOffset

	return xref, nil
}

// LoadXrefFromBuffer 从缓冲区加载 xref 表
// 参数：
//   - reader: io.Reader 要读取的缓冲区
//
// 返回值：
//   - *FileXref 解析后的 xref 表
//   - error 可能出现的错误
func LoadXrefFromBuffer(reader io.Reader) (*FileXref, error) {
	// 读取整个 reader 的内容到内存，这是必需的，因为我们需要多次扫描数据
	data, err := io.ReadAll(reader)
	if err != nil {
		logger.Errorf("读取缓冲区内容失败: %v", err)
		return nil, err
	}

	// 定义一个函数来逐步向后搜索 "startxref"
	findStartXref := func(data []byte) (int64, error) {
		index := bytes.LastIndex(data, []byte("startxref"))
		if index == -1 {
			return 0, fmt.Errorf("未找到startxref标记")
		}
		return int64(index), nil
	}

	// 查找 "startxref"
	startXrefPos, err := findStartXref(data)
	if err != nil {
		logger.Errorf("查找startxref位置失败: %v", err)
		return nil, err
	}

	// 创建一个用于读取数据的新 reader
	dataReader := bytes.NewReader(data)

	// 读取并解析 xref 表的实际偏移量
	if _, err := dataReader.Seek(startXrefPos+int64(len("startxref")), io.SeekStart); err != nil {
		logger.Errorf("定位xref表偏移量失败: %v", err)
		return nil, err
	}

	var xrefOffset int64
	if err := binary.Read(dataReader, binary.BigEndian, &xrefOffset); err != nil {
		logger.Errorf("读取xref表偏移量失败: %v", err)
		return nil, err
	}

	// 定位到 xref 表并读取它
	if _, err := dataReader.Seek(xrefOffset, io.SeekStart); err != nil {
		logger.Errorf("定位xref表失败: %v", err)
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
			logger.Errorf("读取段类型长度失败: %v", err)
			return nil, err
		}

		segmentTypeBytes := make([]byte, segmentTypeLen)
		if _, err := io.ReadFull(dataReader, segmentTypeBytes); err != nil {
			logger.Errorf("读取段类型失败: %v", err)
			return nil, err
		}

		segmentType := string(segmentTypeBytes)

		entry := XrefEntry{}
		if err := binary.Read(dataReader, binary.BigEndian, &entry); err != nil {
			logger.Errorf("读取xref入口失败: %v", err)
			return nil, err
		}

		xref.XrefTable[segmentType] = entry
	}
	xref.StartXref = xrefOffset

	return xref, nil
}

// LoadXref 从文件加载 xref 表
// 参数：
//   - file: *os.File 要读取的文件对象
//
// 返回值：
//   - *FileXref 解析后的 xref 表
//   - error 可能出现的错误
func LoadXref(file *os.File) (*FileXref, error) {
	fi, err := file.Stat()
	if err != nil {
		logger.Errorf("获取文件状态失败: %v", err)
		return nil, err
	}

	buffer := make([]byte, fi.Size())

	// 定位到文件尾部
	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		logger.Errorf("定位文件尾部失败: %v", err)
		return nil, err
	}

	// 定义一个函数来逐步向后搜索 "startxref"
	findStartXref := func(startPos int64, buffer []byte) (int64, error) {
		for {
			// 更新开始位置
			startPos -= int64(len(buffer))
			if startPos < 0 {
				startPos = 0
			}

			// 读取缓冲区
			_, err := file.ReadAt(buffer, startPos)
			if err != nil {
				logger.Errorf("读取缓冲区失败: %v", err)
				return 0, err
			}

			// 寻找 "startxref"
			index := bytes.LastIndex(buffer, []byte("startxref"))
			if index != -1 {
				return startPos + int64(index), nil
			}
		}
	}

	// 从文件尾部开始查找 "startxref"
	startXrefPos, err := findStartXref(pos, buffer)
	if err != nil {
		logger.Errorf("查找startxref位置失败: %v", err)
		return nil, err
	}

	// 读取并解析 xref 表的实际偏移量
	_, err = file.Seek(startXrefPos+int64(len("startxref")), io.SeekStart)
	if err != nil {
		logger.Errorf("定位xref表偏移量失败: %v", err)
		return nil, err
	}

	var xrefOffset int64
	if err := binary.Read(file, binary.BigEndian, &xrefOffset); err != nil {
		logger.Errorf("读取xref表偏移量失败: %v", err)
		return nil, err
	}

	// 定位到 xref 表并读取它
	if _, err := file.Seek(xrefOffset, io.SeekStart); err != nil {
		logger.Errorf("定位xref表失败: %v", err)
		return nil, err
	}

	xref := NewFileXref()
	for {
		var segmentTypeLen uint32
		if err := binary.Read(file, binary.BigEndian, &segmentTypeLen); err == io.EOF {
			break
		} else if segmentTypeLen > maxSegmentTypeLen {
			break
		} else if err != nil {
			logger.Errorf("读取段类型长度失败: %v", err)
			return nil, err
		}

		segmentTypeBytes := make([]byte, segmentTypeLen)
		if _, err := io.ReadFull(file, segmentTypeBytes); err != nil {
			logger.Errorf("读取段类型失败: %v", err)
			return nil, err
		}

		segmentType := string(segmentTypeBytes)

		entry := XrefEntry{}
		if err := binary.Read(file, binary.BigEndian, &entry); err != nil {
			logger.Errorf("读取xref入口失败: %v", err)
			return nil, err
		}

		xref.XrefTable[segmentType] = entry
	}
	xref.StartXref = xrefOffset

	return xref, nil
}
