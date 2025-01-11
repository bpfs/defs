// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试 Find 方法的功能，确保能够根据各种查询条件正确返回数据。
// 测试内容：验证不同条件下的查询操作，包括简单条件查询、复合条件查询、排序、分页等。

package badgerhold_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bpfs/defs/v2/badgerhold"
)

// ItemTest 代表用于测试的数据结构
type ItemTest struct {
	Key         int               // Key 是该项目的唯一键
	ID          int               // ID 是项目的标识符
	Name        string            // Name 是项目的名称
	Category    string            // Category 是项目的类别，并且根据 `badgerholdIndex:"Category"` 创建索引
	Created     time.Time         // Created 是项目的创建时间
	Tags        []string          // Tags 是与项目相关的标签列表
	Color       string            // Color 是项目的颜色属性
	Fruit       string            // Fruit 是与项目相关的水果属性
	UpdateField string            // UpdateField 是一个更新字段的示例
	UpdateIndex string            // UpdateIndex 是一个带有索引的更新字段，并根据 `badgerholdIndex:"UpdateIndex"` 创建索引
	MapVal      map[string]string // MapVal 是一个与项目相关的键值对映射
}

// equal 检查当前 ItemTest 与另一个 ItemTest 实例是否相等
// 参数:
//   - other: *ItemTest 类型，表示要比较的另一个 ItemTest 实例
//
// 返回值:
//   - bool: 返回 true 表示两个实例相等，false 表示不相等
func (i *ItemTest) equal(other *ItemTest) bool {
	if i.ID != other.ID { // 如果 ID 不相等
		return false // 返回 false
	}

	if i.Name != other.Name { // 如果 Name 不相等
		return false // 返回 false
	}

	if i.Category != other.Category { // 如果 Category 不相等
		return false // 返回 false
	}

	if !i.Created.Equal(other.Created) { // 如果 Created 时间不相等
		return false // 返回 false
	}

	return true // 如果所有字段都相等，返回 true
}

// testData 是一个 ItemTest 类型的样本数据集合
var testData = []ItemTest{
	{
		Key:      0,
		ID:       0,
		Name:     "car",
		Category: "vehicle",
		Created:  time.Now().AddDate(-1, 0, 0), // 创建时间为一年前
	},
	{
		Key:      1,
		ID:       1,
		Name:     "truck",
		Category: "vehicle",
		Created:  time.Now().AddDate(0, 30, 0), // 创建时间为当前时间加上 30 天
	},
	{
		Key:      2,
		Name:     "seal",
		Category: "animal",
		Created:  time.Now().AddDate(-1, 0, 0), // 创建时间为一年前
	},
	{
		Key:      3,
		ID:       3,
		Name:     "van",
		Category: "vehicle",
		Created:  time.Now().AddDate(0, 30, 0), // 创建时间为当前时间加上 30 天
	},
	{
		Key:      4,
		ID:       8,
		Name:     "pizza",
		Category: "food",
		Created:  time.Now(),                    // 创建时间为当前时间
		Tags:     []string{"cooked", "takeout"}, // 设置 Tags 为 "cooked" 和 "takeout"
	},
	{
		Key:      5,
		ID:       1,
		Name:     "crow",
		Category: "animal",
		Created:  time.Now(), // 创建时间为当前时间
		Color:    "blue",     // 设置 Color 为 "blue"
		Fruit:    "orange",   // 设置 Fruit 为 "orange"
	},
	{
		Key:      6,
		ID:       5,
		Name:     "van",
		Category: "vehicle",
		Created:  time.Now(), // 创建时间为当前时间
		Color:    "orange",   // 设置 Color 为 "orange"
		Fruit:    "orange",   // 设置 Fruit 为 "orange"
	},
	{
		Key:      7,
		ID:       5,
		Name:     "pizza",
		Category: "food",
		Created:  time.Now(),                    // 创建时间为当前时间
		Tags:     []string{"cooked", "takeout"}, // 设置 Tags 为 "cooked" 和 "takeout"
	},
	{
		Key:      8,
		ID:       6,
		Name:     "lion",
		Category: "animal",
		Created:  time.Now().AddDate(3, 0, 0), // 创建时间为三年后
	},
	{
		Key:      9,
		ID:       7,
		Name:     "bear",
		Category: "animal",
		Created:  time.Now().AddDate(3, 0, 0), // 创建时间为三年后
	},
	{
		Key:      10,
		ID:       9,
		Name:     "tacos",
		Category: "food",
		Created:  time.Now().AddDate(-3, 0, 0),  // 创建时间为三年前
		Tags:     []string{"cooked", "takeout"}, // 设置 Tags 为 "cooked" 和 "takeout"
		Color:    "orange",                      // 设置 Color 为 "orange"
	},
	{
		Key:      11,
		ID:       10,
		Name:     "golf cart",
		Category: "vehicle",
		Created:  time.Now().AddDate(0, 0, 30), // 创建时间为当前时间加上 30 天
		Color:    "pink",                       // 设置 Color 为 "pink"
		Fruit:    "apple"},                     // 设置 Fruit 为 "apple"
	{
		Key:      12,
		ID:       11,
		Name:     "oatmeal",
		Category: "food",
		Created:  time.Now().AddDate(0, 0, -30), // 创建时间为 30 天前
		Tags:     []string{"cooked", "healthy"}, // 设置 Tags 为 "cooked" 和 "healthy"
	},
	{
		Key:      13,
		ID:       8,
		Name:     "mouse",
		Category: "animal",
		Created:  time.Now(), // 创建时间为当前时间
	},
	{
		Key:      14,
		ID:       12,
		Name:     "fish",
		Category: "animal",
		Created:  time.Now().AddDate(0, 0, -1), // 创建时间为 1 天前
	},
	{
		Key:      15,
		ID:       13,
		Name:     "fish",
		Category: "food",
		Created:  time.Now(),                    // 创建时间为当前时间
		Tags:     []string{"cooked", "healthy"}, // 设置 Tags 为 "cooked" 和 "healthy"
	},
	{
		Key:      16,
		ID:       9,
		Name:     "zebra",
		Category: "animal",
		Created:  time.Now(), // 创建时间为当前时间
		MapVal: map[string]string{ // 设置 MapVal 为包含键值对 "test":"testval" 的映射
			"test": "testval",
		},
	},
}

// 定义一个测试结构体，用于存储测试用例的信息
type test struct {
	name   string            // name 表示测试用例的名称
	query  *badgerhold.Query // query 是一个 badgerhold 查询，用于在数据库中查找数据
	result []int             // result 是预期的结果索引列表，对应于测试数据中的索引
}

