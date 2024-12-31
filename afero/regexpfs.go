package afero

import (
	"os"
	"regexp"
	"syscall"
	"time"

	"github.com/bpfs/defs/utils/logger"
)

// RegexpFs 通过正则表达式过滤文件（不包括目录）。只有匹配给定正则表达式的文件才会被允许，
// 其他所有文件都会返回 ENOENT 错误（"没有这样的文件或目录"）。
type RegexpFs struct {
	re     *regexp.Regexp // 正则表达式
	source Afero          // 源文件系统
}

// NewRegexpFs 创建一个新的 RegexpFs
// 参数：
//   - source: Fs 源文件系统
//   - re: *regexp.Regexp 正则表达式
//
// 返回值：
//   - Fs: 过滤后的文件系统
func NewRegexpFs(source Afero, re *regexp.Regexp) Afero {
	return &RegexpFs{source: source, re: re}
}

// RegexpFile 代表一个正则表达式过滤的文件
type RegexpFile struct {
	f  File           // 文件
	re *regexp.Regexp // 正则表达式
}

// matchesName 检查文件名是否匹配正则表达式
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) matchesName(name string) error {
	if r.re == nil {
		return nil // 如果没有正则表达式，返回 nil
	}
	if r.re.MatchString(name) {
		return nil // 如果文件名匹配正则表达式，返回 nil
	}
	return syscall.ENOENT // 否则返回 ENOENT 错误
}

// dirOrMatches 检查文件是否是目录或者是否匹配正则表达式
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) dirOrMatches(name string) error {
	dir, err := IsDir(r.source, name) // 检查是否是目录
	if err != nil {
		logger.Error("应用选项失败:", err)
		return err // 返回错误信息
	}
	if dir {
		return nil // 如果是目录，返回 nil
	}
	return r.matchesName(name) // 否则检查文件名是否匹配正则表达式
}

// Chtimes 更改文件访问和修改时间
// 参数：
//   - name: string 文件名
//   - a: time.Time 访问时间
//   - m: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) Chtimes(name string, a, m time.Time) error {
	if err := r.dirOrMatches(name); err != nil {
		logger.Error("应用选项失败:", err)
		return err // 如果文件不匹配，返回错误
	}
	return r.source.Chtimes(name, a, m) // 调用源文件系统的方法
}

// Chmod 更改文件权限
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) Chmod(name string, mode os.FileMode) error {
	if err := r.dirOrMatches(name); err != nil {
		logger.Error("应用选项失败:", err)
		return err // 如果文件不匹配，返回错误
	}
	return r.source.Chmod(name, mode) // 调用源文件系统的方法
}

// Chown 更改文件所有者
// 参数：
//   - name: string 文件名
//   - uid: int 用户ID
//   - gid: int 组ID
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) Chown(name string, uid, gid int) error {
	if err := r.dirOrMatches(name); err != nil {
		logger.Error("应用选项失败:", err)
		return err // 如果文件不匹配，返回错误
	}
	return r.source.Chown(name, uid, gid) // 调用源文件系统的方法
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (r *RegexpFs) Name() string {
	return "RegexpFs"
}

// Stat 获取文件信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (r *RegexpFs) Stat(name string) (os.FileInfo, error) {
	if err := r.dirOrMatches(name); err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 如果文件不匹配，返回错误
	}
	return r.source.Stat(name) // 调用源文件系统的方法
}

// Rename 重命名文件
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) Rename(oldname, newname string) error {
	dir, err := IsDir(r.source, oldname) // 检查是否是目录
	if err != nil {
		logger.Error("应用选项失败:", err)
		return err // 返回错误信息
	}

	if dir {
		return nil // 如果是目录，返回 nil
	}

	if err := r.matchesName(oldname); err != nil {
		logger.Error("应用选项失败:", err)
		return err // 如果旧文件名不匹配，返回错误
	}

	if err := r.matchesName(newname); err != nil {
		logger.Error("应用选项失败:", err)
		return err // 如果新文件名不匹配，返回错误
	}

	return r.source.Rename(oldname, newname) // 调用源文件系统的方法
}

// RemoveAll 删除文件或目录及其子文件
// 参数：
//   - p: string 路径
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) RemoveAll(p string) error {
	dir, err := IsDir(r.source, p) // 检查是否是目录
	if err != nil {
		logger.Error("应用选项失败:", err)
		return err // 返回错误信息
	}

	if !dir {
		if err := r.matchesName(p); err != nil {
			return err // 如果文件不匹配，返回错误
		}
	}

	return r.source.RemoveAll(p) // 调用源文件系统的方法
}

// Remove 删除文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) Remove(name string) error {
	if err := r.dirOrMatches(name); err != nil {
		logger.Error("应用选项失败:", err)
		return err // 如果文件不匹配，返回错误
	}

	return r.source.Remove(name) // 调用源文件系统的方法
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
func (r *RegexpFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if err := r.dirOrMatches(name); err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 如果文件不匹配，返回错误
	}

	return r.source.OpenFile(name, flag, perm) // 调用源文件系统的方法
}

