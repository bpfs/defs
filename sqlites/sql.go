package sqlites

import (
	"database/sql"
	"fmt"
	"time"
)

// InsertFilesDatabase 插入文件数据
func (db *SqliteDB) InsertFilesDatabase(assetID string, totalPieces int64, operates, status int, modTime time.Time) error {
	data := map[string]interface{}{
		"assetID":     assetID,     // 文件资产的唯一标识
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
func (db *SqliteDB) UpdateFileDatabaseStatus(assetID string, operates, status int) error {
	data := map[string]interface{}{
		"status": status,
	}
	conditions := []string{"assetID = ?", "operates = ?"}
	args := []interface{}{assetID, operates}

	if err := db.Update("files", data, conditions, args); err != nil {
		return fmt.Errorf("数据库操作失败")
	}
	return nil
}

// InsertSlicesDatabase 插入文件片段数据
func (db *SqliteDB) InsertSlicesDatabase(assetID, sliceHash string, current, status int) error {
	data := map[string]interface{}{
		"assetID":    assetID,   // 文件资产的唯一标识
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
func (db *SqliteDB) UpdateSlicesDatabaseStatus(assetID, sliceHash string, status int) error {
	data := map[string]interface{}{
		"status": status, // 状态
	}
	conditions := []string{"assetID = ?", "sliceHash = ?"}
	args := []interface{}{assetID, sliceHash}

	if err := db.Update("slices", data, conditions, args); err != nil {
		return fmt.Errorf("数据库操作失败")
	}
	return nil
}

// SelectOneSlicesDatabaseStatus 查询指定文件的片段数据对象信息
func (db *SqliteDB) SelectOneSlicesDatabaseStatus(assetID string) (*struct {
	SliceHash  string // 文件片段的哈希值
	SliceIndex int    // 文件片段的索引
}, error) {
	columns := []string{
		"sliceHash",  // 文件片段的哈希值
		"sliceIndex", // 文件片段的索引
	}
	conditions := []string{"assetID=?"} // 查询条件
	args := []interface{}{assetID}      // 查询条件对应的值

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

// SelectOneAssetID 查询制定的文件资产是否存在
func (db *SqliteDB) SelectOneAssetID(assetID string) bool {
	columns := []string{
		"assetID", // 文件资产的唯一标识
	}
	conditions := []string{"assetID=?"} // 查询条件
	args := []interface{}{assetID}      // 查询条件对应的值

	row, err := db.SelectOne("slices", columns, conditions, args)
	if err != nil {
		return false
	}

	var s struct {
		AssetID string
	}

	if err := row.Scan(
		&s.AssetID,
	); err != nil {
		if err == sql.ErrNoRows {
			return false
		}
		return false
	}

	if s.AssetID != "" {
		return true
	}

	return false
}
