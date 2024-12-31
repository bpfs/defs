package tarfs

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/bpfs/defs/afero"
)

// File 代表 tar 文件系统中的文件结构
type File struct {
	h      *tar.Header   // tar 头部信息
	data   *bytes.Reader // 文件数据
	closed bool          // 文件是否已关闭
	fs     *Fs           // 关联的文件系统
}

// Close 关闭文件
// 返回值：
//   - error: 错误信息
func (f *File) Close() error {
	if f.closed {
		return afero.ErrFileClosed // 如果文件已关闭，返回错误
	}

	f.closed = true // 设置文件为关闭状态
	f.h = nil       // 清空头部信息
	f.data = nil    // 清空数据
	f.fs = nil      // 清空文件系统

	return nil // 成功关闭文件，返回 nil
}

// Read 从文件中读取数据到 p 中
// 参数：
//   - p: []byte 读取的数据存储到 p 中
//
// 返回值：
//   - n: int 读取的字节数
//   - err: 错误信息
func (f *File) Read(p []byte) (n int, err error) {
	if f.closed {
		return 0, afero.ErrFileClosed // 如果文件已关闭，返回错误
	}

	if f.h.Typeflag == tar.TypeDir {
		return 0, syscall.EISDIR // 如果是目录，返回错误
	}

	return f.data.Read(p) // 从文件数据中读取
}

// ReadAt 从文件的特定偏移量处读取数据到 p 中
// 参数：
//   - p: []byte 读取的数据存储到 p 中
//   - off: int64 偏移量
//
// 返回值：
//   - n: int 读取的字节数
//   - err: 错误信息
func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	if f.closed {
		return 0, afero.ErrFileClosed // 如果文件已关闭，返回错误
	}

	if f.h.Typeflag == tar.TypeDir {
		return 0, syscall.EISDIR // 如果是目录，返回错误
	}

	return f.data.ReadAt(p, off) // 从特定偏移量处读取数据
}

// Seek 移动文件的读取指针到指定位置
// 参数：
//   - offset: int64 偏移量
//   - whence: int 偏移的起始位置
//
// 返回值：
//   - int64: 新的偏移位置
//   - error: 错误信息
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, afero.ErrFileClosed // 如果文件已关闭，返回错误
	}

	if f.h.Typeflag == tar.TypeDir {
		return 0, syscall.EISDIR // 如果是目录，返回错误
	}

	return f.data.Seek(offset, whence) // 移动读取指针
}

// Write 向文件中写入数据
// 参数：
//   - p: []byte 写入的数据
//
// 返回值：
//   - n: int 写入的字节数
//   - err: 错误信息
func (f *File) Write(p []byte) (n int, err error) {
	return 0, syscall.EROFS // 文件系统为只读，返回错误
}

// WriteAt 从文件的特定偏移量处写入数据
// 参数：
//   - p: []byte 写入的数据
//   - off: int64 偏移量
//
// 返回值：
//   - n: int 写入的字节数
//   - err: 错误信息
func (f *File) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, syscall.EROFS // 文件系统为只读，返回错误
}

// Name 返回文件名
// 返回值：
//   - string: 文件名
func (f *File) Name() string {
	return filepath.Join(splitpath(f.h.Name)) // 获取文件名
}

// getDirectoryNames 获取目录下的所有文件名
// 返回值：
//   - []string: 文件名列表
//   - error: 错误信息
func (f *File) getDirectoryNames() ([]string, error) {
	d, ok := f.fs.files[f.Name()]
	if !ok {
		return nil, &os.PathError{Op: "readdir", Path: f.Name(), Err: syscall.ENOENT} // 目录不存在，返回错误
	}

	var names []string
	for n := range d {
		names = append(names, n) // 添加文件名到列表
	}
	sort.Strings(names) // 排序文件名

	return names, nil // 返回文件名列表
}

// Readdir 读取目录内容
// 参数：
//   - count: int 读取的文件数量
//
// 返回值：
//   - []os.FileInfo: 文件信息列表
//   - error: 错误信息
func (f *File) Readdir(count int) ([]os.FileInfo, error) {
	if f.closed {
		return nil, afero.ErrFileClosed // 如果文件已关闭，返回错误
	}

	if !f.h.FileInfo().IsDir() {
		return nil, syscall.ENOTDIR // 如果不是目录，返回错误
	}

	names, err := f.getDirectoryNames() // 获取目录下的文件名
	if err != nil {
		return nil, err // 返回错误信息
	}

	d := f.fs.files[f.Name()]
	var fi []os.FileInfo
	for _, n := range names {
		if n == "" {
			continue
		}

		f := d[n]
		fi = append(fi, f.h.FileInfo()) // 添加文件信息到列表
		if count > 0 && len(fi) >= count {
			break // 如果达到读取数量，停止读取
		}
	}

	return fi, nil // 返回文件信息列表
}

// Readdirnames 读取目录内容并返回文件名列表
// 参数：
//   - n: int 读取的文件数量
//
// 返回值：
//   - []string: 文件名列表
//   - error: 错误信息
func (f *File) Readdirnames(n int) ([]string, error) {
	fi, err := f.Readdir(n) // 读取目录内容
	if err != nil {
		return nil, err // 返回错误信息
	}

	var names []string
	for _, f := range fi {
		names = append(names, f.Name()) // 添加文件名到列表
	}

	return names, nil // 返回文件名列表
}

// Stat 返回文件的文件信息
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (f *File) Stat() (os.FileInfo, error) {
	return f.h.FileInfo(), nil // 返回文件信息
}

// Sync 将文件的更改同步到存储设备
// 返回值：
//   - error: 错误信息
func (f *File) Sync() error {
	return nil // 只读文件系统，无需同步，返回 nil
}

// Truncate 将文件截断到指定大小
// 参数：
//   - size: int64 文件大小
//
// 返回值：
//   - error: 错误信息
func (f *File) Truncate(size int64) error {
	return syscall.EROFS // 文件系统为只读，返回错误
}

// WriteString 向文件中写入字符串
// 参数：
//   - s: string 写入的字符串
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *File) WriteString(s string) (ret int, err error) {
	return 0, syscall.EROFS // 文件系统为只读，返回错误
}
