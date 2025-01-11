package gcsfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
)

const (
	maxWriteSize = 10000 // 最大写入大小
)

// gcsFileResource 表示每个 GCS 对象的单例版本;
// Google 云存储允许用户打开多个写入器，一旦写入关闭，写入的流将被提交。
// 我们在读写同一个文件时做了一些同步底层资源的魔术操作。

type gcsFileResource struct {
	ctx context.Context // 上下文

	fs *Fs // 文件系统引用

	obj      stiface.ObjectHandle // GCS 对象句柄
	name     string               // 文件名
	fileMode os.FileMode          // 文件模式

	currentGcsSize int64          // 当前 GCS 对象大小
	offset         int64          // 偏移量
	reader         io.ReadCloser  // 读操作器
	writer         io.WriteCloser // 写操作器

	closed bool // 关闭状态
}

// Close 关闭 gcsFileResource
func (o *gcsFileResource) Close() error {
	o.closed = true
	// TODO rawGcsObjectsMap ?
	return o.maybeCloseIo()
}

// maybeCloseIo 关闭 IO 资源（读和写）
func (o *gcsFileResource) maybeCloseIo() error {
	if err := o.maybeCloseReader(); err != nil {
		logger.Error("关闭读取器时发生错误", err)
		return fmt.Errorf("关闭读取器时发生错误: %v", err)
	}
	if err := o.maybeCloseWriter(); err != nil {
		logger.Error("关闭写入器时发生错误", err)
		return fmt.Errorf("关闭写入器时发生错误: %v", err)
	}

	return nil
}

// maybeCloseReader 关闭 reader
func (o *gcsFileResource) maybeCloseReader() error {
	if o.reader == nil {
		return nil
	}
	if err := o.reader.Close(); err != nil {
		logger.Error("关闭读取器失败", err)
		return err
	}
	o.reader = nil
	return nil
}

// maybeCloseWriter 关闭 writer
func (o *gcsFileResource) maybeCloseWriter() error {
	if o.writer == nil {
		return nil
	}

	// 在部分写入的情况下（例如，写入到文件流的中间），我们需要
	// 在关闭 reader 之前追加原始文件中的任何剩余数据（并提交结果）。
	// 对于小写入，保持原始 reader 可能更有效率，但这是另一个迭代的内容
	if o.currentGcsSize > o.offset {
		currentFile, err := o.obj.NewRangeReader(o.ctx, o.offset, -1)
		if err != nil {
			logger.Error("无法模拟部分写入，关闭（因此整个文件写入）未提交到 GCS", err)
			return fmt.Errorf(
				"无法模拟部分写入，关闭（因此整个文件写入）未提交到 GCS: %v", err)
		}
		if currentFile != nil && currentFile.Remain() > 0 {
			if _, err := io.Copy(o.writer, currentFile); err != nil {
				logger.Error("写入时发生错误", err)
				return fmt.Errorf("写入时发生错误: %v", err)
			}
		}
	}

	if err := o.writer.Close(); err != nil {
		logger.Error("关闭写入器失败", err)
		return err
	}
	o.writer = nil
	return nil
}

// ReadAt 从指定偏移量读取数据
// 参数：
//   - p: 读取数据的缓冲区
//   - off: 偏移量
//
// 返回：
//   - n: 读取的字节数
//   - err: 可能出现的错误
func (o *gcsFileResource) ReadAt(p []byte, off int64) (n int, err error) {
	if cap(p) == 0 {
		return 0, nil
	}

	// 假设如果 reader 是打开的，它处于正确的偏移量
	// 一个好的性能假设，我们必须确保它成立
	if off == o.offset && o.reader != nil {
		n, err = o.reader.Read(p)
		o.offset += int64(n)
		return n, err
	}

	// 我们必须检查它是否是一个文件夹；文件夹不应有打开的 reader 或 writer，
	// 因此此检查不应被过度调用并导致性能下降
	if o.reader == nil && o.writer == nil {
		var info *FileInfo
		info, err = newFileInfo(o.name, o.fs, o.fileMode)
		if err != nil {
			logger.Error("创建文件信息时发生错误", err)
			return 0, err
		}

		if info.IsDir() {
			// 尝试读取目录必须返回此错误
			return 0, syscall.EISDIR
		}
	}

	// 如果任何 writer 已经写入任何内容；首先提交它，以便我们可以读回它。
	if err = o.maybeCloseIo(); err != nil {
		logger.Error("关闭 IO 时发生错误", err)
		return 0, err
	}

	// 然后在正确的偏移量读取。
	r, err := o.obj.NewRangeReader(o.ctx, off, -1)
	if err != nil {
		logger.Error("创建范围读取器时发生错误", err)
		return 0, err
	}
	o.reader = r
	o.offset = off

	read, err := o.reader.Read(p)
	o.offset += int64(read)
	return read, err
}