// 定义一组测试用例，存储在 testResults 切片中
var testResults = []test{
	{
		name:   "Equal Key",                                          // 测试名称为 "Equal Key"
		query:  badgerhold.Where(badgerhold.Key).Eq(testData[4].Key), // 查询条件为 Key 等于 testData[4] 的 Key
		result: []int{4},                                             // 预期结果是索引 4
	},
	{
		name:   "Equal Field Without Index",                   // 测试名称为 "Equal Field Without Index"
		query:  badgerhold.Where("Name").Eq(testData[1].Name), // 查询条件为 Name 等于 testData[1] 的 Name
		result: []int{1},                                      // 预期结果是索引 1
	},
	{
		name:   "Equal Field With Index",                   // 测试名称为 "Equal Field With Index"
		query:  badgerhold.Where("Category").Eq("vehicle"), // 查询条件为 Category 等于 "vehicle"
		result: []int{0, 1, 3, 6, 11},                      // 预期结果是索引 0, 1, 3, 6, 11
	},
	{
		name:   "Not Equal Key",                                              // 测试名称为 "Not Equal Key"
		query:  badgerhold.Where(badgerhold.Key).Ne(testData[4].Key),         // 查询条件为 Key 不等于 testData[4] 的 Key
		result: []int{0, 1, 2, 3, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, // 预期结果是索引 0 到 16，排除索引 4
	},
	{
		name:   "Not Equal Field Without Index",                              // 测试名称为 "Not Equal Field Without Index"
		query:  badgerhold.Where("Name").Ne(testData[1].Name),                // 查询条件为 Name 不等于 testData[1] 的 Name
		result: []int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, // 预期结果是索引 0 到 16，排除索引 1
	},
	{
		name:   "Not Equal Field With Index",                    // 测试名称为 "Not Equal Field With Index"
		query:  badgerhold.Where("Category").Ne("vehicle"),      // 查询条件为 Category 不等于 "vehicle"
		result: []int{2, 4, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16}, // 预期结果是索引 2, 4, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16
	},
	{
		name:   "Greater Than Key",                                    // 测试名称为 "Greater Than Key"
		query:  badgerhold.Where(badgerhold.Key).Gt(testData[10].Key), // 查询条件为 Key 大于 testData[10] 的 Key
		result: []int{11, 12, 13, 14, 15, 16},                         // 预期结果是索引 11 到 16
	},
	{
		name:   "Greater Than Field Without Index", // 测试名称为 "Greater Than Field Without Index"
		query:  badgerhold.Where("ID").Gt(10),      // 查询条件为 ID 大于 10
		result: []int{12, 14, 15},                  // 预期结果是索引 12, 14, 15
	},
	{
		name:   "Greater Than Field With Index",         // 测试名称为 "Greater Than Field With Index"
		query:  badgerhold.Where("Category").Gt("food"), // 查询条件为 Category 大于 "food"
		result: []int{0, 1, 3, 6, 11},                   // 预期结果是索引 0, 1, 3, 6, 11
	},
	{
		name:   "Less Than Key",                                      // 测试名称为 "Less Than Key"
		query:  badgerhold.Where(badgerhold.Key).Lt(testData[0].Key), // 查询条件为 Key 小于 testData[0] 的 Key
		result: []int{},                                              // 预期结果为空
	},
	{
		name:   "Less Than Field Without Index", // 测试名称为 "Less Than Field Without Index"
		query:  badgerhold.Where("ID").Lt(5),    // 查询条件为 ID 小于 5
		result: []int{0, 1, 2, 3, 5},            // 预期结果是索引 0, 1, 2, 3, 5
	},
	{
		name:   "Less Than Field With Index",            // 测试名称为 "Less Than Field With Index"
		query:  badgerhold.Where("Category").Lt("food"), // 查询条件为 Category 小于 "food"
		result: []int{2, 5, 8, 9, 13, 14, 16},           // 预期结果是索引 2, 5, 8, 9, 13, 14, 16
	},
	{
		name:   "Less Than or Equal To Key",                          // 测试名称为 "Less Than or Equal To Key"
		query:  badgerhold.Where(badgerhold.Key).Le(testData[0].Key), // 查询条件为 Key 小于或等于 testData[0] 的 Key
		result: []int{0},                                             // 预期结果是索引 0
	},
	{
		name:   "Less Than or Equal To Field Without Index", // 测试名称为 "Less Than or Equal To Field Without Index"
		query:  badgerhold.Where("ID").Le(5),                // 查询条件为 ID 小于或等于 5
		result: []int{0, 1, 2, 3, 5, 6, 7},                  // 预期结果是索引 0, 1, 2, 3, 5, 6, 7
	},
	{
		name:   "Less Than Field With Index",                    // 测试名称为 "Less Than Field With Index"
		query:  badgerhold.Where("Category").Le("food"),         // 查询条件为 Category 小于或等于 "food"
		result: []int{2, 5, 8, 9, 13, 14, 16, 4, 7, 10, 12, 15}, // 预期结果是索引 2, 5, 8, 9, 13, 14, 16, 4, 7, 10, 12, 15
	},
	{
		name:   "Greater Than or Equal To Key",                        // 测试名称为 "Greater Than or Equal To Key"
		query:  badgerhold.Where(badgerhold.Key).Ge(testData[10].Key), // 查询条件为 Key 大于或等于 testData[10] 的 Key
		result: []int{10, 11, 12, 13, 14, 15, 16},                     // 预期结果是索引 10, 11, 12, 13, 14, 15, 16
	},
	{
		name:   "Greater Than or Equal To Field Without Index", // 测试名称为 "Greater Than or Equal To Field Without Index"
		query:  badgerhold.Where("ID").Ge(10),                  // 查询条件为 ID 大于或等于 10
		result: []int{11, 12, 14, 15},                          // 预期结果是索引 11, 12, 14, 15
	},
	{
		name:   "Greater Than or Equal To Field With Index", // 测试名称为 "Greater Than or Equal To Field With Index"
		query:  badgerhold.Where("Category").Ge("food"),     // 查询条件为 Category 大于或等于 "food"
		result: []int{0, 1, 3, 6, 11, 4, 7, 10, 12, 15},     // 预期结果是索引 0, 1, 3, 6, 11, 4, 7, 10, 12, 15
	},
	{
		name:   "In",                               // 测试名称为 "In"
		query:  badgerhold.Where("ID").In(5, 8, 3), // 查询条件为 ID 在 5, 8, 3 之间
		result: []int{3, 6, 7, 4, 13},              // 预期结果是索引 3, 6, 7, 4, 13
	},
	{
		name:   "In on data from other index",                        // 测试名称为 "In on data from other index"
		query:  badgerhold.Where("ID").In(5, 8, 3).Index("Category"), // 查询条件为 ID 在 5, 8, 3 之间，并指定使用 "Category" 索引
		result: []int{3, 6, 7, 4, 13},                                // 预期结果是索引 3, 6, 7, 4, 13
	},
	{
		name:   "In on index",                                                       // 测试名称为 "In on index"
		query:  badgerhold.Where("Category").In("food", "animal").Index("Category"), // 查询条件为 Category 在 "food", "animal" 之间，并指定使用 "Category" 索引
		result: []int{4, 2, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16},                     // 预期结果是索引 4, 2, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16
	},
	{
		name:   "Regular Expression",                                      // 测试名称为 "Regular Expression"
		query:  badgerhold.Where("Name").RegExp(regexp.MustCompile("ea")), // 查询条件为 Name 匹配正则表达式 "ea"
		result: []int{2, 9, 12},                                           // 预期结果是索引 2, 9, 12
	},
	{
		name: "Function Field", // 测试名称为 "Function Field"
		query: badgerhold.Where("Name").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) { // 查询条件为 Name 匹配自定义函数
			field := ra.Field()     // 获取字段值
			_, ok := field.(string) // 检查字段是否为字符串类型
			if !ok {
				return false, fmt.Errorf("Field not a string, it's a %T!", field) // 如果字段不是字符串，返回错误
			}

			return strings.HasPrefix(field.(string), "oat"), nil // 检查字段是否以 "oat" 开头
		}),
		result: []int{12}, // 预期结果是索引 12
	},
	{
		name: "Function Record", // 测试名称为 "Function Record"
		query: badgerhold.Where("ID").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) { // 查询条件为 ID 匹配自定义函数
			record := ra.Record()       // 获取记录值
			_, ok := record.(*ItemTest) // 检查记录是否为 ItemTest 类型
			if !ok {
				return false, fmt.Errorf("Record not an ItemTest, it's a %T!", record) // 如果记录不是 ItemTest 类型，返回错误
			}

			return strings.HasPrefix(record.(*ItemTest).Name, "oat"), nil // 检查 Name 是否以 "oat" 开头
		}),
		result: []int{12}, // 预期结果是索引 12
	},
	{
		name: "Function Subquery", // 测试名称为 "Function Subquery"
		query: badgerhold.Where("Name").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) { // 查询条件为 Name 匹配自定义函数，使用子查询
			// 查找同名但属于不同类别的记录
			record, ok := ra.Record().(*ItemTest) // 获取记录值并检查是否为 ItemTest 类型
			if !ok {
				return false, fmt.Errorf("Record is not ItemTest, it's a %T", ra.Record()) // 如果记录不是 ItemTest 类型，返回错误
			}

			var result []ItemTest // 创建一个空的结果切片

			err := ra.SubQuery(&result,
				badgerhold.Where("Name").Eq(record.Name).And("Category").Ne(record.Category)) // 运行子查询
			if err != nil {
				return false, err // 如果子查询出错，返回错误
			}

			if len(result) > 0 { // 如果子查询返回结果不为空
				return true, nil // 返回 true
			}

			return false, nil // 否则返回 false
		}),
		result: []int{14, 15}, // 预期结果是索引 14, 15
	},
	{
		name:   "Time Comparison",                          // 测试名称为 "Time Comparison"
		query:  badgerhold.Where("Created").Gt(time.Now()), // 查询条件为 Created 时间大于当前时间
		result: []int{1, 3, 8, 9, 11},                      // 预期结果是索引 1, 3, 8, 9, 11
	},
	{
		name:   "Chained And Query with non-index lead",                                  // 测试名称为 "Chained And Query with non-index lead"
		query:  badgerhold.Where("Created").Gt(time.Now()).And("Category").Eq("vehicle"), // 查询条件为 Created 时间大于当前时间并且 Category 等于 "vehicle"
		result: []int{1, 3, 11},                                                          // 预期结果是索引 1, 3, 11
	},
	{
		name:   "Multiple Chained And Queries with non-index lead",                                        // 测试名称为 "Multiple Chained And Queries with non-index lead"
		query:  badgerhold.Where("Created").Gt(time.Now()).And("Category").Eq("vehicle").And("ID").Ge(10), // 查询条件为 Created 时间大于当前时间，Category 等于 "vehicle" 并且 ID 大于或等于 10
		result: []int{11},                                                                                 // 预期结果是索引 11
	},
	{
		name:   "Chained And Query with leading Index",                                                    // 测试名称为 "Chained And Query with leading Index"
		query:  badgerhold.Where("Category").Eq("vehicle").And("ID").Ge(10).And("Created").Gt(time.Now()), // 查询条件为 Category 等于 "vehicle"，ID 大于或等于 10 并且 Created 时间大于当前时间
		result: []int{11},                                                                                 // 预期结果是索引 11
	},
	{
		name:   "Chained Or Query with leading index",                                                    // 测试名称为 "Chained Or Query with leading index"
		query:  badgerhold.Where("Category").Eq("vehicle").Or(badgerhold.Where("Category").Eq("animal")), // 查询条件为 Category 等于 "vehicle" 或者 "animal"
		result: []int{0, 1, 3, 6, 11, 2, 5, 8, 9, 13, 14, 16},                                            // 预期结果是索引 0, 1, 3, 6, 11, 2, 5, 8, 9, 13, 14, 16
	},
	{
		name:   "Chained Or Query with unioned data",                                              // 测试名称为 "Chained Or Query with unioned data"
		query:  badgerhold.Where("Category").Eq("animal").Or(badgerhold.Where("Name").Eq("fish")), // 查询条件为 Category 等于 "animal" 或者 Name 等于 "fish"
		result: []int{2, 5, 8, 9, 13, 14, 16, 15},                                                 // 预期结果是索引 2, 5, 8, 9, 13, 14, 16, 15
	},
	{
		name: "Multiple Chained And + Or Query ", // 测试名称为 "Multiple Chained And + Or Query"
		query: badgerhold.Where("Category").Eq("animal").And("Created").Gt(time.Now()). // 查询条件为 Category 等于 "animal" 并且 Created 时间大于当前时间
												Or(badgerhold.Where("Name").Eq("fish").And("ID").Ge(13)), // 或者 Name 等于 "fish" 并且 ID 大于或等于 13
		result: []int{8, 9, 15}, // 预期结果是索引 8, 9, 15
	},
	{
		name:   "Nil Query",                                                     // 测试名称为 "Nil Query"
		query:  nil,                                                             // 查询条件为空，查询所有数据
		result: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, // 预期结果是索引 0 到 16
	},
	{
		name:   "Nil Comparison",                              // 测试名称为 "Nil Comparison"
		query:  badgerhold.Where("Tags").IsNil(),              // 查询条件为 Tags 为空
		result: []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 13, 14, 16}, // 预期结果是索引 0, 1, 2, 3, 5, 6, 8, 9, 11, 13, 14, 16
	},
	{
		name:   "String starts with",                       // 测试名称为 "String starts with"
		query:  badgerhold.Where("Name").HasPrefix("golf"), // 查询条件为 Name 以 "golf" 开头
		result: []int{11},                                  // 预期结果是索引 11
	},
	{
		name:   "String ends with",                         // 测试名称为 "String ends with"
		query:  badgerhold.Where("Name").HasSuffix("cart"), // 查询条件为 Name 以 "cart" 结尾
		result: []int{11},                                  // 预期结果是索引 11
	},
	{
		name:   "Self-Field comparison",                                                     // 测试名称为 "Self-Field comparison"
		query:  badgerhold.Where("Color").Eq(badgerhold.Field("Fruit")).And("Fruit").Ne(""), // 查询条件为 Color 等于 Fruit 并且 Fruit 不为空
		result: []int{6},                                                                    // 预期结果是索引 6
	},
	{
		name:   "Test Key in secondary",                                                         // 测试名称为 "Test Key in secondary"
		query:  badgerhold.Where("Category").Eq("food").And(badgerhold.Key).Eq(testData[4].Key), // 查询条件为 Category 等于 "food" 并且 Key 等于 testData[4] 的 Key
		result: []int{4},                                                                        // 预期结果是索引 4
	},
	{
		name:   "Skip",                                                        // 测试名称为 "Skip"
		query:  badgerhold.Where(badgerhold.Key).Gt(testData[10].Key).Skip(3), // 查询条件为 Key 大于 testData[10] 的 Key 并且跳过前三个结果
		result: []int{14, 15, 16},                                             // 预期结果是索引 14, 15, 16
	},
	{
		name:   "Skip Past Len",                                               // 测试名称为 "Skip Past Len"
		query:  badgerhold.Where(badgerhold.Key).Gt(testData[10].Key).Skip(9), // 查询条件为 Key 大于 testData[10] 的 Key 并且跳过前九个结果
		result: []int{},                                                       // 预期结果为空
	},
	{
		name:   "Skip with Or query",                                                                             // 测试名称为 "Skip with Or query"
		query:  badgerhold.Where("Category").Eq("vehicle").Or(badgerhold.Where("Category").Eq("animal")).Skip(4), // 查询条件为 Category 等于 "vehicle" 或者 "animal"，并且跳过前四个结果
		result: []int{11, 2, 5, 8, 9, 13, 14, 16},                                                                // 预期结果是索引 11, 2, 5, 8, 9, 13, 14, 16
	},
	{
		name:   "Skip with Or query, that crosses or boundary",                                                   // 测试名称为 "Skip with Or query, that crosses or boundary"
		query:  badgerhold.Where("Category").Eq("vehicle").Or(badgerhold.Where("Category").Eq("animal")).Skip(8), // 查询条件为 Category 等于 "vehicle" 或者 "animal"，并且跳过前八个结果
		result: []int{16, 9, 13, 14},                                                                             // 预期结果是索引 16, 9, 13, 14
	},
	{
		name:   "Limit",                                                        // 测试名称为 "Limit"
		query:  badgerhold.Where(badgerhold.Key).Gt(testData[10].Key).Limit(5), // 查询条件为 Key 大于 testData[10] 的 Key，并且限制结果数量为 5
		result: []int{11, 12, 13, 14, 15},                                      // 预期结果是索引 11, 12, 13, 14, 15
	},
	{
		name: "Issue #8 - Function Field on index", // 测试名称为 "Issue #8 - Function Field on index"
		query: badgerhold.Where("Category").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) { // 查询条件为 Category 匹配自定义函数
			field := ra.Field()     // 获取字段值
			_, ok := field.(string) // 检查字段是否为字符串类型
			if !ok {
				return false, fmt.Errorf("Field not a string, it's a %T!", field) // 如果字段不是字符串，返回错误
			}

			return !strings.HasPrefix(field.(string), "veh"), nil // 检查字段是否不以 "veh" 开头
		}),
		result: []int{2, 4, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16}, // 预期结果是索引 2, 4, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16
	},
	{
		name: "Issue #8 - Function Field on a specific index", // 测试名称为 "Issue #8 - Function Field on a specific index"
		query: badgerhold.Where("Category").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) { // 查询条件为 Category 匹配自定义函数，并指定使用 "Category" 索引
			field := ra.Field()     // 获取字段值
			_, ok := field.(string) // 检查字段是否为字符串类型
			if !ok {
				return false, fmt.Errorf("Field not a string, it's a %T!", field) // 如果字段不是字符串，返回错误
			}

			return !strings.HasPrefix(field.(string), "veh"), nil // 检查字段是否不以 "veh" 开头
		}).Index("Category"), // 使用 "Category" 索引
		result: []int{2, 4, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16}, // 预期结果是索引 2, 4, 5, 7, 8, 9, 10, 12, 13, 14, 15, 16
	},
	{
		name: "Find item with max ID in each category - sub aggregate query", // 测试名称为 "Find item with max ID in each category - sub aggregate query"
		query: badgerhold.Where("ID").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) { // 查询条件为 ID 匹配自定义函数，使用子聚合查询
			grp, err := ra.SubAggregateQuery(badgerhold.Where("Category").
				Eq(ra.Record().(*ItemTest).Category), "Category") // 查找具有最大 ID 的每个类别的项目
			if err != nil {
				return false, err // 如果子查询出错，返回错误
			}

			max := &ItemTest{} // 创建一个空的 ItemTest 对象

			grp[0].Max("ID", max)                  // 查找最大 ID
			return ra.Field().(int) == max.ID, nil // 返回是否为最大 ID
		}),
		result: []int{11, 14, 15}, // 预期结果是索引 11, 14, 15
	},
	{
		name:   "Indexed in",                                         // 测试名称为 "Indexed in"
		query:  badgerhold.Where("Category").In("animal", "vehicle"), // 查询条件为 Category 在 "animal", "vehicle" 之间
		result: []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 13, 14, 16},        // 预期结果是索引 0, 1, 2, 3, 5, 6, 8, 9, 11, 13, 14, 16
	},
	{
		name:   "Equal Field With Specific Index",                            // 测试名称为 "Equal Field With Specific Index"
		query:  badgerhold.Where("Category").Eq("vehicle").Index("Category"), // 查询条件为 Category 等于 "vehicle"，并指定使用 "Category" 索引
		result: []int{0, 1, 3, 6, 11},                                        // 预期结果是索引 0, 1, 3, 6, 11
	},
	{
		name:   "Key test after lead field",                                                      // 测试名称为 "Key test after lead field"
		query:  badgerhold.Where("Category").Eq("food").And(badgerhold.Key).Gt(testData[10].Key), // 查询条件为 Category 等于 "food" 并且 Key 大于 testData[10] 的 Key
		result: []int{12, 15},                                                                    // 预期结果是索引 12, 15
	},
	{
		name:   "Key test after lead index",                                                                        // 测试名称为 "Key test after lead index"
		query:  badgerhold.Where("Category").Eq("food").Index("Category").And(badgerhold.Key).Gt(testData[10].Key), // 查询条件为 Category 等于 "food"，并指定使用 "Category" 索引，并且 Key 大于 testData[10] 的 Key
		result: []int{12, 15},                                                                                      // 预期结果是索引 12, 15
	},
	{
		name:   "Contains",                                   // 测试名称为 "Contains"
		query:  badgerhold.Where("Tags").Contains("takeout"), // 查询条件为 Tags 包含 "takeout"
		result: []int{4, 7, 10},                              // 预期结果是索引 4, 7, 10
	},
	{
		name:   "Contains Any",                                             // 测试名称为 "Contains Any"
		query:  badgerhold.Where("Tags").ContainsAny("takeout", "healthy"), // 查询条件为 Tags 包含 "takeout" 或 "healthy"
		result: []int{4, 7, 10, 12, 15},                                    // 预期结果是索引 4, 7, 10, 12, 15
	},
	{
		name:   "Contains All",                                             // 测试名称为 "Contains All"
		query:  badgerhold.Where("Tags").ContainsAll("takeout", "healthy"), // 查询条件为 Tags 包含 "takeout" 和 "healthy"
		result: []int{},                                                    // 预期结果为空
	},
	{
		name:   "Contains All #2",                                         // 测试名称为 "Contains All #2"
		query:  badgerhold.Where("Tags").ContainsAll("cooked", "healthy"), // 查询条件为 Tags 包含 "cooked" 和 "healthy"
		result: []int{12, 15},                                             // 预期结果是索引 12, 15
	},
	{
		name:   "bh.Slice",                                                                               // 测试名称为 "bh.Slice"
		query:  badgerhold.Where("Tags").ContainsAll(badgerhold.Slice([]string{"cooked", "healthy"})...), // 查询条件为 Tags 包含 badgerhold.Slice([]string{"cooked", "healthy"})
		result: []int{12, 15},                                                                            // 预期结果是索引 12, 15
	},
	{
		name:   "Contains on non-slice",                         // 测试名称为 "Contains on non-slice"
		query:  badgerhold.Where("Category").Contains("cooked"), // 查询条件为 Category 包含 "cooked"
		result: []int{},                                         // 预期结果为空
	},
	{
		name:   "Map Has Key",                             // 测试名称为 "Map Has Key"
		query:  badgerhold.Where("MapVal").HasKey("test"), // 查询条件为 MapVal 具有键 "test"
		result: []int{16},                                 // 预期结果是索引 16
	},
	{
		name:   "Map Has Key 2",                            // 测试名称为 "Map Has Key 2"
		query:  badgerhold.Where("MapVal").HasKey("other"), // 查询条件为 MapVal 具有键 "other"
		result: []int{},                                    // 预期结果为空
	},
	{
		name:   "Issue 66 - Keys with In operator",           // 测试名称为 "Issue 66 - Keys with In operator"
		query:  badgerhold.Where(badgerhold.Key).In(1, 2, 3), // 查询条件为 Key 在 1, 2, 3 之间
		result: []int{1, 2, 3},                               // 预期结果是索引 1, 2, 3
	},
}

