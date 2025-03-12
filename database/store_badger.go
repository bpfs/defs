// Package database 提供了数据库相关的功能实现
package database

import (
	"context"
	"path/filepath"

	"runtime"

	"github.com/bpfs/defs/v2/badgerhold"

	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dgraph-io/badger/v4"
)

// NewBadgerDB 创建并初始化一个新的BadgerDB实例
//
// 参数:
//   - ctx context.Context: 上下文对象，用于控制数据库生命周期
//
// 返回值:
//   - *badgerhold.Store: BadgerDB存储实例
//   - error: 如果初始化过程中发生错误，返回错误信息
func NewBadgerDB(ctx context.Context) (*badgerhold.Store, error) {
	// 配置DB路径
	badgerDBDirPath := filepath.Join(paths.GetDatabasePath(), "badgerhold")
	badgerDBValueDirPath := filepath.Join(badgerDBDirPath, "value")
	badgerDBBackupDir := filepath.Join(badgerDBDirPath, "backups")

	// 配置 Badgerhold 数据库的选项，包括数据库目录和 Value 目录
	options := badgerhold.DefaultOptions    // 获取默认配置选项
	options.Dir = badgerDBValueDirPath      // 设置数据库文件存储的目录路径
	options.ValueDir = badgerDBValueDirPath // 设置 Value 文件存储的目录路径
	options.SyncWrites = true               // 设置为同步写入模式，确保数据安全性

	/**
	  // BPFS 优化：调整 Badger 数据库参数
	  options.NumVersionsToKeep = 1 // 只保留一个版本，减少存储空间
	  // options.ValueLogFileSize = 1<<30 - 1 // 恢复默认值 ~1GB，避免文件过小导致频繁创建
	  options.ValueLogMaxEntries = 1000000 // 恢复默认值 100万条记录/文件
	  options.ValueThreshold = 1 << 10     // 1KB，小于1KB的值直接存LSM树，减少vlog写入

	  // 2. 内存和表管理配置
	  // options.MemTableSize = 64 << 20  // 保持默认 64MB
	  options.MemTableSize = 128 << 20 // 增加到128MB，提高索引写入性能
	  options.NumMemtables = 5         // 默认值，允许更多内存表缓存
	  options.MaxLevels = 7            // 默认值，保持7层LSM树结构

	  // 3. LSM树相关配置
	  options.NumLevelZeroTables = 5       // 默认值，减少 Level 0 表数量，提高写入性能
	  options.NumLevelZeroTablesStall = 15 // 默认值，增加 Level 0 表的停滞阈值，减少压缩频率
	  options.BaseLevelSize = 10 << 20     // 默认值 10MB
	  options.LevelSizeMultiplier = 10     // 默认值，每层大小是上层的10倍

	  // 4. 压缩和性能配置
	  options.NumCompactors = 4       // 默认值，4个压缩器更均衡
	  options.CompactL0OnClose = true // 建议开启，关闭时压缩可减少重启后的压力
	  options.NumVersionsToKeep = 1   // 只保留一个版本
	  options.SyncWrites = true       // 默认值，异步写入提高性能

	  // 5. 缓存配置
	  options.BlockCacheSize = 256 << 20 // 默认值 256MB
	  options.BlockSize = 4 * 1024       // 默认值 4KB

	  // 6. GC相关配置
	  options.VLogPercentile = 0.5 // 默认值，禁用自动GC，使用手动GC策略
	*/

	// 确保 Badgerhold 数据库目录已经存在，如果不存在则创建
	if err := paths.AddDirectory(badgerDBDirPath); err != nil {
		logger.Errorf(" 创建数据库目录失败: %v", err)
		return nil, err
	}

	// 确保 Value 目录已经存在，如果不存在则创建
	if err := paths.AddDirectory(badgerDBValueDirPath); err != nil {
		logger.Errorf(" 创建 Value 目录失败: %v", err)
		return nil, err
	}

	// 确保备份目录已经存在，如果不存在则创建
	if err := paths.AddDirectory(badgerDBBackupDir); err != nil {
		logger.Errorf(" 创建备份目录失败: %v", err)
		return nil, err
	}

	// 打开 Badgerhold 数据库
	store, err := badgerhold.Open(options)
	// 如果打开数据库失败
	if err != nil {
		logger.Errorf(" 打开数据库失败: %v", err)
		// 尝试从备份恢复
		restoreErr := RestoreDatabaseFromBackup(options, store, badgerDBBackupDir, badgerDBDirPath, badgerDBValueDirPath)
		if restoreErr != nil {
			logger.Errorf(" 数据库恢复失败: %v", restoreErr)
			return nil, restoreErr
		}

		// 重试打开数据库
		store, err = badgerhold.Open(options)
		if err != nil {
			logger.Errorf(" 数据库恢复后打开失败: %v", err)
			return nil, err
		}
		// logger.Infof(" 数据库已成功从备份恢复并打开")
	}

	return store, nil
}

