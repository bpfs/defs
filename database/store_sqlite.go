package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/v2/utils/paths"

	_ "github.com/mattn/go-sqlite3" // 导入SQLite驱动
)

// NewSqliteDB 创建并初始化一个新的SQLite数据库连接
//
// 参数:
//   - ctx context.Context: 上下文对象，用于控制后台任务的生命周期
//
// 返回值:
//   - *sql.DB: 数据库连接实例
//   - error: 初始化过程中的错误，如果成功则为nil
func NewSqliteDB(ctx context.Context) (*sql.DB, error) {
	// 确保数据库目录存在
	if err := paths.AddDirectory(sqliteDBPath); err != nil {
		// 记录创建目录失败的错误
		logger.Errorf("创建数据库目录失败: %v", err)
		return nil, err
	}

	// 确保备份目录存在
	if err := paths.AddDirectory(sqliteDBBackupDir); err != nil {
		// 记录创建备份目录失败的错误
		logger.Errorf("创建备份目录失败: %v", err)
		return nil, err
	}

	// SQLite优化配置，设置数据库连接参数
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-32000&_busy_timeout=5000&_foreign_keys=ON&_temp_store=MEMORY", sqliteDBFile)

	// 打开数据库连接
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		// 记录打开数据库失败的错误
		logger.Errorf("打开数据库失败: %v", err)
		// 尝试从备份恢复
		if restoreErr := restoreDatabaseFromBackup(sqliteDBBackupDir, sqliteDBFile); restoreErr != nil {
			// 记录数据库恢复失败的错误
			logger.Errorf("数据库恢复失败: %v", restoreErr)
			return nil, fmt.Errorf("数据库恢复失败: %v", restoreErr)
		}

		// 重试打开数据库
		db, err = sql.Open("sqlite3", dsn)
		if err != nil {
			// 记录恢复后打开数据库失败的错误
			logger.Errorf("数据库恢复后打开失败: %v", err)
			return nil, err
		}
		// 记录数据库恢复成功的信息
		logger.Info("数据库已成功从备份恢复并打开")
	}

	// 配置数据库连接池参数
	db.SetMaxOpenConns(100)          // 设置最大打开连接数
	db.SetMaxIdleConns(10)           // 设置最大空闲连接数
	db.SetConnMaxLifetime(time.Hour) // 设置连接最大生命周期

	// 验证数据库连接是否正常
	if err = db.Ping(); err != nil {
		// 记录数据库连接验证失败的错误
		logger.Errorf("数据库连接验证失败: %v", err)
		return nil, err
	}

	return db, nil
}

// backupDatabase 备份数据库文件
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - backupDir string: 备份文件存储目录
//
// 返回值:
//   - error: 备份过程中的错误，如果成功则为nil
func backupDatabase(db *sql.DB, backupDir string) error {
	// 创建备份文件名，使用当前时间作为文件名的一部分
	backupFile := filepath.Join(backupDir,
		fmt.Sprintf("blockchain_%s.db", time.Now().Format("20060102_150405")))

	// 执行VACUUM INTO备份操作
	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", backupFile))
	if err != nil {
		// 记录备份失败的错误
		logger.Errorf("执行VACUUM INTO备份失败: %v", err)
		return err
	}

	// 清理旧备份文件，只保留最近5个
	return cleanOldBackups(backupDir, 5)
}

// restoreDatabaseFromBackup 从最新的备份恢复数据库
//
// 参数:
//   - backupDir string: 备份文件存储目录
//   - dbFile string: 数据库文件路径
//
// 返回值:
//   - error: 恢复过程中的错误，如果成功则为nil
func restoreDatabaseFromBackup(backupDir, dbFile string) error {
	// 获取所有备份文件列表
	backups, err := filepath.Glob(filepath.Join(backupDir, "blockchain_*.db"))
	if err != nil || len(backups) == 0 {
		// 记录获取备份文件失败的错误
		logger.Errorf("获取备份文件失败: %v", err)
		return fmt.Errorf("未找到可用的备份文件")
	}

	// 获取最新的备份文件
	latestBackup := backups[len(backups)-1]

	// 删除可能损坏的数据库文件
	if err := os.Remove(dbFile); err != nil && !os.IsNotExist(err) {
		// 记录删除损坏数据库文件失败的错误
		logger.Errorf("删除损坏的数据库文件失败: %v", err)
		return err
	}

	// 通过硬链接复制备份文件到数据库文件位置
	if err := os.Link(latestBackup, dbFile); err != nil {
		// 记录复制备份文件失败的错误
		logger.Errorf("复制备份文件失败: %v", err)
		return err
	}
	return nil
}

