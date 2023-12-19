package defs

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// 路径管理器
var (
	RootPath = filepath.Join(ObtainRootPath(), "defsdata")

	// 二级目录
	Files = filepath.Join(RootPath, "files") // 文件目录
	DB    = filepath.Join(RootPath, "db")    // 数据库目录
	Logs  = filepath.Join(RootPath, "logs")  // 日志目录

	// 三级目录
	UploadPath     = filepath.Join(Files, "uploads")   // 上传目录
	SlicePath      = filepath.Join(Files, "slices")    // 切片目录
	DownloadPath   = filepath.Join(Files, "downloads") // 下载目录
	BusinessDbPath = filepath.Join(DB, "businessdbs")  // 业务db目录
)

// 获取根目录
func ObtainRootPath() string {
	// 各环境资源存储路径
	var resourceStoragePath string

	// 对于 macOS，它将使用 "Library/Netdisc"；对于 Windows，将使用 "Documents/Netdisc"；对于 Linux 和其他操作系统，将使用 ".netdisc"
	switch runtime.GOOS {
	case "windows":
		resourceStoragePath = "Documents/Netdisc"
	case "darwin":
		resourceStoragePath = "Library/Netdisc"
	case "linux":
		resourceStoragePath = "Library/Netdisc"
	default:
		resourceStoragePath = ".netdisc"
	}

	// 是否go run运行环境
	if IsGorunEnv() {
		// 开发环境
		// 最终方案-全兼容
		return getCurrentAbPath()
	} else {
		// 生产环境
		currentUser, err := user.Current()
		if err != nil {
			logrus.Fatalf(err.Error())
		}
		dir := currentUser.HomeDir
		prefix := filepath.Join(dir, resourceStoragePath)
		return prefix
	}
}

// 最终方案-全兼容
func getCurrentAbPath() string {
	dir := getCurrentAbPathByExecutable()
	tmpDir, _ := filepath.EvalSymlinks(os.TempDir())
	if strings.Contains(dir, tmpDir) {
		return getCurrentAbPathByCaller()
	}

	return dir
}

// 获取当前执行文件绝对路径
func getCurrentAbPathByExecutable() string {
	exePath, err := os.Executable()
	if err != nil {
		logrus.Fatal(err)
	}
	res, _ := filepath.EvalSymlinks(filepath.Dir(exePath))
	return res
}

// 获取当前执行文件绝对路径（go run）
func getCurrentAbPathByCaller() string {
	var abPath string
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		abPath = path.Dir(filename)
	}
	return filepath.Join(abPath, "..")
}

// 是否go run运行环境
func IsGorunEnv() bool {
	dir := getCurrentAbPathByExecutable()
	tmpDir, _ := filepath.EvalSymlinks(os.TempDir())
	return strings.Contains(dir, tmpDir)
}

// GetDefaultDownloadPath 返回操作系统的默认下载路径。
// 它假设用户使用的是操作系统的标准下载文件夹。
func GetDefaultDownloadPath() string {
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果无法获取用户主目录，则返回当前工作目录
		return "."
	}

	// 根据不同操作系统构建下载文件夹路径
	switch runtime.GOOS {
	case "windows":
		// 对于 Windows，通常是在 "Downloads" 文件夹中
		return filepath.Join(homeDir, "Downloads")
	case "darwin", "linux":
		// 对于 macOS (Darwin) 和 Linux，同样是 "Downloads" 文件夹
		return filepath.Join(homeDir, "Downloads")
	default:
		// 对于未知操作系统，返回用户主目录
		return homeDir
	}
}
