//go:build go1.16
// +build go1.16

package afero

import (
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"time"

	"github.com/bpfs/defs/utils/common"
)

// IOFS 采用 afero.Fs 并适配为标准库的 io/fs.FS 接口
type IOFS struct {
	Afero // 内嵌的 afero.Fs 接口
}

// NewIOFS 创建一个新的 IOFS
// 参数：
//   - fs: Afero 源文件系统
//
// 返回值：
//   - IOFS: 新的 IOFS 对象
func NewIOFS(fs Afero) IOFS {
	return IOFS{Afero: fs}
}

// 确保 IOFS 实现了多个标准库的 fs 接口
var (
	_ fs.FS         = IOFS{}
	_ fs.GlobFS     = IOFS{}
	_ fs.ReadDirFS  = IOFS{}
	_ fs.ReadFileFS = IOFS{}
	_ fs.StatFS     = IOFS{}
	_ fs.SubFS      = IOFS{}
)

// Open 打开指定名称的文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - fs.File: 文件对象
//   - error: 错误信息
func (iofs IOFS) Open(name string) (fs.File, error) {
	const op = "open" // 操作名称，用于错误包装

	// 按约定，对于 fs.FS 的实现应执行此检查
	if !fs.ValidPath(name) {
		return nil, iofs.wrapError(op, name, fs.ErrInvalid) // 如果路径无效，返回错误信息
	}

	file, err := iofs.Afero.Open(name) // 打开文件
	if err != nil {
		logger.Error("打开文件失败:", err)
		return nil, iofs.wrapError(op, name, err) // 如果发生错误，返回错误信息
	}

	// 文件应该实现 fs.ReadDirFile 接口
	if _, ok := file.(fs.ReadDirFile); !ok {
		file = readDirFile{file} // 如果没有实现，包装为 readDirFile
	}

	return file, nil // 返回文件对象
}

// Glob 根据模式匹配文件
// 参数：
//   - pattern: string 匹配模式
//
// 返回值：
//   - []string: 匹配的文件列表
//   - error: 错误信息
func (iofs IOFS) Glob(pattern string) ([]string, error) {
	const op = "glob" // 操作名称，用于错误包装

	// afero.Glob 不执行此检查，但对于实现是必需的
	if _, err := path.Match(pattern, ""); err != nil {
		logger.Error("匹配模式无效:", err)
		return nil, iofs.wrapError(op, pattern, err) // 如果模式无效，返回错误信息
	}

	items, err := Glob(iofs.Afero, pattern) // 调用 Glob 函数匹配文件
	if err != nil {
		logger.Error("执行Glob操作失败:", err)
		return nil, iofs.wrapError(op, pattern, err) // 如果发生错误，返回错误信息
	}

	return items, nil // 返回匹配的文件列表
}

// ReadDir 读取目录中的内容
// 参数：
//   - name: string 目录名
//
// 返回值：
//   - []fs.DirEntry: 目录条目列表
//   - error: 错误信息
func (iofs IOFS) ReadDir(name string) ([]fs.DirEntry, error) {
	f, err := iofs.Afero.Open(name) // 打开目录
	if err != nil {
		logger.Error("打开目录失败:", err)
		return nil, iofs.wrapError("readdir", name, err) // 如果发生错误，返回错误信息
	}

	defer f.Close() // 确保在函数返回时关闭文件

	if rdf, ok := f.(fs.ReadDirFile); ok { // 检查文件是否实现了 fs.ReadDirFile 接口
		items, err := rdf.ReadDir(-1) // 读取目录中的所有条目
		if err != nil {
			logger.Error("读取目录条目失败:", err)
			return nil, iofs.wrapError("readdir", name, err) // 如果发生错误，返回错误信息
		}

		sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() }) // 对条目按名称排序
		return items, nil                                                                   // 返回排序后的条目列表
	}

	items, err := f.Readdir(-1) // 读取目录中的所有条目
	if err != nil {
		logger.Error("读取目录条目失败:", err)
		return nil, iofs.wrapError("readdir", name, err) // 如果发生错误，返回错误信息
	}

	sort.Sort(byName(items)) // 对条目按名称排序

	ret := make([]fs.DirEntry, len(items))
	for i := range items {
		ret[i] = common.FileInfoDirEntry{FileInfo: items[i]} // 将 FileInfo 转换为 DirEntry
	}

	return ret, nil // 返回转换后的条目列表
}

// ReadFile 读取指定名称的文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - []byte: 文件内容
//   - error: 错误信息
func (iofs IOFS) ReadFile(name string) ([]byte, error) {
	const op = "readfile" // 操作名称，用于错误包装

	if !fs.ValidPath(name) {
		return nil, iofs.wrapError(op, name, fs.ErrInvalid) // 如果路径无效，返回错误信息
	}

	bytes, err := ReadFile(iofs.Afero, name) // 调用 ReadFile 函数读取文件内容
	if err != nil {
		logger.Error("读取文件失败:", err)
		return nil, iofs.wrapError(op, name, err) // 如果发生错误，返回错误信息
	}

	return bytes, nil // 返回文件内容
}

