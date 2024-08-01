package zipfs

import (
	"archive/zip"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bpfs/defs/afero"
)

// Fs 代表一个只读的 ZIP 文件系统
type Fs struct {
	r     *zip.Reader                     // ZIP 读取器
	files map[string]map[string]*zip.File // 文件映射表
}

// splitpath 分割文件路径为目录和文件名
// 参数：
//   - name: string 文件路径
//
// 返回值：
//   - dir: string 目录路径
//   - file: string 文件名
func splitpath(name string) (dir, file string) {
	name = filepath.ToSlash(name) // 将路径分隔符转换为 '/'
	if len(name) == 0 || name[0] != '/' {
		name = "/" + name
	}
	name = filepath.Clean(name) // 清理路径，去除冗余分隔符和相对路径标记
	dir, file = filepath.Split(name)
	dir = filepath.Clean(dir)
	return
}

// New 创建一个新的 ZIP 文件系统
// 参数：
//   - r: *zip.Reader ZIP 读取器
//
// 返回值：
//   - afero.Afero 文件系统接口
func New(r *zip.Reader) afero.Afero {
	fs := &Fs{r: r, files: make(map[string]map[string]*zip.File)}
	// 遍历 ZIP 文件中的每个文件
	for _, file := range r.File {
		d, f := splitpath(file.Name)
		if _, ok := fs.files[d]; !ok {
			fs.files[d] = make(map[string]*zip.File)
		}
		if _, ok := fs.files[d][f]; !ok {
			fs.files[d][f] = file
		}
		// 如果是目录，确保目录存在于文件映射表中
		if file.FileInfo().IsDir() {
			dirname := filepath.Join(d, f)
			if _, ok := fs.files[dirname]; !ok {
				fs.files[dirname] = make(map[string]*zip.File)
			}
		}
	}
	return fs
}

// Create 创建新文件（只读文件系统，不支持）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - afero.File 文件接口
//   - error 可能的错误
func (fs *Fs) Create(name string) (afero.File, error) { return nil, syscall.EPERM }

// Mkdir 创建新目录（只读文件系统，不支持）
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) Mkdir(name string, perm os.FileMode) error { return syscall.EPERM }

// MkdirAll 创建多级目录（只读文件系统，不支持）
// 参数：
//   - path: string 目录路径
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) MkdirAll(path string, perm os.FileMode) error { return syscall.EPERM }

// Open 打开文件或目录
// 参数：
//   - name: string 文件或目录名
//
// 返回值：
//   - afero.File 文件接口
//   - error 可能的错误
func (fs *Fs) Open(name string) (afero.File, error) {
	d, f := splitpath(name)
	if f == "" {
		return &File{fs: fs, isdir: true}, nil
	}
	if _, ok := fs.files[d]; !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ENOENT}
	}
	file, ok := fs.files[d][f]
	if !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ENOENT}
	}
	return &File{fs: fs, zipfile: file, isdir: file.FileInfo().IsDir()}, nil
}

// OpenFile 打开文件，带有标志和权限（只读文件系统，只支持只读）
// 参数：
//   - name: string 文件名
//   - flag: int 打开标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - afero.File 文件接口
//   - error 可能的错误
func (fs *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag != os.O_RDONLY {
		return nil, syscall.EPERM
	}
	return fs.Open(name)
}

// Remove 删除文件（只读文件系统，不支持）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) Remove(name string) error { return syscall.EPERM }

// RemoveAll 删除目录及其内容（只读文件系统，不支持）
// 参数：
//   - path: string 目录路径
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) RemoveAll(path string) error { return syscall.EPERM }

// Rename 重命名文件或目录（只读文件系统，不支持）
// 参数：
//   - oldname: string 旧名称
//   - newname: string 新名称
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) Rename(oldname, newname string) error { return syscall.EPERM }

// pseudoRoot 代表伪根目录
type pseudoRoot struct{}

// Name 返回伪根目录名称
// 返回值：
//   - string 名称
func (p *pseudoRoot) Name() string { return string(filepath.Separator) }

// Size 返回伪根目录大小
// 返回值：
//   - int64 大小
func (p *pseudoRoot) Size() int64 { return 0 }

// Mode 返回伪根目录权限
// 返回值：
//   - os.FileMode 权限
func (p *pseudoRoot) Mode() os.FileMode { return os.ModeDir | os.ModePerm }

// ModTime 返回伪根目录修改时间
// 返回值：
//   - time.Time 修改时间
func (p *pseudoRoot) ModTime() time.Time { return time.Now() }

// IsDir 返回伪根目录是否是目录
// 返回值：
//   - bool 是否是目录
func (p *pseudoRoot) IsDir() bool { return true }

// Sys 返回伪根目录的系统信息（无）
// 返回值：
//   - interface{} 系统信息
func (p *pseudoRoot) Sys() interface{} { return nil }

// Stat 获取文件或目录的文件信息
// 参数：
//   - name: string 文件或目录名
//
// 返回值：
//   - os.FileInfo 文件信息
//   - error 可能的错误
func (fs *Fs) Stat(name string) (os.FileInfo, error) {
	d, f := splitpath(name)
	if f == "" {
		return &pseudoRoot{}, nil
	}
	if _, ok := fs.files[d]; !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ENOENT}
	}
	file, ok := fs.files[d][f]
	if !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ENOENT}
	}
	return file.FileInfo(), nil
}

// Name 返回文件系统名称
// 返回值：
//   - string 文件系统名称
func (fs *Fs) Name() string { return "zipfs" }

// Chmod 修改文件权限（只读文件系统，不支持）
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件权限
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) Chmod(name string, mode os.FileMode) error { return syscall.EPERM }

// Chown 修改文件所有者（只读文件系统，不支持）
// 参数：
//   - name: string 文件名
//   - uid: int 用户ID
//   - gid: int 组ID
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) Chown(name string, uid, gid int) error { return syscall.EPERM }

// Chtimes 修改文件时间（只读文件系统，不支持）
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error 可能的错误
func (fs *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error { return syscall.EPERM }