// vacuumDatabase 整理数据库空间
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//
// 返回值:
//   - error: 整理过程中的错误，如果成功则为nil
func vacuumDatabase(db *sql.DB) error {
	// 执行VACUUM操作整理数据库空间
	_, err := db.Exec("VACUUM")
	if err != nil {
		// 记录执行VACUUM失败的错误
		logger.Errorf("执行VACUUM失败: %v", err)
	}
	return err
}

// cleanOldBackups 清理旧的备份文件，保留指定数量的最新备份
//
// 参数:
//   - backupDir string: 备份文件存储目录
//   - keep int: 需要保留的最新备份数量
//
// 返回值:
//   - error: 清理过程中的错误，如果成功则为nil
func cleanOldBackups(backupDir string, keep int) error {
	// 获取所有备份文件列表
	backups, err := filepath.Glob(filepath.Join(backupDir, "blockchain_*.db"))
	if err != nil {
		// 记录获取备份文件列表失败的错误
		logger.Errorf("获取备份文件列表失败: %v", err)
		return err
	}

	// 如果备份文件数量不超过需要保留的数量，直接返回
	if len(backups) <= keep {
		return nil
	}

	// 删除多余的旧备份文件
	for _, backup := range backups[:len(backups)-keep] {
		if err := os.Remove(backup); err != nil {
			// 记录删除旧备份文件失败的警告
			logger.Warnf("删除旧备份文件失败: %v", err)
		}
	}

	return nil
}

// CreateTable 创建数据库表
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要创建的表名
//   - columns []string: 列定义字符串的切片，如 []string{"id INTEGER PRIMARY KEY", "name TEXT"}
//
// 返回值:
//   - error: 创建过程中的错误，如果成功则为nil
func CreateTable(db *sql.DB, tableName string, columns []string) error {
	// 构造创建表的SQL语句
	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", tableName, strings.Join(columns, ","))

	// 执行创建表的SQL语句
	_, err := db.Exec(query)
	if err != nil {
		// 记录创建表失败的错误
		logger.Errorf("创建表失败: %v", err)
		return err
	}

	return nil
}

// Insert 向表中插入数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要插入数据的表名
//   - data map[string]interface{}: 键为列名，值为对应的数据
//
// 返回值:
//   - error: 插入过程中的错误，如果成功则为nil
func Insert(db *sql.DB, tableName string, data map[string]interface{}) error {
	// 初始化存储列名、值和占位符的切片
	keys := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	placeholder := make([]string, 0, len(data))

	// 从data中分离出列名、插入的值、以及占位符
	for key, value := range data {
		keys = append(keys, key)
		values = append(values, value)
		placeholder = append(placeholder, "?")
	}

	// 开始数据库事务
	tx, err := db.Begin()
	if err != nil {
		// 记录开始事务失败的错误
		logger.Errorf("开始事务失败: %v", err)
		return err
	}

	// 构造插入语句
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(keys, ","), strings.Join(placeholder, ","))

	// 执行插入语句
	_, err = tx.Exec(query, values...)
	if err != nil {
		// 记录执行插入语句失败的错误
		logger.Errorf("执行插入语句失败: %v", err)

		// 回滚事务
		if rbErr := tx.Rollback(); rbErr != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", rbErr)
		}
		return err
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		// 记录提交事务失败的错误
		logger.Errorf("提交事务失败: %v", err)
		return err
	}

	return nil
}

// TxInsert 在事务中向表中插入数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tx *sql.Tx: 事务实例
//   - tableName string: 要插入数据的表名
//   - data map[string]interface{}: 键为列名，值为对应的数据
//
// 返回值:
//   - error: 插入过程中的错误，如果成功则为nil
func TxInsert(db *sql.DB, tx *sql.Tx, tableName string, data map[string]interface{}) error {
	// 初始化存储列名、值和占位符的切片
	keys := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	placeholder := make([]string, 0, len(data))

	// 从data中分离出列名、插入的值、以及占位符
	for key, value := range data {
		keys = append(keys, key)
		values = append(values, value)
		placeholder = append(placeholder, "?")
	}

	// 构造插入语句
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(keys, ","), strings.Join(placeholder, ","))

	// 执行插入语句
	_, err := tx.Exec(query, values...)
	if err != nil {
		// 记录执行插入语句失败的错误
		logger.Errorf("执行插入语句失败: %v", err)

		// 回滚事务
		if rbErr := tx.Rollback(); rbErr != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", rbErr)
		}
		return err
	}

	return nil
}

