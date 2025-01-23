package paths

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

////////////////////////////////////////////////////////////////////////

// ObtainRootPath 获取根目录路径
// 返回值：
//   - string: 根目录路径
func ObtainRootPath() string {
	// 各环境资源存储路径
	var resourceStoragePath string

	// 根据不同操作系统设置资源存储路径
	switch runtime.GOOS {
	case "windows":
		resourceStoragePath = "Documents/Netdisc"
	case "darwin", "linux":
		resourceStoragePath = "Library/Netdisc"
	default:
		resourceStoragePath = ".netdisc"
	}

	// 判断是否为go run运行环境
	if IsGorunEnv() {
		// 开发环境：使用当前绝对路径
		return getCurrentAbPath()
	} else {
		// 生产环境：使用用户主目录
		currentUser, err := user.Current()
		if err != nil {
			logger.Fatalf(" %v", err)
		}

		// 拼接用户主目录和资源存储路径
		dir := currentUser.HomeDir
		prefix := filepath.Join(dir, resourceStoragePath)
		return prefix
	}
}

// getCurrentAbPath 获取当前绝对路径（兼容方案）
// 返回值：
//   - string: 当前绝对路径
func getCurrentAbPath() string {
	// 获取可执行文件路径
	dir := getCurrentAbPathByExecutable()
	// 获取临时目录路径
	tmpDir, _ := filepath.EvalSymlinks(os.TempDir())
	// 如果可执行文件在临时目录中，则使用调用者路径
	if strings.Contains(dir, tmpDir) {
		return getCurrentAbPathByCaller()
	}
	return dir
}

// getCurrentAbPathByExecutable 获取当前执行文件的绝对路径
// 返回值：
//   - string: 执行文件的绝对路径
func getCurrentAbPathByExecutable() string {
	// 获取可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		logger.Fatalf("%v", err)
	}
	// 解析符号链接并返回目录路径
	res, _ := filepath.EvalSymlinks(filepath.Dir(exePath))
	return res
}

// getCurrentAbPathByCaller 获取调用者的绝对路径（用于go run环境）
// 返回值：
//   - string: 调用者的绝对路径
func getCurrentAbPathByCaller() string {
	var abPath string
	// 获取调用者的文件信息
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		// 获取文件所在目录
		abPath = path.Dir(filename)
	}
	// 返回上一级目录路径
	return filepath.Join(abPath, "../../")
}

// IsGorunEnv 判断是否为go run运行环境
// 返回值：
//   - bool: 是否为go run环境
func IsGorunEnv() bool {
	// 获取可执行文件路径
	dir := getCurrentAbPathByExecutable()
	// 获取临时目录路径
	tmpDir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		logger.Fatalf("%v", err)
	}
	// 判断可执行文件是否在临时目录中
	return strings.Contains(dir, tmpDir)
}

// AddDirectory 动态添加新目录
// 参数:
//   - dirNames: string 要创建的目录路径
//
// 返回值：
//   - error: 如果目录创建失败，返回错误信息
func AddDirectory(dirNames string) error {
	// 检查目录名是否为空
	if dirNames == "" {
		return fmt.Errorf("未指定目录")
	}

	// 创建目录，权限设置为0755
	if err := os.MkdirAll(dirNames, 0755); err != nil {
		logger.Errorf("创建目录失败: %v", err)
		return err
	}

	// 记录成功日志
	logger.Infof("目录添加成功: %s", dirNames)
	return nil
}
