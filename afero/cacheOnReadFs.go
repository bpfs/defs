package afero

import (
	"os"
	"syscall"
	"time"

	"github.com/bpfs/defs/utils/logger"
)

// CacheOnReadFs 类型用于在读取时进行缓存
// 如果缓存时间为 0，则缓存时间将是无限的，即一旦文件进入层中，基础文件系统将不再读取该文件。
// 对于大于 0 的缓存时间，将检查文件的修改时间。
// 注意：许多文件系统实现只允许时间戳的精度为秒。

// 此缓存联合将所有写调用也转发到基础文件系统。
// 为防止写入基础文件系统，请将其包装在只读过滤器中。
// 注意：这也会使覆盖层变为只读，要在覆盖层中写入文件，请直接使用覆盖文件系统，而不是通过联合文件系统。
type CacheOnReadFs struct {
	base      Afero         // 基础文件系统
	layer     Afero         // 缓存层文件系统
	cacheTime time.Duration // 缓存时间
}

// NewCacheOnReadFs 创建一个新的 CacheOnReadFs
// 参数：
//   - base: Fs 基础文件系统
//   - layer: Fs 缓存层文件系统
//   - cacheTime: time.Duration 缓存时间
//
// 返回值：
//   - Fs: 新的 CacheOnReadFs
func NewCacheOnReadFs(base Afero, layer Afero, cacheTime time.Duration) Afero {
	return &CacheOnReadFs{base: base, layer: layer, cacheTime: cacheTime}
}

// cacheState 表示缓存状态
type cacheState int

const (
	// 不在覆盖层中，未知是否存在于基础层：
	cacheMiss cacheState = iota
	// 存在于覆盖层和基础层，基础文件更新：
	cacheStale
	// 存在于覆盖层 - 当 cacheTime == 0 时，它可能存在于基础层，
	// 当 cacheTime > 0 时，它存在于基础层，并且在覆盖层中同年龄或更新
	cacheHit
	// 发生在直接写入覆盖层而不通过此联合时
	cacheLocal
)

// cacheStatus 获取文件的缓存状态
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - cacheState: 缓存状态
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (u *CacheOnReadFs) cacheStatus(name string) (state cacheState, fi os.FileInfo, err error) {
	var lfi, bfi os.FileInfo
	lfi, err = u.layer.Stat(name) // 在缓存层中获取文件信息
	if err == nil {
		if u.cacheTime == 0 { // 如果缓存时间为 0，则返回 cacheHit
			return cacheHit, lfi, nil
		}
		if lfi.ModTime().Add(u.cacheTime).Before(time.Now()) { // 检查文件是否过期
			bfi, err = u.base.Stat(name) // 在基础层中获取文件信息
			if err != nil {
				logger.Error("获取基础层文件信息失败:", err)
				return cacheLocal, lfi, nil
			}
			if bfi.ModTime().After(lfi.ModTime()) { // 如果基础层中文件更新，则返回 cacheStale
				return cacheStale, bfi, nil
			}
		}
		return cacheHit, lfi, nil // 返回 cacheHit
	}

	if err == syscall.ENOENT || os.IsNotExist(err) { // 如果文件不存在，则返回 cacheMiss
		return cacheMiss, nil, nil
	}

	return cacheMiss, nil, err // 返回 cacheMiss
}

// copyToLayer 将文件从基础层复制到覆盖层
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) copyToLayer(name string) error {
	return copyToLayer(u.base, u.layer, name) // 调用 copyToLayer 函数
}

// copyFileToLayer 将文件从基础层复制到覆盖层，并指定标志和权限
// 参数：
//   - name: string 文件名
//   - flag: int 文件标志
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) copyFileToLayer(name string, flag int, perm os.FileMode) error {
	return copyFileToLayer(u.base, u.layer, name, flag, perm) // 调用 copyFileToLayer 函数
}

// Chtimes 更改指定文件的访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) Chtimes(name string, atime, mtime time.Time) error {
	st, _, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return err
	}

	switch st {
	case cacheLocal:
	case cacheHit:
		err = u.base.Chtimes(name, atime, mtime) // 更改基础层中的访问和修改时间
	case cacheStale, cacheMiss:
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("复制文件到覆盖层失败:", err)
			return err
		}
		err = u.base.Chtimes(name, atime, mtime) // 更改基础层中的访问和修改时间
	}

	if err != nil {
		logger.Error("更改基础层文件时间失败:", err)
		return err
	}
	return u.layer.Chtimes(name, atime, mtime) // 更改覆盖层中的访问和修改时间
}

