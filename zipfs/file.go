package zipfs

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// File 代表一个 ZIP 文件或目录
type File struct {
	fs            *Fs           // 文件系统对象
	zipfile       *zip.File     // ZIP 文件对象
	reader        io.ReadCloser // 文件读取器
	offset        int64         // 当前读取偏移量
	isdir, closed bool          // 是否是目录，是否已关闭
	buf           []byte        // 文件读取缓冲区
}

// fillBuffer 填充文件读取缓冲区到指定的偏移量
// 参数：
//   - offset: int64 读取偏移量
//
// 返回值：
//   - error 可能的错误
func (f *File) fillBuffer(offset int64) (err error) {
	if f.reader == nil {
		if f.reader, err = f.zipfile.Open(); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return
		}
	}
	if offset > int64(f.zipfile.UncompressedSize64) {
		offset = int64(f.zipfile.UncompressedSize64)
		err = io.EOF
	}
	if len(f.buf) >= int(offset) {
		return
	}
	buf := make([]byte, int(offset)-len(f.buf))
	if n, readErr := io.ReadFull(f.reader, buf); n > 0 {
		f.buf = append(f.buf, buf[:n]...)
	} else if readErr != nil {
		err = readErr
	}
	return
}

// Close 关闭文件
// 返回值：
//   - error 可能的错误
func (f *File) Close() (err error) {
	f.zipfile = nil
	f.closed = true
	f.buf = nil
	if f.reader != nil {
		err = f.reader.Close()
		f.reader = nil
	}
	return
}

// Read 读取文件内容到指定的字节切片
// 参数：
//   - p: []byte 读取的目标缓冲区
//
// 返回值：
//   - int 读取的字节数
//   - error 可能的错误
func (f *File) Read(p []byte) (n int, err error) {
	if f.isdir {
		return 0, syscall.EISDIR
	}
	if f.closed {
		return 0, afero.ErrFileClosed
	}
	err = f.fillBuffer(f.offset + int64(len(p)))
	n = copy(p, f.buf[f.offset:])
	f.offset += int64(n)
	return
}

// ReadAt 从指定的偏移量读取文件内容到指定的字节切片
// 参数：
//   - p: []byte 读取的目标缓冲区
//   - off: int64 读取的偏移量
//
// 返回值：
//   - int 读取的字节数
//   - error 可能的错误
func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	if f.isdir {
		return 0, syscall.EISDIR
	}
	if f.closed {
		return 0, afero.ErrFileClosed
	}
	err = f.fillBuffer(off + int64(len(p)))
	n = copy(p, f.buf[int(off):])
	return
}

// Seek 移动文件读取指针
// 参数：
//   - offset: int64 读取偏移量
//   - whence: int 偏移基准
//
// 返回值：
//   - int64 新的偏移量
//   - error 可能的错误
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.isdir {
		return 0, syscall.EISDIR
	}
	if f.closed {
		return 0, afero.ErrFileClosed
	}
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.offset
	case io.SeekEnd:
		offset += int64(f.zipfile.UncompressedSize64)
	default:
		return 0, syscall.EINVAL
	}
	if offset < 0 || offset > int64(f.zipfile.UncompressedSize64) {
		return 0, afero.ErrOutOfRange
	}
	f.offset = offset
	return offset, nil
}

// Write 写入数据到文件（只读文件系统）
// 参数：
//   - p: []byte 写入的字节切片
//
// 返回值：
//   - int 写入的字节数
//   - error 可能的错误
func (f *File) Write(p []byte) (n int, err error) { return 0, syscall.EPERM }

// WriteAt 从指定偏移量写入数据到文件（只读文件系统）
// 参数：
//   - p: []byte 写入的字节切片
//   - off: int64 写入的偏移量
//
// 返回值：
//   - int 写入的字节数
//   - error 可能的错误
func (f *File) WriteAt(p []byte, off int64) (n int, err error) { return 0, syscall.EPERM }

// Name 返回文件名
// 返回值：
//   - string 文件名
func (f *File) Name() string {
	if f.zipfile == nil {
		return string(filepath.Separator)
	}
	return filepath.Join(splitpath(f.zipfile.Name))
}

// getDirEntries 获取目录项
// 返回值：
//   - map[string]*zip.File 目录项
//   - error 可能的错误
func (f *File) getDirEntries() (map[string]*zip.File, error) {
	if !f.isdir {
		return nil, syscall.ENOTDIR
	}
	name := f.Name()
	entries, ok := f.fs.files[name]
	if !ok {
		return nil, &os.PathError{Op: "readdir", Path: name, Err: syscall.ENOENT}
	}
	return entries, nil
}

// Readdir 读取目录项
// 参数：
//   - count: int 读取的目录项数量
//
// 返回值：
//   - []os.FileInfo 目录项信息
//   - error 可能的错误
func (f *File) Readdir(count int) (fi []os.FileInfo, err error) {
	zipfiles, err := f.getDirEntries()
	if err != nil {
		return nil, err
	}
	for _, zipfile := range zipfiles {
		fi = append(fi, zipfile.FileInfo())
		if count > 0 && len(fi) >= count {
			break
		}
	}
	return
}

// Readdirnames 读取目录项名称
// 参数：
//   - count: int 读取的目录项数量
//
// 返回值：
//   - []string 目录项名称
//   - error 可能的错误
func (f *File) Readdirnames(count int) (names []string, err error) {
	zipfiles, err := f.getDirEntries()
	if err != nil {
		return nil, err
	}
	for filename := range zipfiles {
		names = append(names, filename)
		if count > 0 && len(names) >= count {
			break
		}
	}
	return
}

// Stat 返回文件信息
// 返回值：
//   - os.FileInfo 文件信息
//   - error 可能的错误
func (f *File) Stat() (os.FileInfo, error) {
	if f.zipfile == nil {
		return &pseudoRoot{}, nil
	}
	return f.zipfile.FileInfo(), nil
}

// Sync 同步文件（无操作）
// 返回值：
//   - error 可能的错误
func (f *File) Sync() error { return nil }

// Truncate 截断文件（只读文件系统）
// 参数：
//   - size: int64 截断后的大小
//
// 返回值：
//   - error 可能的错误
func (f *File) Truncate(size int64) error { return syscall.EPERM }

// WriteString 写入字符串到文件（只读文件系统）
// 参数：
//   - s: string 写入的字符串
//
// 返回值：
//   - int 写入的字节数
//   - error 可能的错误
func (f *File) WriteString(s string) (ret int, err error) { return 0, syscall.EPERM }
