package sqlites

import "fmt"

const (
	DbFile = "database.db"
)

// InitDBTable 数据库表
func (db *SqliteDB) InitDBTable() error {
	// 创建文件资产数据库表
	if err := db.createUploadFileInfoTable(); err != nil {
		return err
	}

	// 创建文件片段数据库表
	if err := db.createUploadSliceInfoTable(); err != nil {
		return err
	}

	return nil
}

// 创建文件资产数据库表
func (s *SqliteDB) createUploadFileInfoTable() error {
	table := []string{
		"id INTEGER PRIMARY KEY AUTOINCREMENT", // id主键自动增长
		"assetID VARCHAR(60)",                  // 文件资产的唯一标识(外部标识)
		"totalPieces INTEGER ",                 // 文件片段的总量
		"operates INTEGER ",                    // 操作(0:下载、1:上传)
		"status INTEGER ",                      // 状态(0:失败、1:成功、2:待开始、3:进行中)
		"times TIMESTAMP",                      // 时间
	}

	// 创建表
	if err := s.CreateTable("files", table); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}

// 创建文件片段数据库表
func (s *SqliteDB) createUploadSliceInfoTable() error {
	table := []string{
		"id INTEGER PRIMARY KEY AUTOINCREMENT", // id主键自动增长
		"assetID VARCHAR(60)",                  // 文件资产的唯一标识(外部标识)
		"sliceHash VARCHAR(60)",                // 文件片段的哈希值(外部标识)
		"sliceIndex INTEGER",                   // 文件片段的索引(该片段在文件中的顺序位置)
		"status INTEGER",                       // 状态(0:失败、1:成功)
	}

	// 创建表
	if err := s.CreateTable("slices", table); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}
