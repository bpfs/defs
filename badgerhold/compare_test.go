// Copyright 2016 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试 badgerhold 中的比较器功能。
// 测试内容：验证自定义的 Compare 方法的正确性，确保能够正确比较不同的结构体实例。

package badgerhold_test

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/bpfs/defs/v2/badgerhold"
)

// CItemTest 定义一个包含 ItemTest 结构体的复合结构体。
type CItemTest struct {
	Inner ItemTest // Inner 是一个嵌套的 ItemTest 结构体。
}

// Compare 方法用于比较两个 ItemTest 对象的 ID 字段。
func (i *ItemTest) Compare(other interface{}) (int, error) {
	// 检查传入的对象是否为 ItemTest 类型。
	if other, ok := other.(ItemTest); ok {
		// 如果两个对象的 ID 相等，返回 0。
		if i.ID == other.ID {
			return 0, nil
		}

		// 如果当前对象的 ID 小于传入对象的 ID，返回 -1。
		if i.ID < other.ID {
			return -1, nil
		}

		// 否则返回 1，表示当前对象的 ID 大于传入对象的 ID。
		return 1, nil
	}

	// 如果类型不匹配，返回类型不匹配错误。
	return 0, &badgerhold.ErrTypeMismatch{Value: i, Other: other}
}

// TestFindWithComparer 测试使用自定义比较器在 badgerhold 中查找数据。
func TestFindWithComparer(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 定义一些测试数据。
		data := []CItemTest{
			{
				Inner: ItemTest{
					Key:      0,
					ID:       0,
					Name:     "car",
					Category: "vehicle",
					Created:  time.Now().AddDate(-1, 0, 0),
				},
			},
			{
				Inner: ItemTest{
					Key:      1,
					ID:       1,
					Name:     "truck",
					Category: "vehicle",
					Created:  time.Now().AddDate(0, 30, 0),
				},
			},
			{
				Inner: ItemTest{
					Key:      2,
					ID:       2,
					Name:     "seal",
					Category: "animal",
					Created:  time.Now().AddDate(-1, 0, 0),
				},
			},
		}

		// 插入测试数据到 store 中。
		for i := range data {
			err := store.Insert(data[i].Inner.Key, data[i])
			if err != nil {
				t.Fatalf("Error inserting CItemData for comparer test %s", err)
			}
		}

		// 定义一个结果集用于存储查询结果。
		var result []CItemTest

		// 在 store 中查找 Inner 字段大于指定值的数据。
		err := store.Find(&result, badgerhold.Where("Inner").Gt(data[1].Inner))
		if err != nil {
			t.Fatalf("Error retrieving data in comparer test: %s", err)
		}

		// 检查查询结果的长度是否为 1。
		if len(result) != 1 {
			if testing.Verbose() {
				t.Fatalf("Find result count is %d wanted %d.  Results: %v", len(result), 1, result)
			}
			t.Fatalf("Find result count is %d wanted %d.", len(result), 1)
		}
	})
}

// DefaultType 定义一个带有 String 方法的结构体。
type DefaultType struct {
	Val string // Val 是一个字符串类型的字段。
}

// String 返回 Val 字段的字符串表示形式。
func (d *DefaultType) String() string {
	return d.Val
}

// All 定义一个包含各种内建类型字段的结构体。
type All struct {
	ATime  time.Time
	AFloat *big.Float
	AInt   *big.Int
	ARat   *big.Rat

	Aint   int
	Aint8  int8
	Aint16 int16
	Aint32 int32
	Aint64 int64

	Auint   uint
	Auint8  uint8
	Auint16 uint16
	Auint32 uint32
	Auint64 uint64

	Afloat32 float32
	Afloat64 float64

	Astring string

	ADefault DefaultType
}

// allCurrent 定义一个 All 类型的实例，表示当前的测试数据。
var allCurrent = All{ // current
	ATime:  time.Date(2016, 1, 1, 0, 0, 0, 0, time.Local),
	AFloat: big.NewFloat(30.5),
	AInt:   big.NewInt(123),
	ARat:   big.NewRat(5, 8),

	Aint:   8,
	Aint8:  8,
	Aint16: 8,
	Aint32: 8,
	Aint64: 8,

	Auint:   8,
	Auint8:  8,
	Auint16: 8,
	Auint32: 8,
	Auint64: 8,

	Afloat32: 8.8,
	Afloat64: 8.8,

	Astring: "btest",

	ADefault: DefaultType{"btest"},
}

