package sqlite

import (
	"fmt"

	"github.com/bpfs/defs/sqlites"
)

// InitDBTable 数据库表
func InitDBTable(db *sqlites.SqliteDB) error {
	// 创建文件数据库表
	if err := createUploadFileInfoTable(db); err != nil {
		return err
	}

	// 创建文件片段数据库表
	if err := createUploadSliceInfoTable(db); err != nil {
		return err
	}

	// 创建文件共享数据库表
	if err := createUploadSharedInfoTable(db); err != nil {
		return err
	}

	return nil
}

// 创建文件数据库表
func createUploadFileInfoTable(db *sqlites.SqliteDB) error {
	table := []string{
		"id INTEGER PRIMARY KEY AUTOINCREMENT", // id主键自动增长
		"fileID VARCHAR(60)",                   // 文件的唯一标识(外部标识)
		"totalPieces INTEGER ",                 // 文件片段的总量
		"operates INTEGER ",                    // 操作(0:下载、1:上传)
		"status INTEGER ",                      // 状态(0:失败、1:成功、2:待开始、3:进行中)
		"times TIMESTAMP",                      // 时间
	}

	// 创建表
	if err := db.CreateTable("files", table); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}

// 创建文件片段数据库表
func createUploadSliceInfoTable(db *sqlites.SqliteDB) error {
	table := []string{
		"id INTEGER PRIMARY KEY AUTOINCREMENT", // id主键自动增长
		"fileID VARCHAR(60)",                   // 文件的唯一标识(外部标识)
		"sliceHash VARCHAR(60)",                // 文件片段的哈希值(外部标识)
		"sliceIndex INTEGER",                   // 文件片段的索引(该片段在文件中的顺序位置)
		"status INTEGER",                       // 状态(0:失败、1:成功)
	}

	// 创建表
	if err := db.CreateTable("slices", table); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}

// 创建文件共享数据库表
func createUploadSharedInfoTable(db *sqlites.SqliteDB) error {
	table := []string{
		"id INTEGER PRIMARY KEY AUTOINCREMENT", // id主键自动增长
		"fileID VARCHAR(60)",                   // 文件的唯一标识(外部标识)
		"name VARCHAR(250)",                    // 文件的名称
		"size INTEGER",                         // 文件的长度
		"uploadTime TIMESTAMP",                 // 上传时间
		"modTime TIMESTAMP",                    // 修改时间
		"xref INTEGER",                         // Xref表中段的数量
	}

	// 创建表
	if err := db.CreateTable("shared", table); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}
