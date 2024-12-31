package afero

import (
	"os"
	"syscall"
	"time"
)

// 确保 ReadOnlyFs 实现了 Lstater 接口
var _ Lstater = (*ReadOnlyFs)(nil)

// ReadOnlyFs 是一个只读文件系统的实现
type ReadOnlyFs struct {
	source Afero // 内嵌的实际文件系统
}

// NewReadOnlyFs 创建一个新的只读文件系统
// 参数：
//   - source: Fs 实际文件系统
//
// 返回值：
//   - Fs: 只读文件系统
func NewReadOnlyFs(source Afero) Afero {
	return &ReadOnlyFs{source: source}
}

// ReadDir 读取目录内容
// 参数：
//   - name: string 目录名
//
// 返回值：
//   - []os.FileInfo: 目录条目列表
//   - error: 错误信息
func (r *ReadOnlyFs) ReadDir(name string) ([]os.FileInfo, error) {
	return ReadDir(r.source, name) // 调用实际文件系统的 ReadDir 方法
}

// Chtimes 更改文件的访问和修改时间
// 参数：
//   - n: string 文件名
//   - a: time.Time 访问时间
//   - m: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) Chtimes(n string, a, m time.Time) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// Chmod 更改文件模式
// 参数：
//   - n: string 文件名
//   - m: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) Chmod(n string, m os.FileMode) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// Chown 更改文件的所有者
// 参数：
//   - n: string 文件名
//   - uid: int 用户 ID
//   - gid: int 组 ID
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) Chown(n string, uid, gid int) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (r *ReadOnlyFs) Name() string {
	return "ReadOnlyFilter" // 返回文件系统名称
}

// Stat 返回文件的信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (r *ReadOnlyFs) Stat(name string) (os.FileInfo, error) {
	return r.source.Stat(name) // 调用实际文件系统的 Stat 方法
}

// LstatIfPossible 尽可能调用 Lstat 方法
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - bool: 是否调用了 Lstat
//   - error: 错误信息
func (r *ReadOnlyFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	if lsf, ok := r.source.(Lstater); ok {
		return lsf.LstatIfPossible(name) // 调用实际文件系统的 LstatIfPossible 方法
	}
	fi, err := r.Stat(name) // 调用 Stat 方法
	return fi, false, err   // 返回文件信息、是否调用了 Lstat 和错误信息
}

// SymlinkIfPossible 创建符号链接（如果可能）
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) SymlinkIfPossible(oldname, newname string) error {
	return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: ErrNoSymlink} // 返回符号链接不支持的错误
}

// ReadlinkIfPossible 读取符号链接（如果可能）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - string: 符号链接的目标路径
//   - error: 错误信息
func (r *ReadOnlyFs) ReadlinkIfPossible(name string) (string, error) {
	if srdr, ok := r.source.(LinkReader); ok {
		return srdr.ReadlinkIfPossible(name) // 调用实际文件系统的 ReadlinkIfPossible 方法
	}
	return "", &os.PathError{Op: "readlink", Path: name, Err: ErrNoReadlink} // 返回读取符号链接不支持的错误
}

// Rename 重命名文件
// 参数：
//   - o: string 旧文件名
//   - n: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) Rename(o, n string) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// RemoveAll 删除指定路径及其包含的所有子目录
// 参数：
//   - p: string 路径名
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) RemoveAll(p string) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// Remove 删除指定文件
// 参数：
//   - n: string 文件名
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) Remove(n string) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// OpenFile 打开文件，支持指定标志和模式
// 参数：
//   - name: string 文件名
//   - flag: int 文件标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (r *ReadOnlyFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, syscall.EPERM // 如果文件标志包含写入操作，返回操作不允许的错误
	}
	return r.source.OpenFile(name, flag, perm) // 调用实际文件系统的 OpenFile 方法
}

// Open 打开指定名称的文件
// 参数：
//   - n: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (r *ReadOnlyFs) Open(n string) (File, error) {
	return r.source.Open(n) // 调用实际文件系统的 Open 方法
}

// Mkdir 创建目录
// 参数：
//   - n: string 目录名
//   - p: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) Mkdir(n string, p os.FileMode) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// MkdirAll 创建目录及其所有父目录
// 参数：
//   - n: string 路径名
//   - p: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (r *ReadOnlyFs) MkdirAll(n string, p os.FileMode) error {
	return syscall.EPERM // 返回操作不允许的错误
}

// Create 创建一个新文件
// 参数：
//   - n: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (r *ReadOnlyFs) Create(n string) (File, error) {
	return nil, syscall.EPERM // 返回操作不允许的错误
}
