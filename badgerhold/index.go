// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"bytes"
	"reflect"
	"sort"

	"github.com/bpfs/defs/utils/logger"
	"github.com/dgraph-io/badger/v4"
)

// indexPrefix 是存储索引的键前缀
const indexPrefix = "_bhIndex"

// iteratorKeyMinCacheSize 是在从磁盘获取更多数据之前存储在内存中的迭代器键的最小缓存大小
const iteratorKeyMinCacheSize = 100

// Index 是一个结构体，包含一个返回可索引的编码字节的函数，以及一个标志来指示索引是否唯一
type Index struct {
	IndexFunc func(name string, value interface{}) ([]byte, error) // IndexFunc 返回传入值的索引编码字节
	Unique    bool                                                 // Unique 指示索引是否唯一
}

// indexAdd 将一个项目添加到索引中
// 参数:
//   - storer: Storer 类型，表示存储器接口，提供索引信息
//   - tx: *badger.Txn 类型，表示当前的 Badger 事务
//   - key: []byte 类型，表示要添加的项目的键
//   - data: interface{} 类型，表示要添加到索引中的数据
//
// 返回值：
//   - error: 如果添加过程中出现错误，则返回错误信息
func (s *Store) indexAdd(storer Storer, tx *badger.Txn, key []byte, data interface{}) error {
	// 获取存储器的所有索引
	indexes := storer.Indexes()
	// 遍历每个索引并进行更新
	for name, index := range indexes {
		err := s.indexUpdate(storer.Type(), name, index, tx, key, data, false)
		if err != nil {
			// 如果更新过程中出现错误，记录日志并返回错误信息
			logger.Error("索引添加失败", "错误", err)
			return err
		}
	}
	// 如果所有索引添加成功，返回 nil
	return nil
}

// indexDelete 从索引中删除一个项目
// 确保传入的是旧记录的数据，而不是新记录的数据
// 参数:
//   - storer: Storer 类型，表示存储器接口，提供索引信息
//   - tx: *badger.Txn 类型，表示当前的 Badger 事务
//   - key: []byte 类型，表示要删除的项目的键
//   - originalData: interface{} 类型，表示要从索引中删除的数据
//
// 返回值：
//   - error: 如果删除过程中出现错误，则返回错误信息
func (s *Store) indexDelete(storer Storer, tx *badger.Txn, key []byte, originalData interface{}) error {
	// 获取存储器的所有索引
	indexes := storer.Indexes()
	// 遍历每个索引并进行更新
	for name, index := range indexes {
		err := s.indexUpdate(storer.Type(), name, index, tx, key, originalData, true)
		if err != nil {
			// 如果更新过程中出现错误，记录日志并返回错误信息
			logger.Error("索引删除失败", "错误", err)
			return err
		}
	}
	// 如果所有索引删除成功，返回 nil
	return nil
}

// indexUpdate 在项目上添加或删除特定索引
// 参数:
//   - typeName: string 类型，表示数据类型的名称
//   - indexName: string 类型，表示索引的名称
//   - index: Index 类型，表示要更新的索引
//   - tx: *badger.Txn 类型，表示当前的 Badger 事务
//   - key: []byte 类型，表示要更新索引的项目的键
//   - value: interface{} 类型，表示要更新索引的项目的数据
//   - delete: bool 类型，指示是否是删除操作
//
// 返回值：
//   - error: 如果更新过程中出现错误，则返回错误信息
func (s *Store) indexUpdate(typeName, indexName string, index Index, tx *badger.Txn, key []byte, value interface{},
	delete bool) error {

	// 使用索引函数生成索引键
	indexKey, err := index.IndexFunc(indexName, value)
	if err != nil {
		// 如果生成索引键时出现错误，记录日志并返回错误信息
		logger.Error("生成索引键失败", "错误", err)
		return err
	}
	// 如果索引键为空，返回 nil
	if indexKey == nil {
		return nil
	}

	// 初始化索引值列表
	indexValue := make(KeyList, 0)

	// 为索引键添加前缀
	indexKey = append(indexKeyPrefix(typeName, indexName), indexKey...)

	// 从事务中获取索引项
	item, err := tx.Get(indexKey)
	if err != nil && err != badger.ErrKeyNotFound {
		// 如果获取索引项时出现其他错误，记录日志并返回错误信息
		logger.Error("获取索引项失败", "错误", err)
		return err
	}

	// 如果索引项存在且不是删除操作，并且索引是唯一的，返回唯一性错误
	if err != badger.ErrKeyNotFound {
		if index.Unique && !delete {
			logger.Error("唯一索引已存在")
			return ErrUniqueExists
		}
		// 解码索引项的值到索引值列表中
		err = item.Value(func(iVal []byte) error {
			return s.decode(iVal, &indexValue)
		})
		if err != nil {
			// 如果解码过程中出现错误，记录日志并返回错误信息
			logger.Error("解码索引值失败", "错误", err)
			return err
		}
	}

	// 如果是删除操作，则从索引值列表中移除键
	if delete {
		indexValue.remove(key)
	} else {
		// 否则，向索引值列表中添加键
		indexValue.add(key)
	}

	// 如果索引值列表为空，则删除索引项
	if len(indexValue) == 0 {
		return tx.Delete(indexKey)
	}

	// 对索引值列表进行编码
	iVal, err := s.encode(indexValue)
	if err != nil {
		// 如果编码过程中出现错误，记录日志并返回错误信息
		logger.Error("编码索引值失败", "错误", err)
		return err
	}

	// 将编码后的索引值设置到索引键中
	return tx.Set(indexKey, iVal)
}

