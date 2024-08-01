package afero

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// httpDir 结构体表示一个 HTTP 目录
type httpDir struct {
	basePath string // 基本路径
	fs       HttpFs // HTTP 文件系统
}

// Open 打开指定名称的文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - http.File: 文件对象
//   - error: 错误信息
func (d httpDir) Open(name string) (http.File, error) {
	// 检查路径分隔符和无效字符
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) ||
		strings.Contains(name, "\x00") {
		return nil, errors.New("http: invalid character in file path") // 返回错误信息
	}
	dir := string(d.basePath) // 获取基本路径
	if dir == "" {
		dir = "." // 如果基本路径为空，则设置为当前目录
	}

	// 打开文件并返回文件对象和错误信息
	f, err := d.fs.Open(filepath.Join(dir, filepath.FromSlash(path.Clean("/"+name))))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err // 返回错误信息
	}
	return f, nil // 返回文件对象
}

// HttpFs 结构体表示一个 HTTP 文件系统
type HttpFs struct {
	source Afero // 源文件系统
}

// NewHttpFs 创建一个新的 HttpFs
// 参数：
//   - source: Afero 源文件系统
//
// 返回值：
//   - *HttpFs: 新的 HttpFs
func NewHttpFs(source Afero) *HttpFs {
	return &HttpFs{source: source}
}

// Dir 返回一个 httpDir 对象
// 参数：
//   - s: string 目录路径
//
// 返回值：
//   - *httpDir: httpDir 对象
func (h HttpFs) Dir(s string) *httpDir {
	return &httpDir{basePath: s, fs: h}
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (h HttpFs) Name() string { return "HttpFs" }

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (h HttpFs) Create(name string) (File, error) {
	return h.source.Create(name) // 调用源文件系统的 Create 方法
}

// Chmod 更改文件模式
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) Chmod(name string, mode os.FileMode) error {
	return h.source.Chmod(name, mode) // 调用源文件系统的 Chmod 方法
}

// Chown 更改文件的所有者
// 参数：
//   - name: string 文件名
//   - uid: int 用户 ID
//   - gid: int 组 ID
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) Chown(name string, uid, gid int) error {
	return h.source.Chown(name, uid, gid) // 调用源文件系统的 Chown 方法
}

// Chtimes 更改文件的访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) Chtimes(name string, atime, mtime time.Time) error {
	return h.source.Chtimes(name, atime, mtime) // 调用源文件系统的 Chtimes 方法
}

// Mkdir 创建目录
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) Mkdir(name string, perm os.FileMode) error {
	return h.source.Mkdir(name, perm) // 调用源文件系统的 Mkdir 方法
}

// MkdirAll 创建目录及其所有父目录
// 参数：
//   - path: string 路径名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) MkdirAll(path string, perm os.FileMode) error {
	return h.source.MkdirAll(path, perm) // 调用源文件系统的 MkdirAll 方法
}

// Open 打开指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - http.File: 文件对象
//   - error: 错误信息
func (h HttpFs) Open(name string) (http.File, error) {
	f, err := h.source.Open(name) // 调用源文件系统的 Open 方法
	if err == nil {
		if httpfile, ok := f.(http.File); ok {
			return httpfile, nil // 如果文件对象实现了 http.File 接口，返回文件对象
		}
	}
	return nil, err // 返回错误信息
}

// OpenFile 打开文件，支持指定标志和模式
// 参数：
//   - name: string 文件名
//   - flag: int 文件标志
//   - perm: os.FileMode 文件模式
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (h HttpFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return h.source.OpenFile(name, flag, perm) // 调用源文件系统的 OpenFile 方法
}

// Remove 删除指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) Remove(name string) error {
	return h.source.Remove(name) // 调用源文件系统的 Remove 方法
}

// RemoveAll 删除指定路径及其包含的所有子目录
// 参数：
//   - path: string 路径名
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) RemoveAll(path string) error {
	return h.source.RemoveAll(path) // 调用源文件系统的 RemoveAll 方法
}

// Rename 重命名文件
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (h HttpFs) Rename(oldname, newname string) error {
	return h.source.Rename(oldname, newname) // 调用源文件系统的 Rename 方法
}

// Stat 返回文件的信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (h HttpFs) Stat(name string) (os.FileInfo, error) {
	return h.source.Stat(name) // 调用源文件系统的 Stat 方法
}
