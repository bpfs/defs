package afero

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bpfs/defs/mem"
	// 使用自定义的 logger 包
	"github.com/bpfs/defs/utils/logger"
)

const chmodBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky // 仅允许更改部分位。记录在 os.Chmod() 中

// MemMapFs 是一个内存映射文件系统的实现
type MemMapFs struct {
	mu   sync.RWMutex             // 读写锁，用于并发控制
	data map[string]*mem.FileData // 存储文件数据的映射
	init sync.Once                // 确保数据初始化只执行一次
}

// NewMemMapFs 创建一个新的内存映射文件系统
// 返回值：
//   - Afero: 内存映射文件系统
func NewMemMapFs() Afero {
	return &MemMapFs{}
}

// getData 获取文件数据的映射
// 返回值：
//   - map[string]*mem.FileData: 文件数据映射
func (m *MemMapFs) getData() map[string]*mem.FileData {
	m.init.Do(func() { // 确保数据初始化只执行一次
		m.data = make(map[string]*mem.FileData)  // 初始化数据映射
		root := mem.CreateDir(FilePathSeparator) // 创建根目录
		mem.SetMode(root, os.ModeDir|0o755)      // 设置根目录的权限
		m.data[FilePathSeparator] = root         // 将根目录添加到数据映射中
	})
	return m.data
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (*MemMapFs) Name() string {
	return "MemMapFS"
}

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (m *MemMapFs) Create(name string) (File, error) {
	name = normalizePath(name)          // 规范化路径
	m.mu.Lock()                         // 加锁
	file := mem.CreateFile(name)        // 创建文件
	m.getData()[name] = file            // 将文件添加到数据映射中
	m.registerWithParent(file, 0)       // 注册文件到其父目录
	m.mu.Unlock()                       // 解锁
	return mem.NewFileHandle(file), nil // 返回文件句柄和错误信息
}

// unRegisterWithParent 从父目录中注销文件
// 参数：
//   - fileName: string 文件名
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) unRegisterWithParent(fileName string) error {
	f, err := m.lockfreeOpen(fileName) // 打开文件
	if err != nil {
		logger.Error("应用选项失败:", err)
		return err // 返回错误信息
	}
	parent := m.findParent(f) // 查找父目录
	if parent == nil {
		logger.Error("应用选项失败:", "parent of "+f.Name()+" is nil")
	}

	parent.Lock()                   // 加锁
	mem.RemoveFromMemDir(parent, f) // 从父目录中移除文件
	parent.Unlock()                 // 解锁
	return nil                      // 返回 nil 表示成功
}

// findParent 查找文件的父目录
// 参数：
//   - f: *mem.FileData 文件数据
//
// 返回值：
//   - *mem.FileData: 父目录的数据
func (m *MemMapFs) findParent(f *mem.FileData) *mem.FileData {
	pdir, _ := filepath.Split(f.Name()) // 分割目录路径和文件名
	pdir = filepath.Clean(pdir)         // 规范化目录路径
	pfile, err := m.lockfreeOpen(pdir)  // 打开目录
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil // 如果发生错误，返回 nil
	}

	return pfile // 返回父目录的数据
}

// findDescendants 查找文件的所有子孙文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - []*mem.FileData: 子孙文件的数据列表
func (m *MemMapFs) findDescendants(name string) []*mem.FileData {
	fData := m.getData()                                // 获取文件数据映射
	descendants := make([]*mem.FileData, 0, len(fData)) // 创建子孙文件的数据列表
	for p, dFile := range fData {
		if strings.HasPrefix(p, name+FilePathSeparator) { // 判断文件是否是子孙文件
			descendants = append(descendants, dFile) // 添加到子孙文件的数据列表中
		}
	}

	sort.Slice(descendants, func(i, j int) bool { // 对子孙文件按路径深度进行排序
		cur := len(strings.Split(descendants[i].Name(), FilePathSeparator))
		next := len(strings.Split(descendants[j].Name(), FilePathSeparator))
		return cur < next // 按路径深度从浅到深排序
	})

	return descendants // 返回子孙文件的数据列表
}

// registerWithParent 将文件注册到其父目录
// 参数：
//   - f: *mem.FileData 文件数据
//   - perm: os.FileMode 文件权限
func (m *MemMapFs) registerWithParent(f *mem.FileData, perm os.FileMode) {
	if f == nil {
		return // 如果文件为空，直接返回
	}

	parent := m.findParent(f) // 查找父目录
	if parent == nil {
		pdir := filepath.Dir(filepath.Clean(f.Name())) // 获取父目录路径
		err := m.lockfreeMkdir(pdir, perm)             // 创建父目录
		if err != nil {
			logger.Error("应用选项失败:", err)
			return // 如果发生错误，直接返回
		}

		parent, err = m.lockfreeOpen(pdir) // 打开父目录
		if err != nil {
			return // 如果发生错误，直接返回
		}
	}

	parent.Lock()              // 加锁
	mem.InitializeDir(parent)  // 初始化父目录
	mem.AddToMemDir(parent, f) // 将文件添加到父目录中
	parent.Unlock()            // 解锁
}

