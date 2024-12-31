// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/bpfs/defs/utils/logger"
	"github.com/dgraph-io/badger/v4"
)

const (
	eq    = iota // 等于 (==)
	ne           // 不等于 (!=)
	gt           // 大于 (>)
	lt           // 小于 (<)
	ge           // 大于等于 (>=)
	le           // 小于等于 (<=)
	in           // 包含 (in)
	re           // 正则表达式 (regular expression)
	fn           // 函数 (func)
	isnil        // 是否为 nil (test's for nil)
	sw           // 字符串以...开头 (string starts with)
	ew           // 字符串以...结尾 (string ends with)
	hk           // 匹配 map 的键 (match map keys)

	contains // 仅适用于 slice，判断是否包含指定值
	any      // 仅适用于 slice，判断是否包含任意一个指定值
	all      // 仅适用于 slice，判断是否包含所有指定值
)

// Key 是用于在 badgerhold 中指定查询键的简写，只返回空字符串
// 例如：Where(badgerhold.Key).Eq("testkey")
const Key = ""

// Query 是一系列条件的集合，badgerhold 中的对象需要匹配这些条件才能被返回
// 空查询会匹配所有记录
type Query struct {
	index         string                  // 用于查询的索引名称
	currentField  string                  // 当前字段名称
	fieldCriteria map[string][]*Criterion // 字段的查询条件
	ors           []*Query                // 用于存储 OR 查询的条件集合

	badIndex bool          // 指示索引是否无效
	dataType reflect.Type  // 查询的数据类型
	tx       *badger.Txn   // 当前事务
	writable bool          // 是否可写
	subquery bool          // 是否为子查询
	bookmark *iterBookmark // 迭代器书签

	limit   int      // 查询结果的最大返回数量
	skip    int      // 跳过的记录数量
	sort    []string // 排序字段集合
	reverse bool     // 是否反转结果集
}

// Slice 将任何类型的切片转换为 []interface{}，通过复制切片值，以便可以轻松传递到接受可变参数的查询中。
// 如果传入的值不是切片，则会触发 panic
// 参数:
//   - value: interface{} 类型，表示要转换的切片
//
// 返回值：
//   - []interface{}: 转换后的接口切片
func Slice(value interface{}) []interface{} {
	slc := reflect.ValueOf(value)

	s := make([]interface{}, slc.Len(), slc.Len()) // 如果值不是切片、数组或 map，会触发 panic
	for i := range s {
		s[i] = slc.Index(i).Interface()
	}
	return s
}

// IsEmpty 如果查询为空，则返回 true。空查询会匹配所有记录。
// 返回值：
//   - bool: 如果查询为空，返回 true，否则返回 false
func (q *Query) IsEmpty() bool {
	if q.index != "" {
		return false
	}
	if len(q.fieldCriteria) != 0 {
		return false
	}

	if q.ors != nil {
		return false
	}

	return true
}

// Criterion 是一个操作符和一个值，字段需要与之匹配
type Criterion struct {
	query    *Query        // 关联的查询
	operator int           // 操作符
	value    interface{}   // 单个值
	values   []interface{} // 多个值
}

// hasMatchFunc 检查条件集合中是否包含函数操作符
// 参数:
//   - criteria: []*Criterion 类型，表示条件集合
//
// 返回值：
//   - bool: 如果包含函数操作符，返回 true，否则返回 false
func hasMatchFunc(criteria []*Criterion) bool {
	for _, c := range criteria {
		if c.operator == fn {
			return true
		}
	}
	return false
}

// Field 允许引用正在比较的结构中的字段
type Field string

// Where 开始一个查询，用于指定 badgerhold 中对象需要匹配的条件
// 例如：
// s.Find(badgerhold.Where("FieldName").Eq(value).And("AnotherField").Lt(AnotherValue).
// Or(badgerhold.Where("FieldName").Eq(anotherValue)
// 由于 Gobs 只编码导出的字段，如果传入的字段名称首字母为小写，则会触发 panic
// 参数:
//   - field: string 类型，表示字段名称
//
// 返回值：
//   - *Criterion: 返回创建的条件
func Where(field string) *Criterion {
	if !startsUpper(field) {
		logger.Error("badgerhold查询中的字段首字母必须大写")
		panic("badgerhold查询中的字段首字母必须大写")
	}

	return &Criterion{
		query: &Query{
			currentField:  field,
			fieldCriteria: make(map[string][]*Criterion),
		},
	}
}

// And 创建另一个应用于查询的条件
// 参数:
//   - field: string 类型，表示字段名称
//
// 返回值：
//   - *Criterion: 返回创建的条件
func (q *Query) And(field string) *Criterion {
	if !startsUpper(field) {
		logger.Error("badgerhold查询中的字段首字母必须大写")
		panic("badgerhold查询中的字段首字母必须大写")
	}

	q.currentField = field
	return &Criterion{
		query: q,
	}
}

// Skip 跳过与查询条件匹配的记录，并不在结果集中返回这些记录。设置 skip 多次或为负数会触发 panic
// 参数:
//   - amount: int 类型，表示要跳过的记录数量
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (q *Query) Skip(amount int) *Query {
	if amount < 0 {
		logger.Error("Skip必须设置为正数")
		panic("Skip必须设置为正数")
	}

	if q.skip != 0 {
		logger.Error(fmt.Sprintf("Skip已经被设置为%d", q.skip))
		panic(fmt.Sprintf("Skip已经被设置为%d", q.skip))
	}

	q.skip = amount

	return q
}

// Limit 设置查询最多返回的记录数量。多次设置 Limit 或将其设置为负值会触发 panic
// 参数:
//   - amount: int 类型，表示最大返回记录数量
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (q *Query) Limit(amount int) *Query {
	if amount < 0 {
		logger.Error("Limit必须设置为正数")
		panic("Limit必须设置为正数")
	}

	if q.limit != 0 {
		logger.Error(fmt.Sprintf("Limit已经被设置为%d", q.limit))
		panic(fmt.Sprintf("Limit已经被设置为%d", q.limit))
	}

	q.limit = amount

	return q
}

// Contains 测试当前字段是否为包含传入值的切片
// 参数:
//   - value: interface{} 类型，表示要检查的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Contains(value interface{}) *Query {
	return c.op(contains, value)
}

// ContainsAll 测试当前字段是否为包含所有传入值的切片。如果切片中不包含任何一个值，则不匹配
// 参数:
//   - values: 可变参数，表示要检查的多个值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) ContainsAll(values ...interface{}) *Query {
	c.operator = all
	c.values = values

	q := c.query
	q.fieldCriteria[q.currentField] = append(q.fieldCriteria[q.currentField], c)

	return q
}

// ContainsAny 测试当前字段是否为包含任意一个传入值的切片。如果切片中包含任何一个值，则匹配
// 参数:
//   - values: 可变参数，表示要检查的多个值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) ContainsAny(values ...interface{}) *Query {
	c.operator = any
	c.values = values

	q := c.query
	q.fieldCriteria[q.currentField] = append(q.fieldCriteria[q.currentField], c)

	return q
}

