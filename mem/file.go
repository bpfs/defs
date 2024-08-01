package mem

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bpfs/defs/internal/common"
)

// FilePathSeparator 是文件路径的分隔符
const FilePathSeparator = string(filepath.Separator)

// 确保 File 实现了 fs.ReadDirFile 接口
var _ fs.ReadDirFile = &File{}

// File 结构体表示一个文件
type File struct {
	at           int64     // 当前文件指针的位置
	readDirCount int64     // 读取目录的计数
	closed       bool      // 文件是否已关闭
	readOnly     bool      // 文件是否只读
	fileData     *FileData // 文件的数据
}

// NewFileHandle 创建一个新的文件句柄
// 参数：
//   - data: *FileData 文件的数据
//
// 返回：
//   - *File 新的文件句柄
func NewFileHandle(data *FileData) *File {
	return &File{fileData: data}
}

// NewReadOnlyFileHandle 创建一个新的只读文件句柄
// 参数：
//   - data: *FileData 文件的数据
//
// 返回：
//   - *File 新的只读文件句柄
func NewReadOnlyFileHandle(data *FileData) *File {
	return &File{fileData: data, readOnly: true}
}

// Data 返回文件的数据
// 返回：
//   - *FileData 文件的数据
func (f File) Data() *FileData {
	return f.fileData
}

// FileData 结构体表示文件的数据
type FileData struct {
	sync.Mutex
	name    string      // 文件名称
	data    []byte      // 文件内容
	memDir  Dir         // 目录信息
	dir     bool        // 是否是目录
	mode    os.FileMode // 文件权限模式
	modtime time.Time   // 文件修改时间
	uid     int         // 文件所有者的用户ID
	gid     int         // 文件所有者的组ID
}

// Name 返回文件的名称
// 返回：
//   - string 文件名称
func (d *FileData) Name() string {
	d.Lock()
	defer d.Unlock()
	return d.name
}

// CreateFile 创建一个新的文件
// 参数：
//   - name: string 文件名称
//
// 返回：
//   - *FileData 新的文件数据
func CreateFile(name string) *FileData {
	return &FileData{name: name, mode: os.ModeTemporary, modtime: time.Now()}
}

// CreateDir 创建一个新的目录
// 参数：
//   - name: string 目录名称
//
// 返回：
//   - *FileData 新的目录数据
func CreateDir(name string) *FileData {
	return &FileData{name: name, memDir: &DirMap{}, dir: true, modtime: time.Now()}
}

// ChangeFileName 修改文件名称
// 参数：
//   - f: *FileData 文件数据
//   - newname: string 新的文件名称
func ChangeFileName(f *FileData, newname string) {
	f.Lock()
	f.name = newname
	f.Unlock()
}

// SetMode 设置文件权限模式
// 参数：
//   - f: *FileData 文件数据
//   - mode: os.FileMode 文件权限模式
func SetMode(f *FileData, mode os.FileMode) {
	f.Lock()
	f.mode = mode
	f.Unlock()
}

// SetModTime 设置文件修改时间
// 参数：
//   - f: *FileData 文件数据
//   - mtime: time.Time 文件修改时间
func SetModTime(f *FileData, mtime time.Time) {
	f.Lock()
	setModTime(f, mtime)
	f.Unlock()
}

// setModTime 内部函数，用于设置文件修改时间
// 参数：
//   - f: *FileData 文件数据
//   - mtime: time.Time 文件修改时间
func setModTime(f *FileData, mtime time.Time) {
	f.modtime = mtime
}

// SetUID 设置文件所有者的用户ID
// 参数：
//   - f: *FileData 文件数据
//   - uid: int 用户ID
func SetUID(f *FileData, uid int) {
	f.Lock()
	f.uid = uid
	f.Unlock()
}

// SetGID 设置文件所有者的组ID
// 参数：
//   - f: *FileData 文件数据
//   - gid: int 组ID
func SetGID(f *FileData, gid int) {
	f.Lock()
	f.gid = gid
	f.Unlock()
}

// GetFileInfo 获取文件信息
// 参数：
//   - f: *FileData 文件数据
//
// 返回：
//   - *FileInfo 文件信息
func GetFileInfo(f *FileData) *FileInfo {
	return &FileInfo{f}
}