// lockfreeMkdir 创建目录（不加锁）
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) lockfreeMkdir(name string, perm os.FileMode) error {
	name = normalizePath(name) // 规范化路径
	x, ok := m.getData()[name] // 获取目录数据
	if ok {
		i := mem.FileInfo{FileData: x} // 获取目录信息
		if !i.IsDir() {
			return ErrFileExists // 如果已存在同名文件，返回文件已存在的错误
		}
	} else {
		item := mem.CreateDir(name)        // 创建目录
		mem.SetMode(item, os.ModeDir|perm) // 设置目录权限
		m.getData()[name] = item           // 将目录添加到数据映射中
		m.registerWithParent(item, perm)   // 将目录注册到其父目录
	}
	return nil // 返回 nil 表示成功
}

// Mkdir 创建目录
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) Mkdir(name string, perm os.FileMode) error {
	perm &= chmodBits          // 仅保留有效的权限位
	name = normalizePath(name) // 规范化路径

	m.mu.RLock() // 读锁
	_, ok := m.getData()[name]
	m.mu.RUnlock()
	if ok {
		return &os.PathError{Op: "mkdir", Path: name, Err: ErrFileExists} // 如果目录已存在，返回错误
	}

	m.mu.Lock() // 写锁
	// 双重检查目录是否存在
	if _, ok := m.getData()[name]; ok {
		m.mu.Unlock()
		return &os.PathError{Op: "mkdir", Path: name, Err: ErrFileExists} // 如果目录已存在，返回错误
	}
	item := mem.CreateDir(name)        // 创建目录
	mem.SetMode(item, os.ModeDir|perm) // 设置目录权限
	m.getData()[name] = item           // 将目录添加到数据映射中
	m.registerWithParent(item, perm)   // 注册目录到其父目录
	m.mu.Unlock()

	return m.setFileMode(name, perm|os.ModeDir) // 设置目录模式并返回结果
}

// MkdirAll 创建目录及其所有父目录
// 参数：
//   - path: string 目录路径
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) MkdirAll(path string, perm os.FileMode) error {
	err := m.Mkdir(path, perm) // 创建目录
	if err != nil {
		logger.Error("应用选项失败:", err)
		if err.(*os.PathError).Err == ErrFileExists {
			return nil // 如果目录已存在，返回 nil
		}
		return err // 返回错误信息
	}
	return nil
}

// normalizePath 规范化路径
// 参数：
//   - path: string 路径
//
// 返回值：
//   - string: 规范化后的路径
func normalizePath(path string) string {
	path = filepath.Clean(path) // 清理路径

	switch path {
	case ".":
		return FilePathSeparator // 返回根目录
	case "..":
		return FilePathSeparator // 返回根目录
	default:
		return path // 返回规范化后的路径
	}
}

// Open 打开文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (m *MemMapFs) Open(name string) (File, error) {
	f, err := m.open(name) // 打开文件
	if f != nil {
		return mem.NewReadOnlyFileHandle(f), err // 返回只读文件句柄和错误信息
	}
	return nil, err // 返回 nil 和错误信息
}

// openWrite 以写入模式打开文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (m *MemMapFs) openWrite(name string) (File, error) {
	f, err := m.open(name) // 打开文件
	if f != nil {
		return mem.NewFileHandle(f), err // 返回文件句柄和错误信息
	}
	return nil, err // 返回 nil 和错误信息
}

// open 打开文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - *mem.FileData: 文件数据
//   - error: 错误信息
func (m *MemMapFs) open(name string) (*mem.FileData, error) {
	name = normalizePath(name) // 规范化路径

	m.mu.RLock() // 读锁
	f, ok := m.getData()[name]
	m.mu.RUnlock()
	if !ok {
		return nil, &os.PathError{Op: "open", Path: name, Err: ErrFileNotFound} // 如果文件不存在，返回错误
	}
	return f, nil // 返回文件数据和错误信息
}

// lockfreeOpen 打开文件（不加锁）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - *mem.FileData: 文件数据
//   - error: 错误信息
func (m *MemMapFs) lockfreeOpen(name string) (*mem.FileData, error) {
	name = normalizePath(name) // 规范化路径
	f, ok := m.getData()[name]
	if ok {
		return f, nil // 返回文件数据
	} else {
		return nil, ErrFileNotFound // 如果文件不存在，返回错误
	}
}