// HasKey 测试字段是否具有与传入值匹配的 map 键
// 参数:
//   - value: interface{} 类型，表示要检查的键值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) HasKey(value interface{}) *Query {
	return c.op(hk, value)
}

// SortBy 根据给定的字段名称排序结果集。可以使用多个字段
// 参数:
//   - fields: 可变参数，表示排序的字段名称
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (q *Query) SortBy(fields ...string) *Query {
	for i := range fields {
		if fields[i] == Key {
			logger.Error("不能按Key排序")
			panic("不能按Key排序")
		}
		var found bool
		for k := range q.sort {
			if q.sort[k] == fields[i] {
				found = true
				break
			}
		}
		if !found {
			q.sort = append(q.sort, fields[i])
		}
	}
	return q
}

// Reverse 反转当前结果集，与 SortBy 一起使用
// 返回值：
//   - *Query: 返回更新后的查询对象
func (q *Query) Reverse() *Query {
	q.reverse = !q.reverse
	return q
}

// Index 指定运行此查询时使用的索引
// 参数:
//   - indexName: string 类型，表示索引名称
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (q *Query) Index(indexName string) *Query {
	if strings.Contains(indexName, ".") {
		// 注：将来可能会重新考虑这个限制
		logger.Error("不支持嵌套索引。只有顶层结构可以被索引")
		panic("不支持嵌套索引。只有顶层结构可以被索引")
	}
	q.index = indexName
	return q
}

// validateIndex 验证查询中指定的索引是否存在
// 参数:
//   - data: interface{} 类型，表示要检查的数据
//
// 返回值：
//   - error: 如果索引无效，返回错误信息
func (q *Query) validateIndex(data interface{}) error {
	if q.index == "" {
		return nil
	}
	if q.dataType == nil {
		logger.Error("在设置查询数据类型之前无法检查索引是否有效")
		panic("在设置查询数据类型之前无法检查索引是否有效")
	}

	// 如果数据类型实现了 Storer 接口，检查索引是否存在
	if storer, ok := data.(Storer); ok {
		if _, ok = storer.Indexes()[q.index]; ok {
			return nil
		} else {
			logger.Error(fmt.Sprintf("索引 %s 不存在", q.index))
			return fmt.Errorf("索引 %s 不存在", q.index)
		}
	}

	// 检查数据类型是否有与索引名称匹配的字段
	if _, ok := q.dataType.FieldByName(q.index); ok {
		return nil
	}

	// 检查数据类型的字段标签是否与索引名称匹配
	for i := 0; i < q.dataType.NumField(); i++ {
		if tag := q.dataType.Field(i).Tag.Get(BadgerHoldIndexTag); tag == q.index {
			q.index = q.dataType.Field(i).Name
			return nil
		}
	}
	// 未找到字段名称或自定义索引名称

	logger.Error(fmt.Sprintf("索引 %s 不存在", q.index))
	return fmt.Errorf("索引 %s 不存在", q.index)
}

// Or 创建另一个单独的查询，并将其与查询中的其他结果合并
// 如果传入的查询包含 limit 或 skip 值，则 Or 会触发 panic，因为它们只允许在顶层查询中使用
// 参数:
//   - query: *Query 类型，表示要合并的查询
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (q *Query) Or(query *Query) *Query {
	if query.skip != 0 || query.limit != 0 {
		logger.Error("Or查询不能包含skip或limit值")
		panic("Or查询不能包含skip或limit值")
	}
	q.ors = append(q.ors, query)
	return q
}

// Matches 返回提供的数据是否与查询匹配。
// 将匹配所有字段条件，包括嵌套的 OR 查询，但忽略限制、跳过、排序顺序等。
// 参数:
//   - s: *Store 类型，表示要查询的存储
//   - data: interface{} 类型，表示要检查的数据
//
// 返回值：
//   - bool: 如果匹配，返回 true，否则返回 false
//   - error: 如果匹配过程中出现错误，返回错误信息
func (q *Query) Matches(s *Store, data interface{}) (bool, error) {
	var key []byte
	dataVal := reflect.ValueOf(data)
	for dataVal.Kind() == reflect.Ptr {
		dataVal = dataVal.Elem()
	}
	data = dataVal.Interface()
	storer := s.newStorer(data)
	if keyField, ok := getKeyField(dataVal.Type()); ok {
		fieldValue := dataVal.FieldByName(keyField.Name)
		var err error
		key, err = s.encodeKey(fieldValue.Interface(), storer.Type())
		if err != nil {
			logger.Error("编码键时出错", err)
			return false, err
		}
	}
	return q.matches(s, key, dataVal, data)
}

// matches 检查提供的数据是否与查询匹配
// 参数:
//   - s: *Store 类型，表示要查询的存储
//   - key: []byte 类型，表示数据的键
//   - value: reflect.Value 类型，表示要检查的值
//   - data: interface{} 类型，表示要检查的数据
//
// 返回值：
//   - bool: 如果匹配，返回 true，否则返回 false
//   - error: 如果匹配过程中出现错误，返回错误信息
func (q *Query) matches(s *Store, key []byte, value reflect.Value, data interface{}) (bool, error) {
	if result, err := q.matchesAllFields(s, key, value, data); result || err != nil {
		return result, err
	}
	for _, orQuery := range q.ors {
		if result, err := orQuery.matches(s, key, value, data); result || err != nil {
			return result, err
		}
	}
	return false, nil
}

// matchesAllFields 检查提供的数据是否与所有字段条件匹配
// 参数:
//   - s: *Store 类型，表示要查询的存储
//   - key: []byte 类型，表示数据的键
//   - value: reflect.Value 类型，表示要检查的值
//   - currentRow: interface{} 类型，表示当前行的数据
//
// 返回值：
//   - bool: 如果匹配，返回 true，否则返回 false
//   - error: 如果匹配过程中出现错误，返回错误信息
func (q *Query) matchesAllFields(s *Store, key []byte, value reflect.Value, currentRow interface{}) (bool, error) {
	if q.IsEmpty() {
		return true, nil
	}

	for field, criteria := range q.fieldCriteria {
		if field == q.index && !q.badIndex && !hasMatchFunc(criteria) {
			// 已经由索引迭代器处理
			continue
		}

		if field == Key {
			ok, err := s.matchesAllCriteria(criteria, key, true, q.dataType.Name(), currentRow)
			if err != nil {
				logger.Error("匹配键条件时出错", err)
				return false, err
			}
			if !ok {
				return false, nil
			}

			continue
		}

		fVal, err := fieldValue(value, field)
		if err != nil {
			logger.Error("获取字段值时出错", err)
			return false, err
		}

		ok, err := s.matchesAllCriteria(criteria, fVal.Interface(), false, "", currentRow)
		if err != nil {
			logger.Error("匹配字段条件时出错", err)
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

// fieldValue 获取给定字段的值
// 参数:
//   - value: reflect.Value 类型，表示要获取字段值的结构体
//   - field: string 类型，表示字段名称
//
// 返回值：
//   - reflect.Value: 返回字段的值
//   - error: 如果字段不存在，返回错误信息
func fieldValue(value reflect.Value, field string) (reflect.Value, error) {
	fields := strings.Split(field, ".")

	current := value
	for i := range fields {
		if current.Kind() == reflect.Ptr {
			current = current.Elem().FieldByName(fields[i])
		} else {
			current = current.FieldByName(fields[i])
		}
		if !current.IsValid() {
			logger.Error(fmt.Sprintf("字段 %s 不存在", field))
			return reflect.Value{}, fmt.Errorf("字段 %s 不存在", field)
		}
	}
	return current, nil
}

// op 将操作符应用于条件并更新查询
// 参数:
//   - op: int 类型，表示操作符
//   - value: interface{} 类型，表示操作符应用的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) op(op int, value interface{}) *Query {
	c.operator = op
	c.value = value

	q := c.query
	q.fieldCriteria[q.currentField] = append(q.fieldCriteria[q.currentField], c)

	return q
}

// Eq 测试当前字段是否等于传入的值
// 参数:
//   - value: interface{} 类型，表示要比较的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Eq(value interface{}) *Query {
	return c.op(eq, value)
}

// Ne 测试当前字段是否不等于传入的值
// 参数:
//   - value: interface{} 类型，表示要比较的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Ne(value interface{}) *Query {
	return c.op(ne, value)
}

