package afero

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/bpfs/defs/utils/logger"
)

// readDirNames 读取指定目录的内容，并返回排序后的目录条目列表。
// 参数：
//   - fs: Afero 文件系统
//   - dirname: string 目录名
//
// 返回值：
//   - []string: 排序后的目录条目名称列表
//   - error: 错误信息
func readDirNames(fs Afero, dirname string) ([]string, error) {
	f, err := fs.Open(dirname) // 打开目录
	if err != nil {
		logger.Error("打开目录失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	names, err := f.Readdirnames(-1) // 读取目录中的所有条目名称
	f.Close()                        // 关闭目录
	if err != nil {
		logger.Error("读取目录名称失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	sort.Strings(names) // 对条目名称进行排序
	return names, nil   // 返回排序后的目录条目名称列表
}

// walk 递归遍历指定路径，调用 walkFn 函数处理每个文件或目录。
// 参数：
//   - fs: Afero 文件系统
//   - path: string 路径
//   - info: os.FileInfo 文件信息
//   - walkFn: filepath.WalkFunc 处理函数
//
// 返回值：
//   - error: 错误信息
func walk(fs Afero, path string, info os.FileInfo, walkFn filepath.WalkFunc) error {
	err := walkFn(path, info, nil) // 调用处理函数处理当前路径
	if err != nil {
		if info.IsDir() && err == filepath.SkipDir {
			return nil // 如果跳过目录，返回 nil
		}
		logger.Error("处理路径失败:", err)
		return err // 返回错误信息
	}

	if !info.IsDir() {
		return nil // 如果不是目录，返回 nil
	}

	names, err := readDirNames(fs, path) // 读取目录中的所有条目名称
	if err != nil {
		logger.Error("读取目录名称失败:", err)
		return walkFn(path, info, err) // 调用处理函数处理错误
	}

	for _, name := range names { // 遍历每个条目名称
		filename := filepath.Join(path, name)          // 拼接完整路径
		fileInfo, err := lstatIfPossible(fs, filename) // 获取文件信息
		if err != nil {
			logger.Error("获取文件信息失败:", err)
			if err := walkFn(filename, fileInfo, err); err != nil && err != filepath.SkipDir {
				return err // 返回错误信息
			}
		} else {
			err = walk(fs, filename, fileInfo, walkFn) // 递归遍历子目录
			if err != nil {
				logger.Error("遍历子目录失败:", err)
				if !fileInfo.IsDir() || err != filepath.SkipDir {
					return err // 返回错误信息
				}
			}
		}
	}
	return nil // 返回 nil 表示成功
}

// lstatIfPossible 如果文件系统支持，使用 Lstat，否则使用 fs.Stat。
// 参数：
//   - fs: Afero 文件系统
//   - path: string 路径
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func lstatIfPossible(fs Afero, path string) (os.FileInfo, error) {
	if lfs, ok := fs.(Lstater); ok {
		fi, _, err := lfs.LstatIfPossible(path) // 调用 LstatIfPossible 方法
		return fi, err                          // 返回文件信息和错误信息
	}
	return fs.Stat(path) // 调用 Stat 方法
}

// Walk 遍历根目录为 root 的文件树，调用 walkFn 函数处理树中的每个文件或目录，包括根目录。
// 参数：
//   - fs: Afero 文件系统
//   - root: string 根目录
//   - walkFn: filepath.WalkFunc 处理函数
//
// 返回值：
//   - error: 错误信息
func Walk(fs Afero, root string, walkFn filepath.WalkFunc) error {
	info, err := lstatIfPossible(fs, root) // 获取根目录的文件信息
	if err != nil {
		logger.Error("获取根目录信息失败:", err)
		return walkFn(root, nil, err) // 调用处理函数处理错误
	}
	return walk(fs, root, info, walkFn) // 递归遍历文件树
}
