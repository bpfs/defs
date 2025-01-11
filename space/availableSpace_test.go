package space

import (
	"os"
	"testing"
)

func TestGetAvailableSpace(t *testing.T) {
	// 示例：计算当前工作目录下的可用存储空间
	path, err := os.Getwd()
	if err != nil {
		logger.Infof("获取当前目录时出错:", err)
		return
	}

	space, err := GetAvailableSpace(path)
	if err != nil {
		logger.Infof("获取可用空间时出错:", err)
		return
	}

	logger.Infof("%s 处的可用空间：%d 字节\n", path, space)
}