// indexKeyPrefix 返回存储此索引的 Badger 键的前缀
// 参数:
//   - typeName: string 类型，表示数据类型的名称
//   - indexName: string 类型，表示索引的名称
//
// 返回值：
//   - []byte: 表示索引键前缀的字节切片
func indexKeyPrefix(typeName, indexName string) []byte {
	return []byte(indexPrefix + ":" + typeName + ":" + indexName + ":")
}

// newIndexKey 返回存储此索引的 Badger 键
// 参数:
//   - typeName: string 类型，表示数据类型的名称
//   - indexName: string 类型，表示索引的名称
//   - value: []byte 类型，表示索引的值
//
// 返回值：
//   - []byte: 表示索引键的字节切片
func newIndexKey(typeName, indexName string, value []byte) []byte {
	return append(indexKeyPrefix(typeName, indexName), value...)
}

// KeyList 是一个唯一的、排序的键（[]byte）的切片，例如索引所指向的内容
type KeyList [][]byte

// add 将键添加到 KeyList 中
// 参数:
//   - key: []byte 类型，表示要添加的键
func (v *KeyList) add(key []byte) {
	// 使用二分查找找到插入位置
	i := sort.Search(len(*v), func(i int) bool {
		return bytes.Compare((*v)[i], key) >= 0
	})

	// 如果键已经存在，则不进行添加
	if i < len(*v) && bytes.Equal((*v)[i], key) {
		return
	}

	// 在插入位置前移一位
	*v = append(*v, nil)
	copy((*v)[i+1:], (*v)[i:])
	(*v)[i] = key
}

// remove 从 KeyList 中移除键
// 参数:
//   - key: []byte 类型，表示要移除的键
func (v *KeyList) remove(key []byte) {
	// 使用二分查找找到键的位置
	i := sort.Search(len(*v), func(i int) bool {
		return bytes.Compare((*v)[i], key) >= 0
	})

	// 如果找到键，则将其移除
	if i < len(*v) {
		copy((*v)[i:], (*v)[i+1:])
		(*v)[len(*v)-1] = nil
		*v = (*v)[:len(*v)-1]
	}
}

// in 检查 KeyList 中是否存在指定的键
// 参数:
//   - key: []byte 类型，表示要检查的键
//
// 返回值：
//   - bool: 如果键存在，则返回 true；否则返回 false
func (v *KeyList) in(key []byte) bool {
	// 使用二分查找找到键的位置
	i := sort.Search(len(*v), func(i int) bool {
		return bytes.Compare((*v)[i], key) >= 0
	})

	// 如果找到键并且与指定的键相等，返回 true
	return (i < len(*v) && bytes.Equal((*v)[i], key))
}

