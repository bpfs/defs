// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"errors"
	"reflect"

	"github.com/dgraph-io/badger/v4"
)

// ErrNotFound 在未找到给定键的数据时返回
var ErrNotFound = errors.New("未找到该键对应的数据")

// Get 从 badgerhold 中检索一个值并将其存入 result。result 必须是指针
// 参数:
//   - key: interface{} 类型，表示要检索记录的键
//   - result: interface{} 类型，表示存储检索结果的变量，必须为指针类型
//
// 返回值：
//   - error: 如果检索过程中出现错误，则返回错误信息
func (s *Store) Get(key, result interface{}) error {
	// 使用 Badger 的 View 方法启动只读事务并执行获取操作
	return s.Badger().View(func(tx *badger.Txn) error {
		// 调用 TxGet 方法执行具体的获取操作
		return s.TxGet(tx, key, result)
	})
}

// TxGet 允许用户传入自己的 badger 事务来从 badgerhold 中检索一个值，并将其存入 result
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - key: interface{} 类型，表示要检索记录的键
//   - result: interface{} 类型，表示存储检索结果的变量，必须为指针类型
//
// 返回值：
//   - error: 如果检索过程中出现错误，则返回错误信息
func (s *Store) TxGet(tx *badger.Txn, key, result interface{}) error {
	// 获取存储器的实例，用于处理特定类型的数据
	storer := s.newStorer(result)

	// 对键进行编码，生成用于存储的键值
	gk, err := s.encodeKey(key, storer.Type())
	if err != nil {
		// 如果编码过程中出现错误，记录日志并返回错误信息
		logger.Error("键编码失败", "错误", err)
		return err
	}

	// 从事务中获取指定键的记录项
	item, err := tx.Get(gk)
	if err == badger.ErrKeyNotFound {
		// 如果记录不存在，记录日志并返回 ErrNotFound 错误
		logger.Error("未找到指定键的记录", "键", key)
		return ErrNotFound
	}
	if err != nil {
		// 如果获取记录项时出现其他错误，记录日志并返回错误信息
		logger.Error("获取记录项失败", "错误", err)
		return err
	}

	// 读取记录项的值并进行解码
	err = item.Value(func(value []byte) error {
		return s.decode(value, result)
	})
	if err != nil {
		// 如果解码过程中出现错误，记录日志并返回错误信息
		logger.Error("记录值解码失败", "错误", err)
		return err
	}

	// 检查 result 的类型是否为指针，如果是，则获取其元素类型
	tp := reflect.TypeOf(result)
	for tp.Kind() == reflect.Ptr {
		tp = tp.Elem()
	}

	// 获取键字段的名称
	keyField, ok := getKeyField(tp)
	if ok {
		// 解码键并将其存储到 result 的键字段中
		err := s.decodeKey(gk, reflect.ValueOf(result).Elem().FieldByName(keyField.Name).Addr().Interface(), storer.Type())
		if err != nil {
			// 如果解码过程中出现错误，记录日志并返回错误信息
			logger.Error("键解码失败", "错误", err)
			return err
		}
	}

	return nil
}

// Find 从 badgerhold 中检索符合传入查询的值集，result 必须是指向切片的指针。
// 查询结果将被追加到传入的 result 切片中，而不是清空传入的切片。
// 参数:
//   - result: interface{} 类型，表示存储检索结果的切片，必须为指针类型
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - error: 如果检索过程中出现错误，则返回错误信息
func (s *Store) Find(result interface{}, query *Query) error {
	// 使用 Badger 的 View 方法启动只读事务并执行查找操作
	return s.Badger().View(func(tx *badger.Txn) error {
		// 调用 TxFind 方法执行具体的查找操作
		return s.TxFind(tx, result, query)
	})
}

