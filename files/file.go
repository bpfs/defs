package files

import (
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/fscfg"
	logging "github.com/dep2p/log"
)

var logger = logging.Logger("files")

// CreateFile 在指定子目录创建一个新文件
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//
// 返回值:
//   - error: 如果创建过程中发生错误，返回相应的错误信息
func CreateFile(opt *fscfg.Options, afe afero.Afero, subDir, fileName string) error {
	// 构建完整的文件路径
	filePath := filepath.Join(subDir, fileName)

	// 创建文件
	file, err := afe.Create(filePath)
	if err != nil {
		logger.Errorf("无法创建文件: %v", err)
		return err
	}

	// 关闭文件
	return file.Close()
}

// Write 写入数据到指定的文件
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//   - data: []byte 类型，要写入的数据
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
// func Write(opt *fscfg.Options, afe afero.Afero, subDir, fileName string, data []byte) error {
// 	// 确保子目录存在
// 	if err := afe.MkdirAll(filepath.Join(subDir), 0755); err != nil {
// 		logger.Errorf("无法创建子目录: %v", err)
// 		return err
// 	}

// 	// 构建完整的文件路径
// 	filePath := filepath.Join(subDir, fileName)

// 	// 写入文件
// 	return afero.WriteFile(afe, filePath, data, 0644)
// }

// Write 写入数据到指定的文件
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//   - data: *[]byte 类型，要写入的数据的指针
//
// 返回值:
//   - error: 如果写入过程中发生错误，返回相应的错误信息
func Write(opt *fscfg.Options, afe afero.Afero, subDir, fileName string, data *[]byte) error {
	// 确保子目录存在
	if err := afe.MkdirAll(filepath.Join(subDir), 0755); err != nil {
		logger.Errorf("无法创建子目录: %v", err)
		return err
	}

	// 构建完整的文件路径
	filePath := filepath.Join(subDir, fileName)

	// 写入文件
	return afero.WriteFile(afe, filePath, *data, 0644)
}

