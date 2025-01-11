package database

import (
	"context"

	"github.com/bpfs/defs/v2/fscfg"

	"go.uber.org/fx"
)

// InitDBTable 定义了初始化 UploadManager 所需的输入参数
type InitDBTableInput struct {
	fx.In

	Ctx context.Context
	Opt *fscfg.Options
	DB  *DB
}

// InitDBTable 初始化 创建数据库 并设置相关的生命周期钩子
// 参数:
//   - lc: fx.Lifecycle 用于管理应用生命周期的对象
//   - input: InitDBTableInput 包含初始化所需的输入参数
//
// 返回值:
//   - error 如果初始化过程中发生错误，则返回相应的错误信息
func InitDBTable(lc fx.Lifecycle, input InitDBTableInput) error {

	// 创建激励记录数据库表
	if err := CreateFileSegmentStorageTable(input.DB.SqliteDB); err != nil {
		logger.Errorf("创建激励记录数据库表时失败: %v", err)
		return err
	}

	// 添加生命周期钩子
	lc.Append(fx.Hook{
		// 启动钩子
		OnStart: func(ctx context.Context) error {

			return nil
		},
		// 停止钩子
		OnStop: func(ctx context.Context) error {

			return nil
		},
	})

	return nil
}