// insertTestData 将测试数据插入到 badgerhold.Store 中
func insertTestData(t *testing.T, store *badgerhold.Store) {
	for i := range testData { // 遍历 testData 切片中的每一项
		err := store.Insert(testData[i].Key, testData[i]) // 将当前项插入到存储中，使用 Key 作为主键
		if err != nil {                                   // 如果插入时发生错误
			t.Fatalf("Error inserting test data for find test: %s", err) // 记录错误并终止测试
		}
	}
}

// TestFind 测试 Find 方法的功能
func TestFind(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store)          // 在测试之前插入测试数据
		for _, tst := range testResults { // 遍历每个测试用例
			t.Run(tst.name, func(t *testing.T) { // 运行每个测试用例
				var result []ItemTest                 // 保存查询结果的切片
				err := store.Find(&result, tst.query) // 使用当前测试用例的查询条件查找数据
				if err != nil {                       // 如果查找时发生错误
					t.Fatalf("Error finding data from badgerhold: %s", err) // 记录错误并终止测试
				}
				if len(result) != len(tst.result) { // 如果结果的长度与预期不符
					if testing.Verbose() { // 如果测试运行在详细模式下
						t.Fatalf("Find result count is %d wanted %d.  Results: %v", len(result),
							len(tst.result), result) // 记录详细错误信息并终止测试
					}
					t.Fatalf("Find result count is %d wanted %d.", len(result), len(tst.result)) // 记录简要错误信息并终止测试
				}

				for i := range result { // 遍历结果切片
					found := false              // 标记是否在预期结果中找到当前项
					for k := range tst.result { // 遍历预期结果
						if result[i].equal(&testData[tst.result[k]]) { // 如果结果项与预期项相等
							found = true // 标记为找到
							break        // 退出预期结果循环
						}
					}

					if !found { // 如果未找到当前结果项
						if testing.Verbose() { // 如果测试运行在详细模式下
							t.Fatalf("%v should not be in the result set! Full results: %v",
								result[i], result) // 记录详细错误信息并终止测试
						}
						t.Fatalf("%v should not be in the result set!", result[i]) // 记录简要错误信息并终止测试
					}
				}
			})
		}
	})
}