// Read 从指定的文件读取数据
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//
// 返回值:
//   - []byte: 读取的文件内容
//   - error: 如果读取过程中发生错误，返回相应的错误信息
func Read(opt *fscfg.Options, afe afero.Afero, subDir, fileName string) ([]byte, error) {
	// 构建完整的文件路径
	filePath := filepath.Join(subDir, fileName)

	// 检查文件是否存在
	exists, err := afero.Exists(afe, filePath)
	if err != nil {
		logger.Errorf("无法检查文件是否存在: %v", err)
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	// 读取文件内容
	return afero.ReadFile(afe, filePath)
}

// 废弃：syscall.Mmap 不兼容问题
// OpenFile 打开指定子目录和文件名的文件，并返回内存映射
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//
// 返回值:
//   - []byte: 内存映射的文件内容
//   - func(): 用于解除内存映射和清理资源的函数
//   - error: 如果过程中发生错误，返回相应的错误信息
// func OpenFile(opt *fscfg.Options, afe afero.Afero, subDir, fileName string) ([]byte, func(), error) {
// 	// 构建完整的文件路径
// 	filePath := filepath.Join(subDir, fileName)

// 	// 检查文件是否存在
// 	if exists, err := afero.Exists(afe, filePath); !exists {
// 		logger.Errorf("未找到文件 %s: %v", fileName, err)
// 		return nil, nil, err
// 	}

// 	// 打开文件
// 	file, err := afe.OpenFile(filePath, os.O_RDWR, 0644)
// 	if err != nil {
// 		logger.Errorf("获取切片失败: %v", err)
// 		return nil, nil, err
// 	}

// 	// 获取文件信息
// 	fileInfo, err := file.Stat()
// 	if err != nil {
// 		file.Close()
// 		return nil, nil, err
// 	}

// 	// 将afero.File转换为*os.File
// 	var osFile *os.File
// 	if basePathFile, ok := file.(*afero.BasePathFile); ok {
// 		if osFile, ok = basePathFile.File.(*os.File); !ok {
// 			file.Close()
// 			return nil, nil, fmt.Errorf("无法转换为*os.File")
// 		}
// 	} else {
// 		file.Close()
// 		return nil, nil, fmt.Errorf("无法转换为*afero.BasePathFile")
// 	}

// 	// 创建内存映射
// 	mmap, err := syscall.Mmap(int(osFile.Fd()), 0, int(fileInfo.Size()), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
// 	if err != nil {
// 		osFile.Close()
// 		return nil, nil, err
// 	}

// 	// 定义清理函数
// 	cleanup := func() {
// 		syscall.Munmap(mmap)
// 		osFile.Close()
// 	}

// 	return mmap, cleanup, nil
// }

// Delete 删除指定的文件
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func Delete(opt *fscfg.Options, afe afero.Afero, subDir, fileName string) error {
	// 构建完整的文件路径
	filePath := filepath.Join(subDir, fileName)

	// 删除文件
	return afe.Remove(filePath)
}

// DeleteAll 删除所有文件
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//
// 返回值:
//   - error: 如果删除过程中发生错误，返回相应的错误信息
func DeleteAll(opt *fscfg.Options, afe afero.Afero, subDir string) error {
	// 构建完整的目录路径
	filePath := filepath.Join(subDir)

	// 删除目录及其所有内容
	return afe.RemoveAll(filePath)
}

// Exists 检查指定的文件是否存在
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，子目录路径
//   - fileName: string 类型，文件名
//
// 返回值:
//   - bool: 文件是否存在
//   - error: 如果检查过程中发生错误，返回相应的错误信息
func Exists(opt *fscfg.Options, afe afero.Afero, subDir, fileName string) (bool, error) {
	// 构建完整的文件路径
	filePath := filepath.Join(subDir, fileName)

	// 检查文件是否存在
	return afero.Exists(afe, filePath)
}

// CopyFile 将文件从源路径复制到目标路径
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - srcSubDir: string 类型，源文件子目录路径
//   - srcFileName: string 类型，源文件名
//   - destSubDir: string 类型，目标文件子目录路径
//   - destFileName: string 类型，目标文件名
//
// 返回值:
//   - error: 如果复制过程中发生错误，返回相应的错误信息
func CopyFile(opt *fscfg.Options, afe afero.Afero, srcSubDir, srcFileName, destSubDir, destFileName string) error {
	// 构建源文件和目标文件的完整路径
	srcFilePath := filepath.Join(srcSubDir, srcFileName)
	destFilePath := filepath.Join(destSubDir, destFileName)

	// 打开源文件
	srcFile, err := afe.Open(srcFilePath)
	if err != nil {
		logger.Errorf("无法打开源文件: %v", err)
		return err
	}
	defer srcFile.Close()

	// 确保目标目录存在
	if err := afe.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
		logger.Errorf("无法创建目标目录: %v", err)
		return err
	}

	// 创建目标文件
	destFile, err := afe.Create(destFilePath)
	if err != nil {
		logger.Errorf("无法创建目标文件: %v", err)
		return err
	}
	defer destFile.Close()

	// 复制文件内容
	if _, err := io.Copy(destFile, srcFile); err != nil {
		logger.Errorf("无法复制文件: %v", err)
		return err
	}

	return nil
}

// RenameFile 重命名或移动文件
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - oldSubDir: string 类型，原文件子目录路径
//   - oldFileName: string 类型，原文件名
//   - newSubDir: string 类型，新文件子目录路径
//   - newFileName: string 类型，新文件名
//
// 返回值:
//   - error: 如果重命名过程中发生错误，返回相应的错误信息
func RenameFile(opt *fscfg.Options, afe afero.Afero, oldSubDir, oldFileName, newSubDir, newFileName string) error {
	// 构建原文件和新文件的完整路径
	oldFilePath := filepath.Join(oldSubDir, oldFileName)
	newFilePath := filepath.Join(newSubDir, newFileName)

	// 确保新文件所在的目录存在
	if err := afe.MkdirAll(filepath.Dir(newFilePath), 0755); err != nil {
		logger.Errorf("无法创建新目录: %v", err)
		return err
	}

	// 重命名文件
	return afe.Rename(oldFilePath, newFilePath)
}

// WalkFiles 遍历指定目录下的文件并执行回调函数
// 参数:
//   - opt: *fscfg.Options 类型，包含文件系统配置选项
//   - afe: afero.Afero 类型，文件系统接口
//   - subDir: string 类型，要遍历的子目录路径
//   - callback: func(filePath string, info os.FileInfo) error 类型，对每个文件执行的回调函数
//
// 返回值:
//   - error: 如果遍历过程中发生错误，返回相应的错误信息
func WalkFiles(opt *fscfg.Options, afe afero.Afero, subDir string, callback func(filePath string, info os.FileInfo) error) error {
	// 构建完整的目录路径
	dirPath := filepath.Join(subDir)

	// 遍历目录
	return afero.Walk(afe, dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Errorf("无法遍历目录: %v", err)
			return err
		}
		if !info.IsDir() {
			return callback(path, info)
		}
		return nil
	})
}