// Chmod 更改指定文件的模式
// 参数：
//   - name: string 文件名
//   - mode: os.FileMode 文件模式
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) Chmod(name string, mode os.FileMode) error {
	st, _, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return err
	}

	switch st {
	case cacheLocal:
	case cacheHit:
		err = u.base.Chmod(name, mode) // 更改基础层中的文件模式
	case cacheStale, cacheMiss:
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("复制文件到覆盖层失败:", err)
			return err
		}
		err = u.base.Chmod(name, mode) // 更改基础层中的文件模式
	}

	if err != nil {
		logger.Error("更改基础层文件模式失败:", err)
		return err
	}
	return u.layer.Chmod(name, mode) // 更改覆盖层中的文件模式
}

// Chown 更改指定文件的 uid 和 gid
// 参数：
//   - name: string 文件名
//   - uid: int 用户ID
//   - gid: int 组ID
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) Chown(name string, uid, gid int) error {
	st, _, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return err
	}

	switch st {
	case cacheLocal:
	case cacheHit:
		err = u.base.Chown(name, uid, gid) // 更改基础层中的 uid 和 gid
	case cacheStale, cacheMiss:
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("复制文件到覆盖层失败:", err)
			return err
		}
		err = u.base.Chown(name, uid, gid) // 更改基础层中的 uid 和 gid
	}

	if err != nil {
		logger.Error("更改基础层文件所有者失败:", err)
		return err
	}
	return u.layer.Chown(name, uid, gid) // 更改覆盖层中的 uid 和 gid
}

// Stat 返回指定文件的文件信息
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - error: 错误信息
func (u *CacheOnReadFs) Stat(name string) (os.FileInfo, error) {
	st, fi, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return nil, err
	}

	switch st {
	case cacheMiss:
		return u.base.Stat(name) // 获取基础层中的文件信息
	default: // cacheStale 包含基础层信息，cacheHit 和 cacheLocal 包含覆盖层信息
		return fi, nil // 返回文件信息
	}
}

// Rename 重命名文件
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) Rename(oldname, newname string) error {
	st, _, err := u.cacheStatus(oldname) // 获取旧文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return err // 如果发生错误，返回错误信息
	}

	switch st {
	case cacheLocal:
	case cacheHit:
		err = u.base.Rename(oldname, newname) // 在基础层重命名文件
	case cacheStale, cacheMiss:
		if err := u.copyToLayer(oldname); err != nil { // 将旧文件复制到覆盖层
			logger.Error("复制文件到覆盖层失败:", err)
			return err // 如果发生错误，返回错误信息
		}
		err = u.base.Rename(oldname, newname) // 在基础层重命名文件
	}

	if err != nil {
		logger.Error("重命名基础层文件失败:", err)
		return err // 如果发生错误，返回错误信息
	}
	return u.layer.Rename(oldname, newname) // 在覆盖层重命名文件
}

// Remove 删除指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) Remove(name string) error {
	st, _, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return err // 如果发生错误，返回错误信息
	}

	switch st {
	case cacheLocal:
	case cacheHit, cacheStale, cacheMiss:
		err = u.base.Remove(name) // 在基础层删除文件
	}

	if err != nil {
		logger.Error("删除基础层文件失败:", err)
		return err // 如果发生错误，返回错误信息
	}
	return u.layer.Remove(name) // 在覆盖层删除文件
}

// RemoveAll 删除指定路径及其包含的所有子目录
// 参数：
//   - name: string 路径名
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) RemoveAll(name string) error {
	st, _, err := u.cacheStatus(name) // 获取路径的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return err // 如果发生错误，返回错误信息
	}
	switch st {
	case cacheLocal:
	case cacheHit, cacheStale, cacheMiss:
		err = u.base.RemoveAll(name) // 在基础层删除路径及其子目录
	}

	if err != nil {
		logger.Error("删除基础层目录及其内容失败:", err)
		return err // 如果发生错误，返回错误信息
	}
	return u.layer.RemoveAll(name) // 在覆盖层删除路径及其子目录
}

