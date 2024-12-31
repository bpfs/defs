package afero

import (
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/bpfs/defs/utils/logger"
)

// UnionFile 实现了 afero.File 接口，当读取目录时（至少在覆盖层中存在）或打开文件进行写入时返回该接口。
//
// 对 Readdir() 和 Readdirnames() 的调用将合并基础层和覆盖层中的文件 os.FileInfo / 名称 -
// 对于同时存在于两个层中的文件，只使用覆盖层中的文件。
//
// 当打开文件进行写入（使用正确标志的 Create() / OpenFile()）时，操作将在两个层中进行，
// 从覆盖层开始。覆盖层中的成功读取将按读取的字节数移动基础层中的光标位置。
type UnionFile struct {
	Base   File          // 基础层文件
	Layer  File          // 覆盖层文件
	Merger DirsMerger    // 目录合并器
	off    int           // 偏移量
	files  []os.FileInfo // 文件信息列表
}

// Close 关闭文件
// 返回值：
//   - error: 错误信息
func (f *UnionFile) Close() error {
	// 先关闭基础层文件，以便在覆盖层中有一个更新的时间戳。如果先关闭覆盖层文件，
	// 下一次访问此文件时将获得 cacheStale -> 缓存将无效 ;-)
	if f.Base != nil {
		f.Base.Close()
	}
	if f.Layer != nil {
		return f.Layer.Close()
	}
	return BADFD // 返回错误
}

// Read 读取文件内容
// 参数：
//   - s: []byte 缓冲区
//
// 返回值：
//   - int: 读取的字节数
//   - error: 错误信息
func (f *UnionFile) Read(s []byte) (int, error) {
	if f.Layer != nil {
		n, err := f.Layer.Read(s) // 从覆盖层读取
		if (err == nil || err == io.EOF) && f.Base != nil {
			// 也在基础层中前进文件位置，下一次调用可能是当前位置的写入（或 SEEK_CUR 的 seek）
			if _, seekErr := f.Base.Seek(int64(n), io.SeekCurrent); seekErr != nil {
				// 只有在 seek 失败的情况下覆盖错误：需要向调用者报告一个可能的 io.EOF
				err = seekErr
			}
		}
		return n, err // 返回读取的字节数和错误信息
	}

	if f.Base != nil {
		return f.Base.Read(s) // 从基础层读取
	}

	return 0, BADFD // 返回错误
}

// ReadAt 从指定偏移量读取文件内容
// 参数：
//   - s: []byte 缓冲区
//   - o: int64 偏移量
//
// 返回值：
//   - int: 读取的字节数
//   - error: 错误信息
func (f *UnionFile) ReadAt(s []byte, o int64) (int, error) {
	if f.Layer != nil {
		n, err := f.Layer.ReadAt(s, o) // 从覆盖层读取
		if (err == nil || err == io.EOF) && f.Base != nil {
			_, err = f.Base.Seek(o+int64(n), io.SeekStart) // 在基础层中 seek
		}
		return n, err // 返回读取的字节数和错误信息
	}

	if f.Base != nil {
		return f.Base.ReadAt(s, o) // 从基础层读取
	}

	return 0, BADFD // 返回错误
}

// Seek 设置文件指针的位置
// 参数：
//   - o: int64 偏移量
//   - w: int 起始位置
//
// 返回值：
//   - int64: 新的文件指针位置
//   - error: 错误信息
func (f *UnionFile) Seek(o int64, w int) (pos int64, err error) {
	if f.Layer != nil {
		pos, err = f.Layer.Seek(o, w) // 在覆盖层中 seek
		if (err == nil || err == io.EOF) && f.Base != nil {
			_, err = f.Base.Seek(o, w) // 在基础层中 seek
		}
		return pos, err // 返回新的文件指针位置和错误信息
	}

	if f.Base != nil {
		return f.Base.Seek(o, w) // 在基础层中 seek
	}

	return 0, BADFD // 返回错误
}

// Write 写入文件内容
// 参数：
//   - s: []byte 缓冲区
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *UnionFile) Write(s []byte) (n int, err error) {
	if f.Layer != nil {
		n, err = f.Layer.Write(s) // 在覆盖层中写入
		if err == nil && f.Base != nil {
			_, err = f.Base.Write(s) // 在基础层中写入
		}
		return n, err // 返回写入的字节数和错误信息
	}

	if f.Base != nil {
		return f.Base.Write(s) // 在基础层中写入
	}

	return 0, BADFD // 返回错误
}

