package afero

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bpfs/defs/utils/logger"
)

// Glob 返回所有与模式匹配的文件名，如果没有匹配的文件则返回 nil。
// 模式的语法与 Match 中的相同。模式可以描述分层名称，例如 /usr/*/bin/ed
// （假设路径分隔符是 '/'）。
//
// Glob 忽略读取目录时的文件系统错误，如 I/O 错误。
// 唯一可能返回的错误是 ErrBadPattern，当模式格式错误时。
//
// 该函数改编自 http://golang.org/pkg/path/filepath 并使用了该包中的多个内建函数。
func Glob(fs Afero, pattern string) (matches []string, err error) {
	if !hasMeta(pattern) {
		// Lstat 不支持所有文件系统。
		if _, err = lstatIfPossible(fs, pattern); err != nil {
			logger.Error("Lstat操作失败:", err)
			return nil, nil // 如果文件不存在，返回 nil
		}
		return []string{pattern}, nil // 返回匹配的文件名
	}

	dir, file := filepath.Split(pattern) // 分割目录和文件名
	switch dir {
	case "":
		dir = "."
	case string(filepath.Separator):
		// nothing
	default:
		dir = dir[0 : len(dir)-1] // 去掉末尾的分隔符
	}

	if !hasMeta(dir) {
		return glob(fs, dir, file, nil) // 递归调用 glob 函数
	}

	var m []string
	m, err = Glob(fs, dir) // 递归调用 Glob 函数
	if err != nil {
		return
	}
	for _, d := range m {
		matches, err = glob(fs, d, file, matches) // 搜索匹配的文件
		if err != nil {
			return
		}
	}
	return
}

// glob 在目录 dir 中搜索与模式 pattern 匹配的文件，并将其添加到 matches 中。
// 如果目录无法打开，则返回现有的 matches。新匹配项按字典顺序添加。
func glob(fs Afero, dir, pattern string, matches []string) (m []string, e error) {
	m = matches
	fi, err := fs.Stat(dir) // 获取目录的信息
	if err != nil {
		return
	}
	if !fi.IsDir() {
		return // 如果不是目录，返回
	}
	d, err := fs.Open(dir) // 打开目录
	if err != nil {
		return
	}
	defer d.Close() // 确保在函数返回时关闭目录

	names, _ := d.Readdirnames(-1) // 读取目录中的所有文件名
	sort.Strings(names)            // 按字典顺序排序

	for _, n := range names {
		matched, err := filepath.Match(pattern, n) // 检查文件名是否与模式匹配
		if err != nil {
			logger.Error("文件名匹配失败:", err)
			return m, err // 如果发生错误，返回错误信息
		}
		if matched {
			m = append(m, filepath.Join(dir, n)) // 添加匹配的文件名到结果中
		}
	}
	return
}

// hasMeta 检查路径中是否包含任何 Match 识别的特殊字符
// 参数：
//   - path: string 路径
//
// 返回值：
//   - bool: 是否包含特殊字符
func hasMeta(path string) bool {
	// TODO(niemeyer): 这里是否需要添加其他特殊字符？
	return strings.ContainsAny(path, "*?[") // 检查路径中是否包含 '*', '?', '[' 字符
}