// OpenFile 打开文件，支持指定标志和模式
// 参数：
//   - name: string 文件名
//   - flag: int 文件标志
//   - perm: os.FileMode 文件模式
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (u *CacheOnReadFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	st, _, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	switch st {
	case cacheLocal, cacheHit:
	default:
		if err := u.copyFileToLayer(name, flag, perm); err != nil { // 将文件复制到覆盖层
			logger.Error("复制文件到覆盖层失败:", err)
			return nil, err // 如果发生错误，返回错误信息
		}
	}

	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		bfi, err := u.base.OpenFile(name, flag, perm) // 在基础层打开文件
		if err != nil {
			logger.Error("打开基础层文件失败:", err)
			return nil, err // 如果发生错误，返回错误信息
		}
		lfi, err := u.layer.OpenFile(name, flag, perm) // 在覆盖层打开文件
		if err != nil {
			logger.Error("打开覆盖层文件失败:", err)
			bfi.Close()     // 关闭基础层文件
			return nil, err // 如果发生错误，返回错误信息
		}
		return &UnionFile{Base: bfi, Layer: lfi}, nil // 返回联合文件对象
	}
	return u.layer.OpenFile(name, flag, perm) // 在覆盖层打开文件
}

// Open 打开指定文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (u *CacheOnReadFs) Open(name string) (File, error) {
	st, fi, err := u.cacheStatus(name) // 获取文件的缓存状态
	if err != nil {
		logger.Error("获取文件缓存状态失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	switch st {
	case cacheLocal:
		return u.layer.Open(name) // 在覆盖层打开文件

	case cacheMiss:
		bfi, err := u.base.Stat(name) // 获取基础层中文件的状态
		if err != nil {
			logger.Error("获取基础层文件状态失败:", err)
			return nil, err // 如果发生错误，返回错误信息
		}
		if bfi.IsDir() {
			return u.base.Open(name) // 如果是目录，则在基础层打开
		}
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("复制文件到覆盖层失败:", err)
			return nil, err // 如果发生错误，返回错误信息
		}
		return u.layer.Open(name) // 在覆盖层打开文件

	case cacheStale:
		if !fi.IsDir() {
			if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
				logger.Error("复制文件到覆盖层失败:", err)
				return nil, err // 如果发生错误，返回错误信息
			}
			return u.layer.Open(name) // 在覆盖层打开文件
		}
	case cacheHit:
		if !fi.IsDir() {
			return u.layer.Open(name) // 在覆盖层打开文件
		}
	}
	// cacheHit 和 cacheStale 的目录处理
	bfile, _ := u.base.Open(name)    // 在基础层打开目录
	lfile, err := u.layer.Open(name) // 在覆盖层打开目录
	if err != nil && bfile == nil {
		return nil, err // 如果发生错误，返回错误信息
	}
	return &UnionFile{Base: bfile, Layer: lfile}, nil // 返回联合文件对象
}

// Mkdir 创建指定目录
// 参数：
//   - name: string 目录名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) Mkdir(name string, perm os.FileMode) error {
	err := u.base.Mkdir(name, perm) // 在基础层创建目录
	if err != nil {
		logger.Error("在基础层创建目录失败:", err)
		return err // 如果发生错误，返回错误信息
	}
	return u.layer.MkdirAll(name, perm) // 在覆盖层创建目录及其所有父目录
}

// Name 返回文件系统的名称
// 返回值：
//   - string: 文件系统名称
func (u *CacheOnReadFs) Name() string {
	return "CacheOnReadFs" // 返回文件系统名称
}

// MkdirAll 创建指定路径及其所有父目录
// 参数：
//   - name: string 路径名
//   - perm: os.FileMode 目录权限
//
// 返回值：
//   - error: 错误信息
func (u *CacheOnReadFs) MkdirAll(name string, perm os.FileMode) error {
	err := u.base.MkdirAll(name, perm) // 在基础层创建路径及其所有父目录
	if err != nil {
		logger.Error("在基础层创建目录及其父目录失败:", err)
		return err // 如果发生错误，返回错误信息
	}
	return u.layer.MkdirAll(name, perm) // 在覆盖层创建路径及其所有父目录
}

// Create 创建一个新文件
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - File: 文件对象
//   - error: 错误信息
func (u *CacheOnReadFs) Create(name string) (File, error) {
	bfh, err := u.base.Create(name) // 在基础层创建文件
	if err != nil {
		logger.Error("在基础层创建文件失败:", err)
		return nil, err // 如果发生错误，返回错误信息
	}

	lfh, err := u.layer.Create(name) // 在覆盖层创建文件
	if err != nil {
		logger.Error("在覆盖层创建文件失败:", err)
		bfh.Close()     // 关闭基础层文件
		return nil, err // 如果发生错误，返回错误信息
	}

	return &UnionFile{Base: bfh, Layer: lfh}, nil // 返回联合文件对象
}
