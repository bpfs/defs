package afero

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// FilePathSeparator 是文件路径分隔符，由 os.Separator 定义。
const FilePathSeparator = string(filepath.Separator)

// WriteReader 将读取器的内容写入文件
// 参数：
//   - fs: Afero 文件系统
//   - path: string 文件路径
//   - r: io.Reader 读取器
//
// 返回值：
//   - error: 错误信息
func WriteReader(fs Afero, path string, r io.Reader) (err error) {
	dir, _ := filepath.Split(path) // 分割路径，获取目录部分
	ospath := filepath.FromSlash(dir)

	if ospath != "" {
		err = fs.MkdirAll(ospath, 0o777) // 创建所有目录
		if err != nil {
			if err != os.ErrExist {
				return err // 返回错误信息
			}
		}
	}

	file, err := fs.Create(path) // 创建文件
	if err != nil {
		logger.Error("应用选项失败:", err)
		return
	}
	defer file.Close()

	_, err = io.Copy(file, r) // 将读取器内容复制到文件
	return
}

// SafeWriteReader 将读取器的内容安全地写入文件，检查文件是否已存在
// 参数：
//   - fs: Afero 文件系统
//   - path: string 文件路径
//   - r: io.Reader 读取器
//
// 返回值：
//   - error: 错误信息
func SafeWriteReader(fs Afero, path string, r io.Reader) (err error) {
	dir, _ := filepath.Split(path) // 分割路径，获取目录部分
	ospath := filepath.FromSlash(dir)

	if ospath != "" {
		err = fs.MkdirAll(ospath, 0o777) // 创建所有目录
		if err != nil {
			logger.Error("应用选项失败:", err)
			return // 返回错误信息
		}
	}

	exists, err := Exists(fs, path) // 检查文件是否存在
	if err != nil {
		logger.Error("应用选项失败:", err)
		return // 返回错误信息
	}
	if exists {
		return fmt.Errorf("%v already exists", path) // 返回文件已存在错误
	}

	file, err := fs.Create(path) // 创建文件
	if err != nil {
		logger.Error("应用选项失败:", err)
		return
	}
	defer file.Close()

	_, err = io.Copy(file, r) // 将读取器内容复制到文件
	return
}

// GetTempDir 返回默认的临时目录，并带有尾部斜杠
// 如果 subPath 不为空，则递归创建该路径，权限为 777 (rwx rwx rwx)
// 参数：
//   - fs: Afero 文件系统
//   - subPath: string 子路径
//
// 返回值：
//   - string: 临时目录路径
func GetTempDir(fs Afero, subPath string) string {
	addSlash := func(p string) string {
		if FilePathSeparator != p[len(p)-1:] {
			p = p + FilePathSeparator // 添加尾部斜杠
		}
		return p
	}
	dir := addSlash(os.TempDir()) // 获取系统临时目录

	if subPath != "" {
		// 保留 Windows 反斜杠
		if FilePathSeparator == "\\" {
			subPath = strings.Replace(subPath, "\\", "____", -1)
		}
		dir = dir + UnicodeSanitize((subPath)) // 处理子路径中的 Unicode 字符
		if FilePathSeparator == "\\" {
			dir = strings.Replace(dir, "____", "\\", -1)
		}

		if exists, _ := Exists(fs, dir); exists {
			return addSlash(dir) // 如果路径存在，返回添加尾部斜杠后的路径
		}

		err := fs.MkdirAll(dir, 0o777) // 创建所有目录
		if err != nil {
			logger.Error("应用选项失败:", err)
			panic(err) // 如果创建目录失败，触发恐慌
		}
		dir = addSlash(dir) // 添加尾部斜杠
	}
	return dir // 返回临时目录路径
}

// UnicodeSanitize 清理字符串，移除非标准路径字符
// 参数：
//   - s: string 输入字符串
//
// 返回值：
//   - string: 清理后的字符串
func UnicodeSanitize(s string) string {
	source := []rune(s)                    // 将字符串转换为rune切片
	target := make([]rune, 0, len(source)) // 创建目标rune切片

	// 遍历source中的每个字符
	for _, r := range source {
		// 如果字符是字母、数字、标记或其他允许的字符
		if unicode.IsLetter(r) ||
			unicode.IsDigit(r) ||
			unicode.IsMark(r) ||
			r == '.' ||
			r == '/' ||
			r == '\\' ||
			r == '_' ||
			r == '-' ||
			r == '%' ||
			r == ' ' ||
			r == '#' {
			target = append(target, r) // 将字符添加到目标切片中
		}
	}

	return string(target) // 将目标切片转换为字符串并返回
}

// NeuterAccents 转换带有重音符号的字符为普通形式
// 参数：
//   - s: string 输入字符串
//
// 返回值：
//   - string: 转换后的字符串
func NeuterAccents(s string) string {
	// 创建一个字符转换链，将重音符号移除
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	// 将字符串进行转换
	result, _, _ := transform.String(t, string(s))

	return result // 返回转换后的字符串
}

// FileContainsBytes 检查文件是否包含指定的字节切片
// 参数：
//   - fs: Afero 文件系统
//   - filename: string 文件名
//   - subslice: []byte 字节切片
//
// 返回值：
//   - bool: 是否包含
//   - error: 错误信息
func FileContainsBytes(fs Afero, filename string, subslice []byte) (bool, error) {
	f, err := fs.Open(filename) // 打开文件
	if err != nil {
		logger.Error("应用选项失败:", err)
		return false, err // 返回错误信息
	}

	defer f.Close()

	return readerContainsAny(f, subslice), nil // 检查文件内容是否包含字节切片
}

