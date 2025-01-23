// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试嵌套结构体的查询和操作。
// 测试内容：验证对于包含嵌套结构的复杂数据类型，badgerhold 能够正确处理查询、插入和更新等操作。

package badgerhold_test

import (
	"reflect"
	"testing"

	"github.com/bpfs/defs/v2/badgerhold"
)

// Nested 结构体定义了一个嵌套的数据结构，包括嵌入的结构体和指针。
type Nested struct {
	Key     int    // Key 是唯一标识符
	Embed          // Embed 是嵌入的结构体，包含 Color 字段
	L1      Nest   // L1 是一个 Nest 类型的字段
	L2      Level2 // L2 是一个包含多层嵌套的 Level2 类型字段
	Pointer *Nest  // Pointer 是一个指向 Nest 结构体的指针
}

// Embed 结构体定义了一个包含 Color 字段的嵌入结构体。
type Embed struct {
	Color string // Color 表示嵌入结构体中的颜色属性
}

// Nest 结构体定义了一个包含 Name 字段的简单结构体。
type Nest struct {
	Name string // Name 表示嵌套结构体中的名称属性
}

// Level2 结构体定义了一个多层嵌套的结构体，其中包含一个嵌套的 Nest 结构体。
type Level2 struct {
	Name string // Name 表示第二层嵌套中的名称属性
	L3   Nest   // L3 是第三层嵌套的 Nest 结构体
}

// nestedData 是一个包含多个 Nested 结构体实例的切片，用于测试数据。
var nestedData = []Nested{
	{
		Key: 0,
		Embed: Embed{
			Color: "red",
		},
		L1: Nest{
			Name: "Joe",
		},
		L2: Level2{
			Name: "Joe",
			L3: Nest{
				Name: "Joe",
			},
		},
		Pointer: &Nest{
			Name: "Joe",
		},
	},
	{
		Key: 1,
		Embed: Embed{
			Color: "red",
		},
		L1: Nest{
			Name: "Jill",
		},
		L2: Level2{
			Name: "Jill",
			L3: Nest{
				Name: "Jill",
			},
		},
		Pointer: &Nest{
			Name: "Jill",
		},
	},
	{
		Key: 2,
		Embed: Embed{
			Color: "orange",
		},
		L1: Nest{
			Name: "Jill",
		},
		L2: Level2{
			Name: "Jill",
			L3: Nest{
				Name: "Jill",
			},
		},
		Pointer: &Nest{
			Name: "Jill",
		},
	},
	{
		Key: 3,
		Embed: Embed{
			Color: "orange",
		},
		L1: Nest{
			Name: "Jill",
		},
		L2: Level2{
			Name: "Jill",
			L3: Nest{
				Name: "Joe",
			},
		}, Pointer: &Nest{
			Name: "Jill",
		},
	},
	{
		Key: 4,
		Embed: Embed{
			Color: "blue",
		},
		L1: Nest{
			Name: "Abner",
		},
		L2: Level2{
			Name: "Abner",
			L3: Nest{
				Name: "Abner",
			},
		}, Pointer: &Nest{
			Name: "Abner",
		},
	},
}

