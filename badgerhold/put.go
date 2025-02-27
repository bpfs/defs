// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"errors"
	"reflect"

	"github.com/dgraph-io/badger/v4"
)

// ErrKeyExists 是在插入已存在的键时返回的错误
var ErrKeyExists = errors.New("键已存在")

// ErrUniqueExists 是在插入违反唯一性约束的值时抛出的错误
var ErrUniqueExists = errors.New("由于字段的唯一性约束，无法写入该值")

// sequence 是一个结构体，用于表示在桶中插入键时的下一个序列值
type sequence struct{}

// NextSequence 用于创建插入时的顺序键
// 插入时使用 uint64 作为键
// store.Insert(badgerhold.NextSequence(), data)
func NextSequence() interface{} {
	return sequence{}
}

// Insert 将传入的数据插入到 badgerhold 中
// 如果键已存在于 badgerhold 中，则返回 ErrKeyExists 错误
// 如果数据结构有一个字段标记为 `badgerholdKey`，并且该字段的类型与插入的键类型相同，
// 且数据结构是通过引用传递的，并且键字段当前设置为该类型的零值，则该字段将设置为插入键的值。
// 要与 badgerhold.NextSequence() 一起使用，请使用 `uint64` 类型的键字段。
// 参数:
//   - key: interface{} 类型，表示要插入记录的键
//   - data: interface{} 类型，表示要插入的数据
//
// 返回值：
//   - error: 如果插入过程中出现错误，则返回错误信息
func (s *Store) Insert(key, data interface{}) error {
	err := s.Badger().Update(func(tx *badger.Txn) error {
		// 调用 TxInsert 方法执行具体的插入操作
		return s.TxInsert(tx, key, data)
	})

	// 如果发生冲突错误，重试插入
	if err == badger.ErrConflict {
		return s.Insert(key, data)
	}
	if err != nil {
		logger.Errorf("插入数据失败: %v", err)
	}
	return err
}

// TxInsert 与 Insert 方法相同，但允许用户指定自己的事务
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - key: interface{} 类型，表示要插入记录的键
//   - data: interface{} 类型，表示要插入的数据
//
// 返回值：
//   - error: 如果插入过程中出现错误，则返回错误信息
func (s *Store) TxInsert(tx *badger.Txn, key, data interface{}) error {
	storer := s.newStorer(data)
	var err error

	// 如果键是 sequence 类型，获取下一个序列值作为键
	if _, ok := key.(sequence); ok {
		key, err = s.getSequence(storer.Type())
		if err != nil {
			logger.Errorf("获取序列值失败: %v", err)
			return err
		}
	}

	// 对键进行编码，生成用于存储的键值
	gk, err := s.encodeKey(key, storer.Type())
	if err != nil {
		logger.Errorf("键编码失败: %v", err)
		return err
	}

	// 检查键是否已存在
	_, err = tx.Get(gk)
	if err != badger.ErrKeyNotFound {
		logger.Errorf("键已存在: %v", err)
		return ErrKeyExists
	}

	// 对数据进行编码
	value, err := s.encode(data)
	if err != nil {
		logger.Errorf("数据编码失败: %v", err)
		return err
	}

	// 插入数据
	err = tx.Set(gk, value)
	if err != nil {
		logger.Errorf("设置数据失败: %v", err)
		return err
	}

	// 插入索引
	err = s.indexAdd(storer, tx, gk, data)
	if err != nil {
		logger.Errorf("添加索引失败: %v", err)
		return err
	}

	dataVal := reflect.Indirect(reflect.ValueOf(data))
	if !dataVal.CanSet() {
		return nil
	}

	// 如果数据结构中有 `badgerholdKey` 标记的字段，并且该字段的值是零值，则设置该字段的值为键
	if keyField, ok := getKeyField(dataVal.Type()); ok {
		fieldValue := dataVal.FieldByName(keyField.Name)
		keyValue := reflect.ValueOf(key)
		if keyValue.Type() != keyField.Type {
			return nil
		}
		if !fieldValue.CanSet() {
			return nil
		}
		if !reflect.DeepEqual(fieldValue.Interface(), reflect.Zero(keyField.Type).Interface()) {
			return nil
		}
		fieldValue.Set(keyValue)
	}

	return nil
}

// Update 更新 badgerhold 中的现有记录
// 如果键在存储中不存在，则返回 ErrNotFound 错误
// 参数:
//   - key: interface{} 类型，表示要更新记录的键
//   - data: interface{} 类型，表示要更新的数据
//
// 返回值：
//   - error: 如果更新过程中出现错误，则返回错误信息
func (s *Store) Update(key interface{}, data interface{}) error {
	err := s.Badger().Update(func(tx *badger.Txn) error {
		// 调用 TxUpdate 方法执行具体的更新操作
		return s.TxUpdate(tx, key, data)
	})
	// 如果发生冲突错误，重试更新
	if err == badger.ErrConflict {
		return s.Update(key, data)
	}
	if err != nil {
		logger.Errorf("更新数据失败: %v", err)
	}
	return err
}