// BadType 是一个测试用的无效类型
type BadType struct{}

// TestFindOnUnknownType 测试在未知类型上查找
func TestFindOnUnknownType(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store)                                           // 在测试之前插入测试数据
		var result []BadType                                               // 声明一个结果切片
		err := store.Find(&result, badgerhold.Where("BadName").Eq("blah")) // 尝试查找不存在的字段
		if err != nil {                                                    // 如果查找时发生错误
			t.Fatalf("Error finding data from badgerhold: %s", err) // 记录错误并终止测试
		}
		if len(result) != 0 { // 如果结果不为空
			t.Fatalf("Find result count is %d wanted %d.  Results: %v", len(result), 0, result) // 记录错误并终止测试
		}
	})
}

// TestFindWithNilValue 测试使用 nil 值进行查找
func TestFindWithNilValue(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store) // 在测试之前插入测试数据

		var result []ItemTest                                        // 声明一个结果切片
		err := store.Find(&result, badgerhold.Where("Name").Eq(nil)) // 尝试查找 Name 为 nil 的项
		if err == nil {                                              // 如果没有返回错误
			t.Fatalf("Comparing with nil did NOT return an error!") // 记录错误并终止测试
		}

		if _, ok := err.(*badgerhold.ErrTypeMismatch); !ok { // 检查错误是否为类型不匹配错误
			t.Fatalf("Comparing with nil did NOT return the correct error.  Got %v", err) // 记录错误并终止测试
		}
	})
}