// Update 在表中更新数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要更新的表名
//   - data map[string]interface{}: 键为列名，值为新的数据
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//
// 返回值:
//   - error: 更新过程中的错误，如果成功则为nil
func Update(db *sql.DB, tableName string, data map[string]interface{}, conditions []string, args []interface{}) error {
	// 从data中分离出列名和新的值
	setValues := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	for key, value := range data {
		setValues = append(setValues, fmt.Sprintf("%s = ?", key))
		values = append(values, value)
	}

	// 开始数据库事务
	tx, err := db.Begin()
	if err != nil {
		// 记录开始事务失败的错误
		logger.Errorf("开始事务失败: %v", err)
		return err
	}

	// 构造更新语句
	query := fmt.Sprintf("UPDATE %s SET %s", tableName, strings.Join(setValues, ","))

	// 如果有WHERE条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
		// 将WHERE条件的参数值添加到values切片中
		values = append(values, args...)
	}

	// 执行更新语句
	_, err = tx.Exec(query, values...)
	if err != nil {
		// 记录执行更新语句失败的错误
		logger.Errorf("执行更新语句失败: %v", err)

		// 回滚事务
		if err := tx.Rollback(); err != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", err)
		}

		return err
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		// 记录提交事务失败的错误
		logger.Errorf("提交事务失败: %v", err)
		return err
	}

	return nil
}

// Delete 在表中删除数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要删除数据的表名
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//
// 返回值:
//   - error: 删除过程中的错误，如果成功则为nil
func Delete(db *sql.DB, tableName string, conditions []string, args []interface{}) error {
	// 开始数据库事务
	tx, err := db.Begin()
	if err != nil {
		// 记录开始事务失败的错误
		logger.Errorf("开始事务失败: %v", err)
		return err
	}

	// 构造删除语句
	query := fmt.Sprintf("DELETE FROM %s", tableName)
	// 如果有WHERE条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// 执行删除语句
	_, err = tx.Exec(query, args...)
	if err != nil {
		// 记录执行删除语句失败的错误
		logger.Errorf("执行删除语句失败: %v", err)

		// 回滚事务
		if err := tx.Rollback(); err != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", err)
		}

		return err
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		// 记录提交事务失败的错误
		logger.Errorf("提交事务失败: %v", err)
		return err
	}

	// 执行VACUUM操作整理数据库空间
	_, err = db.Exec("VACUUM")
	if err != nil {
		// 记录执行VACUUM失败的错误
		logger.Errorf("执行VACUUM失败: %v", err)
		return err
	}

	return nil
}

// TxDelete 在事务中删除表中数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tx *sql.Tx: 事务实例
//   - tableName string: 要删除数据的表名
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//
// 返回值:
//   - error: 删除过程中的错误，如果成功则为nil
func TxDelete(db *sql.DB, tx *sql.Tx, tableName string, conditions []string, args []interface{}) error {
	// 构造删除语句
	query := fmt.Sprintf("DELETE FROM %s", tableName)
	// 如果有WHERE条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// 执行删除语句
	_, err := tx.Exec(query, args...)
	if err != nil {
		// 记录执行删除语句失败的错误
		logger.Errorf("执行删除语句失败: %v", err)

		// 回滚事务
		if err := tx.Rollback(); err != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", err)
		}

		return err
	}

	return nil
}

