package gcsfs

import (
	"context"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/bpfs/defs/afero"

	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
	"google.golang.org/api/option"
)

// GcsFs 结构体包装了一个 GCS 文件系统的源
type GcsFs struct {
	source *Fs // GCS 文件系统的源
}

// NewGcsFS 创建一个 GCS 文件系统，自动实例化和装饰存储客户端。
// 可以提供额外的选项传递给客户端创建，如 cloud.google.com/go/storage 文档所述
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - opts: 可选的客户端选项，用于配置存储客户端
//
// 返回：
//   - afero.Afero: 创建的 GCS 文件系统
//   - error: 可能出现的错误
func NewGcsFS(ctx context.Context, opts ...option.ClientOption) (afero.Afero, error) {
	// 检查环境变量中是否有 GOOGLE_APPLICATION_CREDENTIALS_JSON
	if json := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS_JSON"); json != "" {
		// 使用凭证 JSON 创建选项
		opts = append(opts, option.WithCredentialsJSON([]byte(json)))
	}

	// 创建存储客户端
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		// 记录创建客户端时的错误
		logger.Error("创建存储客户端失败", "错误", err)
		return nil, err
	}

	// 从客户端创建 GCS 文件系统
	return NewGcsFSFromClient(ctx, client)
}

// NewGcsFSWithSeparator 与 NewGcsFS 类似，但文件系统将使用提供的文件夹分隔符
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - folderSeparator: 文件夹分隔符
//   - opts: 可选的客户端选项，用于配置存储客户端
//
// 返回：
//   - afero.Afero: 创建的 GCS 文件系统
//   - error: 可能出现的错误
func NewGcsFSWithSeparator(ctx context.Context, folderSeparator string, opts ...option.ClientOption) (afero.Afero, error) {
	// 创建存储客户端
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		// 记录创建客户端时的错误
		logger.Error("创建存储客户端失败", "错误", err)
		return nil, err
	}

	// 从客户端创建 GCS 文件系统，并使用提供的文件夹分隔符
	return NewGcsFSFromClientWithSeparator(ctx, client, folderSeparator)
}

// NewGcsFSFromClient 从给定的存储客户端创建 GCS 文件系统
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - client: 已创建的存储客户端
//
// 返回：
//   - afero.Afero: 创建的 GCS 文件系统
//   - error: 可能出现的错误
func NewGcsFSFromClient(ctx context.Context, client *storage.Client) (afero.Afero, error) {
	// 适配客户端
	c := stiface.AdaptClient(client)

	// 创建 GcsFs 实例并返回
	return &GcsFs{NewGcsFs(ctx, c)}, nil
}

// NewGcsFSFromClientWithSeparator 与 NewGcsFSFromClient 类似，但文件系统将使用提供的文件夹分隔符
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - client: 已创建的存储客户端
//   - folderSeparator: 文件夹分隔符
//
// 返回：
//   - afero.Afero: 创建的 GCS 文件系统
//   - error: 可能出现的错误
func NewGcsFSFromClientWithSeparator(ctx context.Context, client *storage.Client, folderSeparator string) (afero.Afero, error) {
	// 适配客户端
	c := stiface.AdaptClient(client)

	// 创建 GcsFs 实例并返回
	return &GcsFs{NewGcsFsWithSeparator(ctx, c, folderSeparator)}, nil
}

// GcsFs 包装了一些 gcs.GcsFs 并将一些返回类型转换为 afero 接口。
// Name 返回文件系统的名称
// 返回：
//   - string: 文件系统的名称
func (fs *GcsFs) Name() string {
	// 调用源文件系统的 Name 方法
	return fs.source.Name()
}

// Create 创建一个新的文件
// 参数：
//   - name: 文件名
//
// 返回：
//   - afero.File: 创建的文件
//   - error: 可能出现的错误
func (fs *GcsFs) Create(name string) (afero.File, error) {
	// 调用源文件系统的 Create 方法
	file, err := fs.source.Create(name)
	if err != nil {
		logger.Error("创建文件失败", "文件名", name, "错误", err)
	}
	return file, err
}

// Mkdir 创建一个新的目录
// 参数：
//   - name: 目录名
//   - perm: 权限
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) Mkdir(name string, perm os.FileMode) error {
	// 调用源文件系统的 Mkdir 方法
	err := fs.source.Mkdir(name, perm)
	if err != nil {
		logger.Error("创建目录失败", "目录名", name, "错误", err)
	}
	return err
}

