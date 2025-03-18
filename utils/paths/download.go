package paths // GetDefaultDownloadPath 返回操作系统的默认下载路径。
// 它假设用户使用的是操作系统的标准下载文件夹。
import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// DefaultDownloadPath 返回操作系统的默认下载路径。
// 返回值：
//   - string: 默认下载路径
func DefaultDownloadPath() (string, error) {

	userInfo, err := user.Current()
	if err != nil {
		logger.Warnf("返回当前用户时失败: %v", err)
		return "", err
	}
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果无法获取用户主目录，记录错误并返回当前工作目录
		logger.Errorf("获取用户主目录失败: %v", err)
		return "", err
	}

	var downloadDir = filepath.Join(userInfo.HomeDir, "Downloads")
	// 1. 检查目录状态
	if _, err := os.Stat(downloadDir); err == nil {
		// 目录已存在，无需操作
		logger.Warnf("目录已存在: %s", downloadDir)
	} else if !os.IsNotExist(err) {
		// 非目录不存在的其他错误（如权限问题）
		logger.Errorf("检查目录状态失败: %v", err)
		return "", err
	} else {
		// 目录不存在时创建
		if err := os.MkdirAll(downloadDir, 0755); err != nil {
			logger.Errorf("创建目录失败: %v", err)
			return "", err
		}
		logger.Infof("目录创建成功: %s", downloadDir)
	}

	// 根据不同操作系统构建下载文件夹路径
	switch runtime.GOOS {
	case "windows":
		// 对于 Windows，通常是在 "Downloads" 文件夹中
		return downloadDir, nil
	case "darwin", "linux":
		// 对于 macOS (Darwin) 和 Linux，同样是 "Downloads" 文件夹
		return downloadDir, nil
	default:
		// 对于未知操作系统，记录警告并返回用户主目录
		logger.Warnf("未知操作系统 %s，使用用户主目录作为下载路径 %s", runtime.GOOS, homeDir)
		return homeDir, nil
	}
}