// FileContainsAnyBytes 检查文件是否包含任意一个指定的字节切片
// 参数：
//   - fs: Afero 文件系统
//   - filename: string 文件名
//   - subslices: [][]byte 字节切片数组
//
// 返回值：
//   - bool: 是否包含
//   - error: 错误信息
func FileContainsAnyBytes(fs Afero, filename string, subslices [][]byte) (bool, error) {
	f, err := fs.Open(filename) // 打开文件
	if err != nil {
		logger.Error("应用选项失败:", err)
		return false, err // 返回错误信息
	}

	defer f.Close()

	return readerContainsAny(f, subslices...), nil // 检查文件内容是否包含任意一个字节切片
}

// readerContainsAny 检查读取器中是否包含任意一个指定的字节切片
// 参数：
//   - r: io.Reader 读取器
//   - subslices: ...[]byte 字节切片数组
//
// 返回值：
//   - bool: 是否包含
func readerContainsAny(r io.Reader, subslices ...[]byte) bool {
	if r == nil || len(subslices) == 0 {
		return false // 如果读取器为空或字节切片数组为空，返回false
	}

	largestSlice := 0 // 最大字节切片长度

	// 获取最大的字节切片长度
	for _, sl := range subslices {
		if len(sl) > largestSlice {
			largestSlice = len(sl)
		}
	}

	if largestSlice == 0 {
		return false // 如果最大字节切片长度为0，返回false
	}

	bufflen := largestSlice * 4   // 缓冲区长度
	halflen := bufflen / 2        // 半缓冲区长度
	buff := make([]byte, bufflen) // 创建缓冲区
	var err error
	var n, i int

	for {
		i++
		if i == 1 {
			n, err = io.ReadAtLeast(r, buff[:halflen], halflen) // 读取半缓冲区长度的数据
		} else {
			if i != 2 {
				// 左移缓冲区数据以捕捉重叠匹配
				copy(buff[:], buff[halflen:])
			}
			n, err = io.ReadAtLeast(r, buff[halflen:], halflen) // 读取半缓冲区长度的数据
		}

		if n > 0 {
			// 检查缓冲区中是否包含任意一个字节切片
			for _, sl := range subslices {
				if bytes.Contains(buff, sl) {
					return true // 如果包含，返回true
				}
			}
		}

		if err != nil {
			break // 如果发生错误，跳出循环
		}
	}
	return false // 返回false
}

// DirExists 检查路径是否存在并且是一个目录。
// 参数：
//   - fs: Afero 文件系统
//   - path: string 路径
//
// 返回值：
//   - bool: 是否存在并且是目录
//   - error: 错误信息
func DirExists(fs Afero, path string) (bool, error) {
	fi, err := fs.Stat(path) // 获取文件信息
	if err == nil && fi.IsDir() {
		return true, nil // 如果没有错误且是目录，返回true
	}

	if os.IsNotExist(err) {
		return false, nil // 如果路径不存在，返回false
	}

	return false, err // 返回错误信息
}

// IsDir 检查给定路径是否是目录。
// 参数：
//   - fs: Afero 文件系统
//   - path: string 路径
//
// 返回值：
//   - bool: 是否是目录
//   - error: 错误信息
func IsDir(fs Afero, path string) (bool, error) {
	fi, err := fs.Stat(path) // 获取文件信息
	if err != nil {
		logger.Error("应用选项失败:", err)
		return false, err // 返回错误信息
	}

	return fi.IsDir(), nil // 返回是否是目录
}

// IsEmpty 检查给定文件或目录是否为空。
// 参数：
//   - fs: Afero 文件系统
//   - path: string 路径
//
// 返回值：
//   - bool: 是否为空
//   - error: 错误信息
func IsEmpty(fs Afero, path string) (bool, error) {
	if b, _ := Exists(fs, path); !b {
		return false, fmt.Errorf("%q path does not exist", path) // 路径不存在，返回错误
	}

	fi, err := fs.Stat(path) // 获取文件信息
	if err != nil {
		logger.Error("应用选项失败:", err)
		return false, err // 返回错误信息
	}

	if fi.IsDir() {
		f, err := fs.Open(path) // 打开目录
		if err != nil {
			logger.Error("应用选项失败:", err)
			return false, err // 返回错误信息
		}
		defer f.Close()

		list, err := f.Readdir(-1) // 读取目录内容
		if err != nil {
			logger.Error("应用选项失败:", err)
			return false, err // 返回错误信息
		}
		return len(list) == 0, nil // 返回目录是否为空
	}
	return fi.Size() == 0, nil // 返回文件是否为空
}

// Exists 检查文件或目录是否存在。
// 参数：
//   - fs: Afero 文件系统
//   - path: string 路径
//
// 返回值：
//   - bool: 是否存在
//   - error: 错误信息
func Exists(fs Afero, path string) (bool, error) {
	_, err := fs.Stat(path) // 获取文件信息
	if err == nil {
		return true, nil // 如果没有错误，返回true
	}
	if os.IsNotExist(err) {
		return false, nil // 如果路径不存在，返回false
	}
	return false, err // 返回错误信息
}

// FullBaseFsPath 获取完整的基础文件系统路径。
// 参数：
//   - basePathFs: *BasePathFs 基础路径文件系统
//   - relativePath: string 相对路径
//
// 返回值：
//   - string: 完整路径
func FullBaseFsPath(basePathFs *BasePathFs, relativePath string) string {
	combinedPath := filepath.Join(basePathFs.path, relativePath) // 组合路径
	if parent, ok := basePathFs.source.(*BasePathFs); ok {
		return FullBaseFsPath(parent, combinedPath) // 递归获取完整路径
	}

	return combinedPath // 返回完整路径
}
