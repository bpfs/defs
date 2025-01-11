package tempfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// generateTempFilename 生成唯一的临时文件名
// 返回值：
//   - string: 生成的唯一临时文件名
func generateTempFilename() string {
	// 获取当前时间的纳秒级时间戳
	timestamp := time.Now().UnixNano()

	// 使用时间戳生成唯一的文件名
	filename := fmt.Sprintf("defs_tempfile_%d", timestamp)

	// 将文件名与系统临时目录路径拼接
	fullPath := filepath.Join(os.TempDir(), filename)

	// 记录生成的临时文件名
	logger.Infof("生成临时文件名: %s", fullPath)

	return fullPath
}

// Exists 检查与给定键关联的临时文件是否存在
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - bool: 文件是否存在
//   - error: 如果检查过程中发生错误，返回相应的错误信息
func Exists(key string) (bool, error) {
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return false, nil
	}
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Size 返回与给定键关联的临时文件的大小
// 参数:
//   - key: string 用于检索临时文件的唯一键
//
// 返回值:
//   - int64: 文件大小（字节）
//   - error: 如果获取文件大小过程中发生错误，返回相应的错误信息
func Size(key string) (int64, error) {
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return 0, fmt.Errorf("未找到与键关联的文件")
	}
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return fileInfo.Size(), nil
}