// WriteAt 从指定偏移量写入文件内容
// 参数：
//   - s: []byte 缓冲区
//   - o: int64 偏移量
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *UnionFile) WriteAt(s []byte, o int64) (n int, err error) {
	if f.Layer != nil {
		n, err = f.Layer.WriteAt(s, o) // 在覆盖层中写入
		if err == nil && f.Base != nil {
			_, err = f.Base.WriteAt(s, o) // 在基础层中写入
		}
		return n, err // 返回写入的字节数和错误信息
	}

	if f.Base != nil {
		return f.Base.WriteAt(s, o) // 在基础层中写入
	}

	return 0, BADFD // 返回错误
}

// Name 返回文件名
// 返回值：
//   - string: 文件名
func (f *UnionFile) Name() string {
	if f.Layer != nil {
		return f.Layer.Name() // 返回覆盖层文件名
	}

	return f.Base.Name() // 返回基础层文件名
}

// DirsMerger 是 UnionFile 如何将两个目录组合在一起的方法。
// 它获取来自覆盖层和基础层的 FileInfo 切片，并返回一个单一视图。
type DirsMerger func(lofi, bofi []os.FileInfo) ([]os.FileInfo, error)

// defaultUnionMergeDirsFn 是默认的目录合并函数
// 参数：
//   - lofi: []os.FileInfo 覆盖层文件信息
//   - bofi: []os.FileInfo 基础层文件信息
//
// 返回值：
//   - []os.FileInfo: 合并后的文件信息
//   - error: 错误信息
var defaultUnionMergeDirsFn = func(lofi, bofi []os.FileInfo) ([]os.FileInfo, error) {
	files := make(map[string]os.FileInfo) // 创建文件信息的映射

	for _, fi := range lofi {
		files[fi.Name()] = fi // 将覆盖层文件信息添加到映射中
	}

	for _, fi := range bofi {
		if _, exists := files[fi.Name()]; !exists {
			files[fi.Name()] = fi // 仅在覆盖层不存在时将基础层文件信息添加到映射中
		}
	}

	rfi := make([]os.FileInfo, len(files)) // 创建合并后的文件信息切片

	i := 0
	for _, fi := range files {
		rfi[i] = fi // 将文件信息添加到切片中
		i++
	}

	return rfi, nil // 返回合并后的文件信息
}

// Readdir 将两个目录组合在一起并返回覆盖目录的单一视图。
// 在目录视图的末尾，如果 c > 0，错误为 io.EOF。
// 参数：
//   - c: int 要读取的文件数
//
// 返回值：
//   - []os.FileInfo: 文件信息列表
//   - error: 错误信息
func (f *UnionFile) Readdir(c int) (ofi []os.FileInfo, err error) {
	var merge DirsMerger = f.Merger
	if merge == nil {
		merge = defaultUnionMergeDirsFn // 使用默认的目录合并函数
	}

	if f.off == 0 {
		var lfi []os.FileInfo
		if f.Layer != nil {
			lfi, err = f.Layer.Readdir(-1) // 从覆盖层读取所有文件信息
			if err != nil {
				return nil, err // 返回错误信息
			}
		}

		var bfi []os.FileInfo
		if f.Base != nil {
			bfi, err = f.Base.Readdir(-1) // 从基础层读取所有文件信息
			if err != nil {
				return nil, err // 返回错误信息
			}
		}

		merged, err := merge(lfi, bfi) // 合并文件信息
		if err != nil {
			return nil, err // 返回错误信息
		}
		f.files = append(f.files, merged...) // 将合并后的文件信息添加到 files 列表中
	}
	files := f.files[f.off:] // 获取偏移量后的文件信息

	if c <= 0 {
		return files, nil // 返回所有文件信息
	}

	if len(files) == 0 {
		return nil, io.EOF // 返回 EOF 错误
	}

	if c > len(files) {
		c = len(files) // 如果 c 大于文件数量，调整为文件数量
	}

	defer func() { f.off += c }() // 更新偏移量
	return files[:c], nil         // 返回指定数量的文件信息
}

// Readdirnames 返回目录中所有文件的名称列表
// 参数：
//   - c: int 要读取的文件数
//
// 返回值：
//   - []string: 文件名称列表
//   - error: 错误信息
func (f *UnionFile) Readdirnames(c int) ([]string, error) {
	rfi, err := f.Readdir(c) // 调用 Readdir 方法获取文件信息
	if err != nil {
		return nil, err // 返回错误信息
	}

	var names []string
	for _, fi := range rfi {
		names = append(names, fi.Name()) // 获取文件名并添加到名称列表
	}

	return names, nil // 返回文件名称列表
}

// Stat 返回文件的信息
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (f *UnionFile) Stat() (os.FileInfo, error) {
	if f.Layer != nil {
		return f.Layer.Stat() // 获取覆盖层文件的信息
	}

	if f.Base != nil {
		return f.Base.Stat() // 获取基础层文件的信息
	}

	return nil, BADFD // 返回错误
}

