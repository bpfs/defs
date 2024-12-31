package tempfile

import (
	"sync"

	"github.com/bpfs/defs/utils/logger"
)

// fileMap 使用sync.Map存储键值对映射关系,保证并发安全
var fileMap sync.Map

// addKeyToFileMapping 添加键值对到映射中
// 参数:
//   - key: string 键
//   - filename: string 文件名
//
// 返回值: 无
func addKeyToFileMapping(key, filename string) {
	// 使用Store方法安全地添加键值对
	fileMap.Store(key, filename)
	// 记录日志
	logger.Infof("添加键值对映射: key=%s, filename=%s", key, filename)
}

// getKeyToFileMapping 根据键获取对应的文件名
// 参数:
//   - key: string 键
//
// 返回值:
//   - string: 文件名
//   - bool: 是否找到对应的文件名
func getKeyToFileMapping(key string) (string, bool) {
	// 使用Load方法安全地获取值
	value, ok := fileMap.Load(key)
	if !ok {
		return "", false
	}
	// 类型断言确保返回的是string类型
	filename, ok := value.(string)
	return filename, ok
}

// deleteKeyToFileMapping 从映射中删除指定的键值对
// 参数:
//   - key: string 要删除的键
//
// 返回值: 无
func deleteKeyToFileMapping(key string) {
	// 使用Delete方法安全地删除键值对
	fileMap.Delete(key)
	// 记录日志
	logger.Infof("删除键值对映射: key=%s", key)
}
