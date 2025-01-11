package afero

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// 确保 CopyOnWriteFs 实现了 Lstater 接口
var _ Lstater = (*CopyOnWriteFs)(nil)

// CopyOnWriteFs 是一个联合文件系统：一个只读的基础文件系统，
// 在其上可能有一个可写的层。对文件系统的更改只会在覆盖层中进行：
// 更改基础层中存在但覆盖层中不存在的文件会将文件复制到覆盖层
// （"更改"包括调用例如 Chtimes()、Chmod() 和 Chown() 等函数）。
//
// 读取目录当前仅通过 Open() 支持，而不是 OpenFile()。
type CopyOnWriteFs struct {
	base  Afero // 基础文件系统
	layer Afero // 覆盖层文件系统
}

// NewCopyOnWriteFs 创建一个新的 CopyOnWriteFs
// 参数：
//   - base: Afero 基础文件系统
//   - layer: Afero 覆盖层文件系统
//
// 返回值：
//   - Afero: 新的 CopyOnWriteFs
func NewCopyOnWriteFs(base Afero, layer Afero) Afero {
	return &CopyOnWriteFs{base: base, layer: layer}
}

// isBaseFile 判断文件是否在基础层中
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - bool: 文件是否在基础层中
//   - error: 错误信息
func (u *CopyOnWriteFs) isBaseFile(name string) (bool, error) {
	if _, err := u.layer.Stat(name); err == nil { // 检查文件是否在覆盖层中
		return false, nil // 如果在覆盖层中，返回 false
	}
	_, err := u.base.Stat(name) // 检查文件是否在基础层中
	if err != nil {
		logger.Error("检查文件是否在基础层中失败:", err)
		if oerr, ok := err.(*os.PathError); ok {
			if oerr.Err == os.ErrNotExist || oerr.Err == syscall.ENOENT || oerr.Err == syscall.ENOTDIR {
				return false, nil // 如果文件不存在，返回 false
			}
		}
		if err == syscall.ENOENT {
			return false, nil // 如果文件不存在，返回 false
		}
	}
	return true, err // 如果在基础层中，返回 true 和错误信息
}

// copyToLayer 将文件从基础层复制到覆盖层
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - error: 错误信息
func (u *CopyOnWriteFs) copyToLayer(name string) error {
	return copyToLayer(u.base, u.layer, name) // 调用 copyToLayer 函数
}

