package gcsfs

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/googleapis/google-cloud-go-testing/storage/stiface"

	"cloud.google.com/go/storage"

	"google.golang.org/api/iterator"

	"github.com/bpfs/defs/utils/logger"
)

// GcsFile 是 Afero 版本适配 GCS 的文件类型
type GcsFile struct {
	openFlags int                    // 打开文件的标志
	fhOffset  int64                  // 文件句柄特定的偏移量
	closed    bool                   // 文件是否已关闭
	ReadDirIt stiface.ObjectIterator // 目录读取迭代器
	resource  *gcsFileResource       // GCS 文件资源
}

// NewGcsFile 创建一个新的 GcsFile 实例
// 参数：
//   - ctx: 上下文对象
//   - fs: 文件系统实例
//   - obj: GCS 对象句柄
//   - openFlags: 打开文件的标志
//   - fileMode: 文件模式（暂时未使用）
//   - name: 文件名称
//
// 返回：
//   - *GcsFile: 创建的 GcsFile 实例
func NewGcsFile(
	ctx context.Context,
	fs *Fs,
	obj stiface.ObjectHandle,
	openFlags int,
	fileMode os.FileMode,
	name string,
) *GcsFile {
	return &GcsFile{
		openFlags: openFlags,
		fhOffset:  0,
		closed:    false,
		ReadDirIt: nil,
		resource: &gcsFileResource{
			ctx: ctx,
			fs:  fs,

			obj:      obj,
			name:     name,
			fileMode: fileMode,

			currentGcsSize: 0,

			offset: 0,
			reader: nil,
			writer: nil,
		},
	}
}

// NewGcsFileFromOldFH 从旧的文件句柄资源创建一个新的 GcsFile 实例
// 参数：
//   - openFlags: 打开文件的标志
//   - fileMode: 文件模式
//   - oldFile: 旧的 GCS 文件资源
//
// 返回：
//   - *GcsFile: 创建的 GcsFile 实例
func NewGcsFileFromOldFH(
	openFlags int,
	fileMode os.FileMode,
	oldFile *gcsFileResource,
) *GcsFile {
	res := &GcsFile{
		openFlags: openFlags,
		fhOffset:  0,
		closed:    false,
		ReadDirIt: nil,

		resource: oldFile,
	}
	res.resource.fileMode = fileMode

	return res
}

// Close 关闭文件
// 返回：
//   - error: 可能出现的错误
func (o *GcsFile) Close() error {
	if o.closed {
		// Afero 规范期望在已关闭文件上调用 Close 返回错误
		logger.Error("尝试关闭已关闭的文件")
		return ErrFileClosed
	}
	o.closed = true
	return o.resource.Close()
}

// Seek 设置文件偏移量
// 参数：
//   - newOffset: 新的偏移量
//   - whence: 偏移量的基准位置
//
// 返回：
//   - int64: 新的文件偏移量
//   - error: 可能出现的错误
func (o *GcsFile) Seek(newOffset int64, whence int) (int64, error) {
	if o.closed {
		logger.Error("尝试在已关闭的文件上执行 Seek 操作")
		return 0, ErrFileClosed
	}

	// 检查是否需要进行 Seek 操作
	if (whence == 0 && newOffset == o.fhOffset) || (whence == 1 && newOffset == 0) {
		return o.fhOffset, nil
	}
	log.Printf("警告：触发了 Seek 行为，效率极低。Seek 前的偏移量为 %d\n", o.fhOffset)

	// 重新打开读写器（在正确的偏移量处）
	err := o.Sync()
	if err != nil {
		logger.Error("Seek 操作中同步文件失败", "错误", err)
		return 0, err
	}
	stat, err := o.Stat()
	if err != nil {
		logger.Error("Seek 操作中获取文件状态失败", "错误", err)
		return 0, nil
	}

	switch whence {
	case 0:
		o.fhOffset = newOffset
	case 1:
		o.fhOffset += newOffset
	case 2:
		o.fhOffset = stat.Size() + newOffset
	}
	return o.fhOffset, nil
}

// Read 从文件中读取数据到给定的字节切片中
// 参数：
//   - p: 字节切片
//
// 返回：
//   - int: 读取的字节数
//   - error: 可能出现的错误
func (o *GcsFile) Read(p []byte) (n int, err error) {
	return o.ReadAt(p, o.fhOffset)
}

// ReadAt 从文件的指定偏移量处读取数据到给定的字节切片中
// 参数：
//   - p: 字节切片
//   - off: 偏移量
//
// 返回：
//   - int: 读取的字节数
//   - error: 可能出现的错误
func (o *GcsFile) ReadAt(p []byte, off int64) (n int, err error) {
	if o.closed {
		logger.Error("尝试从已关闭的文件中读取数据")
		return 0, ErrFileClosed
	}

	read, err := o.resource.ReadAt(p, off)
	o.fhOffset += int64(read)
	return read, err
}

// Write 将给定的字节切片写入文件
// 参数：
//   - p: 字节切片
//
// 返回：
//   - int: 写入的字节数
//   - error: 可能出现的错误
func (o *GcsFile) Write(p []byte) (n int, err error) {
	return o.WriteAt(p, o.fhOffset)
}

