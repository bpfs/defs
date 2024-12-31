package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bpfs/defs/utils/logger"
)

// 全局变量来存储根路径
var (
	Config   PathConfig                                    // Config 是全局路径配置实例
	rootPath = filepath.Join(ObtainRootPath(), "defsdata") // 默认值
)

// PathConfig 存储所有相关路径
type PathConfig struct {
	RootPath     string // 根路径
	DatabasePath string // 数据库路径
	//UploadPath   string // 上传路径
	//DownloadPath string // 下载路径
	SlicePath string // 切片路径
	LogPath   string // 日志路径
	// TempPath string // 临时文件路径
}

// PathOptions 定义可选的路径配置
type PathOptions struct {
	RootPath     string // 根路径
	DownloadPath string // 下载路径
}

// NewPathOptions 创建一个新的 PathOptions 实例
// 参数:
//   - rootPath: 根路径
//   - downloadPath: 下载路径
//
// 返回值:
//   - *PathOptions: 新创建的 PathOptions 实例
func NewPathOptions(rootPath, downloadPath string) *PathOptions {
	return &PathOptions{
		RootPath:     rootPath,
		DownloadPath: downloadPath,
	}
}

// InitializePaths 初始化所有必要的路径
// 参数：
//   - opts: 可选的路径配置，可以为 nil
//
// 返回值：
//   - error: 初始化过程中的错误，如果没有错误则为 nil
func InitializePaths(opts *PathOptions) error {

	if opts != nil && opts.RootPath != "" {

		// 检查路径是否已经包含 defsdata
		if strings.HasSuffix(opts.RootPath, "defsdata") {
			rootPath = opts.RootPath
		} else {
			rootPath = filepath.Join(opts.RootPath, "defsdata")
		}
	}

	// 初始化 Config 结构体
	Config = PathConfig{
		RootPath:     rootPath,
		DatabasePath: filepath.Join(rootPath, "database"),
		//UploadPath:   filepath.Join(rootPath, "uploads"),
		//DownloadPath: filepath.Join(rootPath, "downloads"),
		SlicePath: filepath.Join(rootPath, "slices"),
		LogPath:   filepath.Join(rootPath, "logs"),
		//TempPath: filepath.Join(rootPath, "temp"),
	}

	// 需要创建的目录列表
	dirsToCreate := []string{
		Config.DatabasePath,
		//Config.UploadPath,
		//Config.DownloadPath,
		Config.SlicePath,
		Config.LogPath,
		//	Config.TempPath,
	}

	// 遍历并创建所有必要的目录
	for _, dir := range dirsToCreate {

		if err := createDirectoryIfNotExists(dir); err != nil {
			logger.Errorf("初始化路径失败，无法创建目录 %s: %v", dir, err)
			return err
		}
	}

	logger.Infof("所有路径初始化完成")
	return nil
}

// createDirectoryIfNotExists 创建目录（如果不存在）
// 参数：
//   - dir: 要创建的目录路径
//
// 返回值：
//   - error: 创建过程中的错误，如果没有错误则为 nil
func createDirectoryIfNotExists(dir string) error {
	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 创建目录
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			logger.Errorf("创建目录失败 %s: %v", dir, err)
			return err
		}
		logger.Infof("成功创建目录 %s", dir)
	}
	return nil
}

// GetRootPath 返回根目录路径
// 返回值：
//   - string: 根目录路径
func GetRootPath() string {

	return rootPath
}

// GetDatabasePath 返回数据库路径
// 返回值：
//   - string: 数据库路径
func GetDatabasePath() string {

	return filepath.Join(GetRootPath(), "database")
}

// // GetUploadPath 返回上传路径
// // 返回值：
// //   - string: 上传路径
// func GetUploadPath() string {
// 	return Config.UploadPath
// }

// // GetDownloadPath 返回下载路径
// // 返回值：
// //   - string: 下载路径
// func GetDownloadPath() string {
// 	return Config.DownloadPath
// }

// GetSlicePath 返回切片路径
// 返回值：
//   - string: 切片路径
func GetSlicePath() string {

	return filepath.Join(GetRootPath(), "slices")
}

// GetLogPath 返回日志路径
// 返回值：
//   - string: 日志路径
func GetLogPath() string {
	return filepath.Join(GetRootPath(), "logs")
}

// // GetTempPath 返回临时文件路径
// // 返回值：
// //   - string: 临时文件路径
// func GetTempPath() string {
// 	return Config.TempPath
// }