// Chtimes 更改指定文件的访问和修改时间
// 参数：
//   - name: string 文件名
//   - atime: time.Time 访问时间
//   - mtime: time.Time 修改时间
//
// 返回值：
//   - error: 错误信息
func (u *CopyOnWriteFs) Chtimes(name string, atime, mtime time.Time) error {
	b, err := u.isBaseFile(name) // 判断文件是否在基础层中
	if err != nil {
		logger.Error("判断文件是否在基础层中失败:", err)
		return err // 如果发生错误，返回错误信息
	}

	if b {
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("将文件复制到覆盖层失败:", err)
			return err // 如果发生错误，返回错误信息
		}
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
func (u *CopyOnWriteFs) Chmod(name string, mode os.FileMode) error {
	b, err := u.isBaseFile(name) // 判断文件是否在基础层中
	if err != nil {
		logger.Error("判断文件是否在基础层中失败:", err)
		return err // 如果发生错误，返回错误信息
	}

	if b {
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("将文件复制到覆盖层失败:", err)
			return err // 如果发生错误，返回错误信息
		}
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
func (u *CopyOnWriteFs) Chown(name string, uid, gid int) error {
	b, err := u.isBaseFile(name) // 判断文件是否在基础层中
	if err != nil {
		logger.Error("判断文件是否在基础层中失败:", err)
		return err // 如果发生错误，返回错误信息
	}

	if b {
		if err := u.copyToLayer(name); err != nil { // 将文件复制到覆盖层
			logger.Error("将文件复制到覆盖层失败:", err)
			return err // 如果发生错误，返回错误信息
		}
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
func (u *CopyOnWriteFs) Stat(name string) (os.FileInfo, error) {
	fi, err := u.layer.Stat(name) // 获取覆盖层中文件的信息
	if err != nil {
		logger.Error("获取覆盖层中文件信息失败:", err)
		isNotExist := u.isNotExist(err) // 判断错误是否是文件不存在
		if isNotExist {
			return u.base.Stat(name) // 获取基础层中文件的信息
		}
		return nil, err // 返回错误信息
	}
	return fi, nil // 返回覆盖层中文件的信息
}

// LstatIfPossible 返回文件的信息和一个布尔值，指示是否使用了 Lstat
// 参数：
//   - name: string 文件名
//
// 返回值：
//   - os.FileInfo: 文件信息
//   - bool: 是否使用了 Lstat
//   - error: 错误信息
func (u *CopyOnWriteFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	llayer, ok1 := u.layer.(Lstater) // 判断覆盖层是否实现了 Lstater 接口
	lbase, ok2 := u.base.(Lstater)   // 判断基础层是否实现了 Lstater 接口

	if ok1 {
		fi, b, err := llayer.LstatIfPossible(name) // 获取覆盖层中文件的信息
		if err == nil {
			return fi, b, nil // 返回文件的信息和布尔值
		}

		if !u.isNotExist(err) {
			return nil, b, err // 返回错误信息
		}
	}

	if ok2 {
		fi, b, err := lbase.LstatIfPossible(name) // 获取基础层中文件的信息
		if err == nil {
			return fi, b, nil // 返回文件的信息和布尔值
		}
		if !u.isNotExist(err) {
			return nil, b, err // 返回错误信息
		}
	}

	fi, err := u.Stat(name) // 获取文件的信息

	return fi, false, err // 返回文件的信息和布尔值
}

// SymlinkIfPossible 创建符号链接（如果可能）
// 参数：
//   - oldname: string 旧文件名
//   - newname: string 新文件名
//
// 返回值：
//   - error: 错误信息
func (u *CopyOnWriteFs) SymlinkIfPossible(oldname, newname string) error {
	if slayer, ok := u.layer.(Linker); ok {
		return slayer.SymlinkIfPossible(oldname, newname) // 在覆盖层中创建符号链接
	}

	return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: ErrNoSymlink} // 返回错误信息
}

// ReadlinkIfPossible 尝试读取符号链接。
// 参数：
//   - name: string 符号链接的路径。
//
// 返回值：
//   - string: 符号链接指向的路径。
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) ReadlinkIfPossible(name string) (string, error) {
	// 检查 layer 是否实现了 LinkReader 接口
	if rlayer, ok := u.layer.(LinkReader); ok {
		// 如果实现了，调用 layer 的 ReadlinkIfPossible 方法
		return rlayer.ReadlinkIfPossible(name)
	}

	// 检查 base 是否实现了 LinkReader 接口
	if rbase, ok := u.base.(LinkReader); ok {
		// 如果实现了，调用 base 的 ReadlinkIfPossible 方法
		return rbase.ReadlinkIfPossible(name)
	}

	// 如果 layer 和 base 都没有实现 LinkReader 接口，返回错误
	return "", &os.PathError{Op: "readlink", Path: name, Err: ErrNoReadlink}
}

// isNotExist 判断错误是否是文件不存在的错误。
// 参数：
//   - err: error 错误信息。
//
// 返回值：
//   - bool: 如果文件不存在，返回 true，否则返回 false。
func (u *CopyOnWriteFs) isNotExist(err error) bool {
	// 检查错误是否是 os.PathError 类型
	if e, ok := err.(*os.PathError); ok {
		err = e.Err // 获取底层错误
	}
	// 判断错误是否是文件不存在相关的错误
	if err == os.ErrNotExist || err == syscall.ENOENT || err == syscall.ENOTDIR {
		return true
	}
	return false
}

// Rename 重命名文件。如果文件仅存在于基础层，则不允许重命名。
// 参数：
//   - oldname: string 旧文件名。
//   - newname: string 新文件名。
//
// 返回值：
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) Rename(oldname, newname string) error {
	// 检查文件是否在基础层
	b, err := u.isBaseFile(oldname)
	if err != nil {
		logger.Error("检查文件是否在基础层失败:", err)
		return err
	}

	// 如果文件在基础层，则不允许重命名
	if b {
		return syscall.EPERM
	}
	// 调用 layer 的 Rename 方法重命名文件
	return u.layer.Rename(oldname, newname)
}

// Remove 删除文件。如果文件仅存在于基础层，则不允许删除。
// 参数：
//   - name: string 文件名。
//
// 返回值：
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) Remove(name string) error {
	// 尝试删除 layer 中的文件
	err := u.layer.Remove(name)
	switch err {
	case syscall.ENOENT:
		// 如果在 layer 中未找到文件，检查 base 中是否存在
		_, err = u.base.Stat(name)
		if err == nil {
			// 如果文件存在于 base 中，不允许删除
			return syscall.EPERM
		}
		// 文件在 layer 和 base 中都不存在，返回 ENOENT 错误
		return syscall.ENOENT
	default:
		// 返回删除文件时遇到的其他错误
		return err
	}
}

// RemoveAll 删除目录及其内容。如果目录仅存在于基础层，则不允许删除。
// 参数：
//   - name: string 目录名。
//
// 返回值：
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) RemoveAll(name string) error {
	// 尝试删除 layer 中的目录及其内容
	err := u.layer.RemoveAll(name)
	switch err {
	case syscall.ENOENT:
		// 如果在 layer 中未找到目录，检查 base 中是否存在
		_, err = u.base.Stat(name)
		if err == nil {
			// 如果目录存在于 base 中，不允许删除
			return syscall.EPERM
		}
		// 目录在 layer 和 base 中都不存在，返回 ENOENT 错误
		return syscall.ENOENT
	default:
		// 返回删除目录时遇到的其他错误
		return err
	}
}