// TestFindWithNonSlicePtr 测试使用非切片指针进行查找
func TestFindWithNonSlicePtr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with non-slice pointer did not panic!") // 记录错误并终止测试
			}
		}()
		var result []ItemTest                                       // 声明一个结果切片
		_ = store.Find(result, badgerhold.Where("Name").Eq("blah")) // 尝试在非切片指针上查找数据
	})
}

// TestQueryWhereNamePanic 测试在查询中使用小写字段名是否会引发 panic
func TestQueryWhereNamePanic(t *testing.T) {
	defer func() { // 延迟执行代码，以捕获可能的 panic
		if r := recover(); r == nil { // 如果没有发生 panic
			t.Fatalf("Querying with a lower case field did not cause a panic!") // 记录错误并终止测试
		}
	}()

	_ = badgerhold.Where("lower").Eq("test") // 尝试使用小写字段名进行查询
}

// TestQueryAndNamePanic 测试在 And 查询中使用小写字段名是否会引发 panic
func TestQueryAndNamePanic(t *testing.T) {
	defer func() { // 延迟执行代码，以捕获可能的 panic
		if r := recover(); r == nil { // 如果没有发生 panic
			t.Fatalf("Querying with a lower case field did not cause a panic!") // 记录错误并终止测试
		}
	}()

	_ = badgerhold.Where("Upper").Eq("test").And("lower").Eq("test") // 尝试在 And 查询中使用小写字段名
}

// TestFindOnInvalidFieldName 测试在无效字段名上进行查找
func TestFindOnInvalidFieldName(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store) // 在测试之前插入测试数据
		var result []ItemTest    // 声明一个结果切片

		err := store.Find(&result, badgerhold.Where("BadFieldName").Eq("test")) // 尝试使用无效字段名进行查找
		if err == nil {                                                         // 如果没有返回错误
			t.Fatalf("Find query against a bad field name didn't return an error!") // 记录错误并终止测试
		}

	})
}

// TestFindOnInvalidIndex 测试在无效索引上进行查找
func TestFindOnInvalidIndex(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store) // 在测试之前插入测试数据
		var result []ItemTest    // 声明一个结果切片

		err := store.Find(&result, badgerhold.Where("Name").Eq("test").Index("BadIndex")) // 尝试使用无效索引进行查找
		if err == nil {                                                                   // 如果没有返回错误
			t.Fatalf("Find query against a bad index name didn't return an error!") // 记录错误并终止测试
		}

	})
}

// TestFindOnEmptyBucketWithIndex 测试在空数据桶上使用有效索引进行查找
func TestFindOnEmptyBucketWithIndex(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		// 不插入数据
		var result []ItemTest // 声明一个结果切片

		err := store.Find(&result, badgerhold.Where("Category").Eq("animal").Index("Category")) // 尝试在空数据桶上使用有效索引进行查找
		if err != nil {                                                                         // 如果返回了错误
			t.Fatalf("Find query against a valid index name but an empty data bucket return an error!: %s",
				err) // 记录错误并终止测试
		}
		if len(result) > 0 { // 如果结果不为空
			t.Fatalf("Find query against an empty bucket returned results!") // 记录错误并终止测试
		}
	})
}

// TestQueryStringPrint 测试查询字符串的生成和输出
func TestQueryStringPrint(t *testing.T) {
	q := badgerhold.Where("FirstField").Eq("first value").And("SecondField").Gt("Second Value").And("ThirdField").
		Lt("Third Value").And("FourthField").Ge("FourthValue").And("FifthField").Le("FifthValue").And("SixthField").
		Ne("Sixth Value").Or(badgerhold.Where("FirstField").In("val1", "val2", "val3").And("SecondField").IsNil().
		And("ThirdField").RegExp(regexp.MustCompile("test")).Index("IndexName").And("FirstField").
		MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) {
			return true, nil
		})).And("SeventhField").HasPrefix("SeventhValue").And("EighthField").HasSuffix("EighthValue")
	// 构建一个复杂的查询，包含多个条件和逻辑运算

	contains := []string{
		"FirstField == first value",
		"SecondField > Second Value",
		"ThirdField < Third Value",
		"FourthField >= FourthValue",
		"FifthField <= FifthValue",
		"SixthField != Sixth Value",
		"FirstField in [val1 val2 val3]",
		"FirstField matches the function",
		"SecondField is nil",
		"ThirdField matches the regular expression test",
		"Using Index [IndexName]",
		"SeventhField starts with SeventhValue",
		"EighthField ends with EighthValue",
	}
	// 包含预期的查询字符串片段

	tst := q.String() // 将查询转换为字符串表示

	tstLines := strings.Split(tst, "\n") // 将查询字符串按行分割

	for i := range contains { // 遍历预期字符串片段
		found := false            // 标记是否找到片段
		for k := range tstLines { // 遍历查询字符串的每一行
			if strings.Contains(tstLines[k], contains[i]) { // 如果行包含预期片段
				found = true // 标记为找到
				break        // 退出循环
			}
		}

		if !found { // 如果未找到预期片段
			t.Fatalf("Line %s was not found in the result \n%s", contains[i], tst) // 记录错误并终止测试
		}

	}

}

// TestSkip 测试查询的跳过功能
func TestSkip(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store) // 在测试之前插入测试数据
		var result []ItemTest    // 声明一个结果切片

		q := badgerhold.Where("Category").Eq("animal").Or(badgerhold.Where("Name").Eq("fish")) // 构建查询条件

		err := store.Find(&result, q) // 执行查询
		if err != nil {               // 如果查询失败
			t.Fatalf("Error retrieving data for skip test.") // 记录错误并终止测试
		}

		var skipResult []ItemTest // 声明一个跳过后的结果切片
		skip := 5                 // 设置跳过的记录数

		err = store.Find(&skipResult, q.Skip(skip)) // 执行跳过查询
		if err != nil {                             // 如果查询失败
			t.Fatalf("Error retrieving data for skip test on the skip query.") // 记录错误并终止测试
		}

		if len(skipResult) != len(result)-skip { // 检查跳过后的结果数量是否正确
			t.Fatalf("Skip query didn't return the right number of records: Wanted %d got %d",
				(len(result) - skip), len(skipResult)) // 记录错误并终止测试
		}

		// 验证跳过的记录是否正确
		result = result[skip:] // 获取原始结果中跳过后的部分

		for i := range skipResult { // 遍历跳过后的结果
			found := false          // 标记是否找到匹配项
			for k := range result { // 遍历原始结果的跳过部分
				if result[i].equal(&skipResult[k]) { // 如果结果项匹配
					found = true // 标记为找到
					break        // 退出循环
				}
			}

			if !found { // 如果未找到匹配项
				if testing.Verbose() { // 如果测试在详细模式下运行
					t.Fatalf("%v should not be in the result set! Full results: %v",
						result[i], result) // 记录详细错误信息并终止测试
				}
				t.Fatalf("%v should not be in the result set!", result[i]) // 记录简要错误信息并终止测试
			}
		}

	})
}

