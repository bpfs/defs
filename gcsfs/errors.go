package gcsfs

import (
	"errors"
	"syscall"
)

// 定义错误变量
var (
	ErrNoBucketInName     = errors.New("no bucket name found in the name") // 没有找到 bucket 名称的错误
	ErrFileClosed         = errors.New("file is closed")                   // 文件已关闭的错误
	ErrOutOfRange         = errors.New("out of range")                     // 超出范围的错误
	ErrObjectDoesNotExist = errors.New("storage: object doesn't exist")    // 对象不存在的错误
	ErrEmptyObjectName    = errors.New("storage: object name is empty")    // 对象名称为空的错误
	ErrFileNotFound       = syscall.ENOENT                                 // 文件未找到的错误
)