// allData 定义一个 All 类型的切片，表示一组测试数据。
var allData = []All{
	{ // equal
		ATime:  time.Date(2016, 1, 1, 0, 0, 0, 0, time.Local),
		AFloat: big.NewFloat(30.5),
		AInt:   big.NewInt(123),
		ARat:   big.NewRat(5, 8),

		Aint:   8,
		Aint8:  8,
		Aint16: 8,
		Aint32: 8,
		Aint64: 8,

		Auint:   8,
		Auint8:  8,
		Auint16: 8,
		Auint32: 8,
		Auint64: 8,

		Afloat32: 8.8,
		Afloat64: 8.8,

		Astring:  "btest",
		ADefault: DefaultType{"btest"},
	},
	{ // greater
		ATime:  time.Date(2017, 1, 1, 0, 0, 0, 0, time.Local),
		AFloat: big.NewFloat(31.5),
		AInt:   big.NewInt(128),
		ARat:   big.NewRat(14, 16),

		Aint:   9,
		Aint8:  9,
		Aint16: 9,
		Aint32: 9,
		Aint64: 9,

		Auint:   9,
		Auint8:  9,
		Auint16: 9,
		Auint32: 9,
		Auint64: 9,

		Afloat32: 9.8,
		Afloat64: 9.8,

		Astring:  "ctest",
		ADefault: DefaultType{"ctest"},
	},
	{ // less
		ATime:  time.Date(2015, 1, 1, 0, 0, 0, 0, time.Local),
		AFloat: big.NewFloat(30.1),
		AInt:   big.NewInt(121),
		ARat:   big.NewRat(1, 4),

		Aint:   4,
		Aint8:  4,
		Aint16: 4,
		Aint32: 4,
		Aint64: 4,

		Auint:   4,
		Auint8:  4,
		Auint16: 4,
		Auint32: 4,
		Auint64: 4,

		Afloat32: 4.8,
		Afloat64: 4.8,

		Astring:  "atest",
		ADefault: DefaultType{"atest"},
	},
}

// TestFindWithBuiltinTypes 测试使用内建类型在 badgerhold 中查找数据。
func TestFindWithBuiltinTypes(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入 allData 中的每个数据到 store 中。
		for i := range allData {
			err := store.Insert(i, allData[i])
			if err != nil {
				t.Fatalf("Error inserting allData for builtin compare test %s", err)
			}
		}

		// 获取 allCurrent 的反射类型信息。
		to := reflect.TypeOf(allCurrent)

		// 遍历所有字段并测试查找操作。
		for i := 0; i < to.NumField(); i++ {
			// 获取当前字段的值。
			curField := reflect.ValueOf(allCurrent).FieldByName(to.Field(i).Name).Interface()

			// 测试相等查询。
			t.Run(fmt.Sprintf("Builtin type %s equal", to.Field(i).Name), func(t *testing.T) {
				var result []All
				err := store.Find(&result, badgerhold.Where(to.Field(i).Name).Eq(curField))
				if err != nil {
					t.Fatalf("Error finding equal result %s", err)
				}

				if len(result) != 1 {
					if testing.Verbose() {
						t.Fatalf("Find result count is %d wanted %d.  Results: %v",
							len(result), 1, result)
					}
					t.Fatalf("Find result count is %d wanted %d.", len(result), 1)
				}

				if !reflect.DeepEqual(result[0], allData[0]) {
					t.Fatalf("%v is not equal to %v", result[0], allData[0])
				}
			})

			// 测试大于查询。
			t.Run(fmt.Sprintf("Builtin type %s greater than", to.Field(i).Name), func(t *testing.T) {
				var result []All
				err := store.Find(&result, badgerhold.Where(to.Field(i).Name).Gt(curField))
				if err != nil {
					t.Fatalf("Error finding greater result %s", err)
				}

				if len(result) != 1 {
					if testing.Verbose() {
						t.Fatalf("Find result count is %d wanted %d.  Results: %v",
							len(result), 1, result)
					}
					t.Fatalf("Find result count is %d wanted %d.", len(result), 1)
				}

				if !reflect.DeepEqual(result[0], allData[1]) {
					t.Fatalf("%v is not equal to %v", result[0], allData[1])
				}
			})

			// 测试小于查询。
			t.Run(fmt.Sprintf("Builtin type %s less than", to.Field(i).Name), func(t *testing.T) {
				var result []All
				err := store.Find(&result, badgerhold.Where(to.Field(i).Name).Lt(curField))
				if err != nil {
					t.Fatalf("Error finding less result %s", err)
				}

				if len(result) != 1 {
					if testing.Verbose() {
						t.Fatalf("Find result count is %d wanted %d.  Results: %v",
							len(result), 1, result)
					}
					t.Fatalf("Find result count is %d wanted %d.", len(result), 1)
				}

				if !reflect.DeepEqual(result[0], allData[2]) {
					t.Fatalf("%v is not equal to %v", result[0], allData[2])
				}
			})
		}
	})
}