// Open 打开文件
// 返回：
//   - error 错误信息，如果有
func (f *File) Open() error {
	atomic.StoreInt64(&f.at, 0)
	atomic.StoreInt64(&f.readDirCount, 0)
	f.fileData.Lock()
	f.closed = false
	f.fileData.Unlock()
	return nil
}

// Close 关闭文件
// 返回：
//   - error 错误信息，如果有
func (f *File) Close() error {
	f.fileData.Lock() // 加锁以保护文件数据
	f.closed = true   // 标记文件已关闭
	if !f.readOnly {  // 如果文件不是只读的
		setModTime(f.fileData, time.Now()) // 更新文件的修改时间为当前时间
	}
	f.fileData.Unlock() // 解锁
	return nil          // 返回nil表示成功
}

// Name 返回文件的名称
// 返回：
//   - string 文件名称
func (f *File) Name() string {
	return f.fileData.Name() // 返回文件数据的名称
}

// Stat 返回文件的文件信息
// 返回：
//   - os.FileInfo 文件信息
//   - error 错误信息，如果有
func (f *File) Stat() (os.FileInfo, error) {
	return &FileInfo{f.fileData}, nil // 返回文件信息
}

// Sync 同步文件数据到存储设备
// 返回：
//   - error 错误信息，如果有
func (f *File) Sync() error {
	return nil // 内存文件系统不需要同步，直接返回nil
}

// Readdir 读取目录中的文件信息
// 参数：
//   - count: int 要读取的文件数量
//
// 返回：
//   - []os.FileInfo 文件信息切片
//   - error 错误信息，如果有
func (f *File) Readdir(count int) (res []os.FileInfo, err error) {
	if !f.fileData.dir { // 如果不是目录
		return nil, &os.PathError{Op: "readdir", Path: f.fileData.name, Err: errors.New("not a dir")} // 返回错误
	}
	var outLength int64

	f.fileData.Lock()                                   // 加锁以保护文件数据
	files := f.fileData.memDir.Files()[f.readDirCount:] // 获取目录中的文件
	if count > 0 {
		if len(files) < count {
			outLength = int64(len(files)) // 读取文件数量少于请求的数量
		} else {
			outLength = int64(count) // 读取请求数量的文件
		}
		if len(files) == 0 {
			err = io.EOF // 没有更多文件
		}
	} else {
		outLength = int64(len(files)) // 读取所有文件
	}
	f.readDirCount += outLength // 更新读取计数
	f.fileData.Unlock()         // 解锁

	res = make([]os.FileInfo, outLength)
	for i := range res {
		res[i] = &FileInfo{files[i]} // 填充返回的文件信息切片
	}

	return res, err
}

// Readdirnames 读取目录中的文件名
// 参数：
//   - n: int 要读取的文件数量
//
// 返回：
//   - []string 文件名切片
//   - error 错误信息，如果有
func (f *File) Readdirnames(n int) (names []string, err error) {
	fi, err := f.Readdir(n) // 调用 Readdir 读取文件信息
	names = make([]string, len(fi))
	for i, f := range fi {
		_, names[i] = filepath.Split(f.Name()) // 提取文件名并填充返回的文件名切片
	}
	return names, err
}

// ReadDir 实现 fs.ReadDirFile 接口
// 参数：
//   - n: int 要读取的文件数量
//
// 返回：
//   - []fs.DirEntry 目录条目切片
//   - error 错误信息，如果有
func (f *File) ReadDir(n int) ([]fs.DirEntry, error) {
	fi, err := f.Readdir(n) // 调用 Readdir 读取文件信息
	if err != nil {
		return nil, err
	}
	di := make([]fs.DirEntry, len(fi))
	for i, f := range fi {
		di[i] = common.FileInfoDirEntry{FileInfo: f} // 填充返回的目录条目切片
	}
	return di, nil
}

