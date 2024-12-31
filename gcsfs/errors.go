package gcsfs

import (
	"errors"
	"syscall"
)

// 定义错误变量
var (
	ErrNoBucketInName     = errors.New("名称中未找到存储桶名称") // 没有找到 bucket 名称的错误
	ErrFileClosed         = errors.New("文件已关闭")       // 文件已关闭的错误
	ErrOutOfRange         = errors.New("超出范围")        // 超出范围的错误
	ErrObjectDoesNotExist = errors.New("存储：对象不存在")    // 对象不存在的错误
	ErrEmptyObjectName    = errors.New("存储：对象名称为空")   // 对象名称为空的错误
	ErrFileNotFound       = syscall.ENOENT            // 文件未找到的错误
)
