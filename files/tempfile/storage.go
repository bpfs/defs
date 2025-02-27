package tempfile

import (
	"bytes"
	"errors"
	"sync"
	"sync/atomic"
	"time"
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
	// logger.Infof("添加键值对映射: key=%s, filename=%s", key, filename)
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
	// logger.Infof("删除键值对映射: key=%s", key)
}

// NewFilePool 创建新的文件池
func NewFilePool(config Config) *FilePool {
	pool := &FilePool{
		inUse:  make(map[string]*TempFile),
		config: config,
	}

	pool.pool.New = func() interface{} {
		return &TempFile{
			buffer:     bytes.NewBuffer(make([]byte, 0, _32KB)), // 使用固定的32KB缓冲区
			bufferSize: _32KB,
		}
	}

	// 启动清理goroutine
	if config.CleanupInterval > 0 {
		go pool.cleanupLoop()
	}

	return pool
}

// Acquire 获取一个临时文件
func (p *FilePool) Acquire() (*TempFile, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查是否达到最大文件数
	if len(p.inUse) >= p.config.MaxFiles {
		return nil, &TempFileError{Op: "acquire", Err: errors.New("达到最大文件数限制")}
	}

	// 从池中获取
	tf := p.pool.Get().(*TempFile)

	// 初始化文件
	if err := tf.init(p.config); err != nil {
		p.pool.Put(tf)
		logger.Errorf("初始化文件失败: %v", err)
		return nil, err
	}

	// 更新状态
	tf.lastAccess = time.Now()
	atomic.StoreInt32(&tf.refCount, 1)

	// 加入使用中map
	p.inUse[tf.path] = tf

	return tf, nil
}

// Release 释放临时文件
func (p *FilePool) Release(tf *TempFile) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 减少引用计数
	if atomic.AddInt32(&tf.refCount, -1) == 0 {
		// 重置文件状态
		tf.reset()

		// 从使用中map删除
		delete(p.inUse, tf.path)

		// 放回池中
		p.pool.Put(tf)
	}

	return nil
}

// cleanupLoop 定期清理过期文件
func (p *FilePool) cleanupLoop() {
	ticker := time.NewTicker(p.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		p.cleanup()
	}
}

// cleanup 清理过期的临时文件
func (p *FilePool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for path, tf := range p.inUse {
		// 清理超过24小时未访问且引用计数为0的文件
		if now.Sub(tf.lastAccess) > 24*time.Hour && atomic.LoadInt32(&tf.refCount) == 0 {
			tf.close()
			delete(p.inUse, path)
			p.pool.Put(tf)
		}
	}
}
