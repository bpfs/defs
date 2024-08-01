package paths

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// 全局变量来存储根路径
var rootPath = ObtainRootPath() // 默认值

// initDirectories 确保所有预定义的文件夹都存在
func InitDirectories(path string) (afero.Afero, error) {
	rootPath = filepath.Join(path, "defsdata")
	// 创建一个新的 OsFs 实例
	fs := afero.NewOsFs()
	// 创建一个新的 BasePathFs
	afe := afero.NewBasePathFs(fs, rootPath)

	// 所有需要检查的目录
	directories := []string{
		GetFilesPath(), // 文件目录
		// GetDBPath(),       // 数据库目录
		GetLogsPath(),     // 日志目录
		GetUploadPath(),   // 上传目录
		GetSlicePath(),    // 切片目录
		GetDownloadPath(), // 下载目录
	}

	// 遍历每个目录并确保它存在
	for _, dir := range directories {
		// 创建目录
		if err := afe.MkdirAll(dir, 0755); err != nil {
			logrus.Errorf("[%s]创建目录时失败: %v", debug.WhereAmI(), err)
			return afe, err
		}
	}

	return afe, nil
}

// GetRootPath 返回根路径
func GetRootPath() string {
	return filepath.Join(rootPath)
}

// GetFilesPath 返回文件目录路径
func GetFilesPath() string {
	return filepath.Join("files")
}

// GetDBPath 返回数据库目录路径
func GetDBPath() string {
	return filepath.Join("db")
}

// GetLogsPath 返回日志目录路径
func GetLogsPath() string {
	return filepath.Join("logs")
}

// GetUploadPath 返回上传目录路径
func GetUploadPath() string {
	return filepath.Join(GetFilesPath(), "uploads")
}

// GetSlicePath 返回切片目录路径
func GetSlicePath() string {
	return filepath.Join(GetFilesPath(), "slices")
}

// GetDownloadPath 返回下载目录路径
func GetDownloadPath() string {
	return filepath.Join(GetFilesPath(), "downloads")
}

// GetBusinessDbPath 返回业务db目录路径
func GetBusinessDbPath() string {
	return filepath.Join(GetDBPath(), "businessdbs")
}

//////////////////

// 路径管理器
// TODO: 待优化
// var (
// 	RootPath = filepath.Join(ObtainRootPath(), "defsdata")

// 	// 二级目录
// 	Files = filepath.Join(RootPath, "files") // 文件目录
// 	DB    = filepath.Join(RootPath, "db")    // 数据库目录
// 	Logs  = filepath.Join(RootPath, "logs")  // 日志目录

// 	// 三级目录
// 	UploadPath     = filepath.Join(Files, "uploads")   // 上传目录
// 	SlicePath      = filepath.Join(Files, "slices")    // 切片目录
// 	DownloadPath   = filepath.Join(Files, "downloads") // 下载目录
// 	BusinessDbPath = filepath.Join(DB, "businessdbs")  // 业务db目录
// )

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