// OpenFile 打开文件，支持指定标志和模式
// 参数：
//   - name: string 文件名
//   - flag: int 文件标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (m *MemMapFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	perm &= chmodBits // 仅保留有效的权限位
	chmod := false
	file, err := m.openWrite(name) // 以写入模式打开文件
	if err == nil && (flag&os.O_EXCL > 0) {
		return nil, &os.PathError{Op: "open", Path: name, Err: ErrFileExists} // 如果文件已存在，返回错误
	}
	if os.IsNotExist(err) && (flag&os.O_CREATE > 0) {
		file, err = m.Create(name) // 创建文件
		chmod = true
	}
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 返回错误信息
	}

	if flag == os.O_RDONLY {
		file = mem.NewReadOnlyFileHandle(file.(*mem.File).Data()) // 返回只读文件句柄
	}

	if flag&os.O_APPEND > 0 {
		_, err = file.Seek(0, io.SeekEnd) // 移动文件指针到文件末尾
		if err != nil {
			file.Close()    // 关闭文件
			return nil, err // 返回错误信息
		}
	}

	if flag&os.O_TRUNC > 0 && flag&(os.O_RDWR|os.O_WRONLY) > 0 {
		err = file.Truncate(0) // 截断文件
		if err != nil {
			logger.Error("应用选项失败:", err)
			file.Close()    // 关闭文件
			return nil, err // 返回错误信息
		}
	}

	if chmod {
		return file, m.setFileMode(name, perm) // 设置文件模式并返回结果
	}
	return file, nil // 返回文件句柄和错误信息
}

// Remove 删除文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) Remove(name string) error {
	name = normalizePath(name) // 规范化路径

	m.mu.Lock() // 加锁
	defer m.mu.Unlock()

	if _, ok := m.getData()[name]; ok {
		err := m.unRegisterWithParent(name) // 从父目录中注销文件
		if err != nil {
			logger.Error("应用选项失败:", err)
			return &os.PathError{Op: "remove", Path: name, Err: err} // 返回错误信息
		}
		delete(m.getData(), name) // 从数据映射中删除文件
	} else {
		return &os.PathError{Op: "remove", Path: name, Err: os.ErrNotExist} // 如果文件不存在，返回错误
	}
	return nil // 返回 nil 表示成功
}

// RemoveAll 删除指定路径及其所有子路径的所有文件
// 参数：
//   - path: string 路径
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) RemoveAll(path string) error {
	path = normalizePath(path)   // 规范化路径
	m.mu.Lock()                  // 加锁
	m.unRegisterWithParent(path) // 从父目录中注销路径
	m.mu.Unlock()                // 解锁

	m.mu.RLock()         // 读锁
	defer m.mu.RUnlock() // 函数结束时解锁

	for p := range m.getData() { // 遍历文件数据
		if p == path || strings.HasPrefix(p, path+FilePathSeparator) { // 如果路径匹配
			m.mu.RUnlock()         // 解锁读锁
			m.mu.Lock()            // 加锁
			delete(m.getData(), p) // 删除文件数据
			m.mu.Unlock()          // 解锁
			m.mu.RLock()           // 读锁
		}
	}
	return nil // 返回 nil 表示成功
}

// Rename 重命名文件或目录
// 参数：
//   - oldname: string 旧名称
//   - newname: string 新名称
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) Rename(oldname, newname string) error {
	oldname = normalizePath(oldname) // 规范化旧名称
	newname = normalizePath(newname) // 规范化新名称

	if oldname == newname {
		return nil // 如果新旧名称相同，直接返回
	}

	m.mu.RLock()                           // 读锁
	defer m.mu.RUnlock()                   // 函数结束时解锁
	if _, ok := m.getData()[oldname]; ok { // 如果旧名称存在
		m.mu.RUnlock()                         // 解锁读锁
		m.mu.Lock()                            // 加锁
		err := m.unRegisterWithParent(oldname) // 从父目录中注销旧名称
		if err != nil {
			logger.Error("应用选项失败:", err)
			return err // 返回错误信息
		}

		fileData := m.getData()[oldname]      // 获取旧名称的数据
		mem.ChangeFileName(fileData, newname) // 修改文件名称
		m.getData()[newname] = fileData       // 更新数据映射

		err = m.renameDescendants(oldname, newname) // 重命名子孙文件
		if err != nil {
			logger.Error("应用选项失败:", err)
			return err // 返回错误信息
		}

		delete(m.getData(), oldname) // 删除旧名称的数据

		m.registerWithParent(fileData, 0) // 将文件注册到其父目录
		m.mu.Unlock()                     // 解锁
		m.mu.RLock()                      // 读锁
	} else {
		return &os.PathError{Op: "rename", Path: oldname, Err: ErrFileNotFound} // 如果旧名称不存在，返回错误
	}
	return nil // 返回 nil 表示成功
}

