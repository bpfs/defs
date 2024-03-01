package paths

import (
	"fmt"
	"os"

	"github.com/bpfs/defs/afero"
	"github.com/sirupsen/logrus"
)

// 检查路径是否存在并且是一个目录
// 不存在则创建目录路径和所有尚不存在的父级。
func DirExistsAndMkdirAll(path string) error {
	// 检查路径是否存在并且是一个目录
	dirExists, err := afero.DirExists(afero.NewOsFs(), path)
	if err != nil {
		logrus.Error("文件路径检查失败, error:", err)
		return err
	}
	if !dirExists {
		// 创建目录路径和所有尚不存在的父级。
		if err = afero.NewOsFs().MkdirAll(path, 0755); err != nil && !os.IsExist(err) {
			logrus.Error("文件路径创建失败, error:", err)
			return fmt.Errorf("文件路径 %s 创建失败", path)
		}
	}

	return nil
}