// Open 打开文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (r *RegexpFs) Open(name string) (File, error) {
	dir, err := IsDir(r.source, name) // 检查是否是目录
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 返回错误信息
	}

	if !dir {
		if err := r.matchesName(name); err != nil {
			return nil, err // 如果文件不匹配，返回错误
		}
	}

	f, err := r.source.Open(name) // 调用源文件系统的方法
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 返回错误信息
	}

	return &RegexpFile{f: f, re: r.re}, nil // 返回 RegexpFile 对象
}

// Mkdir 创建目录
// 参数：
//   - n: string 目录名
//   - p: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) Mkdir(n string, p os.FileMode) error {
	return r.source.Mkdir(n, p) // 调用源文件系统的方法
}

// MkdirAll 创建目录及其所有父目录
// 参数：
//   - n: string 目录名
//   - p: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (r *RegexpFs) MkdirAll(n string, p os.FileMode) error {
	return r.source.MkdirAll(n, p) // 调用源文件系统的方法
}

// Create 创建文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (r *RegexpFs) Create(name string) (File, error) {
	if err := r.matchesName(name); err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 如果文件名不匹配，返回错误
	}

	return r.source.Create(name) // 调用源文件系统的方法
}

// Close 关闭文件
// 返回值：
//   - error: 错误信息
func (f *RegexpFile) Close() error {
	return f.f.Close() // 调用文件对象的方法
}

// Read 读取文件
// 参数：
//   - s: []byte 缓冲区
//
// 返回值：
//   - int: 读取的字节数
//   - error: 错误信息
func (f *RegexpFile) Read(s []byte) (int, error) {
	return f.f.Read(s) // 调用文件对象的方法
}

// ReadAt 从指定偏移量读取文件
// 参数：
//   - s: []byte 缓冲区
//   - o: int64 偏移量
//
// 返回值：
//   - int: 读取的字节数
//   - error: 错误信息
func (f *RegexpFile) ReadAt(s []byte, o int64) (int, error) {
	return f.f.ReadAt(s, o) // 调用文件对象的方法
}

// Seek 设置文件指针的位置
// 参数：
//   - o: int64 偏移量
//   - w: int 起始位置
//
// 返回值：
//   - int64: 新的文件指针位置
//   - error: 错误信息
func (f *RegexpFile) Seek(o int64, w int) (int64, error) {
	return f.f.Seek(o, w) // 调用文件对象的方法
}

// Write 写入文件
// 参数：
//   - s: []byte 缓冲区
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *RegexpFile) Write(s []byte) (int, error) {
	return f.f.Write(s) // 调用文件对象的方法
}

// WriteAt 从指定偏移量写入文件
// 参数：
//   - s: []byte 缓冲区
//   - o: int64 偏移量
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *RegexpFile) WriteAt(s []byte, o int64) (int, error) {
	return f.f.WriteAt(s, o) // 调用文件对象的方法
}

// Name 返回文件名
// 返回值：
//   - string: 文件名
func (f *RegexpFile) Name() string {
	return f.f.Name() // 调用文件对象的方法
}

// Readdir 读取目录中的文件信息
// 参数：
//   - c: int 读取的文件数
//
// 返回值：
//   - []os.FileInfo: 文件信息列表
//   - error: 错误信息
func (f *RegexpFile) Readdir(c int) (fi []os.FileInfo, err error) {
	var rfi []os.FileInfo
	rfi, err = f.f.Readdir(c) // 调用文件对象的方法
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 返回错误信息
	}

	for _, i := range rfi {
		if i.IsDir() || f.re.MatchString(i.Name()) {
			fi = append(fi, i) // 添加匹配的文件信息
		}
	}

	return fi, nil // 返回文件信息列表
}

// Readdirnames 读取目录中的文件名
// 参数：
//   - c: int 读取的文件数
//
// 返回值：
//   - []string: 文件名列表
//   - error: 错误信息
func (f *RegexpFile) Readdirnames(c int) (n []string, err error) {
	fi, err := f.Readdir(c) // 调用 Readdir 方法
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 返回错误信息
	}

	for _, s := range fi {
		n = append(n, s.Name()) // 添加文件名到列表
	}
	return n, nil // 返回文件名列表
}

// Stat 获取文件信息
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (f *RegexpFile) Stat() (os.FileInfo, error) {
	return f.f.Stat() // 调用文件对象的方法
}

// Sync 同步文件
// 返回值：
//   - error: 错误信息
func (f *RegexpFile) Sync() error {
	return f.f.Sync() // 调用文件对象的方法
}

// Truncate 截断文件
// 参数：
//   - s: int64 新的文件大小
//
// 返回值：
//   - error: 错误信息
func (f *RegexpFile) Truncate(s int64) error {
	return f.f.Truncate(s) // 调用文件对象的方法
}

// WriteString 写入字符串到文件
// 参数：
//   - s: string 字符串
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *RegexpFile) WriteString(s string) (int, error) {
	return f.f.WriteString(s) // 调用文件对象的方法
}
