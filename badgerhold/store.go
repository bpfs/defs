// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"reflect"
	"strings"
	"sync"

	"github.com/bpfs/defs/utils/logger"
	"github.com/dgraph-io/badger/v4"
)

const (
	// BadgerHoldIndexTag 是用于将字段定义为可索引的 badgerhold 结构体标签
	BadgerHoldIndexTag = "badgerholdIndex"

	// BadgerholdKeyTag 是用于将字段定义为键以用于 Find 查询的结构体标签
	BadgerholdKeyTag = "badgerholdKey"

	// badgerholdPrefixTag 是用于结构体标签的替代（更标准）版本的前缀
	badgerholdPrefixTag         = "badgerhold"
	badgerholdPrefixIndexValue  = "index"  // 表示索引标签的值
	badgerholdPrefixKeyValue    = "key"    // 表示键标签的值
	badgerholdPrefixUniqueValue = "unique" // 表示唯一性标签的值
)

// Store 是 badgerhold 的包装器，封装了一个 Badger 数据库
type Store struct {
	db               *badger.DB // Badger 数据库实例
	sequenceBandwith uint64     // 序列带宽，用于生成序列键
	sequences        *sync.Map  // 用于存储序列生成器的同步映射

	encode EncodeFunc // 用于编码的函数
	decode DecodeFunc // 用于解码的函数
}

// Options 允许设置不同于默认值的选项
// 例如，编码和解码函数，默认为 Gob
type Options struct {
	Encoder          EncodeFunc // 用于编码的函数
	Decoder          DecodeFunc // 用于解码的函数
	SequenceBandwith uint64     // 序列带宽，用于生成序列键
	badger.Options              // Badger 的配置选项
}

// DefaultOptions 是一组用于打开 BadgerHold 数据库的默认选项
// 包括 Badger 自己的默认选项
var DefaultOptions = Options{
	Options:          badger.DefaultOptions(""), // 使用 Badger 的默认选项
	Encoder:          DefaultEncode,             // 默认的编码函数为 Gob 编码
	Decoder:          DefaultDecode,             // 默认的解码函数为 Gob 解码
	SequenceBandwith: 100,                       // 默认的序列带宽为 100
}

// Open 打开或创建一个 BadgerHold 文件
// 参数:
//   - options: Options 类型，表示要使用的选项
//
// 返回值：
//   - *Store: 返回打开的 Store 实例
//   - error: 如果打开过程中出现错误，则返回错误信息
func Open(options Options) (*Store, error) {
	// 打开 Badger 数据库
	db, err := badger.Open(options.Options)
	if err != nil {
		logger.Error("打开Badger数据库失败", "错误", err)
		return nil, err
	}

	// 返回初始化后的 Store 实例
	return &Store{
		db:               db,
		sequenceBandwith: options.SequenceBandwith,
		sequences:        &sync.Map{},
		encode:           options.Encoder,
		decode:           options.Decoder,
	}, nil
}

// Badger 返回 BadgerHold 所基于的底层 Badger 数据库
// 返回值：
//   - *badger.DB: 返回底层的 Badger 数据库实例
func (s *Store) Badger() *badger.DB {
	return s.db
}

// Close 关闭 Badger 数据库
// 返回值：
//   - error: 如果关闭过程中出现错误，则返回错误信息
func (s *Store) Close() error {
	var err error
	// 遍历所有的序列生成器并释放它们
	s.sequences.Range(func(key, value interface{}) bool {
		err = value.(*badger.Sequence).Release() // 释放序列生成器
		if err != nil {
			logger.Error("释放序列生成器失败", "错误", err)
		}
		return err == nil
	})
	if err != nil {
		return err
	}
	// 关闭 Badger 数据库
	err = s.db.Close()
	if err != nil {
		logger.Error("关闭Badger数据库失败", "错误", err)
	}
	return err
}

/*
	注意: 不打算实现 ReIndex 和 Remove index
	最初我创建这些功能是为了使从普通的 Bolt 或 Badger DB 迁移更容易，
	但由于存在数据丢失的风险，可能更好地由开发人员自己直接管理数据迁移。
	如果你不同意，欢迎提出 issue，我们可以重新讨论这个问题。
*/

// Storer 是一个接口，用于实现跳过传入 badgerhold 的所有数据的反射调用
type Storer interface {
	Type() string              // 用作 badgerdb 索引前缀
	Indexes() map[string]Index // [索引名称]索引函数
}

// anonStorer 是通过反射创建的未知接口类型的存储器
type anonStorer struct {
	rType   reflect.Type     // 存储器的反射类型
	indexes map[string]Index // 存储器的索引映射
}

// Type 返回通过反射包确定的类型名称
// 返回值：
//   - string: 返回类型名称
func (t *anonStorer) Type() string {
	return t.rType.Name()
}

// Indexes 返回通过反射包确定的此类型的索引
// 返回值：
//   - map[string]Index: 返回索引映射
func (t *anonStorer) Indexes() map[string]Index {
	return t.indexes
}

