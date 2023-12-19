// 定义共享的基类和方法
package filestore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// FileStore 封装了文件存储的操作
type FileStore struct {
	Fs       afero.Fs
	BasePath string
}

// NewFileStore 创建一个新的FileStore实例
func NewFileStore(basePath string) (*FileStore, error) {
	fs := afero.NewOsFs()
	if err := fs.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}
	return &FileStore{Fs: fs, BasePath: basePath}, nil
}

// CreateFile 在指定子目录创建一个新文件
func (fs *FileStore) CreateFile(subDir, fileName string) error {
	filePath := filepath.Join(fs.BasePath, subDir, fileName)
	file, err := fs.Fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	return file.Close()
}

// Write 写入数据到指定的文件
func (fs *FileStore) Write(subDir, fileName string, data []byte) error {
	if err := fs.Fs.MkdirAll(filepath.Join(fs.BasePath, subDir), 0755); err != nil {
		return fmt.Errorf("failed to create sub directory: %w", err)
	}
	filePath := filepath.Join(fs.BasePath, subDir, fileName)
	return afero.WriteFile(fs.Fs, filePath, data, 0644)
}

// Read 从指定的文件读取数据
func (fs *FileStore) Read(subDir, fileName string) ([]byte, error) {
	filePath := filepath.Join(fs.BasePath, subDir, fileName)
	exists, err := afero.Exists(fs.Fs, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check file existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("file '%s' does not exist", filePath)
	}
	return afero.ReadFile(fs.Fs, filePath)
}

// OpenFile 打开指定子目录和文件名的文件
func (fs *FileStore) OpenFile(subDir, fileName string) (*os.File, error) {
	filePath := filepath.Join(fs.BasePath, subDir, fileName)
	if exists, _ := afero.Exists(fs.Fs, filePath); !exists {
		return nil, fmt.Errorf("file '%s' not found", filePath)
	}
	file, err := fs.Fs.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// 类型断言转换为 *os.File
	osFile, ok := file.(*os.File)
	if !ok {
		return nil, fmt.Errorf("file reading exception")
	}

	return osFile, nil
}

// Delete 删除指定的文件
func (fs *FileStore) Delete(subDir, fileName string) error {
	filePath := filepath.Join(fs.BasePath, subDir, fileName)
	return fs.Fs.Remove(filePath)
}

// DeleteAll 删除所有文件
func (fs *FileStore) DeleteAll(subDir string) error {
	filePath := filepath.Join(fs.BasePath, subDir)
	return fs.Fs.RemoveAll(filePath)
}

// Exists 检查指定的文件是否存在
func (fs *FileStore) Exists(subDir, fileName string) (bool, error) {
	filePath := filepath.Join(fs.BasePath, subDir, fileName)
	return afero.Exists(fs.Fs, filePath)
}

// ListFiles 列出指定子目录中的所有文件
func (fs *FileStore) ListFiles(subDir string) ([]string, error) {
	dirPath := filepath.Join(fs.BasePath, subDir)
	files, err := afero.ReadDir(fs.Fs, dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	var fileList []string
	for _, file := range files {
		if !file.IsDir() {
			fileList = append(fileList, file.Name())
		}
	}
	return fileList, nil
}

// CopyFile 将文件从源路径复制到目标路径
func (fs *FileStore) CopyFile(srcSubDir, srcFileName, destSubDir, destFileName string) error {
	srcFilePath := filepath.Join(fs.BasePath, srcSubDir, srcFileName)
	destFilePath := filepath.Join(fs.BasePath, destSubDir, destFileName)

	srcFile, err := fs.Fs.Open(srcFilePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	if err := fs.Fs.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	destFile, err := fs.Fs.Create(destFilePath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// RenameFile 重命名或移动文件
func (fs *FileStore) RenameFile(oldSubDir, oldFileName, newSubDir, newFileName string) error {
	oldFilePath := filepath.Join(fs.BasePath, oldSubDir, oldFileName)
	newFilePath := filepath.Join(fs.BasePath, newSubDir, newFileName)

	if err := fs.Fs.MkdirAll(filepath.Dir(newFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create new directory: %w", err)
	}

	return fs.Fs.Rename(oldFilePath, newFilePath)
}

// WalkFiles 遍历指定目录下的文件并执行回调函数
func (fs *FileStore) WalkFiles(subDir string, callback func(filePath string, info os.FileInfo) error) error {
	dirPath := filepath.Join(fs.BasePath, subDir)
	return afero.Walk(fs.Fs, dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return callback(path, info)
		}
		return nil
	})
}
