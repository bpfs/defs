package gcsfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
)

const (
	defaultFileMode = 0o755   // 默认文件模式
	gsPrefix        = "gs://" // Google Cloud Storage 前缀
)

// Fs 是一个使用 Google Cloud Storage 函数的文件系统实现
type Fs struct {
	ctx       context.Context // 上下文
	client    stiface.Client  // GCS 客户端
	separator string          // 文件夹分隔符

	buckets       map[string]stiface.BucketHandle // 存储桶句柄映射
	rawGcsObjects map[string]*GcsFile             // 原始 GCS 对象映射

	autoRemoveEmptyFolders bool // 自动删除空文件夹的标志
}

// NewGcsFs 创建一个新的 GCS 文件系统
// 参数：
//   - ctx: 上下文
//   - client: GCS 客户端
//
// 返回：
//   - Fs: GCS 文件系统实例
func NewGcsFs(ctx context.Context, client stiface.Client) *Fs {
	return NewGcsFsWithSeparator(ctx, client, "/")
}

// NewGcsFsWithSeparator 创建一个带有自定义文件夹分隔符的 GCS 文件系统
// 参数：
//   - ctx: 上下文
//   - client: GCS 客户端
//   - folderSep: 文件夹分隔符
//
// 返回：
//   - Fs: GCS 文件系统实例
func NewGcsFsWithSeparator(ctx context.Context, client stiface.Client, folderSep string) *Fs {
	return &Fs{
		ctx:           ctx,
		client:        client,
		separator:     folderSep,
		rawGcsObjects: make(map[string]*GcsFile),

		autoRemoveEmptyFolders: true,
	}
}

// normSeparators 将所有 "\\" 和 "/" 规范化为提供的分隔符
// 参数：
//   - s: 输入字符串
//
// 返回：
//   - 规范化后的字符串
func (fs *Fs) normSeparators(s string) string {
	return strings.Replace(strings.Replace(s, "\\", fs.separator, -1), "/", fs.separator, -1)
}

// ensureTrailingSeparator 确保字符串以分隔符结尾
// 参数：
//   - s: 输入字符串
//
// 返回：
//   - 以分隔符结尾的字符串
func (fs *Fs) ensureTrailingSeparator(s string) string {
	if len(s) > 0 && !strings.HasSuffix(s, fs.separator) {
		return s + fs.separator
	}
	return s
}

// ensureNoLeadingSeparator 确保字符串不以分隔符开头
// 参数：
//   - s: 输入字符串
//
// 返回：
//   - 不以分隔符开头的字符串
func (fs *Fs) ensureNoLeadingSeparator(s string) string {
	if len(s) > 0 && strings.HasPrefix(s, fs.separator) {
		s = s[len(fs.separator):]
	}
	return s
}

// ensureNoPrefix 确保字符串没有前缀
// 参数：
//   - s: 输入字符串
//
// 返回：
//   - 没有前缀的字符串
func ensureNoPrefix(s string) string {
	if len(s) > 0 && strings.HasPrefix(s, gsPrefix) {
		return s[len(gsPrefix):]
	}
	return s
}

// validateName 验证名称是否合法
// 参数：
//   - s: 名称字符串
//
// 返回：
//   - 可能的错误
func validateName(s string) error {
	if len(s) == 0 {
		return ErrNoBucketInName
	}
	return nil
}

// splitName 将提供的名称拆分为桶名称和路径
// 参数：
//   - name: 名称字符串
//
// 返回：
//   - bucketName: 桶名称
//   - path: 路径
func (fs *Fs) splitName(name string) (bucketName string, path string) {
	splitName := strings.Split(name, fs.separator)
	return splitName[0], strings.Join(splitName[1:], fs.separator)
}

// getBucket 获取桶
// 参数：
//   - name: 桶名称
//
// 返回：
//   - bucket: 桶句柄
//   - err: 可能的错误
func (fs *Fs) getBucket(name string) (stiface.BucketHandle, error) {
	bucket := fs.buckets[name]
	if bucket == nil {
		bucket = fs.client.Bucket(name)
		_, err := bucket.Attrs(fs.ctx)
		if err != nil {
			return nil, err
		}
	}
	return bucket, nil
}

// getObj 获取对象
// 参数：
//   - name: 对象名称
//
// 返回：
//   - obj: 对象句柄
//   - err: 可能的错误
func (fs *Fs) getObj(name string) (stiface.ObjectHandle, error) {
	bucketName, path := fs.splitName(name)

	bucket, err := fs.getBucket(bucketName)
	if err != nil {
		return nil, err
	}

	return bucket.Object(path), nil
}

// Name 返回文件系统名称
func (fs *Fs) Name() string { return "GcsFs" }