// renameDescendants 重命名子孙文件
// 参数：
//   - oldname: string 旧名称
//   - newname: string 新名称
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) renameDescendants(oldname, newname string) error {
	descendants := m.findDescendants(oldname)      // 查找子孙文件
	removes := make([]string, 0, len(descendants)) // 创建删除列表
	for _, desc := range descendants {             // 遍历子孙文件
		descNewName := strings.Replace(desc.Name(), oldname, newname, 1) // 替换旧名称为新名称
		err := m.unRegisterWithParent(desc.Name())                       // 从父目录中注销子孙文件
		if err != nil {
			logger.Error("应用选项失败:", err)
			return err // 返回错误信息
		}

		removes = append(removes, desc.Name()) // 添加到删除列表
		mem.ChangeFileName(desc, descNewName)  // 修改文件名称
		m.getData()[descNewName] = desc        // 更新数据映射

		m.registerWithParent(desc, 0) // 将文件注册到其父目录
	}
	for _, r := range removes { // 删除旧名称的数据
		delete(m.getData(), r)
	}

	return nil // 返回 nil 表示成功
}

// LstatIfPossible 获取文件信息（尽可能调用 Lstat）
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - bool: 是否调用了 Lstat
//   - error: 错误信息
func (m *MemMapFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	fileInfo, err := m.Stat(name) // 获取文件信息
	return fileInfo, false, err   // 返回文件信息、是否调用了 Lstat 和错误信息
}

// Stat 获取文件信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (m *MemMapFs) Stat(name string) (os.FileInfo, error) {
	f, err := m.Open(name) // 打开文件
	if err != nil {
		logger.Error("应用选项失败:", err)
		return nil, err // 返回错误信息
	}
	fi := mem.GetFileInfo(f.(*mem.File).Data()) // 获取文件信息
	return fi, nil                              // 返回文件信息
}

// Chmod 更改文件权限
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) Chmod(name string, mode os.FileMode) error {
	mode &= chmodBits // 仅保留有效的权限位

	m.mu.RLock() // 读锁
	f, ok := m.getData()[name]
	m.mu.RUnlock()
	if !ok {
		return &os.PathError{Op: "chmod", Path: name, Err: ErrFileNotFound} // 如果文件不存在，返回错误
	}
	prevOtherBits := mem.GetFileInfo(f).Mode() & ^chmodBits // 获取之前的权限位

	mode = prevOtherBits | mode      // 合并新的权限位
	return m.setFileMode(name, mode) // 设置文件模式并返回结果
}

// setFileMode 设置文件模式
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) setFileMode(name string, mode os.FileMode) error {
	name = normalizePath(name) // 规范化路径

	m.mu.RLock() // 读锁
	f, ok := m.getData()[name]
	m.mu.RUnlock()
	if !ok {
		return &os.PathError{Op: "chmod", Path: name, Err: ErrFileNotFound} // 如果文件不存在，返回错误
	}

	m.mu.Lock()          // 写锁
	mem.SetMode(f, mode) // 设置文件模式
	m.mu.Unlock()        // 解锁

	return nil // 返回 nil 表示成功
}

// Chown 更改文件所有者
// 参数：
//   - name: string 文件名
//   - uid: int 用户 ID
//   - gid: int 组 ID
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) Chown(name string, uid, gid int) error {
	name = normalizePath(name) // 规范化路径

	m.mu.RLock() // 读锁
	f, ok := m.getData()[name]
	m.mu.RUnlock()
	if !ok {
		return &os.PathError{Op: "chown", Path: name, Err: ErrFileNotFound} // 如果文件不存在，返回错误
	}

	mem.SetUID(f, uid) // 设置文件的用户 ID
	mem.SetGID(f, gid) // 设置文件的组 ID

	return nil // 返回 nil 表示成功
}

// Chtimes 更改文件访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (m *MemMapFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	name = normalizePath(name) // 规范化路径

	m.mu.RLock() // 读锁
	f, ok := m.getData()[name]
	m.mu.RUnlock()
	if !ok {
		return &os.PathError{Op: "chtimes", Path: name, Err: ErrFileNotFound} // 如果文件不存在，返回错误
	}

	m.mu.Lock()              // 写锁
	mem.SetModTime(f, mtime) // 设置文件的修改时间
	m.mu.Unlock()            // 解锁

	return nil // 返回 nil 表示成功
}

// List 列出所有文件和目录
func (m *MemMapFs) List() {
	for _, x := range m.data { // 遍历文件数据
		y := mem.FileInfo{FileData: x}  // 获取文件信息
		fmt.Println(x.Name(), y.Size()) // 输出文件名称和大小
	}
}
