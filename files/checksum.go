// Package files 提供文件操作相关的功能
package files

import (
	"crypto/sha256"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/bpfs/defs/v2/afero"
)

// GetFileCRC32 计算文件的CRC32校验和 (*os.File版本)
// 参数:
//   - file: 需要计算校验和的文件指针
//
// 返回值:
//   - uint32: 文件的CRC32校验和
//   - error: 计算过程中的错误信息
func GetFileCRC32(file *os.File) (uint32, error) {
	// 保存当前文件指针位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	// 确保在函数返回时恢复文件指针位置
	defer file.Seek(currentPos, io.SeekStart)

	// 将文件指针移到文件开始处
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	// 创建CRC32哈希计算器
	hash := crc32.NewIEEE()
	// 将文件内容复制到哈希计算器中
	if _, err := io.Copy(hash, file); err != nil {
		return 0, err
	}

	// 返回计算得到的校验和
	return hash.Sum32(), nil
}

// GetAferoFileCRC32 计算文件的CRC32校验和 (afero.File版本)
// 参数:
//   - file: 需要计算校验和的afero.File文件接口
//
// 返回值:
//   - uint32: 文件的CRC32校验和
//   - error: 计算过程中的错误信息
func GetAferoFileCRC32(file afero.File) (uint32, error) {
	// 保存当前文件指针位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	// 确保在函数返回时恢复文件指针位置
	defer file.Seek(currentPos, io.SeekStart)

	// 将文件指针移到文件开始处
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	// 创建CRC32哈希计算器
	hash := crc32.NewIEEE()
	// 将文件内容复制到哈希计算器中
	if _, err := io.Copy(hash, file); err != nil {
		return 0, err
	}

	// 返回计算得到的校验和
	return hash.Sum32(), nil
}

// GetBytesCRC32 计算字节切片的CRC32校验和
// 参数:
//   - data: 需要计算校验和的字节切片
//
// 返回值:
//   - uint32: 字节切片的CRC32校验和
func GetBytesCRC32(data []byte) uint32 {
	// 创建CRC32哈希计算器
	hash := crc32.NewIEEE()
	// 将数据写入哈希计算器
	hash.Write(data)
	// 返回计算得到的校验和
	return hash.Sum32()
}

// GetFileSHA256 计算文件的SHA256哈希值 (*os.File版本)
// 参数:
//   - file: 需要计算哈希值的文件指针
//
// 返回值:
//   - []byte: 文件的SHA256哈希值
//   - error: 计算过程中的错误信息
func GetFileSHA256(file *os.File) ([]byte, error) {
	// 保存当前文件指针位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	// 确保在函数返回时恢复文件指针位置
	defer file.Seek(currentPos, io.SeekStart)

	// 将文件指针移到文件开始处
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// 创建SHA256哈希计算器
	hash := sha256.New()
	// 将文件内容复制到哈希计算器中
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}

	// 返回计算得到的哈希值
	return hash.Sum(nil), nil
}

// GetAferoFileSHA256 计算文件的SHA256哈希值 (afero.File版本)
// 参数:
//   - file: 需要计算哈希值的afero.File文件接口
//
// 返回值:
//   - []byte: 文件的SHA256哈希值
//   - error: 计算过程中的错误信息
func GetAferoFileSHA256(file afero.File) ([]byte, error) {
	// 保存当前文件指针位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	// 确保在函数返回时恢复文件指针位置
	defer file.Seek(currentPos, io.SeekStart)

	// 将文件指针移到文件开始处
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// 创建SHA256哈希计算器
	hash := sha256.New()
	// 将文件内容复制到哈希计算器中
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}

	// 返回计算得到的哈希值
	return hash.Sum(nil), nil
}

// GetBytesSHA256 计算字节切片的SHA256哈希值
// 参数:
//   - data: 需要计算哈希值的字节切片
//
// 返回值:
//   - []byte: 字节切片的SHA256哈希值
func GetBytesSHA256(data []byte) []byte {
	// 创建SHA256哈希计算器
	hash := sha256.New()
	// 将数据写入哈希计算器
	hash.Write(data)
	// 返回计算得到的哈希值
	return hash.Sum(nil)
}

// GetFileMIME 获取文件的MIME类型 (*os.File版本)
// 参数:
//   - file: 需要获取MIME类型的文件指针
//
// 返回值:
//   - string: 文件的MIME类型
//   - error: 获取过程中的错误信息
func GetFileMIME(file *os.File) (string, error) {
	// 保存当前文件指针位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	// 确保在函数返回时恢复文件指针位置
	defer file.Seek(currentPos, io.SeekStart)

	// 将文件指针移到文件开始处
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	// 读取文件头部512字节用于MIME类型检测
	buf := make([]byte, 512)
	_, err = file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// 返回检测到的MIME类型
	return http.DetectContentType(buf), nil
}

