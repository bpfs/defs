// // tarfs 包实现了 tar 档案的只读内存表示
package tarfs

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bpfs/defs/v2/afero"
)

// Fs 代表一个只读的内存中的 tar 文件系统
type Fs struct {
	files map[string]map[string]*File // 存储文件路径和文件的映射
}

// splitpath 将路径拆分为目录和文件名
// 参数：
//   - name: string 文件路径
//
// 返回值：
//   - dir: string 目录路径
//   - file: string 文件名
func splitpath(name string) (dir, file string) {
	name = filepath.ToSlash(name) // 将路径分隔符转换为斜杠
	if len(name) == 0 || name[0] != '/' {
		name = "/" + name // 确保路径以斜杠开头
	}
	name = filepath.Clean(name)      // 清理路径
	dir, file = filepath.Split(name) // 拆分路径为目录和文件名
	dir = filepath.Clean(dir)        // 清理目录路径
	return
}

// New 创建一个新的 Fs 实例
// 参数：
//   - t: *tar.Reader tar 文件读取器
//
// 返回值：
//   - *Fs: 返回新创建的文件系统实例
func New(t *tar.Reader) *Fs {
	fs := &Fs{files: make(map[string]map[string]*File)} // 初始化文件系统
	for {
		hdr, err := t.Next()
		if err == io.EOF {
			break // 读取到文件结尾，停止读取
		}
		if err != nil {
			return nil // 读取过程中发生错误，返回 nil
		}

		d, f := splitpath(hdr.Name) // 拆分路径为目录和文件名
		if _, ok := fs.files[d]; !ok {
			fs.files[d] = make(map[string]*File) // 如果目录不存在，则创建目录
		}

		var buf bytes.Buffer
		size, err := buf.ReadFrom(t) // 从 tar 读取数据
		if err != nil {
			panic("tarfs: reading from tar:" + err.Error()) // 读取错误，抛出异常
		}

		if size != hdr.Size {
			panic("tarfs: size mismatch") // 数据大小不匹配，抛出异常
		}

		file := &File{
			h:    hdr,
			data: bytes.NewReader(buf.Bytes()), // 创建文件实例
			fs:   fs,
		}
		fs.files[d][f] = file // 将文件添加到文件系统中

	}

	if fs.files[afero.FilePathSeparator] == nil {
		fs.files[afero.FilePathSeparator] = make(map[string]*File) // 如果根目录不存在，则创建根目录
	}
	// 添加一个伪根目录
	fs.files[afero.FilePathSeparator][""] = &File{
		h: &tar.Header{
			Name:     afero.FilePathSeparator,
			Typeflag: tar.TypeDir,
			Size:     0,
		},
		data: bytes.NewReader(nil),
		fs:   fs,
	}

	return fs
}

// Open 打开指定路径的文件
// 参数：
//   - name: string 文件路径
//
// 返回值：
//   - afero.File: 打开的文件
//   - error: 错误信息
func (fs *Fs) Open(name string) (afero.File, error) {
	d, f := splitpath(name) // 拆分路径为目录和文件名
	if _, ok := fs.files[d]; !ok {
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT} // 目录不存在，返回错误
	}

	file, ok := fs.files[d][f]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT} // 文件不存在，返回错误
	}

	nf := *file // 创建文件的副本

	return &nf, nil // 返回文件副本
}

// Name 返回文件系统名称
// 返回值：
//   - string: 文件系统名称
func (fs *Fs) Name() string { return "tarfs" }

// Create 创建新文件（只读文件系统，返回错误）
// 参数：
//   - name: string 文件路径
//
// 返回值：
//   - afero.File: 创建的文件
//   - error: 错误信息
func (fs *Fs) Create(name string) (afero.File, error) { return nil, syscall.EROFS }

// Mkdir 创建目录（只读文件系统，返回错误）
// 参数：
//   - name: string 目录路径
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) Mkdir(name string, perm os.FileMode) error { return syscall.EROFS }

// MkdirAll 创建所有目录（只读文件系统，返回错误）
// 参数：
//   - path: string 目录路径
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) MkdirAll(path string, perm os.FileMode) error { return syscall.EROFS }

// OpenFile 打开文件（只读模式）
// 参数：
//   - name: string 文件路径
//   - flag: int 打开文件的标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - afero.File: 打开的文件
//   - error: 错误信息
func (fs *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag != os.O_RDONLY {
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.EPERM} // 只支持只读模式，返回错误
	}

	return fs.Open(name) // 打开文件
}

// Remove 删除文件（只读文件系统，返回错误）
// 参数：
//   - name: string 文件路径
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) Remove(name string) error { return syscall.EROFS }

// RemoveAll 删除所有文件（只读文件系统，返回错误）
// 参数：
//   - path: string 路径
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) RemoveAll(path string) error { return syscall.EROFS }

// Rename 重命名文件（只读文件系统，返回错误）
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) Rename(oldname string, newname string) error { return syscall.EROFS }

// Stat 返回文件信息
// 参数：
//   - name: string 文件路径
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (fs *Fs) Stat(name string) (os.FileInfo, error) {
	d, f := splitpath(name) // 拆分路径为目录和文件名
	if _, ok := fs.files[d]; !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ENOENT} // 目录不存在，返回错误
	}

	file, ok := fs.files[d][f]
	if !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ENOENT} // 文件不存在，返回错误
	}

	return file.h.FileInfo(), nil // 返回文件信息
}

// Chmod 更改文件权限（只读文件系统，返回错误）
// 参数：
//   - name: string 文件路径
//   - mode: os.FileMode 文件权限
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) Chmod(name string, mode os.FileMode) error { return syscall.EROFS }

// Chown 更改文件所有者（只读文件系统，返回错误）
// 参数：
//   - name: string 文件路径
//   - uid: int 用户ID
//   - gid: int 组ID
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) Chown(name string, uid, gid int) error { return syscall.EROFS }

// Chtimes 更改文件时间（只读文件系统，返回错误）
// 参数：
//   - name: string 文件路径
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (fs *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error { return syscall.EROFS }
