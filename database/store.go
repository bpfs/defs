package database

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/utils/paths"
	logging "github.com/dep2p/log"

	"go.uber.org/fx"
)

var logger = logging.Logger("database")

// DB 数据库结构体，包含BadgerDB和SqliteDB实例
type DB struct {
	BadgerDB *badgerhold.Store // BadgerDB数据库实例
	SqliteDB *sql.DB           // SqliteDB数据库实例
}

// 定义 BadgerDB 和 SqliteDB 的路径常量
var (
	// BadgerDB 相关路径
	badgerDBDirPath      = filepath.Join(paths.GetDatabasePath(), "badgerhold")
	badgerDBValueDirPath = filepath.Join(badgerDBDirPath, "value")
	badgerDBBackupDir    = filepath.Join(badgerDBDirPath, "backups")

	// SqliteDB 相关路径
	sqliteDBPath      = filepath.Join(paths.GetDatabasePath(), "sqlite")
	sqliteDBFile      = filepath.Join(sqliteDBPath, "blockchain.db")
	sqliteDBBackupDir = filepath.Join(sqliteDBPath, "backups")
)

// NewDBInput 是用于传递给 NewBadgerholdStore 函数的输入结构体。
// 它包含了 fx.In 嵌入结构体和应用的生命周期管理器 LC。
type NewDBInput struct {
	fx.In // 这是一个标记结构体，表示这是一个依赖注入的输入

	Ctx context.Context // 全局上下文
}

// NewBadgerholdStoreOutput 是 NewBadgerholdStore 函数的输出结构体。
// 它包含了 fx.Out 嵌入结构体和 Badgerhold 数据库实例 DB。
type NewDBOutput struct {
	fx.Out // 这是一个标记结构体，表示这是一个依赖注入的输出

	DB *DB // DB 是 Badgerhold 数据库的实例，供其他组件使用
}

// NewBadgerholdStore 是用于创建和初始化 Badgerhold 数据库的构造函数。
// 参数:
//   - lc fx.Lifecycle: 应用的生命周期管理器
//   - input NewBadgerholdStoreInput: 包含全局上下文的输入结构体
//
// 返回值:
//   - out NewBadgerholdStoreOutput: 包含初始化后的数据库实例的输出结构体
//   - err error: 如果初始化过程中发生错误，返回错误信息
func NewDB(lc fx.Lifecycle, input NewDBInput) (out NewDBOutput, err error) {
	// 初始化 BadgerDB
	badgerDB, err := NewBadgerDB(input.Ctx)
	if err != nil {
		logger.Errorf("初始化 BadgerDB 失败: %v", err)
		return out, err
	}

	// 初始化 SqliteDB
	sqliteDB, err := NewSqliteDB(input.Ctx)
	if err != nil {
		logger.Errorf("初始化 SqliteDB 失败: %v", err)
		return out, err
	}

	// 注册关闭数据库的钩子
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Infof("关闭数据库")
			badgerDB.Close()
			sqliteDB.Close()
			return nil
		},
	})

	// 设置输出
	out.DB = &DB{
		BadgerDB: badgerDB,
		SqliteDB: sqliteDB,
	}

	return out, nil
}
