package gcsfs

import (
	"context"
	"io"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/bpfs/defs/afero"

	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
	"google.golang.org/api/iterator"
)

// normSeparators 将文件系统分隔符设置为测试中期望（硬编码）的分隔符
// 参数：
//   - s: 需要标准化的字符串
//
// 返回：
//   - string: 标准化后的字符串，所有反斜杠替换为正斜杠
func normSeparators(s string) string {
	return strings.Replace(s, "\\", "/", -1)
}

// clientMock 模拟客户端结构体
type clientMock struct {
	stiface.Client             // 嵌入 stiface.Client 接口
	fs             afero.Afero // 使用 Afero 内存文件系统
}

// newClientMock 创建一个新的模拟客户端
// 返回：
//   - *clientMock: 新的模拟客户端实例
func newClientMock() *clientMock {
	return &clientMock{fs: afero.NewMemMapFs()}
}

// Bucket 返回模拟的存储桶句柄
// 参数：
//   - name: 存储桶名称
//
// 返回：
//   - stiface.BucketHandle: 模拟的存储桶句柄
func (m *clientMock) Bucket(name string) stiface.BucketHandle {
	return &bucketMock{bucketName: name, fs: m.fs}
}

// bucketMock 模拟存储桶句柄结构体
type bucketMock struct {
	stiface.BucketHandle             // 嵌入 stiface.BucketHandle 接口
	bucketName           string      // 存储桶名称
	fs                   afero.Afero // 使用 Afero 内存文件系统
}

// Attrs 返回存储桶属性
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - *storage.BucketAttrs: 存储桶属性
//   - error: 可能出现的错误
func (m *bucketMock) Attrs(context.Context) (*storage.BucketAttrs, error) {
	return &storage.BucketAttrs{}, nil
}

// Object 返回模拟的对象句柄
// 参数：
//   - name: 对象名称
//
// 返回：
//   - stiface.ObjectHandle: 模拟的对象句柄
func (m *bucketMock) Object(name string) stiface.ObjectHandle {
	return &objectMock{name: name, fs: m.fs}
}

// Objects 返回模拟的对象迭代器
// 参数：
//   - ctx: 上下文
//   - q: 查询条件
//
// 返回：
//   - stiface.ObjectIterator: 模拟的对象迭代器
func (m *bucketMock) Objects(_ context.Context, q *storage.Query) (it stiface.ObjectIterator) {
	return &objectItMock{name: q.Prefix, fs: m.fs}
}

// objectMock 模拟对象句柄结构体
type objectMock struct {
	stiface.ObjectHandle             // 嵌入 stiface.ObjectHandle 接口
	name                 string      // 对象名称
	fs                   afero.Afero // 使用 Afero 内存文件系统
}

// NewWriter 返回模拟的写入器
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - stiface.Writer: 模拟的写入器
func (o *objectMock) NewWriter(_ context.Context) stiface.Writer {
	return &writerMock{name: o.name, fs: o.fs}
}

// NewRangeReader 返回模拟的范围读取器
// 参数：
//   - ctx: 上下文
//   - offset: 偏移量
//   - length: 读取长度
//
// 返回：
//   - stiface.Reader: 模拟的读取器
//   - error: 可能出现的错误
func (o *objectMock) NewRangeReader(_ context.Context, offset, length int64) (stiface.Reader, error) {
	if o.name == "" {
		logger.Error("对象名称为空")
		return nil, ErrEmptyObjectName
	}

	// 打开文件
	file, err := o.fs.Open(o.name)
	if err != nil {
		logger.Errorf("打开文件失败: %v", err)
		return nil, err
	}

	// 如果偏移量大于0，则进行定位
	if offset > 0 {
		_, err = file.Seek(offset, io.SeekStart)
		if err != nil {
			logger.Errorf("文件定位失败: %v", err)
			return nil, err
		}
	}

	// 创建读取器
	res := &readerMock{file: file}
	if length > -1 {
		res.buf = make([]byte, length)
		_, err = file.Read(res.buf)
		if err != nil {
			logger.Errorf("读取文件失败: %v", err)
			return nil, err
		}
	}

	return res, nil
}

// Delete 删除对象
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - error: 可能出现的错误
func (o *objectMock) Delete(_ context.Context) error {
	if o.name == "" {
		logger.Error("对象名称为空")
		return ErrEmptyObjectName
	}
	err := o.fs.Remove(o.name)
	if err != nil {
		logger.Errorf("删除对象失败: %v", err)
	}
	return err
}

// Attrs 返回对象属性
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - *storage.ObjectAttrs: 对象属性
//   - error: 可能出现的错误
func (o *objectMock) Attrs(_ context.Context) (*storage.ObjectAttrs, error) {
	if o.name == "" {
		logger.Error("对象名称为空")
		return nil, ErrEmptyObjectName
	}

	// 获取文件信息
	info, err := o.fs.Stat(o.name)
	if err != nil {
		pathError, ok := err.(*os.PathError)
		if ok {
			if pathError.Err == os.ErrNotExist {
				logger.Error("对象不存在")
				return nil, storage.ErrObjectNotExist
			}
		}

		logger.Errorf("获取文件信息失败: %v", err)
		return nil, err
	}

	// 创建并返回对象属性
	res := &storage.ObjectAttrs{Name: normSeparators(o.name), Size: info.Size(), Updated: info.ModTime()}

	if info.IsDir() {
		// 如果是目录，则返回错误
		logger.Error("对象是一个目录")
		return nil, ErrObjectDoesNotExist
	}

	return res, nil
}

