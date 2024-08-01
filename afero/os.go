package afero

import (
	"os"
	"time"
)

// 确保 OsFs 实现了 Lstater 接口
var _ Lstater = (*OsFs)(nil)

// OsFs 是一个使用 os 包提供的函数实现的文件系统接口
// 有关各方法的详细信息，请查看 os 包的文档 (http://golang.org/pkg/os/)。
type OsFs struct{}

// NewOsFs 创建一个新的 OsFs 实例
// 返回值：
//   - Afero: 新的 OsFs 实例
func NewOsFs() Afero {
	return &OsFs{}
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (OsFs) Name() string {
	return "OsFs"
}

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (OsFs) Create(name string) (File, error) {
	f, e := os.Create(name) // 调用 os 包的 Create 函数创建文件
	if f == nil {
		return nil, e // 返回空指针和错误信息
	}
	return f, e // 返回文件对象和错误信息
}

// Mkdir 创建目录
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (OsFs) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm) // 调用 os 包的 Mkdir 函数创建目录
}

// MkdirAll 创建目录及其所有父目录
// 参数：
//   - path: string 路径名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (OsFs) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm) // 调用 os 包的 MkdirAll 函数创建目录及其所有父目录
}

// Open 打开指定名称的文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (OsFs) Open(name string) (File, error) {
	f, e := os.Open(name) // 调用 os 包的 Open 函数打开文件
	if f == nil {
		return nil, e // 返回空指针和错误信息
	}
	return f, e // 返回文件对象和错误信息
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
func (OsFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	f, e := os.OpenFile(name, flag, perm) // 调用 os 包的 OpenFile 函数打开文件
	if f == nil {
		return nil, e // 返回空指针和错误信息
	}
	return f, e // 返回文件对象和错误信息
}

// Remove 删除指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (OsFs) Remove(name string) error {
	return os.Remove(name) // 调用 os 包的 Remove 函数删除文件
}

// RemoveAll 删除指定路径及其包含的所有子目录
// 参数：
//   - path: string 路径名
//
// 返回值：
//   - error: 错误信息
func (OsFs) RemoveAll(path string) error {
	return os.RemoveAll(path) // 调用 os 包的 RemoveAll 函数删除路径及其所有子目录
}

// Rename 重命名文件
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (OsFs) Rename(oldname, newname string) error {
	return os.Rename(oldname, newname) // 调用 os 包的 Rename 函数重命名文件
}

// Stat 返回文件的信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (OsFs) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name) // 调用 os 包的 Stat 函数获取文件信息
}

// Chmod 更改文件模式
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (OsFs) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode) // 调用 os 包的 Chmod 函数更改文件模式
}

// Chown 更改文件的所有者
// 参数：
//   - name: string 文件名
//   - uid: int 用户 ID
//   - gid: int 组 ID
//
// 返回值：
//   - error: 错误信息
func (OsFs) Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid) // 调用 os 包的 Chown 函数更改文件的所有者
}

// Chtimes 更改文件的访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (OsFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime) // 调用 os 包的 Chtimes 函数更改文件的访问和修改时间
}

// LstatIfPossible 尽可能调用 Lstat 方法
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - bool: 是否调用了 Lstat
//   - error: 错误信息
func (OsFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	fi, err := os.Lstat(name) // 调用 os 包的 Lstat 函数获取文件信息
	return fi, true, err      // 返回文件信息、是否调用了 Lstat 和错误信息
}

// SymlinkIfPossible 创建符号链接（如果可能）
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (OsFs) SymlinkIfPossible(oldname, newname string) error {
	return os.Symlink(oldname, newname) // 调用 os 包的 Symlink 函数创建符号链接
}

// ReadlinkIfPossible 读取符号链接（如果可能）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - string: 符号链接的目标路径
//   - error: 错误信息
func (OsFs) ReadlinkIfPossible(name string) (string, error) {
	return os.Readlink(name) // 调用 os 包的 Readlink 函数读取符号链接的目标路径
}
