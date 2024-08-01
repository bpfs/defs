// 定义共享的基类和方法
package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/sirupsen/logrus"
)

// CreateFile 在指定子目录创建一个新文件
func CreateFile(opt *opts.Options, afe afero.Afero, subDir, fileName string) error {
	filePath := filepath.Join(subDir, fileName)
	file, err := afe.Create(filePath)
	if err != nil {
		logrus.Errorf("[%s]无法创建文件: %v", debug.WhereAmI(), err)
		return err
	}
	return file.Close()
}

// Write 写入数据到指定的文件
func Write(opt *opts.Options, afe afero.Afero, subDir, fileName string, data []byte) error {
	if err := afe.MkdirAll(filepath.Join(subDir), 0755); err != nil {
		logrus.Errorf("[%s]无法创建子目录: %v", debug.WhereAmI(), err)
		return err
	}
	filePath := filepath.Join(subDir, fileName)
	return afero.WriteFile(afe, filePath, data, 0644)
}

// Read 从指定的文件读取数据
func Read(opt *opts.Options, afe afero.Afero, subDir, fileName string) ([]byte, error) {
	filePath := filepath.Join(subDir, fileName)
	exists, err := afero.Exists(afe, filePath)
	if err != nil {
		logrus.Errorf("[%s]无法检查文件是否存在: %v", debug.WhereAmI(), err)
		return nil, err
	}
	if !exists {
		// logrus.Warnf("[%s]文件 '%s' 不存在", debug.WhereAmI(), filePath)
		return nil, nil
	}
	return afero.ReadFile(afe, filePath)
}

// OpenFile 打开指定子目录和文件名的文件
func OpenFile(opt *opts.Options, afe afero.Afero, subDir, fileName string) (*os.File, error) {
	filePath := filepath.Join(subDir, fileName)
	if exists, err := afero.Exists(afe, filePath); !exists {
		logrus.Errorf("[%s]未找到文件 %s: %v", debug.WhereAmI(), fileName, err)
		return nil, err
	}
	file, err := afe.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		logrus.Errorf("[%s]获取切片失败: %v", debug.WhereAmI(), err)
		return nil, err
	}
	// defer file.Close()

	// 将afero.File转换为*os.File
	if basePathFile, ok := file.(*afero.BasePathFile); ok {
		if osFile, ok := basePathFile.File.(*os.File); ok {
			return osFile, nil
		}
	}

	return nil, fmt.Errorf("无法打开文件:'%s'", fileName)
}

// Delete 删除指定的文件
func Delete(opt *opts.Options, afe afero.Afero, subDir, fileName string) error {
	filePath := filepath.Join(subDir, fileName)
	return afe.Remove(filePath)
}

// DeleteAll 删除所有文件
func DeleteAll(opt *opts.Options, afe afero.Afero, subDir string) error {
	filePath := filepath.Join(subDir)
	return afe.RemoveAll(filePath)
}

// Exists 检查指定的文件是否存在
func Exists(opt *opts.Options, afe afero.Afero, subDir, fileName string) (bool, error) {
	filePath := filepath.Join(subDir, fileName)
	return afero.Exists(afe, filePath)
}

// ListFiles 列出指定子目录中的所有文件
// func ListFiles(afe afero.Afero, subDir string) ([]string, error) {
// 	dirPath := filepath.Join(subDir)
// 	files, err := afero.ReadDir(afe, dirPath)
// 	if err != nil {
// 		logrus.Errorf("[%s]无法列出文件: %v", debug.WhereAmI(), err)
// 		return nil, err
// 	}

// 	var fileList []string
// 	for _, file := range files {
// 		if !file.IsDir() {
// 			fileList = append(fileList, file.Name())
// 		}
// 	}

// 	return fileList, nil
// }

// CopyFile 将文件从源路径复制到目标路径
func CopyFile(opt *opts.Options, afe afero.Afero, srcSubDir, srcFileName, destSubDir, destFileName string) error {
	srcFilePath := filepath.Join(srcSubDir, srcFileName)
	destFilePath := filepath.Join(destSubDir, destFileName)

	srcFile, err := afe.Open(srcFilePath)
	if err != nil {
		logrus.Errorf("[%s]无法打开源文件: %v", debug.WhereAmI(), err)
		return err
	}
	defer srcFile.Close()

	if err := afe.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
		logrus.Errorf("[%s]无法创建目标目录: %v", debug.WhereAmI(), err)
		return err
	}

	destFile, err := afe.Create(destFilePath)
	if err != nil {
		logrus.Errorf("[%s]无法创建目标文件: %v", debug.WhereAmI(), err)
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		logrus.Errorf("[%s]无法复制文件: %v", debug.WhereAmI(), err)
		return err
	}

	return nil
}

// RenameFile 重命名或移动文件
func RenameFile(opt *opts.Options, afe afero.Afero, oldSubDir, oldFileName, newSubDir, newFileName string) error {
	oldFilePath := filepath.Join(oldSubDir, oldFileName)
	newFilePath := filepath.Join(newSubDir, newFileName)

	if err := afe.MkdirAll(filepath.Dir(newFilePath), 0755); err != nil {
		logrus.Errorf("[%s]无法创建新目录: %v", debug.WhereAmI(), err)
		return err
	}

	return afe.Rename(oldFilePath, newFilePath)
}

// WalkFiles 遍历指定目录下的文件并执行回调函数
func WalkFiles(opt *opts.Options, afe afero.Afero, subDir string, callback func(filePath string, info os.FileInfo) error) error {
	dirPath := filepath.Join(subDir)
	return afero.Walk(afe, dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}
		if !info.IsDir() {
			return callback(path, info)
		}
		return nil
	})
}