// TxUpdate 与 Update 方法相同，但允许用户指定自己的事务
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - key: interface{} 类型，表示要更新记录的键
//   - data: interface{} 类型，表示要更新的数据
//
// 返回值：
//   - error: 如果更新过程中出现错误，则返回错误信息
func (s *Store) TxUpdate(tx *badger.Txn, key interface{}, data interface{}) error {
	storer := s.newStorer(data)

	// 对键进行编码，生成用于存储的键值
	gk, err := s.encodeKey(key, storer.Type())
	if err != nil {
		logger.Errorf("键编码失败: %v", err)
		return err
	}

	// 获取现有记录
	existingItem, err := tx.Get(gk)
	if err == badger.ErrKeyNotFound {
		logger.Errorf("未找到要更新的记录: %v", err)
		return ErrNotFound
	}
	if err != nil {
		logger.Errorf("获取现有记录失败: %v", err)
		return err
	}

	// 删除任何现有的索引
	existingVal := newElemType(data)
	err = existingItem.Value(func(existing []byte) error {
		return s.decode(existing, existingVal)
	})
	if err != nil {
		logger.Errorf("解码现有记录失败: %v", err)
		return err
	}
	err = s.indexDelete(storer, tx, gk, existingVal)
	if err != nil {
		logger.Errorf("删除现有索引失败: %v", err)
		return err
	}

	// 对数据进行编码
	value, err := s.encode(data)
	if err != nil {
		logger.Errorf("数据编码失败: %v", err)
		return err
	}

	// 插入数据
	err = tx.Set(gk, value)
	if err != nil {
		logger.Errorf("设置更新数据失败: %v", err)
		return err
	}

	// 插入新的索引
	err = s.indexAdd(storer, tx, gk, data)
	if err != nil {
		logger.Errorf("添加新索引失败: %v", err)
	}
	return err
}

// Upsert 如果记录不存在，则将其插入到 badgerhold 中。如果记录已存在，则更新现有记录
// 参数:
//   - key: interface{} 类型，表示要插入或更新记录的键
//   - data: interface{} 类型，表示要插入或更新的数据
//
// 返回值：
//   - error: 如果插入或更新过程中出现错误，则返回错误信息
func (s *Store) Upsert(key interface{}, data interface{}) error {
	err := s.Badger().Update(func(tx *badger.Txn) error {
		// 调用 TxUpsert 方法执行具体的插入或更新操作
		return s.TxUpsert(tx, key, data)
	})

	// 如果发生冲突错误，重试操作
	if err == badger.ErrConflict {
		return s.Upsert(key, data)
	}
	if err != nil {
		logger.Errorf("插入或更新数据失败: %v", err)
	}
	return err
}

// TxUpsert 与 Upsert 方法相同，但允许用户指定自己的事务
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - key: interface{} 类型，表示要插入或更新记录的键
//   - data: interface{} 类型，表示要插入或更新的数据
//
// 返回值：
//   - error: 如果插入或更新过程中出现错误，则返回错误信息
func (s *Store) TxUpsert(tx *badger.Txn, key interface{}, data interface{}) error {
	storer := s.newStorer(data)

	// 对键进行编码，生成用于存储的键值
	gk, err := s.encodeKey(key, storer.Type())
	if err != nil {
		logger.Errorf("键编码失败: %v", err)
		return err
	}

	// 获取现有记录
	existingItem, err := tx.Get(gk)
	if err == nil {
		// 如果找到现有记录，删除任何现有的索引
		existingVal := newElemType(data)
		err = existingItem.Value(func(existing []byte) error {
			return s.decode(existing, existingVal)
		})
		if err != nil {
			logger.Errorf("解码现有记录失败: %v", err)
			return err
		}

		err = s.indexDelete(storer, tx, gk, existingVal)
		if err != nil {
			logger.Errorf("删除现有索引失败: %v", err)
			return err
		}
	} else if err != badger.ErrKeyNotFound {
		logger.Errorf("获取现有记录失败: %v", err)
		return err
	}

	// 对数据进行编码
	value, err := s.encode(data)
	if err != nil {
		logger.Errorf("数据编码失败: %v", err)
		return err
	}

	// 插入数据
	err = tx.Set(gk, value)
	if err != nil {
		logger.Errorf("设置数据失败: %v", err)
		return err
	}

	// 插入新的索引
	err = s.indexAdd(storer, tx, gk, data)
	if err != nil {
		logger.Errorf("添加新索引失败: %v", err)
	}
	return err
}

// UpdateMatching 对匹配查询条件的每个记录运行更新函数
// 注意，更新函数中的记录类型必须是指针
// 参数:
//   - dataType: interface{} 类型，表示要更新的数据类型
//   - query: *Query 类型，表示查询条件
//   - update: func(record interface{}) error 类型，表示更新函数
//
// 返回值：
//   - error: 如果更新过程中出现错误，则返回错误信息
func (s *Store) UpdateMatching(dataType interface{}, query *Query, update func(record interface{}) error) error {
	err := s.Badger().Update(func(tx *badger.Txn) error {
		// 调用 TxUpdateMatching 方法执行具体的批量更新操作
		return s.TxUpdateMatching(tx, dataType, query, update)
	})
	// 如果发生冲突错误，重试更新
	if err == badger.ErrConflict {
		return s.UpdateMatching(dataType, query, update)
	}
	if err != nil {
		logger.Errorf("批量更新匹配记录失败: %v", err)
	}
	return err
}

// TxUpdateMatching 与 UpdateMatching 方法相同，但允许用户指定自己的事务
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - dataType: interface{} 类型，表示要更新的数据类型
//   - query: *Query 类型，表示查询条件
//   - update: func(record interface{}) error 类型，表示更新函数
//
// 返回值：
//   - error: 如果更新过程中出现错误，则返回错误信息
func (s *Store) TxUpdateMatching(tx *badger.Txn, dataType interface{}, query *Query,
	update func(record interface{}) error) error {
	// 调用 updateQuery 方法根据查询条件对匹配的记录执行更新操作
	err := s.updateQuery(tx, dataType, query, update)
	if err != nil {
		logger.Errorf("更新匹配记录失败: %v", err)
	}
	return err
}
