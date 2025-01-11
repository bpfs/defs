// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"fmt"
	"math/big"
	"reflect"
	"time"
)

// ErrTypeMismatch 是在两个类型无法进行比较时抛出的错误类型
type ErrTypeMismatch struct {
	Value interface{} // Value 表示其中一个无法比较的值
	Other interface{} // Other 表示另一个无法比较的值
}

// Error 返回错误的描述信息
// 返回值：
//   - string: 描述无法比较的两个值及其类型的字符串
func (e *ErrTypeMismatch) Error() string {
	return fmt.Sprintf("%v (%T) 无法与 %v (%T) 进行比较", e.Value, e.Value, e.Other, e.Other)
}

// Comparer 接口用于将类型与存储中的编码值进行比较。如果当前值等于 other，结果应该是 0；
// 如果当前值小于 other，则结果应为 -1；如果当前值大于 other，则结果应为 +1。
// 如果结构体中的字段没有指定 comparer，则使用默认比较（转换为字符串并比较）。
// 该接口已经为标准 Go 类型以及更复杂的类型（如 time 和 big）进行了处理。
// 如果类型无法比较，则返回错误。
// 实现此接口的具体类型将始终以非指针形式传递。
type Comparer interface {
	Compare(other interface{}) (int, error) // Compare 方法用于比较当前值与另一个值
}

