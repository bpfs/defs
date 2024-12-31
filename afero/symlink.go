package afero

import (
	"errors"
)

// Symlinker 是 Afero 中的一个可选接口。只有声明支持该接口的文件系统才会实现它。
// 该接口表示支持以下三个与符号链接相关的接口，这些接口实现了 os 包中的方法：
//   - Lstat
//   - Symlink
//   - Readlink
type Symlinker interface {
	Lstater    // 支持 Lstat 方法
	Linker     // 支持 Symlink 方法
	LinkReader // 支持 Readlink 方法
}

// Linker 是 Afero 中的一个可选接口。只有声明支持该接口的文件系统才会实现它。
// 如果文件系统本身是或委托给 os 文件系统，或者文件系统以其他方式支持 Symlink，
// 它将调用 Symlink 方法。
type Linker interface {
	// SymlinkIfPossible 创建符号链接（如果可能）
	// 参数：
	//   - oldname: string 旧文件名
	//   - newname: string 新文件名
	// 返回值：
	//   - error: 错误信息
	SymlinkIfPossible(oldname, newname string) error
}

// ErrNoSymlink 是当文件系统不支持符号链接时，会在 os.LinkError 中包装的错误。
// 通过支持 Linker 接口表达。
var ErrNoSymlink = errors.New("symlink not supported")

// LinkReader 是 Afero 中的一个可选接口。只有声明支持该接口的文件系统才会实现它。
// 该接口表示支持 Readlink 方法。
type LinkReader interface {
	// ReadlinkIfPossible 读取符号链接（如果可能）
	// 参数：
	//   - name: string 文件名
	// 返回值：
	//   - string: 符号链接的目标路径
	//   - error: 错误信息
	ReadlinkIfPossible(name string) (string, error)
}

// ErrNoReadlink 是当文件系统不支持读取符号链接操作时，会在 os.PathError 中包装的错误。
// 通过支持 LinkReader 接口表达。
var ErrNoReadlink = errors.New("readlink not supported")