// indexExists 检查指定类型和索引名称的索引是否存在
// 参数:
//   - it: *badger.Iterator 类型，表示用于遍历 Badger 键的迭代器
//   - typeName: string 类型，表示数据类型的名称
//   - indexName: string 类型，表示索引的名称
//
// 返回值：
//   - bool: 如果索引存在，则返回 true；否则返回 false
func indexExists(it *badger.Iterator, typeName, indexName string) bool {
	iPrefix := indexKeyPrefix(typeName, indexName) // 获取索引键前缀
	tPrefix := typePrefix(typeName)                // 获取类型前缀

	// 检查类型是否存在数据
	it.Seek(tPrefix)
	if !it.ValidForPrefix(tPrefix) {
		// 如果存储中没有该数据类型的数据，则索引可能存在
		// 这样我们就不会因为"坏索引"而失败，因为它们可能只是在对空数据集运行查询
		return true
	}

	// 检查索引是否存在
	it.Seek(iPrefix)
	return it.ValidForPrefix(iPrefix)
}

// iterator 是用于遍历查询结果的迭代器结构体
type iterator struct {
	keyCache [][]byte                                 // keyCache 缓存了当前迭代器中的键
	nextKeys func(*badger.Iterator) ([][]byte, error) // nextKeys 是一个函数，用于获取下一批键
	iter     *badger.Iterator                         // iter 是 Badger 的迭代器
	bookmark *iterBookmark                            // bookmark 存储迭代器的位置，以便在事务中共享单个 RW 迭代器
	lastSeek []byte                                   // lastSeek 存储最后一次 seek 的键
	tx       *badger.Txn                              // tx 是当前事务
	err      error                                    // err 存储迭代器的最后一个错误
}

// iterBookmark 存储一个特定迭代器的 seek 位置，以便在单个事务中共享一个 RW 迭代器
type iterBookmark struct {
	iter    *badger.Iterator // iter 是与 bookmark 关联的 Badger 迭代器
	seekKey []byte           // seekKey 是迭代器上次 seek 的键
}

// newIterator 创建并初始化一个新的迭代器，用于遍历查询结果
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - typeName: string 类型，表示数据类型的名称
//   - query: *Query 类型，表示查询条件
//   - bookmark: *iterBookmark 类型，表示迭代器书签（可选）
//
// 返回值：
//   - *iterator: 返回初始化后的迭代器
func (s *Store) newIterator(tx *badger.Txn, typeName string, query *Query, bookmark *iterBookmark) *iterator {
	i := &iterator{
		tx: tx,
	}

	// 如果提供了 bookmark，则使用其中的迭代器
	if bookmark != nil {
		i.iter = bookmark.iter
	} else {
		// 否则，创建一个新的迭代器
		i.iter = tx.NewIterator(badger.DefaultIteratorOptions)
	}

	var prefix []byte

	// 如果查询中指定了索引，检查索引是否存在
	if query.index != "" {
		query.badIndex = !indexExists(i.iter, typeName, query.index)
	}

	// 获取查询中与索引相关的字段条件
	criteria := query.fieldCriteria[query.index]
	// 如果条件包含匹配函数，不能使用索引，因为匹配函数无法在索引上测试
	if hasMatchFunc(criteria) {
		criteria = nil
	}

	// 如果未指定键字段或索引字段，则测试键字段或返回所有记录
	if query.index == "" || len(criteria) == 0 {
		prefix = typePrefix(typeName) // 获取类型前缀
		i.iter.Seek(prefix)           // 使用前缀初始化迭代器
		i.nextKeys = func(iter *badger.Iterator) ([][]byte, error) {
			var nKeys [][]byte

			// 获取下一批键，直到缓存大小达到 iteratorKeyMinCacheSize
			for len(nKeys) < iteratorKeyMinCacheSize {
				if !iter.ValidForPrefix(prefix) {
					return nKeys, nil
				}

				item := iter.Item()      // 获取当前项目
				key := item.KeyCopy(nil) // 复制键

				var ok bool
				// 如果没有条件，则直接返回键用于值测试
				if len(criteria) == 0 {
					ok = true
				} else {
					val := reflect.New(query.dataType)

					// 解码当前项的值
					err := item.Value(func(v []byte) error {
						return s.decode(v, val.Interface())
					})
					if err != nil {
						logger.Error("解码项目值失败", "错误", err)
						return nil, err
					}

					// 检查是否符合所有条件
					ok, err = s.matchesAllCriteria(criteria, key, true, typeName, val.Interface())
					if err != nil {
						logger.Error("匹配条件失败", "错误", err)
						return nil, err
					}
				}

				// 如果符合条件，将键添加到结果列表
				if ok {
					nKeys = append(nKeys, key)
				}
				i.lastSeek = key // 更新 lastSeek 为当前键
				iter.Next()      // 移动到下一个项目
			}
			return nKeys, nil
		}

		return i
	}

	// 如果指定了索引字段，从索引中获取键
	prefix = indexKeyPrefix(typeName, query.index)
	i.iter.Seek(prefix) // 使用索引前缀初始化迭代器
	i.nextKeys = func(iter *badger.Iterator) ([][]byte, error) {
		var nKeys [][]byte

		// 获取下一批键，直到缓存大小达到 iteratorKeyMinCacheSize
		for len(nKeys) < iteratorKeyMinCacheSize {
			if !iter.ValidForPrefix(prefix) {
				return nKeys, nil
			}

			item := iter.Item()      // 获取当前项目
			key := item.KeyCopy(nil) // 复制键

			// 移除索引前缀进行匹配
			ok, err := s.matchesAllCriteria(criteria, key[len(prefix):], true, "", nil)
			if err != nil {
				logger.Error("匹配条件失败", "错误", err)
				return nil, err
			}

			// 如果符合条件，将键添加到结果列表
			if ok {
				err = item.Value(func(v []byte) error {
					var keys = make(KeyList, 0)
					// 解码并将索引中的键添加到结果列表
					err := s.decode(v, &keys)
					if err != nil {
						logger.Error("解码索引值失败", "错误", err)
						return err
					}

					nKeys = append(nKeys, [][]byte(keys)...)
					return nil
				})
				if err != nil {
					logger.Error("获取索引值失败", "错误", err)
					return nil, err
				}
			}

			i.lastSeek = key // 更新 lastSeek 为当前键
			iter.Next()      // 移动到下一个项目

		}
		return nKeys, nil
	}

	return i
}