// compare 是 Criterion 类型的一个方法，用于比较行值与条件值，并返回比较结果
// 参数:
//   - rowValue: interface{} 类型，表示当前行的值
//   - criterionValue: interface{} 类型，表示用于比较的条件值
//   - currentRow: interface{} 类型，表示当前行的整个数据
//
// 返回值：
//   - int: 如果 rowValue 等于 criterionValue，返回 0；如果 rowValue 小于 criterionValue，返回 -1；如果 rowValue 大于 criterionValue，返回 +1
//   - error: 如果两者无法比较，则返回 ErrTypeMismatch 错误
func (c *Criterion) compare(rowValue, criterionValue interface{}, currentRow interface{}) (int, error) {
	// 如果 rowValue 或 criterionValue 为 nil，进行特殊处理
	if rowValue == nil || criterionValue == nil {
		if rowValue == criterionValue {
			// 如果两者都为 nil，则视为相等，返回 0
			return 0, nil
		}
		// 如果只有其中一个为 nil，返回 ErrTypeMismatch 错误
		err := &ErrTypeMismatch{rowValue, criterionValue}
		logger.Error("比较失败：", err)
		return 0, err
	}

	// 如果 criterionValue 是 Field 类型，将其转换为 currentRow 对应字段的值
	if _, ok := criterionValue.(Field); ok {
		fVal := reflect.ValueOf(currentRow).Elem().FieldByName(string(criterionValue.(Field)))
		// 检查字段是否存在
		if !fVal.IsValid() {
			// 如果字段不存在，返回错误信息
			err := fmt.Errorf("类型 %s 中不存在字段 %s", reflect.TypeOf(currentRow), criterionValue)
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 将字段值赋给 criterionValue
		criterionValue = fVal.Interface()
	}

	// 处理 rowValue，解引用所有指针，直到获取最终值
	value := rowValue
	for reflect.TypeOf(value).Kind() == reflect.Ptr {
		// 解引用指针并更新 value
		value = reflect.ValueOf(value).Elem().Interface()
	}

	// 处理 criterionValue，解引用所有指针，直到获取最终值
	other := criterionValue
	for reflect.TypeOf(other).Kind() == reflect.Ptr {
		// 解引用指针并更新 other
		other = reflect.ValueOf(other).Elem().Interface()
	}

	// 调用 compare 函数进行比较并返回结果
	return compare(value, other)
}

// compare 比较两个接口类型的值，并返回它们的比较结果
// 参数:
//   - value: interface{} 类型，第一个要比较的值
//   - other: interface{} 类型，第二个要比较的值
//
// 返回值：
//   - int: 如果 value 等于 other，返回 0；如果 value 小于 other，返回 -1；如果 value 大于 other，返回 1
//   - error: 如果两个值的类型不匹配，返回 ErrTypeMismatch 错误
func compare(value, other interface{}) (int, error) {
	// 使用类型断言处理不同类型的比较
	switch t := value.(type) {
	case time.Time:
		// 处理 time.Time 类型的比较
		tother, ok := other.(time.Time)
		// 检查 other 是否为 time.Time 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 如果两个时间相等，返回 0
		if value.(time.Time).Equal(tother) {
			return 0, nil
		}

		// 如果 value 时间早于 other，返回 -1
		if value.(time.Time).Before(tother) {
			return -1, nil
		}
		// 如果 value 时间晚于 other，返回 1
		return 1, nil

	case big.Float:
		// 处理 big.Float 类型的比较
		o, ok := other.(big.Float)
		// 检查 other 是否为 big.Float 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用 Cmp 方法比较两个 big.Float 值
		v := value.(big.Float)
		return v.Cmp(&o), nil

	case big.Int:
		// 处理 big.Int 类型的比较
		o, ok := other.(big.Int)
		// 检查 other 是否为 big.Int 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用 Cmp 方法比较两个 big.Int 值
		v := value.(big.Int)
		return v.Cmp(&o), nil

	case big.Rat:
		// 处理 big.Rat 类型的比较
		o, ok := other.(big.Rat)
		// 检查 other 是否为 big.Rat 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用 Cmp 方法比较两个 big.Rat 值
		v := value.(big.Rat)
		return v.Cmp(&o), nil

	case int:
		// 处理 int 类型的比较
		tother, ok := other.(int)
		// 检查 other 是否为 int 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(int) == tother {
			return 0, nil
		}
		if value.(int) < tother {
			return -1, nil
		}
		return 1, nil

	case int8:
		// 处理 int8 类型的比较
		tother, ok := other.(int8)
		// 检查 other 是否为 int8 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(int8) == tother {
			return 0, nil
		}
		if value.(int8) < tother {
			return -1, nil
		}
		return 1, nil

	case int16:
		// 处理 int16 类型的比较
		tother, ok := other.(int16)
		// 检查 other 是否为 int16 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(int16) == tother {
			return 0, nil
		}
		if value.(int16) < tother {
			return -1, nil
		}
		return 1, nil

	case int32:
		// 处理 int32 类型的比较
		tother, ok := other.(int32)
		// 检查 other 是否为 int32 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(int32) == tother {
			return 0, nil
		}
		if value.(int32) < tother {
			return -1, nil
		}
		return 1, nil

	case int64:
		// 处理 int64 类型的比较
		tother, ok := other.(int64)
		// 检查 other 是否为 int64 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(int64) == tother {
			return 0, nil
		}
		if value.(int64) < tother {
			return -1, nil
		}
		return 1, nil

	case uint:
		// 处理 uint 类型的比较
		tother, ok := other.(uint)
		// 检查 other 是否为 uint 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(uint) == tother {
			return 0, nil
		}
		if value.(uint) < tother {
			return -1, nil
		}
		return 1, nil

	case uint8:
		// 处理 uint8 类型的比较
		tother, ok := other.(uint8)
		// 检查 other 是否为 uint8 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(uint8) == tother {
			return 0, nil
		}
		if value.(uint8) < tother {
			return -1, nil
		}
		return 1, nil

	case uint16:
		// 处理 uint16 类型的比较
		tother, ok := other.(uint16)
		// 检查 other 是否为 uint16 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(uint16) == tother {
			return 0, nil
		}
		if value.(uint16) < tother {
			return -1, nil
		}
		return 1, nil

	case uint32:
		// 处理 uint32 类型的比较
		tother, ok := other.(uint32)
		// 检查 other 是否为 uint32 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(uint32) == tother {
			return 0, nil
		}
		if value.(uint32) < tother {
			return -1, nil
		}
		return 1, nil

	case uint64:
		// 处理 uint64 类型的比较
		tother, ok := other.(uint64)
		// 检查 other 是否为 uint64 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(uint64) == tother {
			return 0, nil
		}
		if value.(uint64) < tother {
			return -1, nil
		}
		return 1, nil

	case float32:
		// 处理 float32 类型的比较
		tother, ok := other.(float32)
		// 检查 other 是否为 float32 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(float32) == tother {
			return 0, nil
		}
		if value.(float32) < tother {
			return -1, nil
		}
		return 1, nil

	case float64:
		// 处理 float64 类型的比较
		tother, ok := other.(float64)
		// 检查 other 是否为 float64 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(float64) == tother {
			return 0, nil
		}
		if value.(float64) < tother {
			return -1, nil
		}
		return 1, nil

	case string:
		// 处理 string 类型的比较
		tother, ok := other.(string)
		// 检查 other 是否为 string 类型
		if !ok {
			// 如果类型不匹配，返回 ErrTypeMismatch 错误
			err := &ErrTypeMismatch{t, other}
			logger.Error("比较失败：", err)
			return 0, err
		}

		// 使用标准比较运算符进行比较
		if value.(string) == tother {
			return 0, nil
		}
		if value.(string) < tother {
			return -1, nil
		}
		return 1, nil

	case Comparer:
		// 处理实现了 Comparer 接口的类型的比较
		return value.(Comparer).Compare(other)

	default:
		// 对于未明确处理的类型，转换为字符串后进行比较
		valS := fmt.Sprintf("%s", value)
		otherS := fmt.Sprintf("%s", other)
		if valS == otherS {
			return 0, nil
		}
		if valS < otherS {
			return -1, nil
		}
		return 1, nil
	}
}
