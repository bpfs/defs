// Package database 提供数据库相关的操作功能
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/badgerhold"

	"github.com/dgraph-io/badger/v4"
)

const (
	BackupFileName = "badgerhold_backup.bak" // 备份文件名
)

// CheckDatabaseStatus 检查数据库是否损坏
// 参数：
//   - store: *badgerhold.Store 表示数据库实例
//
// 返回值：
//   - error: 如果数据库损坏或无法访问，返回错误信息
func CheckDatabaseStatus(store *badgerhold.Store) error {
	logger.Infof("开始检查数据库状态...")

	if store == nil {
		return fmt.Errorf("数据库实例为空")
	}

	// 检查数据库是否已关闭
	if store.Badger() == nil {
		return fmt.Errorf("数据库已关闭")
	}

	// 尝试运行垃圾回收
	err := store.Badger().RunValueLogGC(0.5)
	if err != nil && err != badger.ErrNoRewrite {
		logger.Errorf("执行数据库垃圾回收失败: %v", err)
		return err
	}

	// 尝试进行读写测试
	err = store.Badger().Update(func(txn *badger.Txn) error {
		// 写入测试键值对
		testKey := []byte("_test_db_status")
		if err := txn.Set(testKey, []byte("test")); err != nil {
			logger.Errorf("写入测试数据失败: %v", err)
			return err
		}

		// 读取测试键值对
		_, err := txn.Get(testKey)
		if err != nil {
			logger.Errorf("读取测试数据失败: %v", err)
			return err
		}

		// 删除测试键值对
		if err := txn.Delete(testKey); err != nil {
			logger.Errorf("删除测试数据失败: %v", err)
			return err
		}

		return nil
	})

	if err != nil {
		logger.Errorf("数据库读写测试失败: %v", err)
		return fmt.Errorf("数据库状态异常: %v", err)
	}

	logger.Infof("数据库状态检查完成，状态正常")
	return nil
}

// BackupDatabase 备份数据库到指定文件
// 参数：
//   - store: *badgerhold.Store 表示数据库实例
//   - backupPath: string 表示备份文件的路径
//
// 返回值：
//   - error: 如果备份过程中出现错误，返回错误信息
func BackupDatabase(store *badgerhold.Store, backupDir string) error {
	logger.Infof("开始备份数据库到目录: %s", backupDir)

	// 1. 验证输入参数
	if store == nil {
		return fmt.Errorf("数据库实例为空")
	}
	if backupDir == "" {
		return fmt.Errorf("备份目录路径为空")
	}

	// 2. 确保备份目录存在
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		logger.Errorf("创建备份目录失败: %v, 路径: %s", err, backupDir)
		return fmt.Errorf("创建备份目录失败: %v", err)
	}

	// 3. 生成备份文件路径
	backupPath := filepath.Join(backupDir, BackupFileName)
	logger.Infof("备份文件路径: %s", backupPath)

	// 4. 检查数据库状态
	if err := CheckDatabaseStatus(store); err != nil {
		logger.Errorf("数据库状态检查失败: %v", err)
		return fmt.Errorf("数据库状态检查失败: %v", err)
	}

	// 5. 创建临时备份文件
	tempBackupPath := backupPath + ".tmp"
	backupFile, err := os.Create(tempBackupPath)
	if err != nil {
		logger.Errorf("创建临时备份文件失败: %v, 路径: %s", err, tempBackupPath)
		return fmt.Errorf("创建临时备份文件失败: %v", err)
	}
	defer func() {
		if err := backupFile.Close(); err != nil {
			logger.Errorf("关闭临时备份文件失败: %v", err)
		}
		// 如果出错，清理临时文件
		if err != nil {
			if removeErr := os.Remove(tempBackupPath); removeErr != nil {
				logger.Errorf("清理临时备份文件失败: %v", removeErr)
			}
		}
	}()

	// 6. 执行备份
	logger.Infof("开始执行数据库备份...")
	written, err := store.Badger().Backup(backupFile, 0)
	if err != nil {
		logger.Errorf("备份数据库失败: %v, 已写入: %d bytes", err, written)
		return fmt.Errorf("备份数据库失败: %v", err)
	}
	logger.Infof("成功写入备份数据: %d bytes", written)

	// 7. 同步文件
	if err := backupFile.Sync(); err != nil {
		logger.Errorf("同步临时备份文件失败: %v", err)
		return fmt.Errorf("同步临时备份文件失败: %v", err)
	}

	// 8. 使用重试机制替换旧备份文件
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			logger.Infof("第 %d 次尝试替换备份文件", i+1)
		}

		err := os.Rename(tempBackupPath, backupPath)
		if err == nil {
			logger.Infof("备份完成，文件已保存到: %s", backupPath)
			return nil
		}

		if !isFileLockedErr(err) {
			logger.Errorf("替换备份文件失败(非锁定错误): %v", err)
			return fmt.Errorf("替换备份文件失败: %v", err)
		}

		waitTime := time.Second * time.Duration(1<<i)
		logger.Infof("文件被锁定，等待 %v 后重试", waitTime)
		time.Sleep(waitTime)
	}

	logger.Errorf("多次尝试替换备份文件失败，已达到最大重试次数")
	return fmt.Errorf("多次尝试替换备份文件失败，已达到最大重试次数")
}