// writerMock 模拟写入器结构体
type writerMock struct {
	stiface.Writer // 嵌入 stiface.Writer 接口

	name string      // 对象名称
	fs   afero.Afero // 使用 Afero 内存文件系统

	file afero.File // 文件句柄
}

// Write 写入数据到模拟对象
// 参数：
//   - p: 要写入的数据
//
// 返回：
//   - int: 写入的字节数
//   - error: 可能出现的错误
func (w *writerMock) Write(p []byte) (n int, err error) {
	if w.name == "" {
		logger.Error("对象名称为空")
		return 0, ErrEmptyObjectName
	}

	// 如果文件句柄为空，则创建文件
	if w.file == nil {
		w.file, err = w.fs.Create(w.name)
		if err != nil {
			logger.Errorf("创建文件失败: %v", err)
			return 0, err
		}
	}

	// 写入数据到文件
	n, err = w.file.Write(p)
	if err != nil {
		logger.Errorf("写入数据失败: %v", err)
	}
	return n, err
}

// Close 关闭模拟写入器
// 返回：
//   - error: 可能出现的错误
func (w *writerMock) Close() error {
	if w.name == "" {
		logger.Error("对象名称为空")
		return ErrEmptyObjectName
	}
	// 如果文件句柄为空，处理特殊情况
	if w.file == nil {
		var err error
		if strings.HasSuffix(w.name, "/") {
			err = w.fs.Mkdir(w.name, 0o755)
			if err != nil {
				logger.Errorf("创建目录失败: %v", err)
				return err
			}
		} else {
			_, err = w.Write([]byte{})
			if err != nil {
				logger.Errorf("写入空数据失败: %v", err)
				return err
			}
		}
	}
	// 关闭文件句柄
	if w.file != nil {
		err := w.file.Close()
		if err != nil {
			logger.Errorf("关闭文件失败: %v", err)
		}
		return err
	}
	return nil
}

// readerMock 模拟读取器结构体
type readerMock struct {
	stiface.Reader // 嵌入 stiface.Reader 接口

	file afero.File // 文件句柄

	buf []byte // 缓冲区
}

// Remain 返回剩余数据长度
// 返回：
//   - int64: 剩余数据长度
func (r *readerMock) Remain() int64 {
	return 0
}

// Read 从模拟对象中读取数据
// 参数：
//   - p: 用于存储读取数据的缓冲区
//
// 返回：
//   - int: 读取的字节数
//   - error: 可能出现的错误
func (r *readerMock) Read(p []byte) (int, error) {
	// 如果缓冲区不为空，从缓冲区读取数据
	if r.buf != nil {
		copy(p, r.buf)
		return len(r.buf), nil
	}
	// 从文件读取数据
	n, err := r.file.Read(p)
	if err != nil {
		logger.Errorf("读取数据失败: %v", err)
	}
	return n, err
}

// Close 关闭模拟读取器
// 返回：
//   - error: 可能出现的错误
func (r *readerMock) Close() error {
	err := r.file.Close()
	if err != nil {
		logger.Errorf("关闭文件失败: %v", err)
	}
	return err
}

// objectItMock 模拟对象迭代器结构体
type objectItMock struct {
	stiface.ObjectIterator // 嵌入 stiface.ObjectIterator 接口

	name string      // 对象名称
	fs   afero.Afero // 使用 Afero 内存文件系统

	dir   afero.File             // 目录句柄
	infos []*storage.ObjectAttrs // 对象属性信息切片
}

// Next 返回下一个对象属性
// 返回：
//   - *storage.ObjectAttrs: 下一个对象属性
//   - error: 可能出现的错误
func (it *objectItMock) Next() (*storage.ObjectAttrs, error) {
	var err error
	// 如果目录句柄为空，则打开目录
	if it.dir == nil {
		it.dir, err = it.fs.Open(it.name)
		if err != nil {
			logger.Errorf("打开目录失败: %v", err)
			return nil, err
		}

		var isDir bool
		isDir, err = afero.IsDir(it.fs, it.name)
		if err != nil {
			logger.Errorf("判断是否为目录失败: %v", err)
			return nil, err
		}

		it.infos = []*storage.ObjectAttrs{}

		// 如果不是目录，则获取文件信息
		if !isDir {
			var info os.FileInfo
			info, err = it.dir.Stat()
			if err != nil {
				logger.Errorf("获取文件信息失败: %v", err)
				return nil, err
			}
			it.infos = append(it.infos, &storage.ObjectAttrs{Name: normSeparators(info.Name()), Size: info.Size(), Updated: info.ModTime()})
		} else {
			var fInfos []os.FileInfo
			fInfos, err = it.dir.Readdir(0)
			if err != nil {
				logger.Errorf("读取目录内容失败: %v", err)
				return nil, err
			}
			// 如果是目录，则添加前缀
			if it.name != "" {
				it.infos = append(it.infos, &storage.ObjectAttrs{
					Prefix: normSeparators(it.name) + "/",
				})
			}

			// 添加文件信息
			for _, info := range fInfos {
				it.infos = append(it.infos, &storage.ObjectAttrs{Name: normSeparators(info.Name()), Size: info.Size(), Updated: info.ModTime()})
			}
		}
	}

	// 如果没有更多信息，返回迭代完成错误
	if len(it.infos) == 0 {
		return nil, iterator.Done
	}

	// 返回下一个对象属性
	res := it.infos[0]
	it.infos = it.infos[1:]

	return res, err
}
