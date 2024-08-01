package gcsfs

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

const (
	folderSize = 42 // 目录的大小常量
)

// FileInfo 结构体用于保存文件信息
type FileInfo struct {
	name     string      // 文件名称
	size     int64       // 文件大小
	updated  time.Time   // 文件更新时间
	isDir    bool        // 是否为目录
	fileMode os.FileMode // 文件模式
}

// newFileInfo 创建一个新的 FileInfo 对象
// 参数：
//   - name: 文件名称
//   - fs: 文件系统实例
//   - fileMode: 文件模式
//
// 返回：
//   - *FileInfo: 创建的 FileInfo 对象
//   - error: 可能出现的错误
func newFileInfo(name string, fs *Fs, fileMode os.FileMode) (*FileInfo, error) {
	res := &FileInfo{
		name:     name,
		size:     folderSize,
		updated:  time.Time{},
		isDir:    false,
		fileMode: fileMode,
	}

	// 获取对象信息
	obj, err := fs.getObj(name)
	if err != nil {
		return nil, err
	}

	// 获取对象属性
	objAttrs, err := obj.Attrs(fs.ctx)
	if err != nil {
		if err.Error() == ErrEmptyObjectName.Error() {
			// 如果是根目录，立即返回
			res.name = fs.ensureTrailingSeparator(res.name)
			res.isDir = true
			return res, nil
		} else if err.Error() == ErrObjectDoesNotExist.Error() {
			// GCloud 中的文件夹实际上不存在，需要检查是否有前缀的对象存在
			bucketName, bucketPath := fs.splitName(name)
			it := fs.client.Bucket(bucketName).Objects(
				fs.ctx, &storage.Query{Delimiter: fs.separator, Prefix: bucketPath, Versions: false})
			if _, err = it.Next(); err == nil {
				res.name = fs.ensureTrailingSeparator(res.name)
				res.isDir = true
				return res, nil
			}

			return nil, ErrFileNotFound
		}
		return nil, err
	}

	res.size = objAttrs.Size
	res.updated = objAttrs.Updated

	return res, nil
}

// newFileInfoFromAttrs 从对象属性创建 FileInfo 对象
// 参数：
//   - objAttrs: 对象属性
//   - fileMode: 文件模式
//
// 返回：
//   - *FileInfo: 创建的 FileInfo 对象
func newFileInfoFromAttrs(objAttrs *storage.ObjectAttrs, fileMode os.FileMode) *FileInfo {
	res := &FileInfo{
		name:     objAttrs.Name,
		size:     objAttrs.Size,
		updated:  objAttrs.Updated,
		isDir:    false,
		fileMode: fileMode,
	}

	// 如果对象没有名称但有前缀，表示这是一个虚拟文件夹
	if res.name == "" {
		if objAttrs.Prefix != "" {
			res.name = objAttrs.Prefix
			res.size = folderSize
			res.isDir = true
		}
	}

	return res
}

// Name 返回文件名称
// 返回：
//   - string: 文件名称
func (fi *FileInfo) Name() string {
	return filepath.Base(filepath.FromSlash(fi.name))
}

// Size 返回文件大小
// 返回：
//   - int64: 文件大小
func (fi *FileInfo) Size() int64 {
	return fi.size
}

// Mode 返回文件模式
// 返回：
//   - os.FileMode: 文件模式
func (fi *FileInfo) Mode() os.FileMode {
	if fi.IsDir() {
		return os.ModeDir | fi.fileMode
	}
	return fi.fileMode
}

// ModTime 返回文件修改时间
// 返回：
//   - time.Time: 文件修改时间
func (fi *FileInfo) ModTime() time.Time {
	return fi.updated
}

// IsDir 判断是否为目录
// 返回：
//   - bool: 是否为目录
func (fi *FileInfo) IsDir() bool {
	return fi.isDir
}

// Sys 返回文件系统信息
// 返回：
//   - interface{}: 文件系统信息
func (fi *FileInfo) Sys() interface{} {
	return nil
}

// ByName 实现对 FileInfo 切片的排序
type ByName []*FileInfo

// Len 返回切片长度
// 返回：
//   - int: 切片长度
func (a ByName) Len() int { return len(a) }

// Swap 交换切片中两个元素的位置
// 参数：
//   - i: 第一个元素索引
//   - j: 第二个元素索引
func (a ByName) Swap(i, j int) {
	a[i].name, a[j].name = a[j].name, a[i].name
	a[i].size, a[j].size = a[j].size, a[i].size
	a[i].updated, a[j].updated = a[j].updated, a[i].updated
	a[i].isDir, a[j].isDir = a[j].isDir, a[i].isDir
}

// Less 比较切片中两个元素的名称
// 参数：
//   - i: 第一个元素索引
//   - j: 第二个元素索引
//
// 返回：
//   - bool: 第一个元素名称是否小于第二个元素名称
func (a ByName) Less(i, j int) bool { return strings.Compare(a[i].Name(), a[j].Name()) == -1 }