// OpenFile 打开文件。如果文件存在于基础层且需要写操作，则复制到覆盖层。
// 参数：
//   - name: string 文件名。
//   - flag: int 打开文件的标志。
//   - perm: os.FileMode 文件权限。
//
// 返回值：
//   - File: 打开的文件。
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	// 检查文件是否在基础层
	b, err := u.isBaseFile(name)
	if err != nil {
		logger.Error("检查文件是否在基础层失败:", err)
		return nil, err
	}

	// 如果需要写操作，并且文件在基础层
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		if b {
			// 将文件从基础层复制到覆盖层
			if err = u.copyToLayer(name); err != nil {
				logger.Error("将文件从基础层复制到覆盖层失败:", err)
				return nil, err
			}
			// 打开覆盖层中的文件
			return u.layer.OpenFile(name, flag, perm)
		}

		// 检查目录是否存在于基础层
		dir := filepath.Dir(name)
		isaDir, err := IsDir(u.base, dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if isaDir {
			// 创建覆盖层中的目录
			if err = u.layer.MkdirAll(dir, 0o777); err != nil {
				logger.Error("创建覆盖层中的目录失败:", err)
				return nil, err
			}
			// 打开覆盖层中的文件
			return u.layer.OpenFile(name, flag, perm)
		}

		// 检查目录是否存在于覆盖层
		isaDir, err = IsDir(u.layer, dir)
		if err != nil {
			logger.Error("检查目录是否存在于覆盖层失败:", err)
			return nil, err
		}
		if isaDir {
			return u.layer.OpenFile(name, flag, perm)
		}

		// 如果目录不存在，返回错误
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ENOTDIR}
	}
	// 如果文件在基础层，打开基础层中的文件
	if b {
		return u.base.OpenFile(name, flag, perm)
	}
	// 否则，打开覆盖层中的文件
	return u.layer.OpenFile(name, flag, perm)
}

// Open 打开文件或目录。
// 参数：
//   - name: string 文件或目录名。
//
// 返回值：
//   - File: 打开的文件或目录。
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) Open(name string) (File, error) {
	// 检查文件是否在基础层
	b, err := u.isBaseFile(name)
	if err != nil {
		logger.Error("检查文件是否在基础层失败:", err)
		return nil, err
	}

	// 如果文件在基础层，返回基础层中的文件
	if b {
		return u.base.Open(name)
	}

	// 检查覆盖层中的文件是否是目录
	dir, err := IsDir(u.layer, name)
	if err != nil {
		logger.Error("检查覆盖层中的文件是否是目录失败:", err)
		return nil, err
	}
	if !dir {
		return u.layer.Open(name)
	}

	// 检查基础层中的文件是否是目录
	dir, err = IsDir(u.base, name)
	if !dir || err != nil {
		return u.layer.Open(name)
	}

	// 如果基础层和覆盖层都是目录，返回联合目录
	bfile, bErr := u.base.Open(name)
	lfile, lErr := u.layer.Open(name)

	if bErr != nil || lErr != nil {
		return nil, fmt.Errorf("BaseErr: %v\nOverlayErr: %v", bErr, lErr)
	}

	return &UnionFile{Base: bfile, Layer: lfile}, nil
}

// Mkdir 创建目录。
// 参数：
//   - name: string 目录名。
//   - perm: os.FileMode 目录权限。
//
// 返回值：
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) Mkdir(name string, perm os.FileMode) error {
	// 检查基础层中是否存在同名目录
	dir, err := IsDir(u.base, name)
	if err != nil {
		logger.Error("检查基础层中是否存在同名目录失败:", err)
		return u.layer.MkdirAll(name, perm)
	}

	if dir {
		return ErrFileExists
	}

	return u.layer.MkdirAll(name, perm)
}

// Name 返回文件系统的名称。
// 返回值：
//   - string: 文件系统名称。
func (u *CopyOnWriteFs) Name() string {
	return "CopyOnWriteFs"
}

// MkdirAll 创建目录及其所有父目录。
// 参数：
//   - name: string 目录名。
//   - perm: os.FileMode 目录权限。
//
// 返回值：
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) MkdirAll(name string, perm os.FileMode) error {
	// 检查基础层中是否存在同名目录
	dir, err := IsDir(u.base, name)
	if err != nil {
		logger.Error("检查基础层中是否存在同名目录失败:", err)
		return u.layer.MkdirAll(name, perm)
	}

	if dir {
		// 与 os.MkdirAll 的行为保持一致
		return nil
	}
	return u.layer.MkdirAll(name, perm)
}

// Create 创建新文件。
// 参数：
//   - name: string 文件名。
//
// 返回值：
//   - File: 新创建的文件。
//   - error: 如果发生错误，返回错误信息。
func (u *CopyOnWriteFs) Create(name string) (File, error) {
	return u.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o666)
}
