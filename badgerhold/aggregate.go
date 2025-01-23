// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/dgraph-io/badger/v4"
)

// AggregateResult 允许您访问聚合查询的结果
type AggregateResult struct {
	reduction []reflect.Value // reduction 是记录聚合结果的值列表，总是存储为指针
	group     []reflect.Value // group 是用于分组的字段值列表
	sortby    string          // sortby 是用于排序的字段名称
}

// Group 返回在查询中按字段分组的值
// 参数:
//   - result: interface{} 类型的变长参数，表示用于接收分组结果的变量，必须为指针类型
func (a *AggregateResult) Group(result ...interface{}) {
	for i := range result {
		// 将 result[i] 转换为 reflect.Value 类型
		resultVal := reflect.ValueOf(result[i])
		// 检查 result[i] 是否为指针类型
		if resultVal.Kind() != reflect.Ptr {
			// 如果不是指针类型，抛出异常
			panic("结果参数必须是地址")
		}

		// 检查 i 是否超出分组数量的范围
		if i >= len(a.group) {
			// 如果 i 超出分组范围，抛出异常
			panic(fmt.Sprintf("分组中没有 %d 个元素", i))
		}

		// 将分组的值赋给 result[i] 指向的变量
		resultVal.Elem().Set(a.group[i])
	}
}

// Reduction 返回属于 AggregateResult 分组的记录集合
// 参数:
//   - result: interface{} 类型的参数，表示用于接收聚合结果的切片变量，必须为切片指针类型
func (a *AggregateResult) Reduction(result interface{}) {
	// 将 result 转换为 reflect.Value 类型
	resultVal := reflect.ValueOf(result)

	// 检查 result 是否为指针且指向的变量类型为切片
	if resultVal.Kind() != reflect.Ptr || resultVal.Elem().Kind() != reflect.Slice {
		// 如果不是切片指针类型，抛出异常
		panic("结果参数必须是切片地址")
	}

	// 获取切片的反射值
	sliceVal := resultVal.Elem()

	// 获取切片元素的类型
	elType := sliceVal.Type().Elem()

	// 遍历 reduction 并将其追加到切片中
	for i := range a.reduction {
		if elType.Kind() == reflect.Ptr {
			// 如果元素类型是指针，直接追加
			sliceVal = reflect.Append(sliceVal, a.reduction[i])
		} else {
			// 如果元素类型不是指针，追加其解引用后的值
			sliceVal = reflect.Append(sliceVal, a.reduction[i].Elem())
		}
	}

	// 将最终的切片赋值回 result
	resultVal.Elem().Set(sliceVal.Slice(0, sliceVal.Len()))
}

// aggregateResultSort 用于对 AggregateResult 进行排序
type aggregateResultSort AggregateResult

// Len 返回 reduction 的长度
// 返回值：
//   - int: reduction 切片的长度
func (a *aggregateResultSort) Len() int { return len(a.reduction) }

// Swap 交换 reduction 中 i 和 j 的元素
// 参数:
//   - i: int 类型，第一个元素的索引
//   - j: int 类型，第二个元素的索引
func (a *aggregateResultSort) Swap(i, j int) {
	// 交换 i 和 j 索引位置的元素
	a.reduction[i], a.reduction[j] = a.reduction[j], a.reduction[i]
}

// Less 比较 reduction 中 i 和 j 的元素，判断 i 是否小于 j
// 参数:
//   - i: int 类型，第一个元素的索引
//   - j: int 类型，第二个元素的索引
//
// 返回值：
//   - bool: 如果 i 小于 j 则返回 true，否则返回 false
func (a *aggregateResultSort) Less(i, j int) bool {
	// reduction 中的值总是指针
	iVal := a.reduction[i].Elem().FieldByName(a.sortby)
	// 检查 iVal 是否是有效字段
	if !iVal.IsValid() {
		// 如果字段不存在，抛出异常
		panic(fmt.Sprintf("类型 %s 中不存在字段 %s", a.reduction[i].Type(), a.sortby))
	}

	jVal := a.reduction[j].Elem().FieldByName(a.sortby)
	// 检查 jVal 是否是有效字段
	if !jVal.IsValid() {
		// 如果字段不存在，抛出异常
		panic(fmt.Sprintf("类型 %s 中不存在字段 %s", a.reduction[j].Type(), a.sortby))
	}

	// 使用 compare 函数比较 iVal 和 jVal 的值
	c, err := compare(iVal.Interface(), jVal.Interface())
	// 如果 compare 过程中发生错误，抛出异常
	if err != nil {
		panic(err)
	}

	// 返回是否 i 小于 j
	return c == -1
}

// Sort 按传入的字段对聚合结果进行升序排序
// Sort 方法会自动在调用 Min/Max 函数时被调用，以获取最小值和最大值
// 参数:
//   - field: string 类型，表示用于排序的字段名称，必须以大写字母开头
func (a *AggregateResult) Sort(field string) {
	// 检查字段名称的首字母是否大写
	if !startsUpper(field) {
		panic("字段的首字母必须大写")
	}
	// 如果已经按此字段排序，则无需再次排序
	if a.sortby == field {
		return
	}

	// 设置排序字段并进行排序
	a.sortby = field
	sort.Sort((*aggregateResultSort)(a))
}

