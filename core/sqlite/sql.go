package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bpfs/defs/sqlites"
)

// InsertFilesDatabase 插入文件数据
func InsertFilesDatabase(db *sqlites.SqliteDB, fileID string, totalPieces int64, operates, status int, modTime time.Time) error {
	data := map[string]interface{}{
		"fileID":      fileID,      // 文件的唯一标识
		"totalPieces": totalPieces, // 文件片段的总量
		"operates":    operates,    // 操作
		"status":      status,      // 状态
		"times":       modTime,     // 时间
	}

	if err := db.Insert("files", data); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}

// UpdateFileDatabaseStatus 更新文件数据对象的状态
// operates	操作(0:下载、1:上传)
// status	状态(0:失败、1:成功、2:待开始、3:进行中)
func UpdateFileDatabaseStatus(db *sqlites.SqliteDB, fileID string, operates, status int) error {
	data := map[string]interface{}{
		"status": status,
	}
	conditions := []string{"fileID = ?", "operates = ?"}
	args := []interface{}{fileID, operates}

	if err := db.Update("files", data, conditions, args); err != nil {
		return fmt.Errorf("数据库操作失败")
	}
	return nil
}

// InsertSlicesDatabase 插入文件片段数据
func InsertSlicesDatabase(db *sqlites.SqliteDB, fileID, sliceHash string, current, status int) error {
	data := map[string]interface{}{
		"fileID":     fileID,    // 文件的唯一标识
		"sliceHash":  sliceHash, // 文件片段的哈希值
		"sliceIndex": current,   // 文件片段的索引
		"status":     status,    // 状态
	}

	if err := db.Insert("slices", data); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}

// UpdateSlicesDatabaseStatus 更新文件片段数据对象的状态
func UpdateSlicesDatabaseStatus(db *sqlites.SqliteDB, fileID, sliceHash string, status int) error {
	data := map[string]interface{}{
		"status": status, // 状态
	}
	conditions := []string{"fileID = ?", "sliceHash = ?"}
	args := []interface{}{fileID, sliceHash}

	if err := db.Update("slices", data, conditions, args); err != nil {
		return fmt.Errorf("数据库操作失败")
	}
	return nil
}

// SelectOneSlicesDatabaseStatus 查询指定文件的片段数据对象信息
func SelectOneSlicesDatabaseStatus(db *sqlites.SqliteDB, fileID string) (*struct {
	SliceHash  string // 文件片段的哈希值
	SliceIndex int    // 文件片段的索引
}, error) {
	columns := []string{
		"sliceHash",  // 文件片段的哈希值
		"sliceIndex", // 文件片段的索引
	}
	conditions := []string{"fileID=?"} // 查询条件
	args := []interface{}{fileID}      // 查询条件对应的值

	row, err := db.SelectOne("slices", columns, conditions, args)
	if err != nil {
		return nil, fmt.Errorf("数据库操作失败: %v", err)
	}

	var s struct {
		SliceHash  string // 文件片段的哈希值
		SliceIndex int    // 文件片段的索引
	}

	if err := row.Scan(
		&s.SliceHash,
		&s.SliceIndex,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("数据库操作失败: %v", err)
	}

	return &s, nil
}

// SelectOneFileID 查询指定的文件是否存在
func SelectOneFileID(db *sqlites.SqliteDB, fileID string) bool {
	conditions := []string{"fileID=?"} // 查询条件
	args := []interface{}{fileID}      // 查询条件对应的值
	exists, err := db.Exists("files", conditions, args)
	if err != nil {
		return exists
	}
	return exists
}

// InsertSharedDatabase 插入共享数据
func InsertSharedDatabase(db *sqlites.SqliteDB, fileID, name string, size, xref int, uploadTime, modTime time.Time) error {

	data := map[string]interface{}{
		"fileID":     fileID,     // 文件的唯一标识
		"name":       name,       // 文件的名称
		"size":       size,       // 文件的长度
		"uploadTime": uploadTime, // 上传时间
		"modTime":    modTime,    // 修改时间
		"xref":       xref,       // Xref表中段的数量
	}

	if err := db.Insert("shared", data); err != nil {
		return fmt.Errorf("数据库操作失败")
	}

	return nil
}

// UpdateSharedDatabase 更新共享数据
func UpdateSharedDatabase(db *sqlites.SqliteDB, fileID, name string, size, xref int, uploadTime, modTime time.Time) error {

	conditions := []string{"fileID=?"} // 查询条件
	args := []interface{}{fileID}      // 查询条件对应的值
	// 查询数据库中是否有记录
	exists, err := db.Exists("shared", conditions, args)
	if err != nil {
		return errors.New("数据库操作失败")
	}

	// 有记录，更新客户端版本
	data := map[string]interface{}{
		"fileID":     fileID,     // 文件的唯一标识
		"name":       name,       // 文件的名称
		"size":       size,       // 文件的长度
		"uploadTime": uploadTime, // 上传时间
		"modTime":    modTime,    // 修改时间
		"xref":       xref,       // Xref表中段的数量
	}

	if exists {
		if err := db.Update("shared", data, conditions, args); err != nil {
			return errors.New("数据库操作失败")
		}
	} else {
		if err := db.Insert("shared", data); err != nil {
			return fmt.Errorf("数据库操作失败")
		}

	}

	return nil
}

// DeleteSharedDatabase 删除共享数据
func DeleteSharedDatabase(db *sqlites.SqliteDB, fileID string) error {
	conditions := []string{"fileID = ?"}
	args := []interface{}{fileID}

	if err := db.Delete("shared", conditions, args); err != nil {
		return errors.New("数据库操作失败")
	}
	return nil
}

// SelectSharedDatabaseStatus 查询指定文件的片段数据对象信息
func SelectSharedDatabaseStatus(db *sqlites.SqliteDB, name string) (*sql.Rows, error) {
	columns := []string{
		"fileID",     // 文件的唯一标识
		"name",       // 文件的名称
		"size",       // 文件的长度
		"uploadTime", // 上传时间
		"modTime",    // 修改时间
		"xref",       // Xref表中段的数量
	}
	conditions := []string{"name=?"} // 查询条件
	args := []interface{}{name}      // 查询条件对应的值

	return db.Select("shared", columns, conditions, args, 0, 0, "")
}
