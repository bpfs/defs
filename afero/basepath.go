package afero

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

var (
	_ Lstater        = (*BasePathFs)(nil)
	_ fs.ReadDirFile = (*BasePathFile)(nil)
)

// BasePathFs 限制所有操作到一个给定的路径内。
// 传递给该文件系统的文件名将在调用基础文件系统之前添加基本路径。
// 任何在基本路径外的文件名（在 filepath.Clean() 之后）将被视为不存在的文件。
//
// 注意，它不会清理返回的错误消息，因此错误可能会暴露真实路径。
type BasePathFs struct {
	source Afero  // 基础文件系统
	path   string // 基本路径
}

// BasePathFile 表示文件系统中的文件
type BasePathFile struct {
	File
	path string // 文件路径
}

// Name 返回文件的名称，去除基本路径部分
func (f *BasePathFile) Name() string {
	sourcename := f.File.Name()
	return strings.TrimPrefix(sourcename, filepath.Clean(f.path))
}

// ReadDir 读取目录中的内容
// 参数：
//   - n: int 读取目录中的条目数
//
// 返回值：
//   - []fs.DirEntry: 目录条目列表
//   - error: 错误信息
func (f *BasePathFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if rdf, ok := f.File.(fs.ReadDirFile); ok {
		return rdf.ReadDir(n)
	}
	return readDirFile{f.File}.ReadDir(n)
}

// NewBasePathFs 创建一个新的 BasePathFs
// 参数：
//   - source: Fs 基础文件系统
//   - path: string 基本路径
//
// 返回值：
//   - Fs: 新的 BasePathFs
func NewBasePathFs(source Afero, path string) Afero {
	return &BasePathFs{source: source, path: path}
}

// RealPath 返回文件的真实路径，如果文件在基本路径外，则返回错误
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - string: 真实路径
//   - error: 错误信息
func (b *BasePathFs) RealPath(name string) (path string, err error) {
	if err := validateBasePathName(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return name, err
	}

	bpath := filepath.Clean(b.path)
	path = filepath.Clean(filepath.Join(bpath, name))
	if !strings.HasPrefix(path, bpath) {
		return name, os.ErrNotExist
	}

	return path, nil
}

// validateBasePathName 验证基本路径名称是否有效
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func validateBasePathName(name string) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	// 在 Windows 上，常见的错误是提供一个绝对的操作系统路径
	// 我们可以去掉基本部分，但这不是很可移植。
	if filepath.IsAbs(name) {
		return os.ErrNotExist
	}

	return nil
}

// Chtimes 更改指定文件的访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) Chtimes(name string, atime, mtime time.Time) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "chtimes", Path: name, Err: err}
	}
	return b.source.Chtimes(name, atime, mtime)
}

// Chmod 更改指定文件的模式
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) Chmod(name string, mode os.FileMode) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "chmod", Path: name, Err: err}
	}
	return b.source.Chmod(name, mode)
}

// Chown 更改指定文件的 uid 和 gid
// 参数：
//   - name: string 文件名
//   - uid: int 用户ID
//   - gid: int 组ID
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) Chown(name string, uid, gid int) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "chown", Path: name, Err: err}
	}
	return b.source.Chown(name, uid, gid)
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (b *BasePathFs) Name() string {
	return "BasePathFs"
}

// Stat 返回指定文件的文件信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (b *BasePathFs) Stat(name string) (fi os.FileInfo, err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}
	return b.source.Stat(name)
}

// Rename 重命名文件
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) Rename(oldname, newname string) (err error) {
	if oldname, err = b.RealPath(oldname); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "rename", Path: oldname, Err: err}
	}
	if newname, err = b.RealPath(newname); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "rename", Path: newname, Err: err}
	}
	return b.source.Rename(oldname, newname)
}

// RemoveAll 删除指定路径及其包含的所有子目录
// 参数：
//   - name: string 路径名
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) RemoveAll(name string) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "remove_all", Path: name, Err: err}
	}
	return b.source.RemoveAll(name)
}

// Remove 删除指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) Remove(name string) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}
	return b.source.Remove(name)
}

// OpenFile 打开文件，支持指定标志和模式
// 参数：
//   - name: string 文件名
//   - flag: int 打开标志
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (b *BasePathFs) OpenFile(name string, flag int, mode os.FileMode) (f File, err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, &os.PathError{Op: "openfile", Path: name, Err: err}
	}
	sourcef, err := b.source.OpenFile(name, flag, mode)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return &BasePathFile{sourcef, b.path}, nil
}

// Open 打开指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (b *BasePathFs) Open(name string) (f File, err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	sourcef, err := b.source.Open(name)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return &BasePathFile{File: sourcef, path: b.path}, nil
}

// Mkdir 创建指定目录
// 参数：
//   - name: string 目录名
//   - mode: os.FileMode 目录模式
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) Mkdir(name string, mode os.FileMode) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	return b.source.Mkdir(name, mode)
}

// MkdirAll 创建指定路径及其所有父目录
// 参数：
//   - name: string 路径名
//   - mode: os.FileMode 目录模式
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) MkdirAll(name string, mode os.FileMode) (err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	return b.source.MkdirAll(name, mode)
}

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (b *BasePathFs) Create(name string) (f File, err error) {
	if name, err = b.RealPath(name); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, &os.PathError{Op: "create", Path: name, Err: err}
	}
	sourcef, err := b.source.Create(name)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return &BasePathFile{File: sourcef, path: b.path}, nil
}

// LstatIfPossible 返回文件信息和一个布尔值，指示是否使用了 Lstat
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - bool: 是否使用了 Lstat
//   - error: 错误信息
func (b *BasePathFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	name, err := b.RealPath(name)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, false, &os.PathError{Op: "lstat", Path: name, Err: err}
	}
	if lstater, ok := b.source.(Lstater); ok {
		return lstater.LstatIfPossible(name)
	}
	fi, err := b.source.Stat(name)
	return fi, false, err
}

// SymlinkIfPossible 创建符号链接（如果可能）
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (b *BasePathFs) SymlinkIfPossible(oldname, newname string) error {
	oldname, err := b.RealPath(oldname)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: err}
	}
	newname, err = b.RealPath(newname)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: err}
	}
	if linker, ok := b.source.(Linker); ok {
		return linker.SymlinkIfPossible(oldname, newname)
	}
	return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: ErrNoSymlink}
}

// ReadlinkIfPossible 读取符号链接（如果可能）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - string: 符号链接路径
//   - error: 错误信息
func (b *BasePathFs) ReadlinkIfPossible(name string) (string, error) {
	name, err := b.RealPath(name)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return "", &os.PathError{Op: "readlink", Path: name, Err: err}
	}
	if reader, ok := b.source.(LinkReader); ok {
		return reader.ReadlinkIfPossible(name)
	}
	return "", &os.PathError{Op: "readlink", Path: name, Err: ErrNoReadlink}
}
