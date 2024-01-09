package sqlites

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	_ "github.com/mattn/go-sqlite3" // 导入SQLite驱动
	"github.com/sirupsen/logrus"
)

// SqliteDB 是 SQLite 数据库的包装器
type SqliteDB struct {
	DB *sql.DB
}

// NewSqliteDB 创建一个新的 SqliteDB 实例
func NewSqliteDB(businessDbPath, dbFile string) (*SqliteDB, error) {
	dbPath := path.Join(businessDbPath, dbFile)
	// 检查数据库文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Printf("数据库文件 %s 不存在。正在创建...\n", dbFile)
		if err := createDatabaseFile(businessDbPath, dbFile); err != nil {
			return nil, err
		}
	}

	// Open 打开由其数据库驱动程序名称和特定于驱动程序的数据源名称指定的数据库，通常至少包含数据库名称和连接信息。
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %v", err)
	}

	// 检查数据库连接
	if err = db.Ping(); err != nil {
		// 添加重连逻辑
		for i := 0; i < 3; i++ {
			logrus.Printf("重试连接数据库: 第 %d 次...", i+1)
			time.Sleep(2 * time.Second)
			err = db.Ping()
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("连接数据库失败: %v", err)
		}
	}

	// 返回包含数据库连接的SqliteDB结构体实例
	return &SqliteDB{DB: db}, nil
}

// Close关闭SqliteDB中的数据库连接
func (db *SqliteDB) Close() error {
	return db.DB.Close()
}

// CreateTable 方法用于创建数据库表。tableName 是要创建的表名，columns 是列定义字符串的切片，
// 如 []string{"id INTEGER PRIMARY KEY", "name TEXT"}。
func (db *SqliteDB) CreateTable(tableName string, columns []string) error {
	// 创建表的 SQL 语句
	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", tableName, strings.Join(columns, ","))

	// 执行创建表的 SQL 语句
	_, err := db.DB.Exec(query)
	if err != nil {
		return fmt.Errorf("创建表失败: %v", err)
	}

	return nil
}

// Insert方法向表中插入数据。tableName是要插入数据的表的名字，
// data是一个map，其中键是列的名字，值是对应的值。
func (db *SqliteDB) Insert(tableName string, data map[string]interface{}) error {
	keys := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	placeholder := make([]string, 0, len(data))

	//从data中分离出列名、插入的值、以及占位符
	for key, value := range data {
		keys = append(keys, key)
		values = append(values, value)
		placeholder = append(placeholder, "?")
	}

	// 开始数据库事务
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("数据事务失败: %v", err)
	}

	//构造插入语句
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(keys, ","), strings.Join(placeholder, ","))

	// 执行插入用户数据的 SQL 语句
	_, err = tx.Exec(query, values...)
	if err != nil {
		logrus.Printf("插入数据错误: %v", err) // 使用log包来记录错误
		// 回滚事务
		_ = tx.Rollback()
		return fmt.Errorf("数据插入失败: %v", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("数据插入失败: %v", err)
	}

	return nil
}

// Update 方法在表中更新数据。tableName 是要更新的表的名字，
// data 是一个 map，其中键是列名，值是新的值，conditions 是 WHERE 条件，args 是 conditions 中的参数值。
func (db *SqliteDB) Update(tableName string, data map[string]interface{}, conditions []string, args []interface{}) error {
	// 从 data 中分离出列名和新的值
	setValues := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	for key, value := range data {
		setValues = append(setValues, fmt.Sprintf("%s = ?", key))
		values = append(values, value)
	}

	// 开始数据库事务
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("数据事务失败: %v", err)
	}

	// 构造更新语句
	query := fmt.Sprintf("UPDATE %s SET %s", tableName, strings.Join(setValues, ","))

	// 如果有 WHERE 条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
		// 将 WHERE 条件的参数值添加到 values 切片中
		values = append(values, args...)
	}

	// 执行更新用户数据的 SQL 语句
	_, err = tx.Exec(query, values...)
	if err != nil {
		logrus.Printf("更新数据错误: %v", err)
		// 回滚事务
		_ = tx.Rollback()
		return fmt.Errorf("failed to update user: %v", err)
	}

	// 提交事务
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

// Delete 方法在表中删除数据。tableName 是要删除数据的表的名字，
// conditions 是 WHERE 条件，args 是 conditions 中的参数值。
func (db *SqliteDB) Delete(tableName string, conditions []string, args []interface{}) error {
	// 开始数据库事务
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("数据事务失败: %v", err)
	}

	// 构造删除语句
	query := fmt.Sprintf("DELETE FROM %s", tableName)

	// 如果有 WHERE 条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// 执行删除用户数据的 SQL 语句
	_, err = tx.Exec(query, args...)
	if err != nil {
		// 回滚事务
		_ = tx.Rollback()
		return fmt.Errorf("failed to delete user: %v", err)
	}

	// 提交事务
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	// 执行 VACUUM
	_, err = db.DB.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("failed to run VACUUM: %v", err)
	}

	return nil
}

