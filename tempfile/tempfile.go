package tempfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// Write 将值写入临时文件，并将文件名与键关联
func Write(key string, value []byte) error {
	filename := generateTempFilename()

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logrus.Errorf("[%s]创建目录时失败: %v", debug.WhereAmI(), err)
		return err
	}

	err := os.WriteFile(filename, value, 0666)
	if err != nil {
		return err
	}
	addKeyToFileMapping(key, filename)
	return nil
}

// Read 根据键读取临时文件的内容，并在读取成功后删除文件
func Read(key string) ([]byte, error) {
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return nil, fmt.Errorf("key not found")
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = os.Remove(filename)
	if err != nil {
		return nil, err
	}
	deleteKeyToFileMapping(key)
	return content, nil
}

// Delete 根据键删除临时文件
func Delete(key string) error {
	filename, ok := getKeyToFileMapping(key)
	if !ok {
		return fmt.Errorf("key not found")
	}

	err := os.Remove(filename)
	if err != nil {
		return err
	}
	deleteKeyToFileMapping(key)
	return nil
}