// MkdirAll 创建一个新的目录，包括所有必要的父目录
// 参数：
//   - path: 目录路径
//   - perm: 权限
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) MkdirAll(path string, perm os.FileMode) error {
	// 调用源文件系统的 MkdirAll 方法
	err := fs.source.MkdirAll(path, perm)
	if err != nil {
		logger.Error("创建目录及其父目录失败", "路径", path, "错误", err)
	}
	return err
}

// Open 打开一个文件
// 参数：
//   - name: 文件名
//
// 返回：
//   - afero.File: 打开的文件
//   - error: 可能出现的错误
func (fs *GcsFs) Open(name string) (afero.File, error) {
	// 调用源文件系统的 Open 方法
	file, err := fs.source.Open(name)
	if err != nil {
		logger.Error("打开文件失败", "文件名", name, "错误", err)
	}
	return file, err
}

// OpenFile 以指定的模式和权限打开一个文件
// 参数：
//   - name: 文件名
//   - flag: 模式标志
//   - perm: 权限
//
// 返回：
//   - afero.File: 打开的文件
//   - error: 可能出现的错误
func (fs *GcsFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	// 调用源文件系统的 OpenFile 方法
	file, err := fs.source.OpenFile(name, flag, perm)
	if err != nil {
		logger.Error("打开文件失败", "文件名", name, "错误", err)
	}
	return file, err
}

// Remove 删除一个文件
// 参数：
//   - name: 文件名
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) Remove(name string) error {
	// 调用源文件系统的 Remove 方法
	err := fs.source.Remove(name)
	if err != nil {
		logger.Error("删除文件失败", "文件名", name, "错误", err)
	}
	return err
}

// RemoveAll 删除一个目录及其包含的所有内容
// 参数：
//   - path: 目录路径
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) RemoveAll(path string) error {
	// 调用源文件系统的 RemoveAll 方法
	err := fs.source.RemoveAll(path)
	if err != nil {
		logger.Error("删除目录及其内容失败", "路径", path, "错误", err)
	}
	return err
}

// Rename 重命名一个文件或目录
// 参数：
//   - oldname: 旧名称
//   - newname: 新名称
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) Rename(oldname, newname string) error {
	// 调用源文件系统的 Rename 方法
	err := fs.source.Rename(oldname, newname)
	if err != nil {
		logger.Error("重命名失败", "旧名称", oldname, "新名称", newname, "错误", err)
	}
	return err
}

// Stat 获取文件信息
// 参数：
//   - name: 文件名
//
// 返回：
//   - os.FileInfo: 文件信息
//   - error: 可能出现的错误
func (fs *GcsFs) Stat(name string) (os.FileInfo, error) {
	// 调用源文件系统的 Stat 方法
	info, err := fs.source.Stat(name)
	if err != nil {
		logger.Error("获取文件信息失败", "文件名", name, "错误", err)
	}
	return info, err
}

// Chmod 修改文件权限
// 参数：
//   - name: 文件名
//   - mode: 新的文件权限
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) Chmod(name string, mode os.FileMode) error {
	// 调用源文件系统的 Chmod 方法
	err := fs.source.Chmod(name, mode)
	if err != nil {
		logger.Error("修改文件权限失败", "文件名", name, "错误", err)
	}
	return err
}

// Chtimes 修改文件的访问和修改时间
// 参数：
//   - name: 文件名
//   - atime: 新的访问时间
//   - mtime: 新的修改时间
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	// 调用源文件系统的 Chtimes 方法
	err := fs.source.Chtimes(name, atime, mtime)
	if err != nil {
		logger.Error("修改文件时间失败", "文件名", name, "错误", err)
	}
	return err
}

// Chown 修改文件的所有者和所有组
// 参数：
//   - name: 文件名
//   - uid: 新的用户 ID
//   - gid: 新的组 ID
//
// 返回：
//   - error: 可能出现的错误
func (fs *GcsFs) Chown(name string, uid, gid int) error {
	// 调用源文件系统的 Chown 方法
	err := fs.source.Chown(name, uid, gid)
	if err != nil {
		logger.Error("修改文件所有者失败", "文件名", name, "错误", err)
	}
	return err
}