// Sub 返回子文件系统
// 参数：
//   - dir: string 子目录
//
// 返回值：
//   - fs.FS: 子文件系统
//   - error: 错误信息
func (iofs IOFS) Sub(dir string) (fs.FS, error) {
	return IOFS{NewBasePathFs(iofs.Afero, dir)}, nil // 创建并返回新的子文件系统
}

// wrapError 包装错误信息
// 参数：
//   - op: string 操作名称
//   - path: string 路径
//   - err: error 错误信息
//
// 返回值：
//   - error: 包装后的错误信息
func (IOFS) wrapError(op, path string, err error) error {
	if _, ok := err.(*fs.PathError); ok {
		return err // 如果错误已经是 fs.PathError 类型，不需要再次包装
	}

	return &fs.PathError{
		Op:   op,   // 操作名称
		Path: path, // 路径
		Err:  err,  // 错误信息
	}
}

// readDirFile 结构体提供了从 afero.File 到 fs.ReadDirFile 的适配器，
// 这是为了正确实现 Open 方法所需的
type readDirFile struct {
	File // 内嵌的 afero.File 接口
}

// 确保 readDirFile 实现了 fs.ReadDirFile 接口
var _ fs.ReadDirFile = readDirFile{}

// ReadDir 读取目录中的条目
// 参数：
//   - n: int 要读取的条目数
//
// 返回值：
//   - []fs.DirEntry: 目录条目列表
//   - error: 错误信息
func (r readDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	items, err := r.File.Readdir(n) // 调用 Readdir 方法读取目录中的条目
	if err != nil {
		logger.Error("读取目录条目失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	ret := make([]fs.DirEntry, len(items)) // 创建返回结果的切片
	for i := range items {
		ret[i] = common.FileInfoDirEntry{FileInfo: items[i]} // 将 FileInfo 转换为 DirEntry
	}

	return ret, nil // 返回目录条目列表
}

// FromIOFS 采用 io/fs.FS 并将其作为 afero.Fs 使用
// 注意 io/fs.FS 是只读的，因此所有变更方法将返回 fs.PathError 和 fs.ErrPermission 错误
// 要存储修改，可以使用 afero.CopyOnWriteFs
type FromIOFS struct {
	fs.FS // 内嵌的 io/fs.FS 接口
}

// 确保 FromIOFS 实现了 afero.Fs 接口
var _ Afero = FromIOFS{}

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (f FromIOFS) Create(name string) (File, error) {
	return nil, notImplemented("create", name) // 返回未实现的错误
}

// Mkdir 创建目录
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) Mkdir(name string, perm os.FileMode) error {
	return notImplemented("mkdir", name) // 返回未实现的错误
}

// MkdirAll 创建目录及其所有父目录
// 参数：
//   - path: string 路径名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) MkdirAll(path string, perm os.FileMode) error {
	return notImplemented("mkdirall", path) // 返回未实现的错误
}

// Open 打开指定名称的文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (f FromIOFS) Open(name string) (File, error) {
	file, err := f.FS.Open(name) // 调用嵌入的 io/fs.FS 的 Open 方法
	if err != nil {
		return nil, err // 如果发生错误，返回错误信息
	}

	return fromIOFSFile{File: file, name: name}, nil // 返回 fromIOFSFile 对象
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
func (f FromIOFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return f.Open(name) // 调用 Open 方法
}

// Remove 删除指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) Remove(name string) error {
	return notImplemented("remove", name) // 返回未实现的错误
}

// RemoveAll 删除指定路径及其包含的所有子目录
// 参数：
//   - path: string 路径名
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) RemoveAll(path string) error {
	return notImplemented("removeall", path) // 返回未实现的错误
}

// Rename 重命名文件
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) Rename(oldname, newname string) error {
	return notImplemented("rename", oldname) // 返回未实现的错误
}

// Stat 返回文件的信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (f FromIOFS) Stat(name string) (os.FileInfo, error) {
	return fs.Stat(f.FS, name) // 调用嵌入的 io/fs.FS 的 Stat 方法
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (f FromIOFS) Name() string {
	return "fromiofs" // 返回文件系统名称
}

// Chmod 更改文件模式
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) Chmod(name string, mode os.FileMode) error {
	return notImplemented("chmod", name) // 返回未实现的错误
}

// Chown 更改文件的所有者
// 参数：
//   - name: string 文件名
//   - uid: int 用户 ID
//   - gid: int 组 ID
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) Chown(name string, uid, gid int) error {
	return notImplemented("chown", name) // 返回未实现的错误
}

// Chtimes 更改文件的访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (f FromIOFS) Chtimes(name string, atime, mtime time.Time) error {
	return notImplemented("chtimes", name) // 返回未实现的错误
}