// TestSkipNegative 测试负跳过值的处理
func TestSkipNegative(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with negative skip did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest                                                  // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").Skip(-30)) // 尝试使用负跳过值进行查询
	})
}

// TestLimitNegative 测试负限制值的处理
func TestLimitNegative(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with negative limit did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest                                                   // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").Limit(-30)) // 尝试使用负限制值进行查询
	})
}

// TestSkipDouble 测试双重跳过的处理
func TestSkipDouble(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with double skips did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest                                                         // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").Skip(30).Skip(3)) // 尝试使用双重跳过进行查询
	})
}

// TestLimitDouble 测试双重限制的处理
func TestLimitDouble(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with double limits did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest                                                           // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").Limit(30).Limit(3)) // 尝试使用双重限制进行查询
	})
}

// TestSkipInOr 测试在 Or 查询中使用跳过的处理
func TestSkipInOr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with skip in or query did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest                                                                                        // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").Or(badgerhold.Where("Name").Eq("blah").Skip(3))) // 尝试在 Or 查询中使用跳过
	})
}

// TestLimitInOr 测试在 Or 查询中使用限制的处理
func TestLimitInOr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running Find with limit in or query did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest                                                                                         // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").Or(badgerhold.Where("Name").Eq("blah").Limit(3))) // 尝试在 Or 查询中使用限制
	})
}

// TestSlicePointerResult 测试使用切片指针作为查询结果的处理
func TestSlicePointerResult(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		count := 10                  // 要插入的测试数据数量
		for i := 0; i < count; i++ { // 循环插入测试数据
			err := store.Insert(i, &ItemTest{
				Key: i,
				ID:  i,
			})
			if err != nil { // 如果插入失败
				t.Fatalf("Error inserting data for Slice Pointer test: %s", err) // 记录错误并终止测试
			}
		}

		var result []*ItemTest          // 声明一个结果切片，元素类型为指针
		err := store.Find(&result, nil) // 查找所有数据

		if err != nil { // 如果查询失败
			t.Fatalf("Error retrieving data for Slice pointer test: %s", err) // 记录错误并终止测试
		}

		if len(result) != count { // 如果结果数量不匹配
			t.Fatalf("Expected %d, got %d", count, len(result)) // 记录错误并终止测试
		}
	})
}

// TestKeyMatchFunc 测试使用 matchFunc 查询 Key 的处理
func TestKeyMatchFunc(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running matchFunc against Key query did not panic!") // 记录错误并终止测试
			}
		}()

		var result []ItemTest // 声明一个结果切片
		_ = store.Find(&result, badgerhold.Where(badgerhold.Key).MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) {
			field := ra.Field()
			_, ok := field.(string)
			if !ok { // 如果字段不是字符串类型
				return false, fmt.Errorf("Field not a string, it's a %T!", field) // 返回错误
			}

			return strings.HasPrefix(field.(string), "oat"), nil // 返回匹配结果
		})) // 尝试对 Key 使用 matchFunc 进行查询
	})
}

// TestKeyStructTag 测试结构体标签作为键的处理
func TestKeyStructTag(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		type KeyTest struct { // 定义一个包含键标签的结构体
			Key   int `badgerholdKey:"Key"`
			Value string
		}

		key := 3 // 定义一个键值

		err := store.Insert(key, &KeyTest{
			Value: "test value",
		}) // 插入测试数据

		if err != nil { // 如果插入失败
			t.Fatalf("Error inserting KeyTest struct for Key struct tag testing. Error: %s", err) // 记录错误并终止测试
		}

		var result []KeyTest // 声明一个结果切片

		err = store.Find(&result, badgerhold.Where(badgerhold.Key).Eq(key)) // 查找插入的数据
		if err != nil {                                                     // 如果查询失败
			t.Fatalf("Error running Find in TestKeyStructTag. ERROR: %s", err) // 记录错误并终止测试
		}

		if result[0].Key != key { // 如果键值不匹配
			t.Fatalf("Key struct tag was not set correctly.  Expected %d, got %d", key, result[0].Key) // 记录错误并终止测试
		}

	})
}

// TestKeyStructTagIntoPtr 测试将结构体标签作为指针键的处理
func TestKeyStructTagIntoPtr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		type KeyTest struct { // 定义一个包含键标签的结构体，键为指针类型
			Key   *int `badgerholdKey:"Key"`
			Value string
		}

		key := 3 // 定义一个键值

		err := store.Insert(&key, &KeyTest{
			Value: "test value",
		}) // 插入测试数据

		if err != nil { // 如果插入失败
			t.Fatalf("Error inserting KeyTest struct for Key struct tag testing. Error: %s", err) // 记录错误并终止测试
		}

		var result []KeyTest // 声明一个结果切片

		err = store.Find(&result, badgerhold.Where(badgerhold.Key).Eq(key)) // 查找插入的数据
		if err != nil {                                                     // 如果查询失败
			t.Fatalf("Error running Find in TestKeyStructTag. ERROR: %s", err) // 记录错误并终止测试
		}

		if *result[0].Key != key { // 如果键值不匹配
			t.Fatalf("Key struct tag was not set correctly.  Expected %d, got %d", key, result[0].Key) // 记录错误并终止测试
		}

	})
}

// TestQueryNestedIndex 测试使用嵌套索引字段的处理
func TestQueryNestedIndex(t *testing.T) {
	defer func() { // 延迟执行代码，以捕获可能的 panic
		if r := recover(); r == nil { // 如果没有发生 panic
			t.Fatalf("Querying with a nested index field did not panic!") // 记录错误并终止测试
		}
	}()

	_ = badgerhold.Where("Test").Eq("test").Index("Nested.Name") // 尝试使用嵌套索引字段进行查询
}

// TestQueryIterKeyCacheOverflow 测试迭代器键缓存溢出的处理
func TestQueryIterKeyCacheOverflow(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t

		type KeyCacheTest struct { // 定义一个用于键缓存测试的结构体
			Key      int
			IndexKey int `badgerholdIndex:"IndexKey"`
		}

		size := 200 // 插入的数据大小
		stop := 10  // 停止的索引

		for i := 0; i < size; i++ { // 循环插入测试数据
			err := store.Insert(i, &KeyCacheTest{
				Key:      i,
				IndexKey: i,
			})
			if err != nil { // 如果插入失败
				t.Fatalf("Error inserting data for key cache test: %s", err) // 记录错误并终止测试
			}
		}

		tests := []*badgerhold.Query{ // 定义多个查询测试
			badgerhold.Where(badgerhold.Key).Gt(stop),
			badgerhold.Where(badgerhold.Key).Gt(stop).Index(badgerhold.Key),
			badgerhold.Where("Key").Gt(stop),
			badgerhold.Where("IndexKey").Gt(stop).Index("IndexKey"),
			badgerhold.Where("IndexKey").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) {
				field := ra.Field()
				_, ok := field.(int)
				if !ok { // 如果字段不是 int 类型
					return false, fmt.Errorf("Field not an int, it's a %T!", field) // 返回错误
				}

				return field.(int) > stop, nil // 返回匹配结果
			}).Index("IndexKey"),
		}

		for i := range tests { // 循环执行每个查询测试
			t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
				var result []KeyCacheTest

				err := store.Find(&result, tests[i]) // 执行查询
				if err != nil {                      // 如果查询失败
					t.Fatalf("Error getting data from badgerhold: %s", err) // 记录错误并终止测试
				}

				for i := stop; i < 10; i++ { // 验证结果
					if i != result[i].Key {
						t.Fatalf("Value is not correct.  Wanted %d, got %d", i, result[i].Key) // 记录错误并终止测试
					}
				}
			})
		}

	})
}