// WriteAt 将给定的字节切片写入文件的指定偏移量处
// 参数：
//   - b: 字节切片
//   - off: 偏移量
//
// 返回：
//   - int: 写入的字节数
//   - error: 可能出现的错误
func (o *GcsFile) WriteAt(b []byte, off int64) (n int, err error) {
	if o.closed {
		logger.Error("尝试向已关闭的文件写入数据")
		return 0, ErrFileClosed
	}

	if o.openFlags&os.O_RDONLY != 0 {
		logger.Error("尝试向只读文件写入数据")
		return 0, fmt.Errorf("文件以只读模式打开")
	}

	_, err = o.resource.obj.Attrs(o.resource.ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			if o.openFlags&os.O_CREATE == 0 {
				logger.Error("尝试写入不存在的文件，且未设置创建标志")
				return 0, ErrFileNotFound
			}
		} else {
			logger.Error("获取文件属性时出错", "错误", err)
			return 0, fmt.Errorf("获取文件属性时出错：%v", err)
		}
	}

	written, err := o.resource.WriteAt(b, off)
	o.fhOffset += int64(written) // 更新文件句柄偏移量
	return written, err
}

// Name 返回文件的名称
func (o *GcsFile) Name() string {
	return filepath.FromSlash(o.resource.name)
}

// readdirImpl 实现目录读取的内部逻辑
// 参数：
//   - count: 要读取的目录项数
//
// 返回：
//   - []*FileInfo: 读取的文件信息切片
//   - error: 可能出现的错误
func (o *GcsFile) readdirImpl(count int) ([]*FileInfo, error) {
	err := o.Sync()
	if err != nil {
		logger.Error("同步文件失败", "错误", err)
		return nil, err
	}

	var ownInfo os.FileInfo
	ownInfo, err = o.Stat()
	if err != nil {
		logger.Error("获取文件状态失败", "错误", err)
		return nil, err
	}

	if !ownInfo.IsDir() {
		logger.Error("尝试读取非目录文件的目录内容")
		return nil, syscall.ENOTDIR
	}

	path := o.resource.fs.ensureTrailingSeparator(o.resource.name)
	if o.ReadDirIt == nil {
		bucketName, bucketPath := o.resource.fs.splitName(path)

		o.ReadDirIt = o.resource.fs.client.Bucket(bucketName).Objects(
			o.resource.ctx, &storage.Query{Delimiter: o.resource.fs.separator, Prefix: bucketPath, Versions: false})
	}
	var res []*FileInfo
	for {
		object, err := o.ReadDirIt.Next()
		if err == iterator.Done {
			// 重置迭代器
			o.ReadDirIt = nil

			if len(res) > 0 || count <= 0 {
				return res, nil
			}

			return res, io.EOF // 读取完成，返回 EOF
		}
		if err != nil {
			logger.Error("读取目录项时出错", "错误", err)
			return res, err
		}

		tmp := newFileInfoFromAttrs(object, o.resource.fileMode)

		if tmp.Name() == "" {
			// object.Name 和 object.Prefix 都不存在，跳过此项
			continue
		}

		if object.Name == "" && object.Prefix == "" {
			continue
		}

		if tmp.Name() == ownInfo.Name() {
			// 跳过与自身同名的项
			continue
		}

		res = append(res, tmp)
	}
}

// Readdir 读取目录中的目录项
// 参数：
//   - count: 要读取的目录项数
//
// 返回：
//   - []os.FileInfo: 读取的文件信息切片
//   - error: 可能出现的错误
func (o *GcsFile) Readdir(count int) ([]os.FileInfo, error) {
	fi, err := o.readdirImpl(count)
	if len(fi) > 0 {
		sort.Sort(ByName(fi)) // 按名称排序
	}

	if count > 0 {
		fi = fi[:count] // 截取前 count 个目录项
	}

	var res []os.FileInfo
	for _, f := range fi {
		res = append(res, f)
	}
	return res, err
}

// Readdirnames 读取目录中的目录项名称
// 参数：
//   - n: 要读取的目录项数
//
// 返回：
//   - []string: 读取的目录项名称切片
//   - error: 可能出现的错误
func (o *GcsFile) Readdirnames(n int) ([]string, error) {
	fi, err := o.Readdir(n)
	if err != nil && err != io.EOF {
		logger.Error("读取目录名称失败", "错误", err)
		return nil, err
	}
	names := make([]string, len(fi))

	for i, f := range fi {
		names[i] = f.Name()
	}
	return names, err
}

// Stat 返回文件的信息
// 返回：
//   - os.FileInfo: 文件信息
//   - error: 可能出现的错误
func (o *GcsFile) Stat() (os.FileInfo, error) {
	err := o.Sync()
	if err != nil {
		logger.Error("同步文件失败", "错误", err)
		return nil, err
	}

	return newFileInfo(o.resource.name, o.resource.fs, o.resource.fileMode)
}

// Sync 同步文件的状态
// 返回：
//   - error: 可能出现的错误
func (o *GcsFile) Sync() error {
	return o.resource.maybeCloseIo()
}

// Truncate 截断文件到指定大小
// 参数：
//   - wantedSize: 目标大小
//
// 返回：
//   - error: 可能出现的错误
func (o *GcsFile) Truncate(wantedSize int64) error {
	if o.closed {
		logger.Error("尝试截断已关闭的文件")
		return ErrFileClosed
	}
	if o.openFlags == os.O_RDONLY {
		logger.Error("尝试截断只读文件")
		return fmt.Errorf("文件以只读模式打开")
	}
	return o.resource.Truncate(wantedSize)
}

// WriteString 将字符串写入文件
// 参数：
//   - s: 字符串
//
// 返回：
//   - int: 写入的字节数
//   - error: 可能出现的错误
func (o *GcsFile) WriteString(s string) (ret int, err error) {
	return o.Write([]byte(s))
}