// Select 方法从表中查询数据。tableName 是要查询的表的名字，columns 是要查询的列的名字，
// args 是 WHERE 条件中的参数值，start 是查询的起始行，limit 是查询的行数。conditions 是 WHERE 条件，可以为空。
// orderBy 是排序字段，可以包含多个字段，以逗号分隔，例如 "field1 ASC, field2 DESC"。
func (db *SqliteDB) Select(tableName string, columns []string, conditions []string, args []interface{}, start, limit int, orderBy string) (*sql.Rows, error) {
	columnsStr := strings.Join(columns, ",") //将列名切片连接成字符串

	// 构造查询语句
	var where string

	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	var order string
	if orderBy != "" {
		order = " ORDER BY " + orderBy
	}

	var query string
	if limit == 0 {
		// 不需要分页查询
		query = fmt.Sprintf("SELECT %s FROM %s %s %s", columnsStr, tableName, where, order)
	} else {
		// 需要分页查询
		query = fmt.Sprintf("SELECT %s FROM %s %s %s LIMIT %d,%d", columnsStr, tableName, where, order, start, limit)
	}
	//query := fmt.Sprintf("SELECT %s FROM %s %s LIMIT %d,%d", columnsStr, tableName, where, start, limit)

	// 执行查询语句
	return db.DB.Query(query, args...)
}

// Exists 方法用于查询是否存在满足条件的记录。
// tableName 是要查询的表的名字，conditions 是 WHERE 条件，args 是 conditions 中的参数值。
func (db *SqliteDB) Exists(tableName string, conditions []string, args []interface{}) (bool, error) {
	// 构造查询语句
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s", tableName)

	// 如果有 WHERE 条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// 添加 LIMIT 1 限制返回结果的数量
	query += " LIMIT 1)"

	// 执行查询语句
	var exists int
	err := db.DB.QueryRow(query, args...).Scan(&exists)
	if err != nil {
		return false, err
	}
	// exists 0证明不存在
	if exists == 0 {
		return false, nil
	}
	return true, nil
}

// SelectOne 方法从表中查询一条数据。tableName 是要查询的表的名字，columns 是要查询的列的名字，
// args 是 WHERE 条件中的参数值，conditions 是 WHERE 条件，可以为空。
func (db *SqliteDB) SelectOne(tableName string, columns []string, conditions []string, args []interface{}) (*sql.Row, error) {
	columnsStr := strings.Join(columns, ",") //将列名切片连接成字符串

	// 构造查询语句
	var where string

	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf("SELECT %s FROM %s %s LIMIT 1", columnsStr, tableName, where)

	// 执行查询语句
	return db.DB.QueryRow(query, args...), nil
}

// Count 返回总数
func (db *SqliteDB) Count(tableName string, conditions []string, args []interface{}) (int, error) {
	// 构造查询语句
	var where string
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", tableName, where)

	// 执行查询语句
	var count int
	err := db.DB.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// 创建db文件
func createDatabaseFile(businessDbPath, dbFile string) error {
	// 新建文件存储
	fs, err := afero.NewFileStore(businessDbPath)
	if err != nil {
		return err
	}
	// 创建文件
	if err := fs.CreateFile("", dbFile); err != nil {
		return err
	}
	return nil
}

// ModifyColumnType方法修改数据库表中指定列的数据类型。tableName是要修改的表的名字，
// columnName是要修改的列名，newColumnType是要修改成的数据类型。
func (db *SqliteDB) ModifyColumnType(tableName string, columnName string, newColumnType string) error {
	// 开始数据库事务
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("开始数据库事务失败: %v", err)
	}
	// 构造修改表语句
	query := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s", tableName, columnName, newColumnType)
	_, err = db.DB.Exec(query)
	if err != nil {
		return fmt.Errorf("更新字段失败失败: %v", err)
	}
	// 提交事务
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}
	return err
}

// 新增列如果不存在  k是列名，v是数据类型。
func (db *SqliteDB) AddColumnsIfNotExists(tableName string, columnNames map[string]string) error {
	// 开始数据库事务
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("开始数据库事务失败: %v", err)
	}

	// 检查每个列是否存在，如果不存在则添加
	for columnName, columnType := range columnNames {
		// 检查列是否存在
		rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			return fmt.Errorf("获取表信息失败: %v", err)
		}

		exists := false
		for rows.Next() {
			var cid int                   // 列的ID号，按照在表定义中的列的出现顺序排列。
			var name string               // 列的名字。
			var dataType string           // 列的数据类型。SQLite是动态类型的，所以这个字段可能和实际存储的数据的类型不匹配。
			var notnull bool              // 这个字段标识这个列是否有NOT NULL约束。如果有，它的值是1，否则是0.
			var dflt_value sql.NullString // 这个列的默认值。如果列没有默认值，这个字段是NULL.
			var pk int                    // 这个字段标识这个列是否是主键。如果是，它的值是1，否则是0.

			err := rows.Scan(&cid, &name, &dataType, &notnull, &dflt_value, &pk)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("扫描表信息失败: %v", err)
			}

			if strings.EqualFold(name, columnName) {
				exists = true
				break
			}
		}

		if !exists {
			// 如果列不存在，添加列
			_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnType))
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("添加列失败: %v", err)
			}
		} else {
			fmt.Printf("列 %s 已存在于表 %s 中", columnName, tableName)
		}
	}

	// 提交事务
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}

	return nil
}