// Read 读取文件数据到缓冲区
// 参数：
//   - b: []byte 缓冲区
//
// 返回：
//   - int 读取的字节数
//   - error 错误信息，如果有
func (f *File) Read(b []byte) (n int, err error) {
	f.fileData.Lock() // 加锁以保护文件数据
	defer f.fileData.Unlock()
	if f.closed { // 如果文件已关闭
		return 0, ErrFileClosed // 返回错误
	}
	if len(b) > 0 && int(f.at) == len(f.fileData.data) {
		return 0, io.EOF // 文件读取到末尾
	}
	if int(f.at) > len(f.fileData.data) {
		return 0, io.ErrUnexpectedEOF // 文件读取意外结束
	}
	if len(f.fileData.data)-int(f.at) >= len(b) {
		n = len(b) // 读取缓冲区大小的数据
	} else {
		n = len(f.fileData.data) - int(f.at) // 读取剩余的数据
	}
	copy(b, f.fileData.data[f.at:f.at+int64(n)]) // 将数据复制到缓冲区
	atomic.AddInt64(&f.at, int64(n))             // 更新文件指针位置
	return
}

// ReadAt 从指定偏移位置读取文件数据到缓冲区
// 参数：
//   - b: []byte 缓冲区
//   - off: int64 偏移量
//
// 返回：
//   - int 读取的字节数
//   - error 错误信息，如果有
func (f *File) ReadAt(b []byte, off int64) (n int, err error) {
	prev := atomic.LoadInt64(&f.at) // 保存当前文件指针位置
	atomic.StoreInt64(&f.at, off)   // 设置文件指针到指定偏移位置
	n, err = f.Read(b)              // 调用 Read 方法读取数据
	atomic.StoreInt64(&f.at, prev)  // 恢复文件指针位置
	return
}

// Truncate 将文件截断到指定大小
// 参数：
//   - size: int64 要截断的大小
//
// 返回：
//   - error 错误信息，如果有
func (f *File) Truncate(size int64) error {
	if f.closed { // 如果文件已关闭
		return ErrFileClosed // 返回文件关闭错误
	}
	if f.readOnly { // 如果文件是只读的
		return &os.PathError{Op: "truncate", Path: f.fileData.name, Err: errors.New("file handle is read only")} // 返回文件只读错误
	}
	if size < 0 { // 如果截断大小为负
		return ErrOutOfRange // 返回超出范围错误
	}
	f.fileData.Lock() // 加锁以保护文件数据
	defer f.fileData.Unlock()
	if size > int64(len(f.fileData.data)) { // 如果截断大小大于文件当前大小
		diff := size - int64(len(f.fileData.data))                                         // 计算需要扩展的大小
		f.fileData.data = append(f.fileData.data, bytes.Repeat([]byte{0o0}, int(diff))...) // 扩展文件大小并填充零字节
	} else {
		f.fileData.data = f.fileData.data[0:size] // 截断文件大小
	}
	setModTime(f.fileData, time.Now()) // 更新文件修改时间
	return nil                         // 返回nil表示成功
}

// Seek 设置文件指针位置
// 参数：
//   - offset: int64 偏移量
//   - whence: int 偏移量的起始位置
//
// 返回：
//   - int64 新的文件指针位置
//   - error 错误信息，如果有
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.closed { // 如果文件已关闭
		return 0, ErrFileClosed // 返回文件关闭错误
	}
	switch whence {
	case io.SeekStart: // 从文件开头设置偏移量
		atomic.StoreInt64(&f.at, offset)
	case io.SeekCurrent: // 从当前指针位置设置偏移量
		atomic.AddInt64(&f.at, offset)
	case io.SeekEnd: // 从文件末尾设置偏移量
		atomic.StoreInt64(&f.at, int64(len(f.fileData.data))+offset)
	}
	return f.at, nil // 返回新的文件指针位置
}