// ForceValueLogGC 强制执行 value log 垃圾回收
//
// 参数:
//   - db *badgerhold.Store: 数据库实例
//   - ratio float64: GC触发阈值(0.0-1.0)
//
// 返回值:
//   - error: 如果GC过程中发生错误，返回错误信息
func ForceValueLogGC(db *badgerhold.Store, ratio float64) error {
	for {
		// 执行一次值日志垃圾回收
		err := db.Badger().RunValueLogGC(ratio)

		// 当没有更多的数据需要清理时
		if err == badger.ErrNoRewrite {
			// 手动触发Go的垃圾回收
			runtime.GC()
			logger.Info("手动垃圾回收完成")
			return nil
		}

		// 如果发生其他错误
		if err != nil {
			logger.Errorf("值日志垃圾回收失败: %v", err)
			return err
		}
	}
}

// ForceCleanup 强制清理数据库
//
// 参数:
//   - db *badgerhold.Store: 数据库实例
//
// 返回值:
//   - error: 如果清理过程中发生错误，返回错误信息
func ForceCleanup(db *badgerhold.Store) error {
	logger.Info("开始强制清理数据库...")

	// 1. 先执行激进GC
	if err := AggressiveValueLogGC(db); err != nil {
		return err
	}

	// 2. 执行数据库压缩
	if err := db.Badger().Flatten(1); err != nil {
		return err
	}

	// 3. 再次执行GC
	if err := AggressiveValueLogGC(db); err != nil {
		return err
	}

	return nil
}

// AggressiveValueLogGC 执行激进的值日志垃圾回收
//
// 参数:
//   - db *badgerhold.Store: 数据库实例
//
// 返回值:
//   - error: 如果GC过程中发生错误，返回错误信息
func AggressiveValueLogGC(db *badgerhold.Store) error {
	// startTime := time.Now() // 记录开始时间
	totalGCCount := 0 // 初始化总GC次数计数器

	// 从大到小的阈值列表
	ratios := []float64{0.7, 0.5, 0.3, 0.1} // 定义GC阈值列表

	logger.Info("开始执行激进值日志GC...") // 记录开始日志

	// 遍历所有阈值
	for _, ratio := range ratios {
		gcCount := 0 // 当前阈值的GC次数计数器
		// ratioStartTime := time.Now() // 记录当前阈值开始时间

		// 循环执行GC直到没有数据可清理
		for {
			err := db.Badger().RunValueLogGC(ratio) // 执行一次GC

			// 如果没有数据需要清理
			if err == badger.ErrNoRewrite {
				// if gcCount > 0 {
				// 	logger.Infof("阈值 %.2f 完成GC，执行次数: %d, 耗时: %v",
				// 		ratio, gcCount, time.Since(ratioStartTime))
				// }
				break // 退出当前阈值的循环
			}

			// 如果发生错误
			if err != nil {
				logger.Errorf("值日志GC失败: %v", err)
				return err
			}

			gcCount++      // 增加当前阈值GC计数
			totalGCCount++ // 增加总GC计数

			// 每10次GC输出一次进度日志
			if gcCount%10 == 0 {
				logger.Debugf("阈值 %.2f 已执行 %d 次GC", ratio, gcCount)
			}
		}
	}

	// 输出总结日志
	// if totalGCCount > 0 {
	// 	logger.Infof("完成激进值日志GC，总执行次数: %d, 总耗时: %v",
	// 		totalGCCount, time.Since(startTime))
	// } else {
	// 	logger.Info("激进值日志GC未发现需要清理的数据")
	// }

	// 手动触发Go的垃圾回收
	runtime.GC()
	logger.Info("手动垃圾回收完成")

	return nil
}

// ClearDatabase 清空数据库中所有数据
//
// 参数:
//   - db *badgerhold.Store: 数据库实例
//
// 返回值:
//   - error: 如果清空过程中发生错误，返回错误信息
func ClearDatabase(db *badgerhold.Store) error {
	// 调用 badger.DB 的 DropAll 方法清空数据库
	err := db.Badger().DropAll()
	if err != nil {
		logger.Errorf("清空数据库失败: %v", err)
		return err
	}

	return nil
}