// Gt 测试当前字段是否大于传入的值
// 参数:
//   - value: interface{} 类型，表示要比较的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Gt(value interface{}) *Query {
	return c.op(gt, value)
}

// Lt 测试当前字段是否小于传入的值
// 参数:
//   - value: interface{} 类型，表示要比较的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Lt(value interface{}) *Query {
	return c.op(lt, value)
}

// Ge 测试当前字段是否大于或等于传入的值
// 参数:
//   - value: interface{} 类型，表示要比较的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Ge(value interface{}) *Query {
	return c.op(ge, value)
}

// Le 测试当前字段是否小于或等于传入的值
// 参数:
//   - value: interface{} 类型，表示要比较的值
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) Le(value interface{}) *Query {
	return c.op(le, value)
}

// In 测试当前字段是否是传入的值列表的成员
// 参数:
//   - values: 可变参数，表示要比较的值列表
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) In(values ...interface{}) *Query {
	c.operator = in
	c.values = values

	q := c.query
	q.fieldCriteria[q.currentField] = append(q.fieldCriteria[q.currentField], c)

	return q
}

// RegExp 测试字段是否与正则表达式匹配
// 字段值将被转换为字符串 (%s) 进行测试
// 参数:
//   - expression: *regexp.Regexp 类型，表示要匹配的正则表达式
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) RegExp(expression *regexp.Regexp) *Query {
	return c.op(re, expression)
}

// IsNil 测试字段是否等于 nil
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) IsNil() *Query {
	return c.op(isnil, nil)
}

// HasPrefix 测试字段是否以提供的字符串作为前缀
// 参数:
//   - prefix: string 类型，表示要匹配的前缀
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) HasPrefix(prefix string) *Query {
	return c.op(sw, prefix)
}

// HasSuffix 测试字段是否以提供的字符串作为后缀
// 参数:
//   - suffix: string 类型，表示要匹配的后缀
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) HasSuffix(suffix string) *Query {
	return c.op(ew, suffix)
}

// MatchFunc 是一个用于在查询中测试任意匹配值的函数类型
type MatchFunc func(ra *RecordAccess) (bool, error)

// RecordAccess 允许在 MatchFunc 中访问当前记录、字段，或运行子查询
type RecordAccess struct {
	record interface{} // 当前记录
	field  interface{} // 当前字段
	query  *Query      // 关联的查询
	store  *Store      // 关联的存储
}

// Field 返回正在查询的当前字段
// 返回值：
//   - interface{}: 当前字段的值
func (r *RecordAccess) Field() interface{} {
	return r.field
}

// Record 返回 badgerhold 中给定行的完整记录
// 返回值：
//   - interface{}: 当前记录的值
func (r *RecordAccess) Record() interface{} {
	return r.record
}

// SubQuery 允许在父查询的每条记录上运行另一个查询，使用相同的事务
// 参数:
//   - result: interface{} 类型，用于存储子查询结果的变量
//   - query: *Query 类型，表示要运行的子查询
//
// 返回值：
//   - error: 如果子查询过程中发生错误，返回错误信息
func (r *RecordAccess) SubQuery(result interface{}, query *Query) error {
	query.subquery = true
	query.bookmark = r.query.bookmark
	err := r.store.findQuery(r.query.tx, result, query)
	if err != nil {
		logger.Error("执行子查询时出错", err)
	}
	return err
}

// SubAggregateQuery 允许在父查询的每条记录上运行另一个聚合查询，使用相同的事务
// 参数:
//   - query: *Query 类型，表示要运行的聚合子查询
//   - groupBy: 可变参数，表示分组字段
//
// 返回值：
//   - []*AggregateResult: 返回聚合查询的结果集合
//   - error: 如果聚合查询过程中发生错误，返回错误信息
func (r *RecordAccess) SubAggregateQuery(query *Query, groupBy ...string) ([]*AggregateResult, error) {
	query.subquery = true
	query.bookmark = r.query.bookmark
	result, err := r.store.aggregateQuery(r.query.tx, r.record, query, groupBy...)
	if err != nil {
		logger.Error("执行聚合子查询时出错", err)
	}
	return result, err
}

// MatchFunc 测试一个字段是否与传入的函数匹配
// 参数:
//   - match: MatchFunc 类型，表示要匹配的函数
//
// 返回值：
//   - *Query: 返回更新后的查询对象
func (c *Criterion) MatchFunc(match MatchFunc) *Query {
	if c.query.currentField == Key {
		logger.Error("匹配函数不能用于键，因为键的类型在运行时未知，且没有可比较的值")
		panic("匹配函数不能用于键，因为键的类型在运行时未知，且没有可比较的值")
	}

	return c.op(fn, match)
}