// Create 创建文件
// 参数：
//   - name: 文件名称
//
// 返回：
//   - file: 创建的文件
//   - err: 可能的错误
func (fs *Fs) Create(name string) (*GcsFile, error) {
	name = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(name)))
	if err := validateName(name); err != nil {
		return nil, err
	}

	if !fs.autoRemoveEmptyFolders {
		baseDir := filepath.Base(name)
		if stat, err := fs.Stat(baseDir); err != nil || !stat.IsDir() {
			err = fs.MkdirAll(baseDir, 0)
			if err != nil {
				return nil, err
			}
		}
	}

	obj, err := fs.getObj(name)
	if err != nil {
		return nil, err
	}
	w := obj.NewWriter(fs.ctx)
	err = w.Close()
	if err != nil {
		return nil, err
	}
	file := NewGcsFile(fs.ctx, fs, obj, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0, name)

	fs.rawGcsObjects[name] = file
	return file, nil
}

// Mkdir 创建目录
// 参数：
//   - name: 目录名称
//   - _ : 文件模式（未使用）
//
// 返回：
//   - 可能的错误
func (fs *Fs) Mkdir(name string, _ os.FileMode) error {
	name = fs.ensureNoLeadingSeparator(fs.ensureTrailingSeparator(fs.normSeparators(ensureNoPrefix(name))))
	if err := validateName(name); err != nil {
		return err
	}
	// 目录创建逻辑需要额外检查目录名称是否存在
	bucketName, path := fs.splitName(name)
	if bucketName == "" {
		return ErrNoBucketInName
	}
	if path == "" {
		// API 会抛出 "googleapi: Error 400: No object name, required"，但这个错误更一致
		return ErrEmptyObjectName
	}

	obj, err := fs.getObj(name)
	if err != nil {
		return err
	}
	w := obj.NewWriter(fs.ctx)
	return w.Close()
}

// MkdirAll 创建多级目录
// 参数：
//   - path: 目录路径
//   - perm: 文件模式
//
// 返回：
//   - 可能的错误
func (fs *Fs) MkdirAll(path string, perm os.FileMode) error {
	path = fs.ensureNoLeadingSeparator(fs.ensureTrailingSeparator(fs.normSeparators(ensureNoPrefix(path))))
	if err := validateName(path); err != nil {
		return err
	}
	// 目录创建逻辑需要额外检查目录名称是否存在
	bucketName, splitPath := fs.splitName(path)
	if bucketName == "" {
		return ErrNoBucketInName
	}
	if splitPath == "" {
		// API 会抛出 "googleapi: Error 400: No object name, required"，但这个错误更一致
		return ErrEmptyObjectName
	}

	root := ""
	folders := strings.Split(path, fs.separator)
	for i, f := range folders {
		if f == "" && i != 0 {
			continue // 这是最后一个项目 - 它应该是空的
		}
		// 不强制前缀分隔符
		if root != "" {
			root = root + fs.separator + f
		} else {
			// 我们至少要有存储桶名称 + 目录名称才能成功创建
			root = f
			continue
		}

		if err := fs.Mkdir(root, perm); err != nil {
			return err
		}
	}
	return nil
}

// Open 打开文件
// 参数：
//   - name: 文件名称
//
// 返回：
//   - GcsFile: 打开的文件
//   - 可能的错误
func (fs *Fs) Open(name string) (*GcsFile, error) {
	return fs.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile 以指定模式打开文件
// 参数：
//   - name: 文件名称
//   - flag: 打开模式
//   - fileMode: 文件模式
//
// 返回：
//   - GcsFile: 打开的文件
//   - 可能的错误
func (fs *Fs) OpenFile(name string, flag int, fileMode os.FileMode) (*GcsFile, error) {
	var file *GcsFile
	var err error

	name = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(name)))
	if err = validateName(name); err != nil {
		return nil, err
	}

	f, found := fs.rawGcsObjects[name]
	if found {
		file = NewGcsFileFromOldFH(flag, fileMode, f.resource)
	} else {
		var obj stiface.ObjectHandle
		obj, err = fs.getObj(name)
		if err != nil {
			return nil, err
		}
		file = NewGcsFile(fs.ctx, fs, obj, flag, fileMode, name)
	}

	if flag == os.O_RDONLY {
		_, err = file.Stat()
		if err != nil {
			return nil, err
		}
	}

	if flag&os.O_TRUNC != 0 {
		err = file.resource.obj.Delete(fs.ctx)
		if err != nil {
			return nil, err
		}
		return fs.Create(name)
	}

	if flag&os.O_APPEND != 0 {
		_, err = file.Seek(0, 2)
		if err != nil {
			return nil, err
		}
	}

	if flag&os.O_CREATE != 0 {
		_, err = file.Stat()
		if err == nil { // 文件实际存在
			return nil, syscall.EPERM
		}

		_, err = file.WriteString("")
		if err != nil {
			return nil, err
		}
	}
	return file, nil
}

