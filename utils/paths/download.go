package paths // GetDefaultDownloadPath 返回操作系统的默认下载路径。
// 它假设用户使用的是操作系统的标准下载文件夹。
import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/bpfs/defs/utils/logger"
)

// DefaultDownloadPath 返回操作系统的默认下载路径。
// 返回值：
//   - string: 默认下载路径
func DefaultDownloadPath() string {
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果无法获取用户主目录，记录错误并返回当前工作目录
		logger.Errorf("获取用户主目录失败: %v", err)
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
		// 对于未知操作系统，记录警告并返回用户主目录
		logger.Warnf("未知操作系统 %s，使用用户主目录作为下载路径", runtime.GOOS)
		return homeDir
	}
}