// test 测试条件是否通过给定的值
// 参数:
//   - s: *Store 类型，表示要查询的存储
//   - testValue: interface{} 类型，表示要测试的值
//   - encoded: bool 类型，指示值是否已编码
//   - keyType: string 类型，表示键的类型
//   - currentRow: interface{} 类型，表示当前行的数据
//
// 返回值：
//   - bool: 如果条件通过，返回 true，否则返回 false
//   - error: 如果测试过程中发生错误，返回错误信息
func (c *Criterion) test(s *Store, testValue interface{}, encoded bool, keyType string, currentRow interface{}) (bool, error) {
	var recordValue interface{}
	if encoded {
		if len(testValue.([]byte)) != 0 {
			if c.operator == in || c.operator == any || c.operator == all {
				// 值是一个值的切片，使用 c.values
				recordValue = newElemType(c.values[0])
			} else {
				recordValue = newElemType(c.value)
			}

			// 用于键
			if keyType != "" {
				err := s.decodeKey(testValue.([]byte), recordValue, keyType)
				if err != nil {
					return false, err
				}
			} else {
				err := s.decode(testValue.([]byte), recordValue)
				if err != nil {
					return false, err
				}
			}
		}
	} else {
		recordValue = testValue
	}

	switch c.operator {
	case in:
		for i := range c.values {
			result, err := c.compare(recordValue, c.values[i], currentRow)
			if err != nil {
				return false, err
			}
			if result == 0 {
				return true, nil
			}
		}

		return false, nil
	case re:
		return c.value.(*regexp.Regexp).Match([]byte(fmt.Sprintf("%s", recordValue))), nil
	case hk:
		v := reflect.ValueOf(recordValue).MapIndex(reflect.ValueOf(c.value))
		return !reflect.ValueOf(v).IsZero(), nil
	case fn:
		return c.value.(MatchFunc)(&RecordAccess{
			field:  recordValue,
			record: currentRow,
			query:  c.query,
			store:  s,
		})
	case isnil:
		return reflect.ValueOf(recordValue).IsNil(), nil
	case sw:
		return strings.HasPrefix(fmt.Sprintf("%s", getElem(recordValue)), fmt.Sprintf("%s", c.value)), nil
	case ew:
		return strings.HasSuffix(fmt.Sprintf("%s", getElem(recordValue)), fmt.Sprintf("%s", c.value)), nil
	case contains, any, all:
		slc := reflect.ValueOf(recordValue)
		kind := slc.Kind()
		if kind != reflect.Slice && kind != reflect.Array {
			// 创建包含 recordValue 的切片
			for slc.Kind() == reflect.Ptr {
				slc = slc.Elem()
			}
			slc = reflect.Append(reflect.MakeSlice(reflect.SliceOf(slc.Type()), 0, 1), slc)
		}

		if c.operator == contains {
			for i := 0; i < slc.Len(); i++ {
				result, err := c.compare(slc.Index(i), c.value, currentRow)
				if err != nil {
					return false, err
				}
				if result == 0 {
					return true, nil
				}
			}
			return false, nil
		}

		if c.operator == any {
			for i := 0; i < slc.Len(); i++ {
				for k := range c.values {
					result, err := c.compare(slc.Index(i), c.values[k], currentRow)
					if err != nil {
						return false, err
					}
					if result == 0 {
						return true, nil
					}
				}
			}

			return false, nil
		}

		// c.operator == all
		for k := range c.values {
			found := false
			for i := 0; i < slc.Len(); i++ {
				result, err := c.compare(slc.Index(i), c.values[k], currentRow)
				if err != nil {
					return false, err
				}
				if result == 0 {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}
		}

		return true, nil
	default:
		// 比较操作符
		result, err := c.compare(recordValue, c.value, currentRow)
		if err != nil {
			return false, err
		}

		switch c.operator {
		case eq:
			return result == 0, nil
		case ne:
			return result != 0, nil
		case gt:
			return result > 0, nil
		case lt:
			return result < 0, nil
		case le:
			return result < 0 || result == 0, nil
		case ge:
			return result > 0 || result == 0, nil
		default:
			panic("invalid operator")
		}
	}
}

// matchesAllCriteria 检查是否所有条件都匹配给定的值
// 参数:
//   - criteria: []*Criterion 类型，表示要测试的条件集合
//   - value: interface{} 类型，表示要测试的值
//   - encoded: bool 类型，指示值是否已编码
//   - keyType: string 类型，表示键的类型
//   - currentRow: interface{} 类型，表示当前行的数据
//
// 返回值：
//   - bool: 如果所有条件都匹配，返回 true，否则返回 false
//   - error: 如果测试过程中发生错误，返回错误信息
func (s *Store) matchesAllCriteria(criteria []*Criterion, value interface{}, encoded bool, keyType string,
	currentRow interface{}) (bool, error) {

	for i := range criteria {
		ok, err := criteria[i].test(s, value, encoded, keyType, currentRow)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

// startsUpper 检查字符串是否以大写字母开头
// 参数:
//   - str: string 类型，表示要检查的字符串
//
// 返回值：
//   - bool: 如果字符串以大写字母开头，返回 true，否则返回 false
func startsUpper(str string) bool {
	if str == "" {
		return true
	}

	for _, r := range str {
		return unicode.IsUpper(r)
	}

	return false
}

// String 返回查询对象的字符串表示形式
// 返回值：
//   - string: 查询对象的字符串表示形式
func (q *Query) String() string {
	s := ""

	if q.index != "" {
		s += "Using Index [" + q.index + "] "
	}

	s += "Where "
	for field, criteria := range q.fieldCriteria {
		for i := range criteria {
			s += field + " " + criteria[i].String()
			s += "\n\tAND "
		}
	}

	// 移除最后一个 AND
	s = s[:len(s)-6]

	for i := range q.ors {
		s += "\nOr " + q.ors[i].String()
	}

	return s
}

// String 返回条件的字符串表示形式
// 返回值：
//   - string: 条件的字符串表示形式
func (c *Criterion) String() string {
	s := ""
	switch c.operator {
	case eq:
		s += "=="
	case ne:
		s += "!="
	case gt:
		s += ">"
	case lt:
		s += "<"
	case le:
		s += "<="
	case ge:
		s += ">="
	case in:
		return "in " + fmt.Sprintf("%v", c.values)
	case re:
		s += "matches the regular expression"
	case fn:
		s += "matches the function"
	case isnil:
		return "is nil"
	case sw:
		return "starts with " + fmt.Sprintf("%+v", c.value)
	case ew:
		return "ends with " + fmt.Sprintf("%+v", c.value)
	default:
		panic("invalid operator")
	}
	return s + " " + fmt.Sprintf("%v", c.value)
}

type record struct {
	key   []byte        // 键的字节表示
	value reflect.Value // 值的反射表示，用于存储具体的数据
}

// runQuery 运行查询，遍历匹配的数据并对其执行指定的操作
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - dataType: interface{} 类型，表示要查询的数据类型
//   - query: *Query 类型，表示要运行的查询
//   - retrievedKeys: KeyList 类型，表示已经检索过的键列表
//   - skip: int 类型，表示需要跳过的记录数量
//   - action: 函数类型，表示要对每个匹配记录执行的操作
//
// 返回值：
//   - error: 如果查询或操作过程中发生错误，返回错误信息
func (s *Store) runQuery(tx *badger.Txn, dataType interface{}, query *Query, retrievedKeys KeyList, skip int,
	action func(r *record) error) error {
	storer := s.newStorer(dataType) // 创建一个满足 Storer 接口的数据存储器

	tp := dataType // 获取数据类型

	for reflect.TypeOf(tp).Kind() == reflect.Ptr { // 如果数据类型是指针，获取其底层值的类型
		tp = reflect.ValueOf(tp).Elem().Interface()
	}

	query.dataType = reflect.TypeOf(tp)  // 设置查询的数据类型
	err := query.validateIndex(dataType) // 验证查询中使用的索引是否合法
	if err != nil {
		return err // 如果索引无效，返回错误
	}

	if len(query.sort) > 0 { // 如果查询需要排序
		return s.runQuerySort(tx, dataType, query, action) // 执行带有排序的查询
	}

	iter := s.newIterator(tx, storer.Type(), query, query.bookmark) // 创建新的迭代器以遍历数据
	if (query.writable || query.subquery) && query.bookmark == nil {
		query.bookmark = iter.createBookmark() // 如果查询可写或是子查询，并且没有书签，则创建书签
	}

	defer func() {
		iter.Close()         // 确保迭代器在函数结束时关闭
		query.bookmark = nil // 清除查询的书签
	}()

	if query.index != "" && query.badIndex { // 如果指定了索引但索引无效
		return fmt.Errorf("The index %s does not exist", query.index) // 返回索引不存在的错误
	}

	newKeys := make(KeyList, 0) // 创建一个空的键列表，用于跟踪新检索到的键

	limit := query.limit - len(retrievedKeys) // 计算查询结果的剩余限制数量

	for k, v := iter.Next(); k != nil; k, v = iter.Next() { // 遍历迭代器中的所有记录
		if len(retrievedKeys) != 0 {
			if retrievedKeys.in(k) { // 如果当前记录的键已被检索过，跳过
				continue
			}
		}

		val := reflect.New(reflect.TypeOf(tp)) // 创建一个新的反射值来存储当前记录的解码值

		err := s.decode(v, val.Interface()) // 解码记录的值
		if err != nil {
			return err // 如果解码失败，返回错误
		}

		query.tx = tx // 设置查询的事务

		ok, err := query.matchesAllFields(s, k, val, val.Interface()) // 检查记录是否匹配查询条件
		if err != nil {
			return err // 如果匹配过程中出错，返回错误
		}

		if ok { // 如果记录匹配
			if skip > 0 { // 如果需要跳过记录，继续跳过
				skip--
				continue
			}

			err = action(&record{
				key:   k,   // 记录的键
				value: val, // 记录的值
			})
			if err != nil {
				return err // 如果操作失败，返回错误
			}

			newKeys.add(k) // 记录当前键已被添加到结果列表中

			if query.limit != 0 { // 如果查询有结果数量限制
				limit--
				if limit == 0 {
					break // 如果已达到限制，停止处理
				}
			}
		}
	}

	if iter.Error() != nil { // 检查迭代器是否出错
		return iter.Error()
	}

	if query.limit != 0 && limit == 0 { // 如果达到查询限制且没有更多要处理的记录
		return nil
	}

	if len(query.ors) > 0 { // 如果查询有 OR 条件
		iter.Close() // 关闭当前迭代器
		for i := range newKeys {
			retrievedKeys.add(newKeys[i]) // 将新检索到的键添加到已检索的键列表中
		}

		for i := range query.ors { // 对 OR 查询条件递归运行查询
			err := s.runQuery(tx, tp, query.ors[i], retrievedKeys, skip, action)
			if err != nil {
				return err // 如果递归查询失败，返回错误
			}
		}
	}

	return nil // 查询成功完成，没有错误
}

// runQuerySort 执行没有排序、跳过或限制的查询，然后对整个结果集应用这些操作
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - dataType: interface{} 类型，表示要查询的数据类型
//   - query: *Query 类型，表示要运行的查询
//   - action: 函数类型，表示要对每个匹配记录执行的操作
//
// 返回值：
//   - error: 如果查询或操作过程中发生错误，返回错误信息
func (s *Store) runQuerySort(tx *badger.Txn, dataType interface{}, query *Query, action func(r *record) error) error {
	err := validateSortFields(query) // 验证查询的排序字段是否合法
	if err != nil {
		return err // 如果排序字段无效，返回错误
	}

	// 运行没有排序、跳过或限制的查询，将结果存储在 records 中
	qCopy := *query
	qCopy.sort = nil
	qCopy.limit = 0
	qCopy.skip = 0

	var records []*record
	err = s.runQuery(tx, dataType, &qCopy, nil, 0,
		func(r *record) error {
			records = append(records, r) // 将每个匹配记录添加到 records 列表中
			return nil
		})

	if err != nil {
		return err // 如果查询失败，返回错误
	}

	sort.Slice(records, func(i, j int) bool { // 对结果集进行排序
		return sortFunction(query, records[i].value, records[j].value)
	})

	startIndex, endIndex := getSkipAndLimitRange(query, len(records)) // 获取要返回的结果范围
	records = records[startIndex:endIndex]                            // 应用跳过和限制

	for i := range records { // 对每个匹配的记录执行操作
		err = action(records[i])
		if err != nil {
			return err // 如果操作失败，返回错误
		}
	}

	return nil // 查询成功完成，没有错误
}

// getSkipAndLimitRange 计算查询的跳过和限制范围
// 参数:
//   - query: *Query 类型，表示要运行的查询
//   - recordsLen: int 类型，表示记录的总长度
//
// 返回值：
//   - startIndex: int 类型，表示查询结果的起始索引
//   - endIndex: int 类型，表示查询结果的结束索引
func getSkipAndLimitRange(query *Query, recordsLen int) (startIndex, endIndex int) {
	if query.skip > recordsLen { // 如果跳过的记录数大于总记录数
		return 0, 0 // 返回起始和结束索引都为 0
	}
	startIndex = query.skip                // 设置起始索引为跳过的记录数
	endIndex = recordsLen                  // 设置结束索引为记录总长度
	limitIndex := query.limit + startIndex // 计算应用查询限制后的结束索引

	if query.limit > 0 && limitIndex <= recordsLen { // 如果限制有效并且限制后的索引在总记录长度范围内
		endIndex = limitIndex // 更新结束索引为限制后的索引
	}
	return startIndex, endIndex // 返回计算后的起始和结束索引
}

// sortFunction 根据查询条件对两个记录进行排序
// 参数:
//   - query: *Query 类型，表示要运行的查询
//   - first: reflect.Value 类型，第一个要比较的记录值
//   - second: reflect.Value 类型，第二个要比较的记录值
//
// 返回值：
//   - bool: 如果第一个值小于第二个值，返回 true，否则返回 false
func sortFunction(query *Query, first, second reflect.Value) bool {
	for _, field := range query.sort { // 遍历所有排序字段
		val, err := fieldValue(reflect.Indirect(first), field) // 获取第一个记录的字段值
		if err != nil {
			panic(err.Error()) // 如果发生错误，触发 panic
		}
		value := val.Interface() // 将字段值转换为接口类型

		val, err = fieldValue(reflect.Indirect(second), field) // 获取第二个记录的字段值
		if err != nil {
			panic(err.Error()) // 如果发生错误，触发 panic
		}

		other := val.Interface() // 将字段值转换为接口类型

		if query.reverse { // 如果需要反转排序顺序
			value, other = other, value // 交换比较的两个值
		}

		cmp, cerr := compare(value, other) // 比较两个字段值
		if cerr != nil {
			// 如果比较出错，则使用字典顺序比较
			valS := fmt.Sprintf("%s", value)
			otherS := fmt.Sprintf("%s", other)
			if valS < otherS {
				return true
			} else if valS == otherS {
				continue
			}
			return false
		}

		if cmp == -1 { // 如果第一个值小于第二个值
			return true
		} else if cmp == 0 { // 如果两个值相等，继续比较下一个字段
			continue
		}
		return false // 如果第一个值大于第二个值
	}
	return false // 如果没有字段使第一个值小于第二个值
}

// validateSortFields 验证查询中的排序字段是否存在
// 参数:
//   - query: *Query 类型，表示要验证的查询
//
// 返回值：
//   - error: 如果验证失败，返回错误信息
func validateSortFields(query *Query) error {
	for _, field := range query.sort { // 遍历所有排序字段
		fields := strings.Split(field, ".") // 将字段按点号分割，支持嵌套字段

		current := query.dataType // 获取当前数据类型
		for i := range fields {
			var structField reflect.StructField
			found := false
			if current.Kind() == reflect.Ptr {
				structField, found = current.Elem().FieldByName(fields[i]) // 获取指针类型的字段
			} else {
				structField, found = current.FieldByName(fields[i]) // 获取非指针类型的字段
			}

			if !found { // 如果字段不存在
				return fmt.Errorf("The field %s does not exist in the type %v", field, query.dataType) // 返回错误信息
			}
			current = structField.Type // 更新当前数据类型为字段的类型
		}
	}
	return nil // 所有字段验证通过，返回 nil
}

// findQuery 根据查询条件在存储中查找记录
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - result: interface{} 类型，表示存储结果的切片指针
//   - query: *Query 类型，表示要运行的查询
//
// 返回值：
//   - error: 如果查询过程中发生错误，返回错误信息
func (s *Store) findQuery(tx *badger.Txn, result interface{}, query *Query) error {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}

	query.writable = false // 设置查询为不可写

	resultVal := reflect.ValueOf(result) // 获取结果的反射值
	if resultVal.Kind() != reflect.Ptr || resultVal.Elem().Kind() != reflect.Slice {
		panic("result argument must be a slice address") // 如果结果不是切片指针，触发 panic
	}

	if isFindByIndexQuery(query) { // 如果是通过索引查询
		return s.findByIndexQuery(tx, resultVal, query) // 运行索引查询
	}

	sliceVal := resultVal.Elem() // 获取结果切片的反射值

	elType := sliceVal.Type().Elem() // 获取切片元素的类型

	tp := elType // 获取元素类型

	for tp.Kind() == reflect.Ptr { // 如果元素类型是指针，获取其底层值的类型
		tp = tp.Elem()
	}

	keyField, hasKeyField := getKeyField(tp) // 获取键字段及其存在性

	val := reflect.New(tp) // 创建一个新的反射值来存储当前记录的解码值

	err := s.runQuery(tx, val.Interface(), query, nil, query.skip,
		func(r *record) error {
			var rowValue reflect.Value

			if elType.Kind() == reflect.Ptr { // 如果元素类型是指针
				rowValue = r.value // 直接使用值
			} else {
				rowValue = r.value.Elem() // 获取非指针值
			}

			if hasKeyField { // 如果存在键字段
				rowKey := rowValue
				for rowKey.Kind() == reflect.Ptr {
					rowKey = rowKey.Elem()
				}
				err := s.decodeKey(r.key, rowKey.FieldByName(keyField.Name).Addr().Interface(), tp.Name()) // 解码键字段
				if err != nil {
					return err
				}
			}

			sliceVal = reflect.Append(sliceVal, rowValue) // 将值追加到结果切片

			return nil
		})

	if err != nil {
		return err // 如果查询失败，返回错误
	}

	resultVal.Elem().Set(sliceVal.Slice(0, sliceVal.Len())) // 设置最终的结果切片

	return nil
}

// isFindByIndexQuery 判断查询是否是通过索引查找的
// 参数:
//   - query: *Query 类型，表示要判断的查询
//
// 返回值：
//   - bool: 如果是通过索引查找的查询，返回 true，否则返回 false
func isFindByIndexQuery(query *Query) bool {
	if query.index == "" || len(query.fieldCriteria) == 0 || len(query.fieldCriteria[query.index]) != 1 || len(query.ors) > 0 {
		return false // 如果没有索引，或者字段条件不符合条件，返回 false
	}

	operator := query.fieldCriteria[query.index][0].operator
	return operator == eq || operator == in // 仅支持等于或包含的查询
}

// deleteQuery 执行删除查询，删除符合条件的记录
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - dataType: interface{} 类型，表示要操作的数据类型
//   - query: *Query 类型，表示要执行的查询
//
// 返回值：
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) deleteQuery(tx *badger.Txn, dataType interface{}, query *Query) error {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}
	query.writable = true // 设置查询为可写操作

	var records []*record // 创建一个记录切片，用于存储查询结果

	// 运行查询，将符合条件的记录存储到 records 切片中
	err := s.runQuery(tx, dataType, query, nil, query.skip,
		func(r *record) error {
			records = append(records, r) // 将记录添加到切片中
			return nil
		})

	if err != nil { // 如果查询过程中发生错误
		return err // 返回错误信息
	}

	storer := s.newStorer(dataType) // 获取数据存储器

	for i := range records { // 遍历所有匹配的记录
		err := tx.Delete(records[i].key) // 删除记录
		if err != nil {
			return err // 如果删除过程中发生错误，返回错误信息
		}

		// 删除与记录相关的索引
		err = s.indexDelete(storer, tx, records[i].key, records[i].value.Interface())
		if err != nil {
			return err // 如果删除索引过程中发生错误，返回错误信息
		}
	}

	return nil // 删除操作成功完成，返回 nil
}

