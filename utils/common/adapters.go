package common

import "io/fs"

// FileInfoDirEntry 提供了一个从 os.FileInfo 到 fs.DirEntry 的适配器
type FileInfoDirEntry struct {
	fs.FileInfo // 嵌入的 fs.FileInfo 接口
}

// 确保 FileInfoDirEntry 实现了 fs.DirEntry 接口
var _ fs.DirEntry = FileInfoDirEntry{}

// Type 返回文件模式类型，实现 fs.DirEntry 接口的 Type 方法
// 返回：
//   - fs.FileMode 文件模式类型
func (d FileInfoDirEntry) Type() fs.FileMode {
	return d.FileInfo.Mode().Type() // 返回嵌入的 FileInfo 的模式类型
}

// Info 返回文件信息，实现 fs.DirEntry 接口的 Info 方法
// 返回：
//   - fs.FileInfo 文件信息
//   - error 错误信息，如果有
func (d FileInfoDirEntry) Info() (fs.FileInfo, error) {
	return d.FileInfo, nil // 返回嵌入的 FileInfo 和 nil 错误
}