// Write 向文件写入数据
// 参数：
//   - b: []byte 要写入的数据
//
// 返回：
//   - int 写入的字节数
//   - error 错误信息，如果有
func (f *File) Write(b []byte) (n int, err error) {
	if f.closed { // 如果文件已关闭
		return 0, ErrFileClosed // 返回文件关闭错误
	}
	if f.readOnly { // 如果文件是只读的
		return 0, &os.PathError{Op: "write", Path: f.fileData.name, Err: errors.New("file handle is read only")} // 返回文件只读错误
	}
	n = len(b)                     // 获取要写入的字节数
	cur := atomic.LoadInt64(&f.at) // 获取当前文件指针位置
	f.fileData.Lock()              // 加锁以保护文件数据
	defer f.fileData.Unlock()
	diff := cur - int64(len(f.fileData.data)) // 计算当前指针位置与文件末尾的差值
	var tail []byte
	if n+int(cur) < len(f.fileData.data) {
		tail = f.fileData.data[n+int(cur):] // 获取文件指针后的数据
	}
	if diff > 0 { // 如果指针位置超过文件末尾
		f.fileData.data = append(f.fileData.data, append(bytes.Repeat([]byte{0o0}, int(diff)), b...)...) // 填充零字节并写入数据
		f.fileData.data = append(f.fileData.data, tail...)                                               // 追加原始尾部数据
	} else {
		f.fileData.data = append(f.fileData.data[:cur], b...) // 替换文件中的数据
		f.fileData.data = append(f.fileData.data, tail...)    // 追加原始尾部数据
	}
	setModTime(f.fileData, time.Now()) // 更新文件修改时间

	atomic.AddInt64(&f.at, int64(n)) // 更新文件指针位置
	return
}

// WriteAt 从指定偏移量开始写入文件数据
// 参数：
//   - b: []byte 要写入的数据
//   - off: int64 偏移量
//
// 返回：
//   - int 写入的字节数
//   - error 错误信息，如果有
func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	atomic.StoreInt64(&f.at, off) // 设置文件指针到指定偏移位置
	return f.Write(b)             // 调用 Write 方法写入数据
}

// WriteString 向文件写入字符串
// 参数：
//   - s: string 要写入的字符串
//
// 返回：
//   - int 写入的字节数
//   - error 错误信息，如果有
func (f *File) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s)) // 将字符串转换为字节切片并调用 Write 方法写入数据
}

// Info 返回文件的 FileInfo
// 返回：
//   - *FileInfo 文件信息
func (f *File) Info() *FileInfo {
	return &FileInfo{f.fileData} // 返回文件信息
}

// FileInfo 结构体，用于表示文件信息
type FileInfo struct {
	*FileData
}

// Name 返回文件名，实现 os.FileInfo 接口
// 返回：
//   - string 文件名
func (s *FileInfo) Name() string {
	s.Lock()                          // 加锁以保护文件数据
	_, name := filepath.Split(s.name) // 获取文件名
	s.Unlock()                        // 解锁
	return name                       // 返回文件名
}

// Mode 返回文件模式，实现 os.FileInfo 接口
// 返回：
//   - os.FileMode 文件模式
func (s *FileInfo) Mode() os.FileMode {
	s.Lock() // 加锁以保护文件数据
	defer s.Unlock()
	return s.mode // 返回文件模式
}

// ModTime 返回文件修改时间，实现 os.FileInfo 接口
// 返回：
//   - time.Time 文件修改时间
func (s *FileInfo) ModTime() time.Time {
	s.Lock() // 加锁以保护文件数据
	defer s.Unlock()
	return s.modtime // 返回文件修改时间
}

// IsDir 返回是否为目录，实现 os.FileInfo 接口
// 返回：
//   - bool 是否为目录
func (s *FileInfo) IsDir() bool {
	s.Lock() // 加锁以保护文件数据
	defer s.Unlock()
	return s.dir // 返回是否为目录
}

// Sys 返回底层数据结构，实现 os.FileInfo 接口
// 返回：
//   - interface{} 底层数据结构
func (s *FileInfo) Sys() interface{} { return nil }

// Size 返回文件大小，实现 os.FileInfo 接口
// 返回：
//   - int64 文件大小
func (s *FileInfo) Size() int64 {
	if s.IsDir() { // 如果是目录
		return int64(42) // 返回固定大小
	}
	s.Lock() // 加锁以保护文件数据
	defer s.Unlock()
	return int64(len(s.data)) // 返回文件大小
}

var (
	ErrFileClosed        = errors.New("File is closed") // 文件已关闭错误
	ErrOutOfRange        = errors.New("out of range")   // 超出范围错误
	ErrTooLarge          = errors.New("too large")      // 文件过大错误
	ErrFileNotFound      = os.ErrNotExist               // 文件未找到错误
	ErrFileExists        = os.ErrExist                  // 文件已存在错误
	ErrDestinationExists = os.ErrExist                  // 目标已存在错误
)