// updateQuery 执行更新查询，更新符合条件的记录
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - dataType: interface{} 类型，表示要操作的数据类型
//   - query: *Query 类型，表示要执行的查询
//   - update: 函数类型，表示更新操作的具体实现
//
// 返回值：
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) updateQuery(tx *badger.Txn, dataType interface{}, query *Query, update func(record interface{}) error) error {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}

	query.writable = true // 设置查询为可写操作
	var records []*record // 创建一个记录切片，用于存储查询结果

	// 运行查询，将符合条件的记录存储到 records 切片中
	err := s.runQuery(tx, dataType, query, nil, query.skip,
		func(r *record) error {
			records = append(records, r) // 将记录添加到切片中
			return nil
		})

	if err != nil { // 如果查询过程中发生错误
		return err // 返回错误信息
	}

	storer := s.newStorer(dataType) // 获取数据存储器
	for i := range records {        // 遍历所有匹配的记录
		upVal := records[i].value.Interface() // 获取记录的值

		// 删除记录的现有索引
		err := s.indexDelete(storer, tx, records[i].key, upVal)
		if err != nil {
			return err // 如果删除索引过程中发生错误，返回错误信息
		}

		err = update(upVal) // 执行更新操作
		if err != nil {
			return err // 如果更新过程中发生错误，返回错误信息
		}

		encVal, err := s.encode(upVal) // 将更新后的值编码为字节
		if err != nil {
			return err // 如果编码过程中发生错误，返回错误信息
		}

		err = tx.Set(records[i].key, encVal) // 将编码后的值保存到 Badger DB 中
		if err != nil {
			return err // 如果保存过程中发生错误，返回错误信息
		}

		// 为更新后的记录添加新的索引
		err = s.indexAdd(storer, tx, records[i].key, upVal)
		if err != nil {
			return err // 如果添加索引过程中发生错误，返回错误信息
		}
	}

	return nil // 更新操作成功完成，返回 nil
}

