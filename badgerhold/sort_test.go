// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试数据排序功能。
// 测试内容：确保 SortBy 方法能够正确地对查询结果进行排序，并能够处理多字段排序。

package badgerhold_test

import (
	"fmt"
	"testing"

	"github.com/bpfs/defs/badgerhold"
)

// 定义一组用于排序的测试用例
var sortTests = []test{
	{
		name:   "Sort By Name",                                           // 测试按 Name 字段排序
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name"), // 条件是 Category 等于 "animal" 并按 Name 排序
		result: []int{9, 5, 14, 8, 13, 2, 16},                            // 预期的结果顺序
	},
	{
		name:   "Sort By Name Reversed",                                            // 测试按 Name 字段倒序排序
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Reverse(), // 条件是 Category 等于 "animal" 并按 Name 倒序排序
		result: []int{16, 2, 13, 8, 14, 5, 9},                                      // 预期的结果顺序
	},
	{
		name:   "Sort By Multiple Fields",                                      // 测试按多个字段排序
		query:  badgerhold.Where("ID").In(8, 3, 13).SortBy("Category", "Name"), // 条件是 ID 在给定的集合中，并按 Category 和 Name 排序
		result: []int{13, 15, 4, 3},                                            // 预期的结果顺序
	},
	{
		name:   "Sort By Multiple Fields Reversed",                                       // 测试按多个字段倒序排序
		query:  badgerhold.Where("ID").In(8, 3, 13).SortBy("Category", "Name").Reverse(), // 条件是 ID 在给定的集合中，并按 Category 和 Name 倒序排序
		result: []int{3, 4, 15, 13},                                                      // 预期的结果顺序
	},
	{
		name:   "Sort By Duplicate Field Names",                                            // 测试按重复字段名称排序
		query:  badgerhold.Where("ID").In(8, 3, 13).SortBy("Category", "Name", "Category"), // 条件是 ID 在给定的集合中，并按 Category, Name 和 Category 排序
		result: []int{13, 15, 4, 3},                                                        // 预期的结果顺序
	},
	{
		name:   "Sort By Name with limit",                                         // 测试按 Name 排序并限制结果数量
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Limit(3), // 条件是 Category 等于 "animal" 并按 Name 排序且限制结果数量为 3
		result: []int{9, 5, 14},                                                   // 预期的结果顺序
	},
	{
		name:   "Sort By Name with skip",                                         // 测试按 Name 排序并跳过部分结果
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Skip(3), // 条件是 Category 等于 "animal" 并按 Name 排序且跳过前 3 个结果
		result: []int{8, 13, 2, 16},                                              // 预期的结果顺序
	},
	{
		name:   "Sort By Name with skip and limit",                                        // 测试按 Name 排序并跳过部分结果且限制结果数量
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Skip(2).Limit(3), // 条件是 Category 等于 "animal" 并按 Name 排序且跳过前 2 个结果并限制结果数量为 3
		result: []int{14, 8, 13},                                                          // 预期的结果顺序
	},
	{
		name:   "Sort By Name Reversed with limit",                                        // 测试按 Name 倒序排序并限制结果数量
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Skip(2).Limit(3), // 条件是 Category 等于 "animal" 并按 Name 倒序排序且跳过前 2 个结果并限制结果数量为 3
		result: []int{14, 8, 13},                                                          // 预期的结果顺序
	},
	{
		name:   "Sort By Name Reversed with skip",                                // 测试按 Name 倒序排序并跳过部分结果
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Skip(4), // 条件是 Category 等于 "animal" 并按 Name 倒序排序且跳过前 4 个结果
		result: []int{13, 2, 16},                                                 // 预期的结果顺序
	},
	{
		name:   "Sort By Name Reversed with skip and limit",                               // 测试按 Name 倒序排序并跳过部分结果且限制结果数量
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Skip(2).Limit(3), // 条件是 Category 等于 "animal" 并按 Name 倒序排序且跳过前 2 个结果并限制结果数量为 3
		result: []int{14, 8, 13},                                                          // 预期的结果顺序
	},
	{
		name:   "Sort By Name with skip greater than length",                      // 测试按 Name 排序并跳过超过结果数量的项
		query:  badgerhold.Where("Category").Eq("animal").SortBy("Name").Skip(10), // 条件是 Category 等于 "animal" 并按 Name 排序且跳过 10 个结果（超过了结果数量）
		result: []int{},                                                           // 预期的结果为空
	},
}

// TestSortedFind 测试按排序条件进行查找
func TestSortedFind(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		insertTestData(t, store) // 插入测试数据

		for _, tst := range sortTests { // 遍历每个排序测试用例
			t.Run(tst.name, func(t *testing.T) {
				var result []ItemTest
				err := store.Find(&result, tst.query) // 根据测试用例中的查询条件进行查找
				if err != nil {
					t.Fatalf("Error finding sort data from badgerhold: %s", err) // 如果查找失败，记录错误信息并使测试失败
				}
				if len(result) != len(tst.result) { // 检查结果数量是否与预期一致
					if testing.Verbose() {
						t.Fatalf("Sorted Find result count is %d wanted %d.  Results: %v", len(result),
							len(tst.result), result)
					}
					t.Fatalf("Sorted Find result count is %d wanted %d.", len(result), len(tst.result))
				}

				for i := range result { // 检查每个结果项是否与预期一致
					if !result[i].equal(&testData[tst.result[i]]) {
						if testing.Verbose() {
							t.Fatalf("Expected index %d to be %v, Got %v Results: %v", i, &testData[tst.result[i]],
								result[i], result)
						}
						t.Fatalf("Expected index %d to be %v, Got %v", i, &testData[tst.result[i]], result[i])
					}
				}
			})
		}
	})
}