// createBookmark 创建并返回一个迭代器书签，保存当前迭代器的位置
// 返回值：
//   - *iterBookmark: 返回创建的迭代器书签
func (i *iterator) createBookmark() *iterBookmark {
	return &iterBookmark{
		iter:    i.iter,     // 保存当前迭代器
		seekKey: i.lastSeek, // 保存最后一次 seek 的键
	}
}

// Next 返回下一个符合迭代器条件的键值对
// 如果没有更多的键值可用，则返回 nil；如果发生错误，也返回 nil，并通过 iterator.Error() 返回错误
// 返回值：
//   - key: []byte 类型，下一个键值对的键
//   - value: []byte 类型，下一个键值对的值
func (i *iterator) Next() (key []byte, value []byte) {
	if i.err != nil {
		return nil, nil
	}

	// 如果 keyCache 为空，获取下一批键
	if len(i.keyCache) == 0 {
		newKeys, err := i.nextKeys(i.iter)
		if err != nil {
			i.err = err
			logger.Error("获取下一批键失败", "错误", err)
			return nil, nil
		}

		if len(newKeys) == 0 {
			return nil, nil
		}

		i.keyCache = append(i.keyCache, newKeys...)
	}

	// 从 keyCache 中获取下一个键
	key = i.keyCache[0]
	i.keyCache = i.keyCache[1:]

	// 获取键对应的值
	item, err := i.tx.Get(key)
	if err != nil {
		i.err = err
		logger.Error("获取键对应的值失败", "错误", err)
		return nil, nil
	}

	// 读取值并将其返回
	err = item.Value(func(val []byte) error {
		value = val
		return nil
	})
	if err != nil {
		i.err = err
		logger.Error("读取值失败", "错误", err)
		return nil, nil
	}

	return
}

// Error 返回迭代器的最后一个错误，如果存在错误，iterator.Next() 将不会继续
// 返回值：
//   - error: 迭代器的最后一个错误
func (i *iterator) Error() error {
	return i.err
}

// Close 关闭迭代器
func (i *iterator) Close() {
	if i.bookmark != nil {
		// 如果存在书签，将迭代器定位到书签位置
		i.iter.Seek(i.bookmark.seekKey)
		return
	}

	// 否则关闭迭代器
	i.iter.Close()
}