// aggregateQuery 执行聚合查询，按指定字段分组并聚合结果
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - dataType: interface{} 类型，表示要操作的数据类型
//   - query: *Query 类型，表示要执行的查询
//   - groupBy: 字符串可变参数，表示按哪些字段分组
//
// 返回值：
//   - []*AggregateResult: 返回聚合结果的切片
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) aggregateQuery(tx *badger.Txn, dataType interface{}, query *Query, groupBy ...string) ([]*AggregateResult, error) {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}

	query.writable = false        // 设置查询为只读操作
	var result []*AggregateResult // 创建一个切片，用于存储聚合结果

	if len(groupBy) == 0 { // 如果没有指定分组字段
		result = append(result, &AggregateResult{}) // 创建一个默认的聚合结果
	}

	// 运行查询，按指定字段分组并聚合结果
	err := s.runQuery(tx, dataType, query, nil, query.skip,
		func(r *record) error {
			if len(groupBy) == 0 { // 如果没有指定分组字段
				result[0].reduction = append(result[0].reduction, r.value) // 将记录添加到默认的聚合结果中
				return nil
			}

			grouping := make([]reflect.Value, len(groupBy)) // 创建一个切片用于存储分组字段的值

			for i := range groupBy { // 遍历所有分组字段
				fVal := r.value.Elem().FieldByName(groupBy[i]) // 获取分组字段的值
				if !fVal.IsValid() {                           // 如果字段不存在
					return fmt.Errorf("The field %s does not exist in the type %s", groupBy[i], r.value.Type()) // 返回错误信息
				}
				grouping[i] = fVal // 将字段值存储到切片中
			}

			var err error
			var c int
			var allEqual bool

			// 在现有的聚合结果中查找是否已经存在相同的分组
			i := sort.Search(len(result), func(i int) bool {
				for j := range grouping { // 遍历所有分组字段
					c, err = compare(result[i].group[j].Interface(), grouping[j].Interface()) // 比较分组字段值
					if err != nil {
						return true // 如果比较出错，停止搜索
					}
					if c != 0 {
						return c >= 0 // 如果分组字段值不相等，返回比较结果
					}
				}
				allEqual = true // 如果所有分组字段值都相等，标记为相等
				return true
			})

			if err != nil {
				return err // 如果搜索过程中发生错误，返回错误信息
			}

			if i < len(result) {
				if allEqual { // 如果找到相同的分组
					result[i].reduction = append(result[i].reduction, r.value) // 将记录添加到现有的聚合结果中
					return nil
				}
			}

			// 如果没有找到相同的分组，创建一个新的分组
			result = append(result, nil)
			copy(result[i+1:], result[i:])
			result[i] = &AggregateResult{
				group:     grouping,                 // 设置分组字段的值
				reduction: []reflect.Value{r.value}, // 将记录添加到新创建的分组中
			}

			return nil
		})

	if err != nil {
		return nil, err // 如果查询或聚合过程中发生错误，返回错误信息
	}

	return result, nil // 返回聚合结果
}