// Max 返回聚合分组的最大值，使用 Comparer 接口
// 参数:
//   - field: string 类型，表示用于获取最大值的字段名称
//   - result: interface{} 类型，表示用于接收最大值的变量，必须为指针类型
func (a *AggregateResult) Max(field string, result interface{}) {
	// 对字段进行排序
	a.Sort(field)

	// 将 result 转换为 reflect.Value 类型
	resultVal := reflect.ValueOf(result)
	// 检查 result 是否为指针类型
	if resultVal.Kind() != reflect.Ptr {
		panic("结果参数必须是地址")
	}

	// 检查 result 是否为 nil
	if resultVal.IsNil() {
		panic("结果参数不能为 nil")
	}

	// 将最大值赋给 result 指向的变量
	resultVal.Elem().Set(a.reduction[len(a.reduction)-1].Elem())
}

// Min 返回聚合分组的最小值，使用 Comparer 接口
// 参数:
//   - field: string 类型，表示用于获取最小值的字段名称
//   - result: interface{} 类型，表示用于接收最小值的变量，必须为指针类型
func (a *AggregateResult) Min(field string, result interface{}) {
	// 对字段进行排序
	a.Sort(field)

	// 将 result 转换为 reflect.Value 类型
	resultVal := reflect.ValueOf(result)
	// 检查 result 是否为指针类型
	if resultVal.Kind() != reflect.Ptr {
		panic("结果参数必须是地址")
	}

	// 检查 result 是否为 nil
	if resultVal.IsNil() {
		panic("结果参数不能为 nil")
	}

	// 将最小值赋给 result 指向的变量
	resultVal.Elem().Set(a.reduction[0].Elem())
}

// Avg 返回聚合分组的平均值
// 如果字段不能转换为 float64，则抛出异常
// 参数:
//   - field: string 类型，表示用于计算平均值的字段名称
//
// 返回值：
//   - float64: 计算出的平均值
func (a *AggregateResult) Avg(field string) float64 {
	// 计算字段值的总和
	sum := a.Sum(field)
	// 返回平均值
	return sum / float64(len(a.reduction))
}

// Sum 返回聚合分组的总和
// 如果字段不能转换为 float64，则抛出异常
// 参数:
//   - field: string 类型，表示用于计算总和的字段名称
//
// 返回值：
//   - float64: 计算出的总和
func (a *AggregateResult) Sum(field string) float64 {
	var sum float64

	// 遍历 reduction 并计算字段值的总和
	for i := range a.reduction {
		fVal := a.reduction[i].Elem().FieldByName(field)
		// 检查 fVal 是否是有效字段
		if !fVal.IsValid() {
			// 如果字段不存在，抛出异常
			panic(fmt.Sprintf("类型 %s 中不存在字段 %s", a.reduction[i].Type(), field))
		}

		// 尝试将字段值转换为 float64 并累加到 sum
		sum += tryFloat(fVal)
	}

	// 返回总和
	return sum
}

// Count 返回聚合分组中的记录数量
// 返回值：
//   - uint64: 记录的数量
func (a *AggregateResult) Count() uint64 {
	return uint64(len(a.reduction))
}

// FindAggregate 返回传入查询的聚合分组
// groupBy 是可选参数，用于指定分组字段
// 参数:
//   - dataType: interface{} 类型，表示查询的数据类型
//   - query: *Query 类型，表示要执行的查询
//   - groupBy: 变长字符串参数，表示分组的字段名称
//
// 返回值：
//   - []*AggregateResult: 包含聚合结果的切片
//   - error: 如果查询失败，返回错误信息
func (s *Store) FindAggregate(dataType interface{}, query *Query, groupBy ...string) ([]*AggregateResult, error) {
	var result []*AggregateResult
	var err error
	// 使用 Badger 事务执行查询
	err = s.Badger().View(func(tx *badger.Txn) error {
		result, err = s.TxFindAggregate(tx, dataType, query, groupBy...)
		return err
	})

	if err != nil {
		// 如果查询失败，返回错误信息
		return nil, err
	}

	// 返回聚合结果
	return result, nil
}

// TxFindAggregate 与 FindAggregate 类似，但您可以指定自己的事务
// groupBy 是可选参数，用于指定分组字段
// 参数:
//   - tx: *badger.Txn 类型，表示要使用的事务
//   - dataType: interface{} 类型，表示查询的数据类型
//   - query: *Query 类型，表示要执行的查询
//   - groupBy: 变长字符串参数，表示分组的字段名称
//
// 返回值：
//   - []*AggregateResult: 包含聚合结果的切片
//   - error: 如果查询失败，返回错误信息
func (s *Store) TxFindAggregate(tx *badger.Txn, dataType interface{}, query *Query,
	groupBy ...string) ([]*AggregateResult, error) {
	// 执行聚合查询并返回结果
	return s.aggregateQuery(tx, dataType, query, groupBy...)
}

// tryFloat 尝试将反射值转换为 float64 类型
// 参数:
//   - val: reflect.Value 类型，表示要转换的反射值
//
// 返回值：
//   - float64: 转换后的浮点值
//
// 如果字段类型不支持转换，抛出异常
func tryFloat(val reflect.Value) float64 {
	switch val.Kind() {
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int8:
		// 如果字段是整数类型，将其转换为 float64 并返回
		return float64(val.Int())
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint8:
		// 如果字段是无符号整数类型，将其转换为 float64 并返回
		return float64(val.Uint())
	case reflect.Float32, reflect.Float64:
		// 如果字段是浮点数类型，直接返回
		return val.Float()
	default:
		// 如果字段类型不支持转换，抛出异常
		panic(fmt.Sprintf("字段类型为 %s，无法转换为 float64", val.Kind()))
	}
}