// WriteAt 在指定偏移量写入数据
// 参数：
//   - b: 写入的数据
//   - off: 偏移量
//
// 返回：
//   - n: 写入的字节数
//   - err: 可能出现的错误
func (o *gcsFileResource) WriteAt(b []byte, off int64) (n int, err error) {
	// 如果 writer 已打开且位于正确的偏移量，我们可以直接写入
	if off == o.offset && o.writer != nil {
		n, err = o.writer.Write(b)
		o.offset += int64(n)
		return n, err
	}

	// 确保 reader 必须重新打开，如果 writer 在另一个偏移量处活动，则首先提交它
	if err = o.maybeCloseIo(); err != nil {
		logger.Error("关闭 IO 时发生错误", err)
		return 0, err
	}

	w := o.obj.NewWriter(o.ctx)
	// 警告：这看起来像是一个 hack，但由于 GCS 的强一致性，它有效。
	// 我们将打开并写入同一个文件；只有当 writer 关闭时，内容才会被提交到 GCS。
	// 一般思路如下：
	// Objectv1[:offset] -> Objectv2
	// newData1 -> Objectv2
	// Objectv1[offset+len(newData1):] -> Objectv2
	// Objectv2.Close
	//
	// 这需要下载和上传原始文件，但如果我们要支持 GCS 上的 seek-write 操作，这是不可避免的。
	objAttrs, err := o.obj.Attrs(o.ctx)
	if err != nil {
		if off > 0 {
			logger.Error("写入到不存在的文件", err)
			return 0, err // 写入到一个不存在的文件
		}

		o.currentGcsSize = 0
	} else {
		o.currentGcsSize = objAttrs.Size
	}

	if off > o.currentGcsSize {
		logger.Error("偏移量超出范围", nil)
		return 0, ErrOutOfRange
	}

	if off > 0 {
		var r stiface.Reader
		r, err = o.obj.NewReader(o.ctx)
		if err != nil {
			logger.Error("创建读取器时发生错误", err)
			return 0, err
		}
		if _, err = io.CopyN(w, r, off); err != nil {
			logger.Error("复制数据时发生错误", err)
			return 0, err
		}
		if err = r.Close(); err != nil {
			logger.Error("关闭读取器时发生错误", err)
			return 0, err
		}
	}

	o.writer = w
	o.offset = off

	written, err := o.writer.Write(b)

	o.offset += int64(written)
	return written, err
}

// min 返回两个整数中的较小值
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// Truncate 截断文件到指定大小
// 参数：
//   - wantedSize: 目标大小
//
// 返回：
//   - err: 可能出现的错误
func (o *gcsFileResource) Truncate(wantedSize int64) error {
	if wantedSize < 0 {
		logger.Error("截断大小超出范围", nil)
		return ErrOutOfRange
	}

	if err := o.maybeCloseIo(); err != nil {
		logger.Error("关闭 IO 时发生错误", err)
		return err
	}

	r, err := o.obj.NewRangeReader(o.ctx, 0, wantedSize)
	if err != nil {
		logger.Error("创建范围读取器时发生错误", err)
		return err
	}

	w := o.obj.NewWriter(o.ctx)
	written, err := io.Copy(w, r)
	if err != nil {
		logger.Error("复制数据时发生错误", err)
		return err
	}

	for written < wantedSize {
		// 批量写入填充字节
		paddingBytes := bytes.Repeat([]byte(" "), min(maxWriteSize, int(wantedSize-written)))

		n := 0
		if n, err = w.Write(paddingBytes); err != nil {
			logger.Error("写入填充字节时发生错误", err)
			return err
		}

		written += int64(n)
	}
	if err = r.Close(); err != nil {
		logger.Error("关闭读取器时发生错误", err)
		return fmt.Errorf("关闭读取器时发生错误: %v", err)
	}
	if err = w.Close(); err != nil {
		logger.Error("关闭写入器时发生错误", err)
		return fmt.Errorf("关闭写入器时发生错误: %v", err)
	}
	return nil
}
