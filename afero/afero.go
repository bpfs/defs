package afero

import (
	"errors"
	"io"
	"os"
	"time"
)

// File 表示文件系统中的一个文件
// 该接口包含了多种操作文件的方法
type File interface {
	// Close 关闭文件
	io.Closer

	// Read 从文件中读取数据
	io.Reader

	// ReadAt 从文件的指定位置读取数据
	io.ReaderAt

	// Seek 设置文件的偏移量，用于下次读写操作
	io.Seeker

	// Write 向文件中写入数据
	io.Writer

	// WriteAt 向文件的指定位置写入数据
	io.WriterAt

	// Name 返回文件的名称
	Name() string

	// Readdir 读取目录的内容，返回文件信息列表
	// count 参数指定读取的文件数量，若为负数，则读取所有文件
	Readdir(count int) ([]os.FileInfo, error)

	// Readdirnames 读取目录的内容，返回文件名称列表
	// n 参数指定读取的文件名称数量，若为负数，则读取所有文件名称
	Readdirnames(n int) ([]string, error)

	// Stat 返回文件的文件信息
	Stat() (os.FileInfo, error)

	// Sync 将文件的内容同步到存储设备
	Sync() error

	// Truncate 改变文件的大小
	// 若文件变大，则新增部分内容未定义；若变小，则多余部分被丢弃
	Truncate(size int64) error

	// WriteString 向文件中写入字符串
	// 返回写入的字节数和可能出现的错误
	WriteString(s string) (ret int, err error)
}

// Afero 是文件系统接口
//
// 任何模拟或真实的文件系统都应实现此接口
type Afero interface {
	// Create 在文件系统中创建一个文件，返回文件和错误信息（如果有）
	Create(name string) (File, error)

	// Mkdir 在文件系统中创建一个目录，如果有错误则返回错误信息
	Mkdir(name string, perm os.FileMode) error

	// MkdirAll 创建一个目录路径及所有不存在的父目录
	MkdirAll(path string, perm os.FileMode) error

	// Open 打开一个文件，返回文件或错误信息（如果有）
	Open(name string) (File, error)

	// OpenFile 使用给定的标志和模式打开一个文件
	OpenFile(name string, flag int, perm os.FileMode) (File, error)

	// Remove 删除一个文件，通过名称标识，返回错误信息（如果有）
	Remove(name string) error

	// RemoveAll 删除一个目录路径及其包含的任何子目录。如果路径不存在，则不返回错误（返回nil）
	RemoveAll(path string) error

	// Rename 重命名一个文件
	Rename(oldname, newname string) error

	// Stat 返回描述指定文件的 FileInfo 或错误信息（如果有）
	Stat(name string) (os.FileInfo, error)

	// 返回此文件系统的名称
	Name() string

	// Chmod 更改指定文件的模式
	Chmod(name string, mode os.FileMode) error

	// Chown 更改指定文件的 uid 和 gid
	Chown(name string, uid, gid int) error

	// Chtimes 更改指定文件的访问和修改时间
	Chtimes(name string, atime time.Time, mtime time.Time) error
}

// 常见错误定义
var (
	ErrFileClosed        = errors.New("文件已关闭")
	ErrOutOfRange        = errors.New("超出范围")
	ErrTooLarge          = errors.New("文件太大")
	ErrFileNotFound      = os.ErrNotExist // 文件不存在
	ErrFileExists        = os.ErrExist    // 文件已存在
	ErrDestinationExists = os.ErrExist    // 目标已存在
)