// isFileLockedErr 判断是否由于文件被占用导致错误
// 参数：
//   - err: error 表示错误信息
//
// 返回值：
//   - bool: 如果是文件锁定错误，返回 true，否则返回 false
func isFileLockedErr(err error) bool {
	return os.IsExist(err) || strings.Contains(err.Error(), "being used by another process")
}

// RestoreDatabaseFromBackup 从备份文件恢复数据库
// 参数：
//   - backupPath: string 表示备份文件的路径
//   - dbPath: string 表示数据库文件的路径
//
// 返回值：
//   - error: 如果恢复过程中出现错误，返回错误信息
func RestoreDatabaseFromBackup(options badgerhold.Options, store *badgerhold.Store, backupPath, dbPath, valueDirPath string) error {
	// 检查备份文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		logger.Errorf(" 备份文件不存在: %v", err)
		return fmt.Errorf("备份文件不存在: %v", err)
	}

	// 关闭当前数据库 badgerhold
	if err := store.Close(); err != nil {
		logger.Errorf(" 关闭数据库时出错: %v", err)
		return err
	}

	// 删除当前数据库文件夹下的所有内容
	files, err := os.ReadDir(dbPath)
	if err != nil {
		logger.Errorf(" 读取数据库目录失败: %v", err)
		return fmt.Errorf("读取数据库目录失败: %v", err)
	}
	for _, file := range files {
		if err := os.RemoveAll(filepath.Join(dbPath, file.Name())); err != nil {
			logger.Errorf(" 删除数据库文件失败: %v", err)
			return fmt.Errorf("删除数据库文件失败: %v", err)
		}
	}

	// 删除当前 valueDirPath 文件夹下的所有内容
	files, err = os.ReadDir(valueDirPath)
	if err != nil {
		logger.Errorf(" 读取 value 目录失败: %v", err)
		return fmt.Errorf("读取 value 目录失败: %v", err)
	}
	for _, file := range files {
		if err := os.RemoveAll(filepath.Join(valueDirPath, file.Name())); err != nil {
			logger.Errorf(" 删除 value 文件失败: %v", err)
			return fmt.Errorf("删除 value 文件失败: %v", err)
		}
	}

	// 打开备份文件
	backupFile, err := os.Open(filepath.Join(backupPath, BackupFileName))
	if err != nil {
		logger.Errorf(" 无法打开备份文件: %v", err)
		return fmt.Errorf("备份文件不存在")
	}
	defer backupFile.Close()

	// 重新打开数据库
	newStore, err := badgerhold.Open(options) // 使用适当的选项重新打开数据库
	if err != nil {
		logger.Errorf(" 打开数据库时出错: %v", err)
		return err
	}
	defer newStore.Close() // 确保在函数结束时关闭新数据库

	// maxPendingWrites 表示在恢复过程中允许的最大待处理写入操作数。
	// 从备份恢复数据库
	err = newStore.Badger().Load(backupFile, 1000) // maxPendingWrites 设置为 1000
	if err != nil {
		logger.Errorf(" 恢复数据库时出错: %v", err)
		return err
	}

	logger.Infof(" 数据库恢复成功！")
	return nil
}