// findOneQuery 执行查询，查找单个符合条件的记录
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - result: interface{} 类型，表示存储结果的变量
//   - query: *Query 类型，表示要执行的查询
//
// 返回值：
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) findOneQuery(tx *badger.Txn, result interface{}, query *Query) error {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}
	originalLimit := query.limit // 记录查询的原始限制

	query.limit = 1 // 设置查询限制为 1，只查找一个记录

	query.writable = false // 设置查询为只读操作

	resultVal := reflect.ValueOf(result) // 获取结果的反射值
	if resultVal.Kind() != reflect.Ptr {
		panic("result argument must be an address") // 如果结果不是指针，触发 panic
	}

	elType := resultVal.Elem().Type() // 获取结果类型
	tp := elType

	for tp.Kind() == reflect.Ptr { // 如果结果类型是指针，获取其底层值的类型
		tp = tp.Elem()
	}

	keyField, hasKeyField := getKeyField(tp) // 获取键字段及其存在性

	val := reflect.New(tp) // 创建一个新的反射值来存储当前记录的解码值

	found := false // 标记是否找到记录

	// 运行查询，查找符合条件的记录
	err := s.runQuery(tx, val.Interface(), query, nil, query.skip,
		func(r *record) error {
			found = true // 标记为找到记录
			var rowValue reflect.Value

			if elType.Kind() == reflect.Ptr { // 如果结果类型是指针
				rowValue = r.value // 直接使用值
			} else {
				rowValue = r.value.Elem() // 获取非指针值
			}

			if hasKeyField { // 如果存在键字段
				rowKey := rowValue
				for rowKey.Kind() == reflect.Ptr {
					rowKey = rowKey.Elem()
				}
				err := s.decodeKey(r.key, rowKey.FieldByName(keyField.Name).Addr().Interface(), tp.Name()) // 解码键字段
				if err != nil {
					return err
				}
			}

			resultVal.Elem().Set(r.value.Elem()) // 将找到的记录设置到结果变量中

			return nil
		})

	query.limit = originalLimit // 恢复查询的原始限制
	if err != nil {
		return err // 如果查询过程中发生错误，返回错误信息
	}

	if !found { // 如果没有找到记录
		return ErrNotFound // 返回记录未找到的错误
	}

	return nil
}

// forEach 执行遍历查询，对每个符合条件的记录运行指定的函数
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - query: *Query 类型，表示要执行的查询
//   - fn: interface{} 类型，表示要对每个记录执行的函数
//
// 返回值：
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) forEach(tx *badger.Txn, query *Query, fn interface{}) error {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}

	fnVal := reflect.ValueOf(fn)        // 获取函数的反射值
	argType := reflect.TypeOf(fn).In(0) // 获取函数的参数类型

	if argType.Kind() == reflect.Ptr {
		argType = argType.Elem() // 如果参数类型是指针，获取其底层值的类型
	}

	keyField, hasKeyField := getKeyField(argType) // 获取键字段及其存在性

	dataType := reflect.New(argType).Interface() // 创建一个新的数据类型实例
	storer := s.newStorer(dataType)              // 获取数据存储器

	return s.runQuery(tx, dataType, query, nil, query.skip, func(r *record) error {

		if hasKeyField { // 如果存在键字段
			err := s.decodeKey(r.key, r.value.Elem().FieldByName(keyField.Name).Addr().Interface(), storer.Type()) // 解码键字段
			if err != nil {
				return err
			}
		}

		out := fnVal.Call([]reflect.Value{r.value}) // 调用函数，传入当前记录

		if len(out) != 1 {
			return fmt.Errorf("foreach function does not return an error") // 如果函数没有返回错误，返回错误信息
		}

		if out[0].IsNil() { // 如果函数没有返回错误
			return nil
		}

		return out[0].Interface().(error) // 返回函数返回的错误
	})
}

