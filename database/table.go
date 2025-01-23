package database

import (
	"context"
	"path/filepath"
	"runtime"
	"time"

	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/utils/paths"

	"go.uber.org/fx"
)

// InitializeDatabaseInput 定义了初始化 UploadManager 所需的输入参数
type InitializeDatabaseInput struct {
	fx.In

	Ctx context.Context
	Opt *fscfg.Options
	DB  *DB
}

// InitializeDatabase 初始化数据库维护任务，包括定期GC和备份
// 参数:
//   - lc: fx.Lifecycle 用于管理应用生命周期的对象
//   - input: InitDBTableInput 包含初始化所需的输入参数
//
// 返回值:
//   - error 如果初始化过程中发生错误，则返回相应的错误信息
func InitializeDatabase(lc fx.Lifecycle, input InitializeDatabaseInput) error {

	// 创建激励记录数据库表
	if err := CreateFileSegmentStorageTable(input.DB.SqliteDB); err != nil {
		logger.Errorf("创建激励记录数据库表时失败: %v", err)
		return err
	}

	// 添加生命周期钩子
	lc.Append(fx.Hook{
		// 启动钩子
		OnStart: func(ctx context.Context) error {
			// 启动数据库维护任务，包括定期GC和备份
			go startDatabaseMaintenance(input)
			return nil
		},
		// 停止钩子
		OnStop: func(ctx context.Context) error {

			return nil
		},
	})

	return nil
}

// startDatabaseMaintenance 启动数据库维护任务，包括定期GC和备份
//
// 该方法启动一个后台goroutine，执行以下维护任务：
// 1. GC任务：
//   - 首次执行：程序启动3分钟后
//   - 定期执行：每30分钟执行一次
//   - 每次GC后会强制执行内存回收
//
// 2. 备份任务：
//   - 定期执行：每83分钟执行一次
//   - 同时备份BadgerDB和SqliteDB
//
// 参数:
//   - input InitializeDatabaseMaintenanceInput: 包含数据库实例的输入结构体
func startDatabaseMaintenance(input InitializeDatabaseInput) {

	// 初始化sqlite存储路径
	sqliteDBPath := filepath.Join(paths.GetDatabasePath(), "sqlite")
	sqliteDBBackupDir := filepath.Join(sqliteDBPath, "backups")

	// 初始化badger存储路径
	badgerDBDirPath := filepath.Join(paths.GetDatabasePath(), "badgerhold")
	badgerDBBackupDir := filepath.Join(badgerDBDirPath, "backups")

	firstRun := true // 标记首次运行
	// 创建一个timer，首次执行设定为3分钟后
	gcTimer := time.NewTimer(3 * time.Minute)
	defer gcTimer.Stop()

	aggressiveGCTicker := time.NewTicker(183 * time.Minute) // 每183分钟执行一次激进GC
	defer aggressiveGCTicker.Stop()

	// 创建一个ticker，用于定期备份，首次执行设定为83分钟后
	backupTicker := time.NewTicker(83 * time.Minute)
	defer backupTicker.Stop()

	// 无限循环，持续监听事件
	for {
		select {
		// 监听上下文的取消信号
		case <-input.Ctx.Done():
			logger.Info("退出数据库维护任务")
			return

		// GC定时器
		case <-gcTimer.C:
			if firstRun {
				logger.Info("首次执行数据库GC...")
				// 首次执行后将timer重置为30分钟的ticker
				gcTimer = time.NewTimer(30 * time.Minute)
				firstRun = false
			} else {
				logger.Info("开始执行定期数据库GC...")
			}

			// 执行GC
			if err := ForceValueLogGC(input.DB.BadgerDB, 0.5); err != nil {
				logger.Errorf("数据库GC失败: %v", err)
			} else {
				logger.Info("数据库GC完成")
			}

			// 强制执行Go的垃圾回收
			runtime.GC()
			logger.Debug("已执行强制内存回收")

		case <-aggressiveGCTicker.C:
			// 执行激进GC
			logger.Info("开始执行激进GC...")
			if err := AggressiveValueLogGC(input.DB.BadgerDB); err != nil {
				logger.Errorf("激进GC失败: %v", err)
			}

			// 强制执行Go的垃圾回收
			runtime.GC()
			logger.Debug("已执行强制内存回收")

		// 数据库备份定时器
		case <-backupTicker.C:
			logger.Info("开始执行数据库备份...")
			// 执行BadgerDB备份
			if err := BackupDatabase(input.DB.BadgerDB, badgerDBBackupDir); err != nil {
				logger.Errorf("BadgerDB备份失败: %v", err)
			}
			// 执行SqliteDB备份
			if err := backupDatabase(input.DB.SqliteDB, sqliteDBBackupDir); err != nil {
				logger.Errorf("SqliteDB备份失败: %v", err)
			}

			// 强制执行Go的垃圾回收
			runtime.GC()
			logger.Info("数据库备份完成")
		}
	}
}
