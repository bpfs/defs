package afero

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bpfs/defs/utils/logger"
)

// byName 实现了 sort.Interface 接口，用于根据文件名排序
type byName []os.FileInfo

// Len 返回元素的数量
func (f byName) Len() int {
	return len(f) // 返回切片的长度
}

// Less 比较两个元素的大小
func (f byName) Less(i, j int) bool {
	return f[i].Name() < f[j].Name() // 按文件名的字典顺序比较
}

// Swap 交换两个元素的位置
func (f byName) Swap(i, j int) {
	f[i], f[j] = f[j], f[i] // 交换两个元素的位置
}

// ReadDir 读取指定目录的内容并返回排序后的目录条目列表
// 参数：
//   - fs: Afero 文件系统
//   - dirname: string 目录名
//
// 返回值：
//   - []os.FileInfo: 目录条目列表
//   - error: 错误信息
func ReadDir(fs Afero, dirname string) ([]os.FileInfo, error) {
	f, err := fs.Open(dirname) // 打开目录
	if err != nil {
		logger.Error("打开目录失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	list, err := f.Readdir(-1) // 读取目录中的所有条目
	f.Close()                  // 关闭文件
	if err != nil {
		logger.Error("读取目录失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	sort.Sort(byName(list)) // 对条目列表按名称排序
	return list, nil        // 返回排序后的目录条目列表
}

// ListFileNamesRecursively 列出指定目录及其所有子目录中的文件名
// 参数：
//   - fs: Afero 文件系统
//   - rootDir: string 根目录路径
//
// 返回值：
//   - []string: 文件名列表
//   - error: 错误信息
func ListFileNamesRecursively(fs Afero, rootDir string) ([]string, error) {
	var fileList []string

	// Walk 是递归遍历 rootDir 及其子目录的所有文件和目录
	err := Walk(fs, rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error("遍历目录失败:", err)
			return err // 如果发生错误，返回错误信息
		}

		// 如果不是目录，添加文件名到列表中
		if !info.IsDir() {
			fileList = append(fileList, filepath.Base(path))
		}
		return nil
	})

	if err != nil {
		// 无法列出文件
		logger.Error("列出文件失败:", err)
		return nil, err
	}

	// 对文件名列表进行排序
	sort.Strings(fileList)

	return fileList, nil
}

// ReadFile 读取指定文件的内容并返回
// 参数：
//   - fs: Afero 文件系统
//   - filename: string 文件名
//
// 返回值：
//   - []byte: 文件内容
//   - error: 错误信息
func ReadFile(fs Afero, filename string) ([]byte, error) {
	// 打开文件
	f, err := fs.Open(filename)
	if err != nil {
		logger.Error("打开文件失败:", err)
		return nil, err
	}
	// 确保函数结束时关闭文件
	defer f.Close()

	// 使用 bytes.Buffer 来读取文件内容
	var buf bytes.Buffer

	// 使用较大的缓冲区来提高读取效率
	_, err = io.Copy(&buf, f)
	if err != nil {
		logger.Error("读取文件内容失败:", err)
		return nil, err
	}

	// 返回读取的内容
	return buf.Bytes(), nil
}

// ReadFile 读取指定文件的内容并返回
// 参数：
//   - fs: Afero 文件系统
//   - filename: string 文件名
//
// 返回值：
//   - []byte: 文件内容
//   - error: 错误信息
// func ReadFile(fs Afero, filename string) ([]byte, error) {
// 	// 打开文件
// 	f, err := fs.Open(filename)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// 确保函数结束时关闭文件
// 	defer f.Close()

// 	// 获取文件信息
// 	fi, err := f.Stat()
// 	if err != nil {
// 		return nil, err
// 	}

// 	// 获取文件大小
// 	size := fi.Size()

// 	// 如果文件小于1MB，使用readAll函数读取
// 	if size < 1024*1024 { // 1MB
// 		return readAll(f, size+bytes.MinRead)
// 	}

// 	// 尝试将文件转换为os.File类型
// 	osFile, ok := f.(*os.File)
// 	if !ok {
// 		return nil, fmt.Errorf("无法获取底层 File 接口")
// 	}

// 	// 使用内存映射读取文件
// 	mmap, err := syscall.Mmap(int(osFile.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// 确保函数结束时解除内存映射
// 	defer syscall.Munmap(mmap)

// 	// 创建一个新的字节切片来存储文件内容
// 	data := make([]byte, len(mmap))
// 	// 将内存映射的内容复制到新的切片中
// 	copy(data, mmap)

// 	// 返回文件内容和nil错误
// 	return data, nil
// }

// ReadFile 读取指定文件的内容并返回
// 参数：
//   - fs: Afero 文件系统
//   - filename: string 文件名
//
// 返回值：
//   - []byte: 文件内容
//   - error: 错误信息
// func ReadFile(fs Afero, filename string) ([]byte, error) {
// 	f, err := fs.Open(filename) // 打开文件
// 	if err != nil {
// 		logger.Error("打开文件失败:", err)
// 		return nil, err // 如果发生错误，返回错误信息
// 	}
// 	defer f.Close() // 确保在函数返回时关闭文件
// 	var n int64

// 	if fi, err := f.Stat(); err == nil {
// 		if size := fi.Size(); size < 1e9 {
// 			n = size // 预分配缓冲区的大小
// 		}
// 	}
// 	return readAll(f, n+bytes.MinRead) // 调用 readAll 函数读取文件内容
// }

// readAll 从 r 读取直到遇到错误或 EOF，并返回读取的数据
// 参数：
//   - r: io.Reader 读取器
//   - capacity: int64 初始容量
//
// 返回值：
//   - []byte: 读取的数据
//   - error: 错误信息
func readAll(r io.Reader, capacity int64) (b []byte, err error) {
	buf := bytes.NewBuffer(make([]byte, 0, capacity)) // 创建带有初始容量的缓冲区
	defer func() {
		e := recover()
		if e == nil {
			return
		}
		if panicErr, ok := e.(error); ok && panicErr == bytes.ErrTooLarge {
			err = panicErr // 如果缓冲区溢出，返回错误
			logger.Error("缓冲区溢出:", err)
		} else {
			panic(e) // 重新抛出其他 panic
		}
	}()
	_, err = buf.ReadFrom(r) // 从读取器读取数据到缓冲区
	if err != nil {
		logger.Error("读取数据失败:", err)
	}
	return buf.Bytes(), err // 返回读取的数据和错误信息
}

// ReadAll 从 r 读取直到遇到错误或 EOF，并返回读取的数据
// 参数：
//   - r: io.Reader 读取器
//
// 返回值：
//   - []byte: 读取的数据
//   - error: 错误信息
func ReadAll(r io.Reader) ([]byte, error) {
	return readAll(r, bytes.MinRead) // 调用 readAll 函数读取数据
}

// WriteFile 将数据写入指定名称的文件
// 参数：
//   - fs: Afero 文件系统
//   - filename: string 文件名
//   - data: []byte 数据
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - error: 错误信息
func WriteFile(fs Afero, filename string, data []byte, perm os.FileMode) error {
	f, err := fs.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) // 打开文件进行写入
	if err != nil {
		logger.Error("打开文件失败:", err)
		return err // 如果发生错误，返回错误信息
	}

	n, err := f.Write(data) // 将数据写入文件
	if err != nil {
		logger.Error("写入数据失败:", err)
		return err
	}
	if n < len(data) {
		err = io.ErrShortWrite // 如果写入的字节数小于数据长度，返回短写错误
		logger.Error("写入数据不完整:", err)
		return err
	}

	if err1 := f.Close(); err1 != nil {
		logger.Error("关闭文件失败:", err1)
		return err1 // 关闭文件时发生错误
	}

	return nil // 返回错误信息
}

// 随机数状态，用于生成随机的临时文件名，确保临时文件名的唯一性
var (
	randNum uint32     // 随机数种子
	randmu  sync.Mutex // 互斥锁，确保并发安全
)

// reseed 重新生成随机数种子
// 返回值：
//   - uint32: 新的随机数种子
func reseed() uint32 {
	return uint32(time.Now().UnixNano() + int64(os.Getpid())) // 使用当前时间和进程ID生成种子
}

// nextRandom 生成下一个随机数字符串
// 返回值：
//   - string: 随机数字符串
func nextRandom() string {
	randmu.Lock() // 加锁，确保并发安全
	r := randNum  // 获取当前随机数种子
	if r == 0 {
		r = reseed() // 如果种子为0，重新生成种子
	}

	r = r*1664525 + 1013904223                // 使用数值配方中的常数生成下一个随机数
	randNum = r                               // 更新随机数种子
	randmu.Unlock()                           // 解锁
	return strconv.Itoa(int(1e9 + r%1e9))[1:] // 生成随机数字符串并返回
}

// TempFile 在指定目录中创建一个新的临时文件
// 参数：
//   - fs: Afero 文件系统
//   - dir: string 目录路径
//   - pattern: string 文件名模式
//
// 返回值：
//   - File: 临时文件对象
//   - error: 错误信息
func TempFile(fs Afero, dir, pattern string) (f File, err error) {
	if dir == "" {
		dir = os.TempDir() // 如果目录为空，使用系统默认的临时目录
	}

	var prefix, suffix string
	if pos := strings.LastIndex(pattern, "*"); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:] // 如果模式中包含 "*"，分割前缀和后缀
	} else {
		prefix = pattern // 否则，整个模式作为前缀
	}

	nconflict := 0
	for i := 0; i < 10000; i++ { // 尝试最多10000次创建临时文件
		name := filepath.Join(dir, prefix+nextRandom()+suffix)             // 生成随机文件名
		f, err = fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) // 尝试创建文件
		if os.IsExist(err) {                                               // 如果文件已存在
			if nconflict++; nconflict > 10 {
				randmu.Lock()
				randNum = reseed() // 重新生成随机数种子
				randmu.Unlock()
			}
			continue // 继续尝试
		}
		if err != nil {
			logger.Error("创建临时文件失败:", err)
		}
		break // 成功创建文件，跳出循环
	}

	return // 返回文件对象和错误信息
}

// TempDir 在指定目录中创建一个新的临时目录
// 参数：
//   - fs: Afero 文件系统
//   - dir: string 目录路径
//   - prefix: string 目录名前缀
//
// 返回值：
//   - string: 目录路径
//   - error: 错误信息
func TempDir(fs Afero, dir, prefix string) (name string, err error) {
	if dir == "" {
		dir = os.TempDir() // 如果目录为空，使用系统默认的临时目录
	}

	nconflict := 0
	for i := 0; i < 10000; i++ { // 尝试最多10000次创建临时目录
		try := filepath.Join(dir, prefix+nextRandom()) // 生成随机目录名
		err = fs.Mkdir(try, 0o700)                     // 尝试创建目录
		if os.IsExist(err) {                           // 如果目录已存在
			if nconflict++; nconflict > 10 {
				randmu.Lock()
				randNum = reseed() // 重新生成随机数种子
				randmu.Unlock()
			}
			continue // 继续尝试
		}
		if err != nil {
			logger.Error("创建临时目录失败:", err)
		} else {
			name = try // 成功创建目录，设置目录路径
		}
		break // 成功创建目录或发生其他错误，跳出循环
	}

	return // 返回目录路径和错误信息
}