// TruncateTable 删除表中所有数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要清空的表名
//   - vacuum bool: 是否在删除后执行VACUUM操作来回收空间
//
// 返回值:
//   - int64: 删除的记录数
//   - error: 删除过程中的错误，如果成功则为nil
func TruncateTable(db *sql.DB, tableName string, vacuum bool) (int64, error) {
	// 开始数据库事务
	tx, err := db.Begin()
	if err != nil {
		// 记录开始事务失败的错误
		logger.Errorf("开始事务失败: %v", err)
		return 0, err
	}

	// 删除表中所有数据
	result, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", tableName))
	if err != nil {
		// 记录删除表数据失败的错误
		logger.Errorf("删除表数据失败: %v", err)

		// 回滚事务
		if rbErr := tx.Rollback(); rbErr != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", rbErr)
		}
		return 0, err
	}

	// 获取删除的记录数
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// 记录获取影响行数失败的错误
		logger.Errorf("获取影响行数失败: %v", err)

		// 回滚事务
		if rbErr := tx.Rollback(); rbErr != nil {
			// 记录回滚事务失败的错误
			logger.Errorf("回滚事务失败: %v", rbErr)
		}
		return 0, err
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		// 记录提交事务失败的错误
		logger.Errorf("提交事务失败: %v", err)
		return 0, err
	}

	// 如果需要执行VACUUM操作
	if vacuum {
		_, err = db.Exec("VACUUM")
		if err != nil {
			// 记录执行VACUUM失败的错误
			logger.Errorf("执行VACUUM失败: %v", err)
			return rowsAffected, err
		}
	}

	// 记录成功删除记录的信息
	logger.Infof("成功删除表 %s 中的 %d 条记录", tableName, rowsAffected)
	return rowsAffected, nil
}

// TruncateTableTx 在事务中删除表中所有数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tx *sql.Tx: 事务实例
//   - tableName string: 要清空的表名
//
// 返回值:
//   - int64: 删除的记录数
//   - error: 删除过程中的错误，如果成功则为nil
func TruncateTableTx(db *sql.DB, tx *sql.Tx, tableName string) (int64, error) {
	// 删除表中所有数据
	result, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", tableName))
	if err != nil {
		// 记录删除表数据失败的错误
		logger.Errorf("删除表数据失败: %v", err)
		return 0, err
	}

	// 获取删除的记录数
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// 记录获取影响行数失败的错误
		logger.Errorf("获取影响行数失败: %v", err)
		return 0, err
	}

	// 记录成功删除记录的信息
	logger.Infof("成功删除表 %s 中的 %d 条记录", tableName, rowsAffected)
	return rowsAffected, nil
}

// Select 从表中查询数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要查询的表名
//   - columns []string: 要查询的列名
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//   - start int: 查询的起始行
//   - limit int: 查询的行数
//   - orderBy string: 排序字段，可包含多个字段，如"field1 ASC, field2 DESC"
//
// 返回值:
//   - *sql.Rows: 查询结果集
//   - error: 查询过程中的错误，如果成功则为nil
func Select(db *sql.DB, tableName string, columns []string, conditions []string, args []interface{}, start, limit int, orderBy string) (*sql.Rows, error) {
	// 将列名切片连接成字符串
	columnsStr := strings.Join(columns, ",")

	// 构造查询语句的WHERE子句
	var where string
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// 构造查询语句的ORDER BY子句
	var order string
	if orderBy != "" {
		order = " ORDER BY " + orderBy
	}

	// 构造完整的查询语句
	var query string
	if limit == 0 {
		// 不需要分页查询
		query = fmt.Sprintf("SELECT %s FROM %s %s %s", columnsStr, tableName, where, order)
	} else {
		// 需要分页查询
		query = fmt.Sprintf("SELECT %s FROM %s %s %s LIMIT %d,%d", columnsStr, tableName, where, order, start, limit)
	}

	// 执行查询语句
	rows, err := db.Query(query, args...)
	if err != nil {
		// 记录执行查询语句失败的错误
		logger.Errorf("执行查询语句失败: %v", err)
		return nil, err
	}
	return rows, nil
}

// Exists 查询是否存在满足条件的记录
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要查询的表名
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//
// 返回值:
//   - bool: 是否存在满足条件的记录
//   - error: 查询过程中的错误，如果成功则为nil
func Exists(db *sql.DB, tableName string, conditions []string, args []interface{}) (bool, error) {
	// 构造查询语句
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s", tableName)

	// 如果有WHERE条件，则添加到查询语句中
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// 添加LIMIT 1限制返回结果的数量
	query += " LIMIT 1)"

	// 执行查询语句
	var exists int
	err := db.QueryRow(query, args...).Scan(&exists)
	if err != nil {
		// 记录执行查询语句失败的错误
		logger.Errorf("执行查询语句失败: %v", err)
		return false, err
	}

	// exists为0表示不存在记录
	if exists == 0 {
		return false, nil
	}
	return true, nil
}