// countQuery 执行查询，统计符合条件的记录数量
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - dataType: interface{} 类型，表示要操作的数据类型
//   - query: *Query 类型，表示要执行的查询
//
// 返回值：
//   - uint64: 返回记录的数量
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) countQuery(tx *badger.Txn, dataType interface{}, query *Query) (uint64, error) {
	if query == nil { // 如果查询为空
		query = &Query{} // 创建一个新的空查询
	}

	var count uint64 // 初始化计数器

	// 运行查询，统计符合条件的记录数量
	err := s.runQuery(tx, dataType, query, nil, query.skip,
		func(r *record) error {
			count++ // 每找到一条记录，计数器加一
			return nil
		})

	if err != nil {
		return 0, err // 如果查询过程中发生错误，返回错误信息
	}

	return count, nil // 返回记录数量
}

// findByIndexQuery 根据索引执行查询，查找符合条件的记录
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - resultSlice: reflect.Value 类型，表示存储结果的切片
//   - query: *Query 类型，表示要执行的查询
//
// 返回值：
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) findByIndexQuery(tx *badger.Txn, resultSlice reflect.Value, query *Query) (err error) {
	criteria := query.fieldCriteria[query.index][0] // 获取查询的字段条件
	sliceType := resultSlice.Elem().Type()          // 获取结果切片的元素类型
	query.dataType = dereference(sliceType.Elem())  // 获取解引用后的数据类型

	data := reflect.New(query.dataType).Interface() // 创建一个新的数据类型实例
	storer := s.newStorer(data)                     // 获取数据存储器
	err = query.validateIndex(data)                 // 验证查询的索引
	if err != nil {
		return err // 如果索引验证失败，返回错误信息
	}
	err = validateSortFields(query) // 验证查询的排序字段
	if err != nil {
		return err // 如果排序字段验证失败，返回错误信息
	}

	var keyList KeyList
	if criteria.operator == in { // 如果操作符是 in
		keyList, err = s.fetchIndexValues(tx, query, storer.Type(), criteria.values...) // 获取索引对应的键列表
	} else {
		keyList, err = s.fetchIndexValues(tx, query, storer.Type(), criteria.value) // 获取索引对应的单个键
	}
	if err != nil {
		return err // 如果获取键列表失败，返回错误信息
	}

	keyField, hasKeyField := getKeyField(query.dataType) // 获取键字段及其存在性

	slice := reflect.MakeSlice(sliceType, 0, len(keyList)) // 创建一个切片来存储查询结果
	for i := range keyList {                               // 遍历所有键
		item, err := tx.Get(keyList[i]) // 从数据库中获取对应的记录
		if err == badger.ErrKeyNotFound {
			panic("inconsistency between keys stored in index and in Badger directly") // 如果键和数据不一致，触发 panic
		}
		if err != nil {
			return err // 如果获取记录失败，返回错误信息
		}

		newElement := reflect.New(query.dataType) // 创建一个新的记录实例
		err = item.Value(func(val []byte) error {
			return s.decode(val, newElement.Interface()) // 将数据库中的记录解码到实例中
		})
		if err != nil {
			return err // 如果解码失败，返回错误信息
		}
		if hasKeyField { // 如果存在键字段
			err = s.setKeyField(keyList[i], newElement, keyField, storer.Type()) // 设置键字段的值
			if err != nil {
				return err // 如果设置键字段失败，返回错误信息
			}
		}

		ok, err := query.matchesAllFields(s, keyList[i], newElement, newElement.Interface()) // 检查记录是否符合查询条件
		if err != nil {
			return err // 如果检查失败，返回错误信息
		}
		if !ok {
			continue // 如果记录不符合查询条件，跳过该记录
		}

		if sliceType.Elem().Kind() != reflect.Ptr {
			newElement = newElement.Elem() // 如果结果切片元素类型不是指针，获取非指针值
		}
		slice = reflect.Append(slice, newElement) // 将符合条件的记录添加到结果切片中
	}

	if len(query.sort) > 0 { // 如果查询有排序要求
		sort.Slice(slice.Interface(), func(i, j int) bool {
			return sortFunction(query, slice.Index(i), slice.Index(j)) // 对结果切片进行排序
		})
	}

	startIndex, endIndex := getSkipAndLimitRange(query, slice.Len()) // 获取跳过和限制的范围
	slice = slice.Slice(startIndex, endIndex)                        // 根据跳过和限制范围截取结果切片

	resultSlice.Elem().Set(slice) // 将最终的结果设置到结果变量中
	return nil
}

// fetchIndexValues 获取与索引键对应的记录键列表
// 参数:
//   - tx: *badger.Txn 类型，表示 Badger 事务
//   - query: *Query 类型，表示要执行的查询
//   - typeName: string 类型，表示数据类型名称
//   - indexKeys: interface{} 类型的可变参数，表示索引键
//
// 返回值：
//   - KeyList: 返回记录键的列表
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) fetchIndexValues(tx *badger.Txn, query *Query, typeName string, indexKeys ...interface{}) (KeyList, error) {
	keyList := KeyList{}       // 初始化键列表
	for i := range indexKeys { // 遍历所有索引键
		indexKeyValue, err := s.encode(indexKeys[i]) // 将索引键编码为字节
		if err != nil {
			return nil, err // 如果编码失败，返回错误信息
		}

		indexKey := newIndexKey(typeName, query.index, indexKeyValue) // 创建索引键的完整键值

		item, err := tx.Get(indexKey) // 从数据库中获取索引项
		if err == badger.ErrKeyNotFound {
			continue // 如果索引项不存在，跳过
		}
		if err != nil {
			return nil, err // 如果获取索引项失败，返回错误信息
		}

		indexValue := KeyList{} // 初始化存储键列表
		err = item.Value(func(val []byte) error {
			return s.decode(val, &indexValue) // 解码索引项中的键列表
		})
		if err != nil {
			return nil, err // 如果解码失败，返回错误信息
		}
		keyList = append(keyList, indexValue...) // 将解码后的键列表添加到结果中
	}
	return keyList, nil
}

// setKeyField 设置记录中的键字段
// 参数:
//   - data: []byte 类型，表示键字段的字节数据
//   - key: reflect.Value 类型，表示记录的反射值
//   - keyField: reflect.StructField 类型，表示键字段
//   - typeName: string 类型，表示数据类型名称
//
// 返回值：
//   - error: 如果操作过程中发生错误，返回错误信息
func (s *Store) setKeyField(data []byte, key reflect.Value, keyField reflect.StructField, typeName string) error {
	return s.decodeKey(data, key.Elem().FieldByName(keyField.Name).Addr().Interface(), typeName) // 解码并设置键字段的值
}

// dereference 递归解引用类型，直到获取底层非指针类型
// 参数:
//   - value: reflect.Type 类型，表示要解引用的类型
//
// 返回值：
//   - reflect.Type: 返回解引用后的类型
func dereference(value reflect.Type) reflect.Type {
	result := value                    // 初始化结果为传入的类型
	for result.Kind() == reflect.Ptr { // 递归解引用，直到获取非指针类型
		result = result.Elem()
	}
	return result // 返回解引用后的类型
}