// fromIOFSFile 结构体提供了从 io/fs.File 到 afero.File 的适配器
type fromIOFSFile struct {
	fs.File        // 内嵌的 io/fs.File 接口
	name    string // 文件名
}

// ReadAt 从指定偏移量开始读取数据
// 参数：
//   - p: []byte 读取缓冲区
//   - off: int64 偏移量
//
// 返回值：
//   - int: 读取的字节数
//   - error: 错误信息
func (f fromIOFSFile) ReadAt(p []byte, off int64) (n int, err error) {
	readerAt, ok := f.File.(io.ReaderAt) // 检查文件是否实现了 io.ReaderAt 接口
	if !ok {
		return -1, notImplemented("readat", f.name) // 返回未实现的错误
	}

	return readerAt.ReadAt(p, off) // 调用 ReadAt 方法读取数据
}

// Seek 设置文件的偏移量
// 参数：
//   - offset: int64 偏移量
//   - whence: int 位置
//
// 返回值：
//   - int64: 新的偏移量
//   - error: 错误信息
func (f fromIOFSFile) Seek(offset int64, whence int) (int64, error) {
	seeker, ok := f.File.(io.Seeker) // 检查文件是否实现了 io.Seeker 接口
	if !ok {
		return -1, notImplemented("seek", f.name) // 返回未实现的错误
	}

	return seeker.Seek(offset, whence) // 调用 Seek 方法设置偏移量
}

// Write 写入数据到文件
// 参数：
//   - p: []byte 写入缓冲区
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f fromIOFSFile) Write(p []byte) (n int, err error) {
	return -1, notImplemented("write", f.name) // 返回未实现的错误
}

// WriteAt 从指定偏移量开始写入数据
// 参数：
//   - p: []byte 写入缓冲区
//   - off: int64 偏移量
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f fromIOFSFile) WriteAt(p []byte, off int64) (n int, err error) {
	return -1, notImplemented("writeat", f.name) // 返回未实现的错误
}

// Name 返回文件名
// 返回值：
//   - string: 文件名
func (f fromIOFSFile) Name() string {
	return f.name // 返回文件名
}

// Readdir 读取目录中的条目
// 参数：
//   - count: int 要读取的条目数
//
// 返回值：
//   - []os.FileInfo: 目录条目列表
//   - error: 错误信息
func (f fromIOFSFile) Readdir(count int) ([]os.FileInfo, error) {
	rdfile, ok := f.File.(fs.ReadDirFile) // 检查文件是否实现了 fs.ReadDirFile 接口
	if !ok {
		return nil, notImplemented("readdir", f.name) // 返回未实现的错误
	}

	entries, err := rdfile.ReadDir(count) // 调用 ReadDir 方法读取条目
	if err != nil {
		logger.Error("读取目录条目失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	ret := make([]os.FileInfo, len(entries)) // 创建返回结果的切片
	for i := range entries {
		ret[i], err = entries[i].Info() // 获取每个条目的信息

		if err != nil {
			logger.Error("获取条目信息失败:", err)
			return nil, err // 如果发生错误，返回错误信息
		}
	}

	return ret, nil // 返回目录条目列表
}

// Readdirnames 读取目录中的条目名称
// 参数：
//   - n: int 要读取的条目数
//
// 返回值：
//   - []string: 目录条目名称列表
//   - error: 错误信息
func (f fromIOFSFile) Readdirnames(n int) ([]string, error) {
	rdfile, ok := f.File.(fs.ReadDirFile) // 检查文件是否实现了 fs.ReadDirFile 接口
	if !ok {
		return nil, notImplemented("readdir", f.name) // 返回未实现的错误
	}

	entries, err := rdfile.ReadDir(n) // 调用 ReadDir 方法读取条目
	if err != nil {
		logger.Error("读取目录条目失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	ret := make([]string, len(entries)) // 创建返回结果的切片
	for i := range entries {
		ret[i] = entries[i].Name() // 获取每个条目的名称
	}

	return ret, nil // 返回目录条目名称列表
}

// Sync 同步文件内容
// 返回值：
//   - error: 错误信息
func (f fromIOFSFile) Sync() error {
	return nil // 同步成功，返回 nil
}

// Truncate 截断文件
// 参数：
//   - size: int64 文件大小
//
// 返回值：
//   - error: 错误信息
func (f fromIOFSFile) Truncate(size int64) error {
	return notImplemented("truncate", f.name) // 返回未实现的错误
}

// WriteString 写入字符串到文件
// 参数：
//   - s: string 字符串
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f fromIOFSFile) WriteString(s string) (ret int, err error) {
	return -1, notImplemented("writestring", f.name) // 返回未实现的错误
}

// notImplemented 返回未实现的错误
// 参数：
//   - op: string 操作名称
//   - path: string 路径
//
// 返回值：
//   - error: 错误信息
func notImplemented(op, path string) error {
	return &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission} // 返回未实现的错误
}