// TxFind 允许用户传入自己的 badger 事务来从 badgerhold 中检索符合查询条件的一组值
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - result: interface{} 类型，表示存储检索结果的切片，必须为指针类型
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - error: 如果检索过程中出现错误，则返回错误信息
func (s *Store) TxFind(tx *badger.Txn, result interface{}, query *Query) error {
	// 调用 findQuery 方法根据查询条件检索匹配的记录
	return s.findQuery(tx, result, query)
}

// FindOne 返回单个记录，因此 result 不是切片，而是指向结构体的指针。
// 如果没有找到符合查询条件的记录，则返回 ErrNotFound 错误。
// 参数:
//   - result: interface{} 类型，表示存储检索结果的变量，必须为指向结构体的指针
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - error: 如果未找到记录或检索过程中出现错误，则返回错误信息
func (s *Store) FindOne(result interface{}, query *Query) error {
	// 使用 Badger 的 View 方法启动只读事务并执行查找操作
	return s.Badger().View(func(tx *badger.Txn) error {
		// 调用 TxFindOne 方法执行具体的查找操作
		return s.TxFindOne(tx, result, query)
	})
}

// TxFindOne 允许用户传入自己的 badger 事务来从 badgerhold 中检索单个记录
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - result: interface{} 类型，表示存储检索结果的变量，必须为指向结构体的指针
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - error: 如果未找到记录或检索过程中出现错误，则返回错误信息
func (s *Store) TxFindOne(tx *badger.Txn, result interface{}, query *Query) error {
	// 调用 findOneQuery 方法根据查询条件检索单个匹配的记录
	return s.findOneQuery(tx, result, query)
}

// Count 返回当前数据类型的记录数量
// 参数:
//   - dataType: interface{} 类型，表示要统计记录数量的数据类型
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - uint64: 当前数据类型的记录数量
//   - error: 如果统计过程中出现错误，则返回错误信息
func (s *Store) Count(dataType interface{}, query *Query) (uint64, error) {
	var count uint64
	// 使用 Badger 的 View 方法启动只读事务并执行计数操作
	err := s.Badger().View(func(tx *badger.Txn) error {
		var txErr error
		// 调用 TxCount 方法执行具体的计数操作
		count, txErr = s.TxCount(tx, dataType, query)
		return txErr
	})
	return count, err
}

// TxCount 返回在指定事务内的当前数据类型的记录数量
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - dataType: interface{} 类型，表示要统计记录数量的数据类型
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - uint64: 当前数据类型的记录数量
//   - error: 如果统计过程中出现错误，则返回错误信息
func (s *Store) TxCount(tx *badger.Txn, dataType interface{}, query *Query) (uint64, error) {
	// 调用 countQuery 方法根据查询条件统计匹配的记录数量
	return s.countQuery(tx, dataType, query)
}

// ForEach 针对每个匹配查询条件的记录运行函数 fn
// 当处理大批量数据时非常有用，因为你不需要将整个结果集保存在内存中，类似于数据库游标
// 如果 fn 返回错误，将停止游标的迭代
// 参数:
//   - query: *Query 类型，表示查询条件
//   - fn: interface{} 类型，表示对每个记录执行的函数
//
// 返回值：
//   - error: 如果迭代过程中出现错误，则返回错误信息
func (s *Store) ForEach(query *Query, fn interface{}) error {
	// 使用 Badger 的 View 方法启动只读事务并执行遍历操作
	return s.Badger().View(func(tx *badger.Txn) error {
		// 调用 TxForEach 方法执行具体的遍历操作
		return s.TxForEach(tx, query, fn)
	})
}

// TxForEach 与 ForEach 方法相同，但允许用户指定自己的事务
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - query: *Query 类型，表示查询条件
//   - fn: interface{} 类型，表示对每个记录执行的函数
//
// 返回值：
//   - error: 如果迭代过程中出现错误，则返回错误信息
func (s *Store) TxForEach(tx *badger.Txn, query *Query, fn interface{}) error {
	// 调用 forEach 方法，根据查询条件遍历每个匹配的记录并执行 fn 函数
	return s.forEach(tx, query, fn)
}
