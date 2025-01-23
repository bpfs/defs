package segment

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// prepareFileForWriting 准备文件以进行写入，返回写入的起始位置
// 参数:
//   - file: *os.File 文件指针
//   - xref: *FileXref 文件交叉引用结构
//
// 返回值:
//   - int64 写入的起始位置
//   - error 可能的错误
func prepareFileForWriting(file *os.File, xref *FileXref) (int64, error) {
	// 判断是否是空页
	isEmptyPage := len(xref.XrefTable) == 0
	if isEmptyPage {
		// 如果是空页，则从文件末尾开始写入
		pos, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			logger.Errorf("定位文件末尾失败: %v", err)
		}
		return pos, err
	}
	// 否则从交叉引用表的起始位置开始写入
	pos, err := file.Seek(xref.StartXref, io.SeekStart)
	if err != nil {
		logger.Errorf("定位交叉引用表起始位置失败: %v", err)
	}
	return pos, err
}

// writeXrefTableAndStartXref 写入交叉引用表和startxref标记
// 参数:
//   - file: *os.File 文件指针
//   - xref: *FileXref 文件交叉引用结构
//   - newXrefStart: int64 新的交叉引用表起始位置
//
// 返回值:
//   - error 可能的错误
func writeXrefTableAndStartXref(file *os.File, xref *FileXref, newXrefStart int64) error {
	// 移动文件指针到新的交叉引用表起始位置
	_, err := file.Seek(newXrefStart, io.SeekStart)
	if err != nil {
		logger.Errorf("定位新的交叉引用表位置失败: %v", err)
		return err
	}

	// 遍历交叉引用表，写入每个段类型及其对应的条目
	for segmentType, entry := range xref.XrefTable {
		// 写入段类型的长度（以大端序写入）
		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
			logger.Errorf("写入段类型长度失败: %v", err)
			return err
		}
		// 写入段类型字符串
		if _, err := file.WriteString(segmentType); err != nil {
			logger.Errorf("写入段类型失败: %v", err)
			return err
		}
		// 写入条目
		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
			logger.Errorf("写入交叉引用表条目失败: %v", err)
			return err
		}
	}

	// 写入 "startxref" 标记
	if _, err := file.WriteString("startxref"); err != nil {
		logger.Errorf("写入startxref标记失败: %v", err)
		return err
	}

	// 写入新的交叉引用表起始位置
	if err := binary.Write(file, binary.BigEndian, newXrefStart); err != nil {
		logger.Errorf("写入新的交叉引用表起始位置失败: %v", err)
		return err
	}

	// 更新交叉引用表起始位置
	xref.StartXref = newXrefStart
	return nil
}

// AppendSegmentToFile 将一个段附加到文件
// 参数:
//   - file: *os.File 文件指针
//   - segmentType: string 段类型
//   - data: []byte 段数据
//   - xref: *FileXref 文件交叉引用结构
//
// 返回值:
//   - error 可能的错误
func AppendSegmentToFile(file *os.File, segmentType string, data []byte, xref *FileXref) error {
	// 加锁以确保线程安全
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 检查段类型是否为空
	if len(segmentType) == 0 {
		logger.Errorf("段类型不能为空")
		return fmt.Errorf("段类型不能为空")
	}

	// 准备文件以进行写入，获取写入起始位置
	offset, err := prepareFileForWriting(file, xref)
	if err != nil {
		logger.Errorf("%v", err)
		return err
	}

	// 写入段数据
	totalWritten, err := writeSegmentInternal(file, segmentType, data, xref, offset)
	if err != nil {
		logger.Errorf("%v", err)
		return err
	}

	// 计算新的交叉引用表起始位置
	newXrefStart := offset + int64(totalWritten)
	// 写入交叉引用表和startxref标记
	return writeXrefTableAndStartXref(file, xref, newXrefStart)
}

// AppendSegmentsToFile 将多个段附加到文件
// 参数:
//   - file: *os.File 文件指针
//   - segments: map[string][]byte 段类型和段数据的映射
//   - xref: *FileXref 文件交叉引用结构
//
// 返回值:
//   - error 可能的错误
func AppendSegmentsToFile(file *os.File, segments map[string][]byte, xref *FileXref) error {
	// 加锁以确保线程安全
	xref.mu.Lock()
	defer xref.mu.Unlock()

	// 准备文件以进行写入，获取写入起始位置
	offset, err := prepareFileForWriting(file, xref)
	if err != nil {
		logger.Errorf("%v", err)
		return err
	}

	// 遍历每个段类型和段数据
	for segmentType, data := range segments {
		// 检查段类型是否为空
		if len(segmentType) == 0 {
			logger.Errorf("段类型不能为空")
			return fmt.Errorf("段类型不能为空")
		}

		// 写入段数据
		totalWritten, err := writeSegmentInternal(file, segmentType, data, xref, offset)
		if err != nil {
			logger.Errorf("%v", err)
			return err
		}
		// 更新偏移量
		offset += int64(totalWritten)
	}

	// 写入交叉引用表和startxref标记
	return writeXrefTableAndStartXref(file, xref, offset)
}

