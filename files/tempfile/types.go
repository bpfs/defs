package tempfile

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"
)

// Config 定义临时文件管理器的配置参数
type Config struct {
	MaxFileSize     int64         // 单个文件的最大大小(字节)
	MaxFiles        int           // 允许的最大文件数量
	MinFileAge      time.Duration // 文件的最小保留时间(默认1小时)
	MaxFileAge      time.Duration // 文件的最大保留时间(默认24小时)
	CleanupInterval time.Duration // 清理检查的时间间隔
	TempDir         string        // 临时文件存储目录
}

// TempFile 表示一个临时文件实例及其状态
type TempFile struct {
	path       string        // 文件在磁盘上的路径
	size       int64         // 当前文件大小(字节)
	maxSize    int64         // 允许的最大文件大小(字节)
	bufferSize int           // 内部缓冲区大小(字节)
	lastAccess time.Time     // 最后访问时间
	refCount   int32         // 引用计数,用于跟踪使用情况
	file       *os.File      // 文件句柄
	buffer     *bytes.Buffer // 写入缓冲区
}

// FilePool 管理临时文件池
type FilePool struct {
	mu     sync.RWMutex
	pool   sync.Pool
	inUse  map[string]*TempFile
	config Config
}

// TempFileError 定义临时文件操作的错误信息
type TempFileError struct {
	Op   string // 发生错误的操作名称
	Path string // 相关的文件路径
	Err  error  // 原始错误信息
}

// Error 实现error接口,提供格式化的错误信息
func (e *TempFileError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("操作[%s]失败: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("操作[%s]失败: 文件=%s 错误=%v", e.Op, e.Path, e.Err)
}