// Sync 同步文件内容到存储
// 返回值：
//   - error: 错误信息
func (f *UnionFile) Sync() (err error) {
	if f.Layer != nil {
		err = f.Layer.Sync() // 同步覆盖层文件内容
		if err == nil && f.Base != nil {
			err = f.Base.Sync() // 同步基础层文件内容
		}
		return err // 返回错误信息
	}

	if f.Base != nil {
		return f.Base.Sync() // 同步基础层文件内容
	}

	return BADFD // 返回错误
}

// Truncate 截断文件到指定大小
// 参数：
//   - s: int64 文件大小
//
// 返回值：
//   - error: 错误信息
func (f *UnionFile) Truncate(s int64) (err error) {
	if f.Layer != nil {
		err = f.Layer.Truncate(s) // 截断覆盖层文件
		if err == nil && f.Base != nil {
			err = f.Base.Truncate(s) // 截断基础层文件
		}
		return err // 返回错误信息
	}

	if f.Base != nil {
		return f.Base.Truncate(s) // 截断基础层文件
	}

	return BADFD // 返回错误
}

// WriteString 写入字符串到文件
// 参数：
//   - s: string 要写入的字符串
//
// 返回值：
//   - int: 写入的字节数
//   - error: 错误信息
func (f *UnionFile) WriteString(s string) (n int, err error) {
	if f.Layer != nil {
		n, err = f.Layer.WriteString(s) // 写入字符串到覆盖层文件
		if err == nil && f.Base != nil {
			_, err = f.Base.WriteString(s) // 写入字符串到基础层文件
		}
		return n, err // 返回写入的字节数和错误信息
	}

	if f.Base != nil {
		return f.Base.WriteString(s) // 写入字符串到基础层文件
	}

	return 0, BADFD // 返回错误
}

// copyFile 将文件从基础层复制到覆盖层
// 参数：
//   - base: Afero 基础文件系统
//   - layer: Afero 覆盖层文件系统
//   - name: string 文件名
//   - bfh: File 基础层文件句柄
//
// 返回值：
//   - error: 错误信息
func copyFile(layer Afero, name string, bfh File) error {
	// func copyFile(base Afero, layer Afero, name string, bfh File) error {
	// 首先确保目录存在
	exists, err := Exists(layer, filepath.Dir(name))
	if err != nil {
		return err // 返回错误信息
	}

	if !exists {
		err = layer.MkdirAll(filepath.Dir(name), 0o777) // 创建目录
		if err != nil {
			return err // 返回错误信息
		}
	}

	// 在覆盖层创建文件
	lfh, err := layer.Create(name)
	if err != nil {
		return err // 返回错误信息
	}

	n, err := io.Copy(lfh, bfh) // 复制文件内容
	if err != nil {
		logger.Error("复制文件失败:", err)
		// 如果出现错误，清理文件
		layer.Remove(name)
		lfh.Close()
		return err // 返回错误信息
	}

	bfi, err := bfh.Stat() // 获取基础层文件信息
	if err != nil || bfi.Size() != n {
		layer.Remove(name)
		lfh.Close()
		return syscall.EIO // 返回 I/O 错误
	}

	err = lfh.Close() // 关闭覆盖层文件
	if err != nil {
		logger.Error("关闭文件失败:", err)
		layer.Remove(name)
		lfh.Close()
		return err // 返回错误信息
	}
	return layer.Chtimes(name, bfi.ModTime(), bfi.ModTime()) // 修改文件时间
}

// copyToLayer 将文件从基础层复制到覆盖层
// 参数：
//   - base: Afero 基础文件系统
//   - layer: Afero 覆盖层文件系统
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func copyToLayer(base Afero, layer Afero, name string) error {
	bfh, err := base.Open(name) // 打开基础层文件
	if err != nil {
		logger.Error("打开基础层文件失败:", err)
		return err // 返回错误信息
	}

	defer bfh.Close()

	return copyFile(layer, name, bfh) // 调用 copyFile 进行文件复制
}

// copyFileToLayer 将文件从基础层复制到覆盖层，并指定标志和权限
// 参数：
//   - base: Afero 基础文件系统
//   - layer: Afero 覆盖层文件系统
//   - name: string 文件名
//   - flag: int 文件标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - error: 错误信息
func copyFileToLayer(base Afero, layer Afero, name string, flag int, perm os.FileMode) error {
	bfh, err := base.OpenFile(name, flag, perm) // 打开基础层文件
	if err != nil {
		logger.Error("打开基础层文件失败:", err)
		return err // 返回错误信息
	}
	defer bfh.Close()

	return copyFile(layer, name, bfh) // 调用 copyFile 进行文件复制
}
