package sftpfs

import (
	"os"

	"github.com/pkg/sftp"
)

// File 表示一个远程文件
type File struct {
	client *sftp.Client // SFTP客户端
	fd     *sftp.File   // 文件描述符
}

// FileOpen 打开远程文件
// 参数：
//   - s: *sftp.Client SFTP客户端
//   - name: string 文件名
//
// 返回值：
//   - *File 文件对象
//   - error 可能的错误
func FileOpen(s *sftp.Client, name string) (*File, error) {
	fd, err := s.Open(name) // 打开文件
	if err != nil {
		return &File{}, err
	}
	return &File{fd: fd, client: s}, nil
}

// FileCreate 创建远程文件
// 参数：
//   - s: *sftp.Client SFTP客户端
//   - name: string 文件名
//
// 返回值：
//   - *File 文件对象
//   - error 可能的错误
func FileCreate(s *sftp.Client, name string) (*File, error) {
	fd, err := s.Create(name) // 创建文件
	if err != nil {
		return &File{}, err
	}
	return &File{fd: fd, client: s}, nil
}

// Close 关闭文件
// 返回值：
//   - error 可能的错误
func (f *File) Close() error {
	return f.fd.Close()
}

// Name 返回文件名
// 返回值：
//   - string 文件名
func (f *File) Name() string {
	return f.fd.Name()
}

// Stat 返回文件信息
// 返回值：
//   - os.FileInfo 文件信息
//   - error 可能的错误
func (f *File) Stat() (os.FileInfo, error) {
	return f.fd.Stat()
}

// Sync 同步文件（SFTP不支持，返回nil）
func (f *File) Sync() error {
	return nil
}

// Truncate 截断文件
// 参数：
//   - size: int64 截断后的文件大小
//
// 返回值：
//   - error 可能的错误
func (f *File) Truncate(size int64) error {
	return f.fd.Truncate(size)
}

// Read 读取文件内容
// 参数：
//   - b: []byte 读取缓冲区
//
// 返回值：
//   - n int 读取的字节数
//   - error 可能的错误
func (f *File) Read(b []byte) (n int, err error) {
	return f.fd.Read(b)
}

// ReadAt 从指定位置读取文件内容
// 参数：
//   - b: []byte 读取缓冲区
//   - off: int64 读取偏移量
//
// 返回值：
//   - n int 读取的字节数
//   - error 可能的错误
func (f *File) ReadAt(b []byte, off int64) (n int, err error) {
	return f.fd.ReadAt(b, off)
}

// Readdir 读取目录内容
// 参数：
//   - count: int 读取的文件数
//
// 返回值：
//   - res []os.FileInfo 文件信息列表
//   - error 可能的错误
func (f *File) Readdir(count int) (res []os.FileInfo, err error) {
	res, err = f.client.ReadDir(f.Name())
	if err != nil {
		return
	}
	if count > 0 {
		if len(res) > count {
			res = res[:count]
		}
	}
	return
}

// Readdirnames 读取目录内容并返回文件名列表
// 参数：
//   - n: int 读取的文件数
//
// 返回值：
//   - names []string 文件名列表
//   - error 可能的错误
func (f *File) Readdirnames(n int) (names []string, err error) {
	data, err := f.Readdir(n)
	if err != nil {
		return nil, err
	}
	for _, v := range data {
		names = append(names, v.Name())
	}
	return
}

// Seek 设置文件偏移量
// 参数：
//   - offset: int64 偏移量
//   - whence: int 偏移方式
//
// 返回值：
//   - int64 新的偏移量
//   - error 可能的错误
func (f *File) Seek(offset int64, whence int) (int64, error) {
	return f.fd.Seek(offset, whence)
}

// Write 写入数据到文件
// 参数：
//   - b: []byte 要写入的数据
//
// 返回值：
//   - n int 写入的字节数
//   - error 可能的错误
func (f *File) Write(b []byte) (n int, err error) {
	return f.fd.Write(b)
}

// WriteAt 写入数据到文件的指定位置（TODO）
// 参数：
//   - b: []byte 要写入的数据
//   - off: int64 写入的偏移量
//
// 返回值：
//   - n int 写入的字节数
//   - error 可能的错误
func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, nil
}

// WriteString 写入字符串到文件
// 参数：
//   - s: string 要写入的字符串
//
// 返回值：
//   - ret int 写入的字节数
//   - error 可能的错误
func (f *File) WriteString(s string) (ret int, err error) {
	return f.fd.Write([]byte(s))
}