// TestNestedStructPointer 测试嵌套结构体指针的处理
func TestNestedStructPointer(t *testing.T) {

	type notification struct { // 定义一个嵌套结构体
		Enabled bool
	}

	type device struct { // 定义一个包含嵌套结构体指针的结构体
		ID            string `badgerhold:"key"`
		Notifications *notification
	}

	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		id := "1" // 定义设备 ID
		store.Insert(id, &device{
			ID: id,
			Notifications: &notification{
				Enabled: true,
			},
		}) // 插入设备数据

		devices := []*device{}           // 声明设备结果切片
		err := store.Find(&devices, nil) // 查找所有设备数据
		if err != nil {                  // 如果查询失败
			t.Fatalf("Error finding data for nested struct testing: %s", err) // 记录错误并终止测试
		}

		device := &device{}         // 声明一个设备变量
		err = store.Get(id, device) // 根据 ID 获取设备数据
		if err != nil {             // 如果获取失败
			t.Fatalf("Error getting data for nested struct testing: %s", err) // 记录错误并终止测试
		}

		if devices[0].ID != id { // 验证获取的设备 ID
			t.Fatalf("ID Expected %s, got %s", id, devices[0].ID) // 记录错误并终止测试
		}

		if !devices[0].Notifications.Enabled { // 验证通知状态
			t.Fatalf("Notifications.Enabled Expected  %t, got %t", true, devices[0].Notifications.Enabled) // 记录错误并终止测试
		}

		if device.ID != id { // 验证直接获取的设备 ID
			t.Fatalf("ID Expected %s, got %s", id, device.ID) // 记录错误并终止测试
		}

		if !device.Notifications.Enabled { // 验证直接获取的设备通知状态
			t.Fatalf("Notifications.Enabled Expected  %t, got %t", true, device.Notifications.Enabled) // 记录错误并终止测试
		}
	})
}

// TestGetKeyStructTag 测试使用带有 `badgerholdKey` 标签的结构体字段作为键的处理
func TestGetKeyStructTag(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		type KeyTest struct { // 定义一个带有 `badgerholdKey` 标签的结构体
			Key   int    `badgerholdKey:"Key"` // 将 Key 字段标记为存储键
			Value string // 普通值字段
		}

		key := 3 // 定义一个键值

		err := store.Insert(key, &KeyTest{ // 插入一条数据
			Value: "test value", // 设置 Value 字段
		})

		if err != nil { // 如果插入失败
			t.Fatalf("Error inserting KeyTest struct for Key struct tag testing. Error: %s", err) // 记录错误并终止测试
		}

		var result KeyTest            // 定义一个 KeyTest 类型的变量，用于存储查询结果
		err = store.Get(key, &result) // 通过键值获取数据

		if err != nil { // 如果获取失败
			t.Fatalf("Error running Get in TestKeyStructTag. ERROR: %s", err) // 记录错误并终止测试
		}

		if result.Key != key { // 验证获取的键值是否正确
			t.Fatalf("Key struct tag was not set correctly.  Expected %d, got %d", key, result.Key) // 记录错误并终止测试
		}
	})
}

// TestGetKeyStructTagIntoPtr 测试使用指针类型的 `badgerholdKey` 标签的结构体字段作为键的处理
func TestGetKeyStructTagIntoPtr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		type KeyTest struct { // 定义一个带有 `badgerholdKey` 标签的结构体，键为指针类型
			Key   *int   `badgerholdKey:"Key"` // 将 Key 字段标记为存储键
			Value string // 普通值字段
		}

		key := 5 // 定义一个键值

		err := store.Insert(&key, &KeyTest{ // 插入一条数据，键为指针类型
			Value: "test value", // 设置 Value 字段
		})

		if err != nil { // 如果插入失败
			t.Fatalf("Error inserting KeyTest struct for Key struct tag testing. Error: %s", err) // 记录错误并终止测试
		}

		var result KeyTest // 定义一个 KeyTest 类型的变量，用于存储查询结果

		err = store.Get(key, &result) // 通过键值获取数据
		if err != nil {               // 如果获取失败
			t.Fatalf("Error running Get in TestKeyStructTag. ERROR: %s", err) // 记录错误并终止测试
		}

		if result.Key == nil || *result.Key != key { // 验证获取的键值是否正确
			t.Fatalf("Key struct tag was not set correctly.  Expected %d, got %d", key, result.Key) // 记录错误并终止测试
		}
	})
}

// TestFindOne 测试查找单条记录的功能
func TestFindOne(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store)          // 插入测试数据
		for _, tst := range testResults { // 循环遍历所有测试用例
			t.Run(tst.name, func(t *testing.T) { // 为每个测试用例创建一个子测试
				result := &ItemTest{}                   // 定义一个 ItemTest 类型的变量，用于存储查询结果
				err := store.FindOne(result, tst.query) // 查找一条记录

				if len(tst.result) == 0 && err == badgerhold.ErrNotFound { // 如果没有结果且错误是 ErrNotFound，测试通过
					return
				}

				if err != nil { // 如果查询出错
					t.Fatalf("Error finding one data from badgerhold: %s", err) // 记录错误并终止测试
				}

				if !result.equal(&testData[tst.result[0]]) { // 如果查询结果与期望结果不符
					t.Fatalf("Result doesnt match the first record in the testing result set. "+
						"Expected key of %d got %d", &testData[tst.result[0]].Key, result.Key) // 记录错误并终止测试
				}
			})
		}
	})
}

// TestFindOneWithNonPtr 测试在非指针类型参数上使用 FindOne 时的行为
func TestFindOneWithNonPtr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		defer func() { // 延迟执行代码，以捕获可能的 panic
			if r := recover(); r == nil { // 如果没有发生 panic
				t.Fatalf("Running FindOne with non pointer did not panic!") // 记录错误并终止测试
			}
		}()
		result := ItemTest{}                                           // 定义一个非指针类型的结果变量
		_ = store.FindOne(result, badgerhold.Where("Name").Eq("blah")) // 尝试用非指针类型调用 FindOne
	})
}

// TestCount 测试 Count 功能
func TestCount(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store)          // 插入测试数据
		for _, tst := range testResults { // 循环遍历所有测试用例
			t.Run(tst.name, func(t *testing.T) { // 为每个测试用例创建一个子测试
				count, err := store.Count(ItemTest{}, tst.query) // 获取符合查询条件的记录数
				if err != nil {                                  // 如果查询出错
					t.Fatalf("Error counting data from badgerhold: %s", err) // 记录错误并终止测试
				}

				equals(t, uint64(len(tst.result)), count) // 验证查询结果是否与期望值匹配
			})
		}
	})
}

// TestIssue74HasPrefixOnKeys 测试在键上使用 HasPrefix 和 HasSuffix 的功能
func TestIssue74HasPrefixOnKeys(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		type Item struct { // 定义一个结构体用于测试
			ID       string
			Category string `badgerholdIndex:"Category"` // 使用 badgerholdIndex 标签的字段
			Created  time.Time
		}

		id := "test_1_test" // 定义一个 ID 值

		ok(t, store.Insert(id, &Item{ // 插入一条测试数据
			ID: id, // 设置 ID 字段
		}))

		result := &Item{} // 定义一个 Item 类型的变量，用于存储查询结果

		ok(t, store.FindOne(result, badgerhold.Where(badgerhold.Key).HasPrefix("test"))) // 测试 HasPrefix 功能
		ok(t, store.FindOne(result, badgerhold.Where(badgerhold.Key).HasSuffix("test"))) // 测试 HasSuffix 功能
	})
}