// newStorer 基于传入的数据类型的反射创建一个满足 Storer 接口的类型
// 如果类型不满足 Storer 的要求（例如没有名称），则会触发 panic
// 通过在类型上实现 Storer 接口，可以避免任何反射开销
// 参数:
//   - dataType: interface{} 类型，表示要反射的数据类型
//
// 返回值：
//   - Storer: 返回实现了 Storer 接口的存储器实例
func (s *Store) newStorer(dataType interface{}) Storer {
	// 如果 dataType 已经实现了 Storer 接口，则直接返回
	if storer, ok := dataType.(Storer); ok {
		return storer
	}

	// 获取数据类型的反射类型
	tp := reflect.TypeOf(dataType)
	for tp.Kind() == reflect.Ptr {
		tp = tp.Elem()
	}

	// 创建 anonStorer 实例
	storer := &anonStorer{
		rType:   tp,
		indexes: make(map[string]Index),
	}

	// 如果类型名称为空，触发 panic
	if storer.rType.Name() == "" {
		logger.Error("无效的Storer类型", "错误", "类型未命名")
		panic("无效的Storer类型：类型未命名")
	}

	// 如果类型不是结构体，触发 panic
	if storer.rType.Kind() != reflect.Struct {
		logger.Error("无效的Storer类型", "错误", "BadgerHold只能处理结构体")
		panic("无效的Storer类型：BadgerHold只能处理结构体")
	}

	// 遍历结构体的所有字段并检查索引标签
	for i := 0; i < storer.rType.NumField(); i++ {
		indexName := ""
		unique := false

		// 检查字段是否具有 BadgerHoldIndexTag 标签
		if strings.Contains(string(storer.rType.Field(i).Tag), BadgerHoldIndexTag) {
			indexName = storer.rType.Field(i).Tag.Get(BadgerHoldIndexTag)

			// 如果索引名称为空，则使用字段名称作为索引名称
			if indexName == "" {
				indexName = storer.rType.Field(i).Name
			}
		} else if tag := storer.rType.Field(i).Tag.Get(badgerholdPrefixTag); tag != "" {
			// 检查字段是否具有 badgerhold 前缀标签
			if tag == badgerholdPrefixIndexValue {
				// 索引名称规范化为字段名称
				indexName = storer.rType.Field(i).Name
			} else if tag == badgerholdPrefixUniqueValue {
				indexName = storer.rType.Field(i).Name
				unique = true
			}
		}

		// 如果找到索引名称，则创建索引并添加到索引映射中
		if indexName != "" {
			storer.indexes[indexName] = Index{
				IndexFunc: func(name string, value interface{}) ([]byte, error) {
					tp := reflect.ValueOf(value)
					for tp.Kind() == reflect.Ptr {
						tp = tp.Elem()
					}
					// 使用 Store 的 encode 方法对字段值进行编码
					return s.encode(tp.FieldByName(name).Interface())
				},
				Unique: unique,
			}
		}
	}

	return storer
}

// getSequence 获取指定类型的下一个序列值
// 参数:
//   - typeName: string 类型，表示数据类型的名称
//
// 返回值：
//   - uint64: 返回下一个序列值
//   - error: 如果获取序列值时发生错误，返回错误信息
func (s *Store) getSequence(typeName string) (uint64, error) {
	// 尝试从 sequences 映射中加载序列生成器
	seq, ok := s.sequences.Load(typeName)
	if !ok {
		// 如果未找到，创建新的序列生成器
		newSeq, err := s.Badger().GetSequence([]byte(typeName), s.sequenceBandwith)
		if err != nil {
			logger.Error("创建序列生成器失败", "错误", err)
			return 0, err
		}
		// 存储新的序列生成器到 sequences 映射中
		s.sequences.Store(typeName, newSeq)
		seq = newSeq
	}

	// 返回下一个序列值
	return seq.(*badger.Sequence).Next()
}

// typePrefix 返回指定类型的前缀，用于存储在 Badger 中的键
// 参数:
//   - typeName: string 类型，表示数据类型的名称
//
// 返回值：
//   - []byte: 返回类型前缀的字节切片
func typePrefix(typeName string) []byte {
	return []byte("bh_" + typeName + ":")
}

// getKeyField 获取结构体类型中标记为键的字段
// 参数:
//   - tp: reflect.Type 类型，表示结构体的反射类型
//
// 返回值：
//   - reflect.StructField: 返回结构体字段
//   - bool: 如果找到键字段，返回 true；否则返回 false
func getKeyField(tp reflect.Type) (reflect.StructField, bool) {
	for i := 0; i < tp.NumField(); i++ {
		// 检查字段是否具有 BadgerholdKeyTag 标签
		if strings.HasPrefix(string(tp.Field(i).Tag), BadgerholdKeyTag) {
			return tp.Field(i), true
		}

		// 检查字段是否具有 badgerhold 前缀标签，并且值为键
		if tag := tp.Field(i).Tag.Get(badgerholdPrefixTag); tag == badgerholdPrefixKeyValue {
			return tp.Field(i), true
		}
	}

	return reflect.StructField{}, false
}

// newElemType 创建数据类型的新实例
// 参数:
//   - datatype: interface{} 类型，表示要创建实例的数据类型
//
// 返回值：
//   - interface{}: 返回新创建的实例
func newElemType(datatype interface{}) interface{} {
	tp := reflect.TypeOf(datatype)
	for tp.Kind() == reflect.Ptr {
		tp = tp.Elem()
	}

	return reflect.New(tp).Interface()
}

// getElem 确保操作的接口不是指针
// 参数:
//   - value: interface{} 类型，表示要获取元素的值
//
// 返回值：
//   - interface{}: 返回非指针类型的值
func getElem(value interface{}) interface{} {
	for reflect.TypeOf(value).Kind() == reflect.Ptr {
		value = reflect.ValueOf(value).Elem().Interface()
	}
	return value
}