// SelectOne 从表中查询一条数据
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要查询的表名
//   - columns []string: 要查询的列名
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//
// 返回值:
//   - *sql.Row: 查询结果
//   - error: 查询过程中的错误，如果成功则为nil
func SelectOne(db *sql.DB, tableName string, columns []string, conditions []string, args []interface{}) (*sql.Row, error) {
	// 将列名切片连接成字符串
	columnsStr := strings.Join(columns, ",")

	// 构造查询语句的WHERE子句
	var where string
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// 构造完整的查询语句
	query := fmt.Sprintf("SELECT %s FROM %s %s LIMIT 1", columnsStr, tableName, where)

	// 执行查询语句
	row := db.QueryRow(query, args...)
	if row == nil {
		// 记录查询返回空结果的错误
		logger.Error("执行查询语句失败: 返回空结果")
		return nil, fmt.Errorf("查询返回空结果")
	}
	return row, nil
}

// Count 返回表中记录总数
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要查询的表名
//   - conditions []string: WHERE条件语句
//   - args []interface{}: conditions中的参数值
//
// 返回值:
//   - int: 记录总数
//   - error: 查询过程中的错误，如果成功则为nil
func Count(db *sql.DB, tableName string, conditions []string, args []interface{}) (int, error) {
	// 构造查询语句
	var where string
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", tableName, where)

	// 执行查询语句
	var count int
	err := db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		logger.Errorf("执行查询语句失败: %v", err)
		return 0, err
	}

	return count, nil
}

// ModifyColumnType 修改数据库表中指定列的数据类型
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要修改的表名
//   - columnName string: 要修改的列名
//   - newColumnType string: 新的数据类型
//
// 返回值:
//   - error: 修改过程中的错误，如果成功则为nil
func ModifyColumnType(db *sql.DB, tableName string, columnName string, newColumnType string) error {
	// 开始数据库事务
	tx, err := db.Begin()
	if err != nil {
		logger.Errorf("开始事务失败: %v", err)
		return fmt.Errorf("开始数据库事务失败: %v", err)
	}
	// 构造修改表语句
	query := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s", tableName, columnName, newColumnType)
	_, err = db.Exec(query)
	if err != nil {
		logger.Errorf("更新字段失败: %v", err)
		return err
	}
	// 提交事务
	err = tx.Commit()
	if err != nil {
		logger.Errorf("提交事务失败: %v", err)
		return fmt.Errorf("提交事务失败: %v", err)
	}
	return err
}

// AddColumn 在表中添加新列
//
// 参数:
//   - db *sql.DB: 数据库连接实例
//   - tableName string: 要添加列的表名
//   - columnName string: 要添加的列名
//   - columnType string: 列的数据类型
//
// 返回值:
//   - error: 添加过程中的错误，如果成功则为nil
func AddColumn(db *sql.DB, tableName string, columnName string, columnType string) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		logger.Errorf("开始事务失败: %v", err)
		return err
	}

	// 获取表的所有列信息
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		logger.Errorf("获取表信息失败: %v", err)
		if err := tx.Rollback(); err != nil {
			logger.Errorf("回滚事务失败: %v", err)
		}
		return err
	}
	defer rows.Close()

	// 检查列是否已存在
	exists := false
	for rows.Next() {
		var (
			cid       int
			name      string
			type_     string
			notnull   bool
			dfltValue interface{}
			pk        bool
		)

		if err := rows.Scan(&cid, &name, &type_, &notnull, &dfltValue, &pk); err != nil {
			logger.Errorf("扫描行数据失败: %v", err)
			if err := tx.Rollback(); err != nil {
				logger.Errorf("回滚事务失败: %v", err)
			}
			return err
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
			logger.Errorf("添加列失败: %v", err)

			// 回滚事务
			if err := tx.Rollback(); err != nil {
				logger.Errorf("回滚事务失败: %v", err)
			}
			return err
		}
	} else {
		fmt.Printf("列 %s 已存在于表 %s 中", columnName, tableName)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		logger.Errorf("提交事务失败: %v", err)
		return err
	}

	return nil
}