// GetAferoFileMIME 获取文件的MIME类型 (afero.File版本)
// 参数:
//   - file: 需要获取MIME类型的afero.File文件接口
//
// 返回值:
//   - string: 文件的MIME类型
//   - error: 获取过程中的错误信息
func GetAferoFileMIME(file afero.File) (string, error) {
	// 保存当前文件指针位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	// 确保在函数返回时恢复文件指针位置
	defer file.Seek(currentPos, io.SeekStart)

	// 将文件指针移到文件开始处
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	// 读取文件头部512字节用于MIME类型检测
	buf := make([]byte, 512)
	_, err = file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// 返回检测到的MIME类型
	return http.DetectContentType(buf), nil
}

// GetFileName 获取文件名 (*os.File版本)
// 参数:
//   - file: 需要获取文件名的文件指针
//
// 返回值:
//   - string: 文件名
//   - error: 获取过程中的错误信息
func GetFileName(file *os.File) (string, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}
	// 返回文件名
	return fileInfo.Name(), nil
}

// GetAferoFileName 获取文件名 (afero.File版本)
// 参数:
//   - file: 需要获取文件名的afero.File文件接口
//
// 返回值:
//   - string: 文件名
//   - error: 获取过程中的错误信息
func GetAferoFileName(file afero.File) (string, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}
	// 返回文件名
	return fileInfo.Name(), nil
}

// GetFileSize 获取文件大小 (*os.File版本)
// 参数:
//   - file: 需要获取大小的文件指针
//
// 返回值:
//   - int64: 文件大小(字节)
//   - error: 获取过程中的错误信息
func GetFileSize(file *os.File) (int64, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}
	// 返回文件大小
	return fileInfo.Size(), nil
}

// GetAferoFileSize 获取文件大小 (afero.File版本)
// 参数:
//   - file: 需要获取大小的afero.File文件接口
//
// 返回值:
//   - int64: 文件大小(字节)
//   - error: 获取过程中的错误信息
func GetAferoFileSize(file afero.File) (int64, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}
	// 返回文件大小
	return fileInfo.Size(), nil
}

// GetFileMode 获取文件模式和权限 (*os.File版本)
// 参数:
//   - file: 需要获取模式和权限的文件指针
//
// 返回值:
//   - os.FileMode: 文件的模式和权限
//   - error: 获取过程中的错误信息
func GetFileMode(file *os.File) (os.FileMode, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}
	// 返回文件模式和权限
	return fileInfo.Mode(), nil
}

// GetAferoFileMode 获取文件模式和权限 (afero.File版本)
// 参数:
//   - file: 需要获取模式和权限的afero.File文件接口
//
// 返回值:
//   - os.FileMode: 文件的模式和权限
//   - error: 获取过程中的错误信息
func GetAferoFileMode(file afero.File) (os.FileMode, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}
	// 返回文件模式和权限
	return fileInfo.Mode(), nil
}

// GetFileModTime 获取文件最后修改时间 (*os.File版本)
// 参数:
//   - file: 需要获取修改时间的文件指针
//
// 返回值:
//   - time.Time: 文件的最后修改时间
//   - error: 获取过程中的错误信息
func GetFileModTime(file *os.File) (time.Time, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return time.Time{}, err
	}
	// 返回文件修改时间
	return fileInfo.ModTime(), nil
}

// GetAferoFileModTime 获取文件最后修改时间 (afero.File版本)
// 参数:
//   - file: 需要获取修改时间的afero.File文件接口
//
// 返回值:
//   - time.Time: 文件的最后修改时间
//   - error: 获取过程中的错误信息
func GetAferoFileModTime(file afero.File) (time.Time, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return time.Time{}, err
	}
	// 返回文件修改时间
	return fileInfo.ModTime(), nil
}

// IsFileDir 判断是否为目录 (*os.File版本)
// 参数:
//   - file: 需要判断的文件指针
//
// 返回值:
//   - bool: 是否为目录
//   - error: 判断过程中的错误信息
func IsFileDir(file *os.File) (bool, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return false, err
	}
	// 返回是否为目录
	return fileInfo.IsDir(), nil
}

// IsAferoFileDir 判断是否为目录 (afero.File版本)
// 参数:
//   - file: 需要判断的afero.File文件接口
//
// 返回值:
//   - bool: 是否为目录
//   - error: 判断过程中的错误信息
func IsAferoFileDir(file afero.File) (bool, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return false, err
	}
	// 返回是否为目录
	return fileInfo.IsDir(), nil
}

// GetFileSys 获取底层系统特定的文件信息 (*os.File版本)
// 参数:
//   - file: 需要获取系统信息的文件指针
//
// 返回值:
//   - interface{}: 系统特定的文件信息
//   - error: 获取过程中的错误信息
func GetFileSys(file *os.File) (interface{}, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	// 返回系统特定信息
	return fileInfo.Sys(), nil
}

// GetAferoFileSys 获取底层系统特定的文件信息 (afero.File版本)
// 参数:
//   - file: 需要获取系统信息的afero.File文件接口
//
// 返回值:
//   - interface{}: 系统特定的文件信息
//   - error: 获取过程中的错误信息
func GetAferoFileSys(file afero.File) (interface{}, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	// 返回系统特定信息
	return fileInfo.Sys(), nil
}