// TestFindIndexedWithSort 测试在索引字段上使用排序、跳过和限制的功能
func TestFindIndexedWithSort(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store)          // 插入测试数据
		results := make([]ItemTest, 0, 3) // 定义一个结果切片，用于存储查询结果
		ok(t, store.Find(                 // 执行查询，按 Name 排序，跳过 1 条记录，限制返回 3 条记录
			&results,
			badgerhold.Where("Category").Eq("vehicle").Index("Category").
				SortBy("Name").Skip(1).Limit(3),
		))

		expectedIDs := []int{11, 1, 3}            // 定义期望的结果 ID 列表
		equals(t, len(expectedIDs), len(results)) // 验证结果数量是否匹配

		for i := range results { // 验证结果内容是否匹配
			assert(t, testData[expectedIDs[i]].equal(&results[i]), "incorrect rows returned") // 如果不匹配，记录错误
		}
	})
}

// TestFindIndexedWithResultSliceOfPointers 测试使用指针类型的结果切片进行查询
func TestFindIndexedWithResultSliceOfPointers(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		insertTestData(t, store)           // 插入测试数据
		results := make([]*ItemTest, 0, 3) // 定义一个指针类型的结果切片，用于存储查询结果
		ok(t, store.Find(                  // 执行查询，按 Category 索引进行查询
			&results,
			badgerhold.Where("Category").Eq("vehicle").Index("Category"),
		))

		expectedIDs := []int{0, 1, 3, 6, 11}      // 定义期望的结果 ID 列表
		equals(t, len(expectedIDs), len(results)) // 验证结果数量是否匹配

		for i := range results { // 验证结果内容是否匹配
			assert(t, testData[expectedIDs[i]].equal(results[i]), "incorrect rows returned") // 如果不匹配，记录错误
		}
	})
}

// TestFindWithStorerImplementation 测试使用自定义 Storer 实现进行查询
func TestFindWithStorerImplementation(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		customStorerItem := &ItemWithStorer{Name: "pizza"} // 创建一个自定义 Storer 类型的实例
		ok(t, store.Insert(1, customStorerItem))           // 插入自定义 Storer 类型的实例

		results := make([]ItemWithStorer, 1) // 定义一个结果切片，用于存储查询结果
		ok(t, store.Find(                    // 执行查询，按 Name 索引进行查询
			&results,
			badgerhold.Where("Name").Eq("pizza").Index("Name"),
		))

		equals(t, *customStorerItem, results[0]) // 验证查询结果是否与插入的数据匹配
	})
}

// queryMatchTest 结构体定义，用于测试复杂查询匹配
type queryMatchTest struct {
	Key     int       `badgerholdKey:"Key"` // 定义键字段
	Age     int       // 定义年龄字段
	Color   string    // 定义颜色字段
	Created time.Time // 定义创建时间字段
}

// TestComplexQueryMatch 测试复杂查询匹配逻辑
func TestComplexQueryMatch(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		item := queryMatchTest{ // 创建一个测试数据实例
			Key:     1,                 // 设置键
			Age:     2,                 // 设置年龄
			Color:   "color",           // 设置颜色
			Created: time.UnixMicro(0), // 设置创建时间
		}
		query := badgerhold.Where("Key").Eq(1).And("Age").Eq(3).Or(badgerhold.Where("Key").Eq(2).And("Age").Eq(2)) // 创建一个复杂查询
		if m, err := query.Matches(store, item); m || err != nil {                                                 // 验证查询不匹配
			t.Errorf("wanted %+v to not match %+v, but got %v, %v", query, item, m, err) // 记录错误
		}
		query = badgerhold.Where("Key").Eq(1).And("Age").Eq(3).Or(badgerhold.Where("Key").Eq(1).And("Age").Eq(2)) // 创建另一个复杂查询
		if m, err := query.Matches(store, item); !m || err != nil {                                               // 验证查询匹配
			t.Errorf("wanted %+v to match %+v, but got %v, %v", query, item, m, err) // 记录错误
		}
		query = badgerhold.Where("Key").Eq(1).And("Age").Eq(1).Or(badgerhold.Where("Key").Eq(2).And("Age").Eq(2).Or(badgerhold.Where("Key").Eq(1).And("Age").Eq(2))) // 创建复杂嵌套查询
		if m, err := query.Matches(store, item); !m || err != nil {                                                                                                  // 验证查询匹配
			t.Errorf("wanted %+v to match %+v, but got %v, %v", query, item, m, err) // 记录错误
		}
	})
}

// TestQueryMatch 测试查询匹配逻辑
func TestQueryMatch(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) { // 包裹测试逻辑，提供 store 和 t
		item := queryMatchTest{ // 创建一个测试数据实例
			Key:     1,                 // 设置键
			Age:     2,                 // 设置年龄
			Color:   "color",           // 设置颜色
			Created: time.UnixMicro(0), // 设置创建时间
		}
		for _, tc := range []struct { // 定义多个测试用例
			query     *badgerhold.Query // 查询条件
			wantMatch bool              // 期望匹配结果
			title     string            // 测试标题
		}{
			{
				query:     badgerhold.Where("Key").Eq(1), // 测试单一字段匹配
				wantMatch: true,                          // 期望匹配成功
				title:     "SingleKeyFieldMatch",
			},
			{
				query:     badgerhold.Where("Key").Eq(2), // 测试单一字段不匹配
				wantMatch: false,                         // 期望匹配失败
				title:     "SingleKeyFieldMismatch",
			},
			{
				query:     badgerhold.Where("Age").Eq(2), // 测试单一整数字段匹配
				wantMatch: true,                          // 期望匹配成功
				title:     "SingleIntFieldMatch",
			},
			{
				query:     badgerhold.Where("Age").Eq(3), // 测试单一整数字段不匹配
				wantMatch: false,                         // 期望匹配失败
				title:     "SingleIntFieldMismatch",
			},
			{
				query:     badgerhold.Where("Key").Eq(1).And("Color").Eq("color"), // 测试多字段 AND 匹配
				wantMatch: true,                                                   // 期望匹配成功
				title:     "MultiFieldAndMatch",
			},
			{
				query:     badgerhold.Where("Key").Eq(1).And("Color").Eq("notcolor"), // 测试多字段 AND 不匹配
				wantMatch: false,                                                     // 期望匹配失败
				title:     "MultiFieldAndMismatch",
			},
			{
				query:     badgerhold.Where("Key").Eq(2).Or(badgerhold.Where("Color").Eq("color")), // 测试多字段 OR 匹配
				wantMatch: true,                                                                    // 期望匹配成功
				title:     "MultiFieldOrMatch",
			},
			{
				query:     badgerhold.Where("Key").Eq(2).Or(badgerhold.Where("Color").Eq("notcolor")), // 测试多字段 OR 不匹配
				wantMatch: false,                                                                      // 期望匹配失败
				title:     "MultiFieldOrMismatch",
			},
			{
				query:     badgerhold.Where("Created").Eq(time.UnixMicro(0)), // 测试单一时间字段匹配
				wantMatch: true,                                              // 期望匹配成功
				title:     "SingleTimeFieldMatch",
			},
			{
				query:     badgerhold.Where("Created").Eq(time.UnixMicro(1)), // 测试单一时间字段不匹配
				wantMatch: false,                                             // 期望匹配失败
				title:     "SingleTimeFieldMismatch",
			},
		} {
			t.Run(tc.title+"StructReceiver", func(t *testing.T) { // 使用结构体接收者进行测试
				gotMatch, err := tc.query.Matches(store, item) // 执行匹配查询
				if err != nil {                                // 如果发生错误
					t.Fatal(err) // 记录错误并终止测试
				}
				if gotMatch != tc.wantMatch { // 验证查询结果是否与期望匹配
					t.Errorf("wanted %+v to return %v for %+v, got %v", tc.query, tc.wantMatch, item, gotMatch) // 如果不匹配，记录错误
				}
			})
			t.Run(tc.title+"PtrReceiver", func(t *testing.T) { // 使用指针接收者进行测试
				gotMatch, err := tc.query.Matches(store, &item) // 执行匹配查询
				if err != nil {                                 // 如果发生错误
					t.Fatal(err) // 记录错误并终止测试
				}
				if gotMatch != tc.wantMatch { // 验证查询结果是否与期望匹配
					t.Errorf("wanted %+v to return %v for %+v, got %v", tc.query, tc.wantMatch, &item, gotMatch) // 如果不匹配，记录错误
				}
			})
		}
	})
}