// nestedTests 是一个包含多个测试用例的切片，用于验证嵌套查询和排序功能。
var nestedTests = []test{
	{
		name:   "Nested",                              // 测试名称
		query:  badgerhold.Where("L1.Name").Eq("Joe"), // 根据 L1.Name 字段查询 "Joe"
		result: []int{0},                              // 预期结果的索引
	},
	{
		name:   "Embedded",                          // 测试嵌入字段
		query:  badgerhold.Where("Color").Eq("red"), // 根据嵌入的 Color 字段查询 "red"
		result: []int{0, 1},                         // 预期结果的索引
	},
	{
		name:   "Embedded Explicit",                       // 测试显式嵌入字段
		query:  badgerhold.Where("Embed.Color").Eq("red"), // 根据显式嵌入的 Color 字段查询 "red"
		result: []int{0, 1},                               // 预期结果的索引
	},
	{
		name:   "Nested Multiple Levels",                 // 测试多层嵌套字段
		query:  badgerhold.Where("L2.L3.Name").Eq("Joe"), // 根据多层嵌套的 L2.L3.Name 字段查询 "Joe"
		result: []int{0, 3},                              // 预期结果的索引
	},
	{
		name:   "Pointer",                                   // 测试指针字段
		query:  badgerhold.Where("Pointer.Name").Eq("Jill"), // 根据指针字段 Pointer.Name 查询 "Jill"
		result: []int{1, 2, 3},                              // 预期结果的索引
	},
	{
		name:   "Sort",                                             // 测试排序
		query:  badgerhold.Where("Key").Ge(0).SortBy("L2.L3.Name"), // 根据 L2.L3.Name 字段进行排序
		result: []int{4, 1, 2, 0, 3},                               // 预期结果的排序索引
	},
	{
		name:   "Sort On Pointer",                                    // 测试基于指针字段的排序
		query:  badgerhold.Where("Key").Ge(0).SortBy("Pointer.Name"), // 根据 Pointer.Name 字段进行排序
		result: []int{4, 1, 2, 0, 3},                                 // 预期结果的排序索引
	},
}

// TestNested 测试嵌套查询和排序功能。
func TestNested(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 遍历 nestedData 切片，将每个元素插入到 badgerhold 存储中。
		for i := range nestedData {
			// 将当前的 Nested 数据插入到 badgerhold 存储中。
			err := store.Insert(nestedData[i].Key, nestedData[i])
			// 如果插入操作发生错误，则报告错误并终止测试。
			if err != nil {
				t.Fatalf("Error inserting nested test data for nested find test: %s", err)
			}
		}

		// 遍历 nestedTests 切片，执行每个测试用例。
		for _, tst := range nestedTests {
			// 使用 t.Run 启动一个新的子测试，以 tst.name 作为测试名称。
			t.Run(tst.name, func(t *testing.T) {
				var result []Nested // 定义一个切片用于存储查询结果。

				// 使用 store.Find 方法执行查询，将结果存储在 result 切片中。
				err := store.Find(&result, tst.query)
				// 如果查询操作发生错误，则报告错误并终止测试。
				if err != nil {
					t.Fatalf("Error finding data from badgerhold: %s", err)
				}

				// 检查查询结果的数量是否与预期结果的数量相符。
				if len(result) != len(tst.result) {
					// 如果测试处于详细模式下，报告结果不匹配的详细信息。
					if testing.Verbose() {
						t.Fatalf("Find result count is %d wanted %d.  Results: %v", len(result),
							len(tst.result), result)
					}
					// 如果结果数量不匹配，则报告错误并终止测试。
					t.Fatalf("Find result count is %d wanted %d.", len(result), len(tst.result))
				}

				// 遍历查询结果，检查每个结果是否在预期结果中。
				for i := range result {
					found := false // 初始化 found 标志，用于跟踪结果是否匹配。
					// 遍历预期结果的索引，检查当前结果是否与预期数据匹配。
					for k := range tst.result {
						// 使用 reflect.DeepEqual 函数比较当前结果与预期数据。
						if reflect.DeepEqual(result[i], nestedData[tst.result[k]]) {
							found = true // 如果找到匹配项，则将 found 标志设置为 true。
							break        // 退出内层循环。
						}
					}

					// 如果没有找到匹配项，则报告错误并终止测试。
					if !found {
						// 如果测试处于详细模式下，报告未匹配项的详细信息。
						if testing.Verbose() {
							t.Fatalf("%v should not be in the result set! Full results: %v",
								result[i], result)
						}
						// 报告未匹配项的错误信息并终止测试。
						t.Fatalf("%v should not be in the result set!", result[i])
					}
				}
			})
		}
	})
}