// AppendSegmentToFile 打开现有文件并添加一个新的段（segmentType 和 data），同时更新 xref 表
// func AppendSegmentToFile(file *os.File, segmentType string, data []byte, xref *FileXref) error {
// 	xref.mu.Lock()
// 	defer xref.mu.Unlock()

// 	// 检查 segmentType 是否为空
// 	if len(segmentType) == 0 {
// 		return fmt.Errorf("segmentType cannot be empty")
// 	}

// 	var offset int64
// 	var err error

// 	// 根据 xref 表判断文件是否为空白页面
// 	isEmptyPage := len(xref.XrefTable) == 0

// 	if isEmptyPage {
// 		// 空白页面，追加在文件末尾
// 		offset, err = file.Seek(0, io.SeekEnd)
// 	} else {
// 		// 非空白页面，从原 xref 表的起始位置开始写
// 		offset, err = file.Seek(xref.StartXref, io.SeekStart)
// 	}

// 	if err != nil {
// 		return err
// 	}

// 	// 写入新的 segment
// 	totalWritten, err := writeSegmentInternal(file, segmentType, data, xref, offset)
// 	if err != nil {
// 		return err
// 	}

// 	// 计算新的 xref 表的起始位置
// 	newXrefStart := offset + int64(totalWritten)

// 	// 重新定位到新的 xref 表的起始位置
// 	_, err = file.Seek(newXrefStart, io.SeekStart)
// 	if err != nil {
// 		return err
// 	}

// 	// 写入新的 xref 表
// 	for segmentType, entry := range xref.XrefTable {
// 		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
// 			return err
// 		}
// 		if _, err := file.WriteString(segmentType); err != nil {
// 			return err
// 		}
// 		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
// 			return err
// 		}
// 	}

// 	// 写入新的 "startxref" 标记
// 	if _, err := file.WriteString("startxref"); err != nil {
// 		return err
// 	}

// 	// 写入新的 xref 表的起始位置
// 	if err := binary.Write(file, binary.BigEndian, newXrefStart); err != nil {
// 		return err
// 	}

// 	// 更新内存中的 startxref 位置
// 	xref.StartXref = newXrefStart

// 	return nil
// }

// AppendSegmentsToFile 打开现有文件并批量添加新的段，同时更新 xref 表
// func AppendSegmentsToFile(file *os.File, segments map[string][]byte, xref *FileXref) error {
// 	xref.mu.Lock()
// 	defer xref.mu.Unlock()

// 	var offset int64
// 	var err error

// 	// 根据 xref 表判断文件是否为空白页面
// 	isEmptyPage := len(xref.XrefTable) == 0

// 	if isEmptyPage {
// 		// 空白页面，追加在文件末尾
// 		offset, err = file.Seek(0, io.SeekEnd)
// 	} else {
// 		// 非空白页面，从原 xref 表的起始位置开始写
// 		offset, err = file.Seek(xref.StartXref, io.SeekStart)
// 	}

// 	if err != nil {
// 		return err
// 	}

// 	for segmentType, data := range segments {
// 		if len(segmentType) == 0 {
// 			return fmt.Errorf("segmentType cannot be empty")
// 		}

// 		// 写入新的 segment
// 		totalWritten, err := writeSegmentInternal(file, segmentType, data, xref, offset)
// 		if err != nil {
// 			return err
// 		}

// 		// 更新偏移量
// 		offset += int64(totalWritten)
// 	}

// 	// 重新定位到新的 xref 表的起始位置
// 	newXrefStart := offset
// 	_, err = file.Seek(newXrefStart, io.SeekStart)
// 	if err != nil {
// 		return err
// 	}

// 	// 写入新的 xref 表
// 	for segmentType, entry := range xref.XrefTable {
// 		if err := binary.Write(file, binary.BigEndian, uint32(len(segmentType))); err != nil {
// 			return err
// 		}
// 		if _, err := file.WriteString(segmentType); err != nil {
// 			return err
// 		}
// 		if err := binary.Write(file, binary.BigEndian, entry); err != nil {
// 			return err
// 		}
// 	}

// 	// 写入新的 "startxref" 标记
// 	if _, err := file.WriteString("startxref"); err != nil {
// 		return err
// 	}

// 	// 写入新的 xref 表的起始位置
// 	if err := binary.Write(file, binary.BigEndian, newXrefStart); err != nil {
// 		return err
// 	}

// 	// 更新内存中的 startxref 位置
// 	xref.StartXref = newXrefStart

// 	return nil
// }
// }
