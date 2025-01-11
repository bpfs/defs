package space

import (
	"syscall"

	logging "github.com/dep2p/log"
)

var logger = logging.Logger("space")

// GetAvailableSpace 返回指定路径下的可用存储空间（字节）
func GetAvailableSpace(path string) (uint64, error) {
	// 使用 syscall.Statfs 获取文件系统的统计信息
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		// 错误处理：无法获取文件系统信息
		return 0, err
	}

	// 计算可用空间：块大小 * 可用块数
	availableSpace := uint64(stat.Bsize) * stat.Bavail

	return availableSpace, nil
}
