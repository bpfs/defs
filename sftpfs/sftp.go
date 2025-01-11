package sftpfs

import (
	"os"
	"time"

	"github.com/bpfs/defs/v2/afero"
	"github.com/pkg/sftp"
)

// Fs 是一个实现了 afero.Fs 接口的文件系统，使用 sftp 包提供的功能。
// 对于任何方法的详细信息，请参考 sftp 包的文档（github.com/pkg/sftp）。
type Fs struct {
	client *sftp.Client // SFTP 客户端
}

// New 创建一个新的 Fs 实例
// 参数：
//   - client: *sftp.Client SFTP 客户端
//
// 返回值：
//   - afero.Afero 新的 Fs 实例
func New(client *sftp.Client) afero.Afero {
	return &Fs{client: client}
}

// Name 返回文件系统的名称
// 返回值：
//   - string 文件系统名称
func (s Fs) Name() string { return "sftpfs" }

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - afero.File 文件对象
//   - error 可能的错误
func (s Fs) Create(name string) (afero.File, error) {
	return FileCreate(s.client, name)
}

// Mkdir 创建一个新目录
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error 可能的错误
func (s Fs) Mkdir(name string, perm os.FileMode) error {
	err := s.client.Mkdir(name)
	if err != nil {
		return err
	}
	return s.client.Chmod(name, perm)
}

// MkdirAll 递归创建目录
// 参数：
//   - path: string 目录路径
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error 可能的错误
func (s Fs) MkdirAll(path string, perm os.FileMode) error {
	// 快速路径：如果我们可以确定路径是目录或文件，则停止并返回成功或错误。
	dir, err := s.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return err
	}

	// 慢速路径：确保父目录存在，然后调用 Mkdir 创建目录。
	i := len(path)
	for i > 0 && os.IsPathSeparator(path[i-1]) { // 跳过路径尾部的路径分隔符。
		i--
	}

	j := i
	for j > 0 && !os.IsPathSeparator(path[j-1]) { // 向后扫描路径元素。
		j--
	}

	if j > 1 {
		// 创建父目录
		err = s.MkdirAll(path[0:j-1], perm)
		if err != nil {
			return err
		}
	}

	// 父目录现在已存在；调用 Mkdir 并使用其结果。
	err = s.Mkdir(path, perm)
	if err != nil {
		// 处理类似 "foo/." 的参数，通过
		// 再次检查目录是否存在。
		dir, err1 := s.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// Open 打开文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - afero.File 文件对象
//   - error 可能的错误
func (s Fs) Open(name string) (afero.File, error) {
	return FileOpen(s.client, name)
}

// OpenFile 调用 SSHFS 连接上的 OpenFile 方法。mode 参数被忽略
// 因为 github.com/pkg/sftp 实现中忽略了该参数。
// 参数：
//   - name: string 文件名
//   - flag: int 文件打开标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - afero.File 文件对象
//   - error 可能的错误
func (s Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	sshfsFile, err := s.client.OpenFile(name, flag)
	if err != nil {
		return nil, err
	}
	err = sshfsFile.Chmod(perm)
	return &File{fd: sshfsFile}, err
}

// Remove 删除文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error 可能的错误
func (s Fs) Remove(name string) error {
	return s.client.Remove(name)
}

// RemoveAll 递归删除路径
// 参数：
//   - path: string 路径
//
// 返回值：
//   - error 可能的错误
func (s Fs) RemoveAll(path string) error {
	// TODO 查看 os.RemoveAll 的实现
	// https://github.com/golang/go/blob/master/src/os/path.go#L66
	return nil
}

// Rename 重命名文件或目录
// 参数：
//   - oldname: string 旧名称
//   - newname: string 新名称
//
// 返回值：
//   - error 可能的错误
func (s Fs) Rename(oldname, newname string) error {
	return s.client.Rename(oldname, newname)
}

// Stat 返回文件信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo 文件信息
//   - error 可能的错误
func (s Fs) Stat(name string) (os.FileInfo, error) {
	return s.client.Stat(name)
}

// Lstat 返回符号链接文件信息
// 参数：
//   - p: string 路径
//
// 返回值：
//   - os.FileInfo 文件信息
//   - error 可能的错误
func (s Fs) Lstat(p string) (os.FileInfo, error) {
	return s.client.Lstat(p)
}

// Chmod 修改文件权限
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件权限
//
// 返回值：
//   - error 可能的错误
func (s Fs) Chmod(name string, mode os.FileMode) error {
	return s.client.Chmod(name, mode)
}

// Chown 修改文件拥有者
// 参数：
//   - name: string 文件名
//   - uid: int 用户ID
//   - gid: int 组ID
//
// 返回值：
//   - error 可能的错误
func (s Fs) Chown(name string, uid, gid int) error {
	return s.client.Chown(name, uid, gid)
}

// Chtimes 修改文件访问时间和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error 可能的错误
func (s Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return s.client.Chtimes(name, atime, mtime)
}
