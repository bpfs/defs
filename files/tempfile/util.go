package tempfile

import (
	"fmt"
	"os"
)

// generateTempFilename 生成唯一的临时文件名并创建文件
// 返回值：
//   - string: 生成的临时文件路径
//   - error: 如果创建失败则返回错误
func generateTempFilename() (string, error) {
	// 使用系统临时目录创建临时文件
	f, err := os.CreateTemp("", "defs_tempfile_*")
	if err != nil {
		logger.Errorf("生成临时文件名失败: %v", err)
		return "", err
	}
	defer f.Close()

	return f.Name(), nil
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
		logger.Errorf("检查文件是否存在失败: %v", err)
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
		logger.Errorf("获取文件大小失败: %v", err)
		return 0, err
	}
	return fileInfo.Size(), nil
}