// TestSortedUpdateMatching 测试按排序条件进行匹配更新
func TestSortedUpdateMatching(t *testing.T) {
	for _, tst := range sortTests { // 遍历每个排序测试用例
		t.Run(tst.name, func(t *testing.T) {
			testWrap(t, func(store *badgerhold.Store, t *testing.T) {

				insertTestData(t, store) // 插入测试数据

				err := store.UpdateMatching(&ItemTest{}, tst.query, func(record interface{}) error {
					update, ok := record.(*ItemTest)
					if !ok {
						return fmt.Errorf("Record isn't the correct type!  Wanted ItemTest, got %T", record)
					}

					update.UpdateField = "updated"       // 更新字段值
					update.UpdateIndex = "updated index" // 更新索引值

					return nil
				})

				if err != nil {
					t.Fatalf("Error updating data from badgerhold: %s", err) // 如果更新失败，记录错误信息并使测试失败
				}

				var result []ItemTest
				err = store.Find(&result, badgerhold.Where("UpdateIndex").Eq("updated index").And("UpdateField").Eq("updated"))
				if err != nil {
					t.Fatalf("Error finding result after update from badgerhold: %s", err) // 如果查找失败，记录错误信息并使测试失败
				}

				if len(result) != len(tst.result) { // 检查更新后的结果数量是否与预期一致
					if testing.Verbose() {
						t.Fatalf("Find result count after update is %d wanted %d.  Results: %v",
							len(result), len(tst.result), result)
					}
					t.Fatalf("Find result count after update is %d wanted %d.", len(result),
						len(tst.result))
				}

				for i := range result { // 检查每个更新后的结果项是否与预期一致
					found := false
					for k := range tst.result {
						if result[i].Key == testData[tst.result[k]].Key &&
							result[i].UpdateField == "updated" &&
							result[i].UpdateIndex == "updated index" {
							found = true
							break
						}
					}

					if !found {
						if testing.Verbose() {
							t.Fatalf("Could not find %v in the update result set! Full results: %v",
								result[i], result)
						}
						t.Fatalf("Could not find %v in the updated result set!", result[i])
					}
				}

			})

		})
	}
}

// TestSortedDeleteMatching 测试按排序条件进行匹配删除
func TestSortedDeleteMatching(t *testing.T) {
	for _, tst := range sortTests { // 遍历每个排序测试用例
		t.Run(tst.name, func(t *testing.T) {
			testWrap(t, func(store *badgerhold.Store, t *testing.T) {

				insertTestData(t, store) // 插入测试数据

				err := store.DeleteMatching(&ItemTest{}, tst.query)
				if err != nil {
					t.Fatalf("Error deleting data from badgerhold: %s", err) // 如果删除失败，记录错误信息并使测试失败
				}

				var result []ItemTest
				err = store.Find(&result, nil)
				if err != nil {
					t.Fatalf("Error finding result after delete from badgerhold: %s", err) // 如果查找失败，记录错误信息并使测试失败
				}

				if len(result) != (len(testData) - len(tst.result)) { // 检查删除后的结果数量是否与预期一致
					if testing.Verbose() {
						t.Fatalf("Delete result count is %d wanted %d.  Results: %v", len(result),
							(len(testData) - len(tst.result)), result)
					}
					t.Fatalf("Delete result count is %d wanted %d.", len(result),
						(len(testData) - len(tst.result)))

				}

				for i := range result { // 检查删除后的结果中是否仍然包含已删除的项
					found := false
					for k := range tst.result {
						if result[i].equal(&testData[tst.result[k]]) {
							found = true
							break
						}
					}

					if found {
						if testing.Verbose() {
							t.Fatalf("Found %v in the result set when it should've been deleted! Full results: %v", result[i], result)
						}
						t.Fatalf("Found %v in the result set when it should've been deleted!", result[i])
					}
				}

			})

		})
	}
}

// TestSortOnKey 测试在 Key 字段上进行排序是否会触发 panic
func TestSortOnKey(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Sort on Key field did not panic!")
			}
		}()

		var result []ItemTest
		_ = store.Find(&result, badgerhold.Where("Name").Eq("blah").SortBy(badgerhold.Key)) // 试图对 Key 字段排序，期望触发 panic
	})
}

// TestSortedFindOnInvalidFieldName 测试在无效字段名称上进行排序是否会返回错误
func TestSortedFindOnInvalidFieldName(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		insertTestData(t, store) // 插入测试数据
		var result []ItemTest

		err := store.Find(&result, badgerhold.Where("BadFieldName").Eq("test").SortBy("BadFieldName")) // 使用无效字段名进行查找和排序
		if err == nil {
			t.Fatalf("Sorted find query against a bad field name didn't return an error!") // 期望返回错误，但未返回
		}

	})
}

// TestSortedFindWithNonSlicePtr 测试使用非切片指针进行查找是否会触发 panic
func TestSortedFindWithNonSlicePtr(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Find with non-slice pointer did not panic!")
			}
		}()
		var result []ItemTest
		_ = store.Find(result, badgerhold.Where("Name").Eq("blah").SortBy("Name")) // 试图使用非切片指针进行查找，期望触发 panic
	})
}
