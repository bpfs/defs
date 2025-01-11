package paths

import (
	"fmt"
	"os"

	"github.com/bpfs/defs/afero"
)

// DirExistsAndMkdirAll 检查路径是否存在并且是一个目录，如果不存在则创建目录路径和所有尚不存在的父级。
// 参数:
//   - afe: afero.Afero 文件系统接口
//   - path: string 要检查和创建的目录路径
//
// 返回值:
//   - error 如果出现错误，返回相应的错误信息
func DirExistsAndMkdirAll(afe afero.Afero, path string) error {
	// 检查路径是否存在并且是一个目录
	dirExists, err := afero.DirExists(afe, path)
	if err != nil {
		logger.Error("目录路径检查失败, error:", err)
		return err
	}

	// 如果目录不存在，则创建
	if !dirExists {
		// 创建目录路径和所有尚不存在的父级
		if err = afe.MkdirAll(path, 0755); err != nil && !os.IsExist(err) {
			logger.Error("目录路径创建失败, error:", err)
			return fmt.Errorf("目录路径 %s 创建失败", path)
		}
	}

	return nil
}
