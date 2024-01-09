package segment

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// prepareFileForWriting 准备文件以进行写入，返回写入的起始位置
func prepareFileForWriting(file *os.File, xref *FileXref) (int64, error) {
	isEmptyPage := len(xref.XrefTable) == 0
	if isEmptyPage {
		return file.Seek(0, io.SeekEnd)
	}
	return file.Seek(xref.StartXref, io.SeekStart)
}

// writeXrefTableAndStartXref 写入xref表和startxref标记
func writeXrefTableAndStartXref(file *os.File, xref *FileXref, newXrefStart int64) error {
	_, err := file.Seek(newXrefStart, io.SeekStart)
	if err != nil {
		return err
	}

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

	if _, err := file.WriteString("startxref"); err != nil {
		return err
	}

	if err := binary.Write(file, binary.BigEndian, newXrefStart); err != nil {
		return err
	}

	xref.StartXref = newXrefStart
	return nil
}

// AppendSegmentToFile 函数
func AppendSegmentToFile(file *os.File, segmentType string, data []byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	if len(segmentType) == 0 {
		return fmt.Errorf("segmentType cannot be empty")
	}

	offset, err := prepareFileForWriting(file, xref)
	if err != nil {
		return err
	}

	totalWritten, err := writeSegmentInternal(file, segmentType, data, xref, offset)
	if err != nil {
		return err
	}

	newXrefStart := offset + int64(totalWritten)
	return writeXrefTableAndStartXref(file, xref, newXrefStart)
}

// AppendSegmentsToFile 函数
func AppendSegmentsToFile(file *os.File, segments map[string][]byte, xref *FileXref) error {
	xref.mu.Lock()
	defer xref.mu.Unlock()

	offset, err := prepareFileForWriting(file, xref)
	if err != nil {
		return err
	}

	for segmentType, data := range segments {
		if len(segmentType) == 0 {
			return fmt.Errorf("segmentType cannot be empty")
		}

		totalWritten, err := writeSegmentInternal(file, segmentType, data, xref, offset)
		if err != nil {
			return err
		}
		offset += int64(totalWritten)
	}

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