// Remove 删除文件或目录
// 参数：
//   - name: 文件或目录名称
//
// 返回：
//   - 可能的错误
func (fs *Fs) Remove(name string) error {
	name = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(name)))
	if err := validateName(name); err != nil {
		return err
	}

	obj, err := fs.getObj(name)
	if err != nil {
		return err
	}
	info, err := fs.Stat(name)
	if err != nil {
		return err
	}
	delete(fs.rawGcsObjects, name)

	if info.IsDir() {
		// 如果是目录，需要检查其内容 - 如果不为空，则不能删除
		var dir *GcsFile
		dir, err = fs.Open(name)
		if err != nil {
			return err
		}
		var infos []os.FileInfo
		infos, err = dir.Readdir(0)
		if err != nil {
			return err
		}
		if len(infos) > 0 {
			return syscall.ENOTEMPTY
		}

		// 这是一个空目录，可以继续删除
		name = fs.ensureTrailingSeparator(name)
		obj, err = fs.getObj(name)
		if err != nil {
			return err
		}

		return obj.Delete(fs.ctx)
	}
	return obj.Delete(fs.ctx)
}

// RemoveAll 删除指定路径及其内容
// 参数：
//   - path: 路径名称
//
// 返回：
//   - 可能的错误
func (fs *Fs) RemoveAll(path string) error {
	path = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(path)))
	if err := validateName(path); err != nil {
		return err
	}

	pathInfo, err := fs.Stat(path)
	if errors.Is(err, ErrFileNotFound) {
		// 如果文件不存在，提前返回
		return nil
	}
	if err != nil {
		return err
	}

	if !pathInfo.IsDir() {
		return fs.Remove(path)
	}

	var dir *GcsFile
	dir, err = fs.Open(path)
	if err != nil {
		return err
	}

	var infos []os.FileInfo
	infos, err = dir.Readdir(0)
	if err != nil {
		return err
	}
	for _, info := range infos {
		nameToRemove := fs.normSeparators(info.Name())
		err = fs.RemoveAll(path + fs.separator + nameToRemove)
		if err != nil {
			return err
		}
	}

	return fs.Remove(path)
}

// Rename 重命名文件或目录
// 参数：
//   - oldName: 旧名称
//   - newName: 新名称
//
// 返回：
//   - 可能的错误
func (fs *Fs) Rename(oldName, newName string) error {
	oldName = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(oldName)))
	if err := validateName(oldName); err != nil {
		return err
	}

	newName = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(newName)))
	if err := validateName(newName); err != nil {
		return err
	}

	src, err := fs.getObj(oldName)
	if err != nil {
		return err
	}
	dst, err := fs.getObj(newName)
	if err != nil {
		return err
	}

	if _, err = dst.CopierFrom(src).Run(fs.ctx); err != nil {
		return err
	}
	delete(fs.rawGcsObjects, oldName)
	return src.Delete(fs.ctx)
}

// Stat 获取文件或目录的信息
// 参数：
//   - name: 文件或目录名称
//
// 返回：
//   - os.FileInfo: 文件或目录信息
//   - 可能的错误
func (fs *Fs) Stat(name string) (os.FileInfo, error) {
	name = fs.ensureNoLeadingSeparator(fs.normSeparators(ensureNoPrefix(name)))
	if err := validateName(name); err != nil {
		return nil, err
	}

	return newFileInfo(name, fs, defaultFileMode)
}

// Chmod 修改文件或目录的权限
// 参数：
//   - name: 文件或目录名称
//   - mode: 文件模式
//
// 返回：
//   - 错误（因为此方法未实现）
func (fs *Fs) Chmod(_ string, _ os.FileMode) error {
	return errors.New("method Chmod is not implemented in GCS")
}

// Chtimes 修改文件或目录的访问时间和修改时间
// 参数：
//   - name: 文件或目录名称
//   - atime: 访问时间
//   - mtime: 修改时间
//
// 返回：
//   - 错误（因为此方法未实现）
func (fs *Fs) Chtimes(_ string, _, _ time.Time) error {
	return errors.New("method Chtimes is not implemented. Create, Delete, Updated times are read only fields in GCS and set implicitly")
}

// Chown 修改文件或目录的所有者
// 参数：
//   - name: 文件或目录名称
//   - uid: 用户ID
//   - gid: 组ID
//
// 返回：
//   - 错误（因为此方法未实现）
func (fs *Fs) Chown(_ string, _, _ int) error {
	return errors.New("method Chown is not implemented for GCS")
}
