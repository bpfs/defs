// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// 添加批次大小常量
const (
	batchSize     = 1000             // 增加批次大小，但要注意不超过事务限制
	deleteTimeout = 30 * time.Second // 添加超时控制
)

// Delete 从 badgerhold 中删除一条记录，dataType 只需要是存储类型的一个示例，以便正确更新桶和索引
// 参数:
//   - key: interface{} 类型，表示要删除记录的键
//   - dataType: interface{} 类型，表示要删除记录的类型
//
// 返回值：
//   - error: 如果删除过程中出现错误，则返回错误信息
func (s *Store) Delete(key, dataType interface{}) error {
	// 使用 Badger 的 Update 方法启动事务并执行删除操作
	return s.Badger().Update(func(tx *badger.Txn) error {
		// 调用 TxDelete 方法执行具体的删除操作
		return s.TxDelete(tx, key, dataType)
	})
}

// TxDelete 与 Delete 方法相同，但允许用户指定自己的事务
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - key: interface{} 类型，表示要删除记录的键
//   - dataType: interface{} 类型，表示要删除记录的类型
//
// 返回值：
//   - error: 如果删除过程中出现错误，则返回错误信息
func (s *Store) TxDelete(tx *badger.Txn, key, dataType interface{}) error {
	// 获取存储器的实例，用于处理特定类型的数据
	storer := s.newStorer(dataType)
	// 对键进行编码，生成用于存储的键值
	gk, err := s.encodeKey(key, storer.Type())

	// 检查编码过程中是否出现错误
	if err != nil {
		logger.Error("键编码失败：", err)
		return err
	}

	// 创建 dataType 类型的新实例，用于存储解码后的值
	value := newElemType(dataType)

	// 获取指定键的记录项
	item, err := tx.Get(gk)
	// 如果记录不存在，返回 ErrNotFound 错误
	if err == badger.ErrKeyNotFound {
		logger.Error("记录未找到")
		return ErrNotFound
	}
	// 如果获取记录项时出现其他错误，返回错误信息
	if err != nil {
		logger.Error("获取记录失败：", err)
		return err
	}

	// 读取记录项的值并进行解码
	err = item.Value(func(bVal []byte) error {
		// 将字节数组解码为具体的值类型
		return s.decode(bVal, value)
	})
	// 检查解码过程中是否出现错误
	if err != nil {
		logger.Error("记录解码失败：", err)
		return err
	}

	// 删除数据
	err = tx.Delete(gk)
	// 检查删除过程中是否出现错误
	if err != nil {
		logger.Error("删除记录失败：", err)
		return err
	}

	// 删除索引
	err = s.indexDelete(storer, tx, gk, value)
	if err != nil {
		logger.Error("删除索引失败：", err)
		return err
	}

	return nil
}

// DeleteMatching 删除所有与传入查询匹配的记录
// 参数:
//   - dataType: interface{} 类型，表示要删除记录的类型
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - error: 如果删除过程中出现错误，则返回错误信息
func (s *Store) DeleteMatching(dataType interface{}, query *Query) error {
	// 确保 query 不为空
	if query == nil {
		query = &Query{}
	}

	totalDeleted := 0
	start := time.Now()

	for {
		if time.Since(start) > deleteTimeout {
			return fmt.Errorf("删除操作超时，已删除 %d 条记录", totalDeleted)
		}

		var keys []interface{}
		err := s.Badger().View(func(tx *badger.Txn) error {
			// 设置查询限制
			query.limit = batchSize
			return s.findMatchingKeys(tx, dataType, query, &keys)
		})
		if err != nil {
			return fmt.Errorf("查找匹配键失败: %v, 已删除 %d 条记录", err, totalDeleted)
		}

		if len(keys) == 0 {
			break
		}

		// 记录删除进度
		failed := 0
		for _, key := range keys {
			if err := s.Delete(key, dataType); err != nil {
				failed++
				logger.Errorf("删除记录失败 key=%v: %v", key, err)
				continue
			}
			totalDeleted++
		}

		// 输出进度日志
		// logger.Infof("批次删除进度: 成功=%d, 失败=%d, 总计=%d",
		// 	len(keys)-failed, failed, totalDeleted)
	}

	// logger.Infof("删除完成: 总计删除=%d, 耗时=%v", totalDeleted, time.Since(start))
	return nil
}

// findMatchingKeys 查找匹配的键
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - dataType: interface{} 类型，表示要查找匹配键的类型
//   - query: *Query 类型，表示查询条件
//   - keys: *[]interface{} 类型，表示存储匹配键的切片
//

func (s *Store) findMatchingKeys(tx *badger.Txn, dataType interface{}, query *Query, keys *[]interface{}) error {
	storer := s.newStorer(dataType)
	bookmark := &iterBookmark{}
	iter := s.newIterator(tx, storer.Type(), query, bookmark)
	defer iter.Close()

	for k, _ := iter.Next(); k != nil; k, _ = iter.Next() {
		key := s.decodeKey(k, dataType, storer.Type())
		*keys = append(*keys, key)

		if len(*keys) >= batchSize {
			break
		}
	}

	return nil
}

// TxDeleteMatching 修改为使用批量处理
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - dataType: interface{} 类型，表示要查找匹配键的类型
//   - query: *Query 类型，表示查询条件
//
// 返回值：
//   - error: 如果删除过程中出现错误，则返回错误信息
func (s *Store) TxDeleteMatching(tx *badger.Txn, dataType interface{}, query *Query) error {
	// 由于需要多个事务，不再支持外部传入事务
	logger.Warn("TxDeleteMatching 已弃用，请使用 DeleteMatching")
	return s.DeleteMatching(dataType, query)
}
