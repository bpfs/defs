// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试 badgerhold 中对数据进行聚合操作的功能。聚合操作通常包括对数据进行分组、统计、计算等操作，例如 SUM、AVG、COUNT 等。
// 测试内容：验证 FindAggregate 方法的正确性，确保能够正确地对数据进行分组和聚合。

package badgerhold_test

import (
	"fmt"
	"testing"

	"github.com/bpfs/defs/badgerhold"
)

// TestFindAggregateGroup 测试使用 FindAggregate 方法根据 "Category" 字段对数据进行分组和聚合操作。
func TestFindAggregateGroup(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, err := store.FindAggregate(&ItemTest{}, nil, "Category")

		// 如果在调用 FindAggregate 时出现错误，则记录错误信息并使测试失败。
		if err != nil {
			t.Fatalf("Error finding aggregate data from badgerhold: %s", err)
		}

		// 检查返回的分组数量是否等于预期的 3 个分组，如果不相符，则测试失败。
		if len(result) != 3 {
			t.Fatalf("Wrong number of groupings.  Wanted %d got %d", 3, len(result))
		}

		// 遍历每个分组，检查分组内的所有项是否正确属于该分组。
		for i := range result {
			var items []ItemTest // 用于存储分组内的项
			var group string     // 用于存储当前分组的名称

			// 从当前分组中提取项和分组名称。
			result[i].Reduction(&items)
			result[i].Group(&group)

			// 检查每个项的类别是否与分组名称一致。
			for j := range items {
				if items[j].Category != group {
					t.Fatalf("Reduction item is not in the proper grouping.  Wanted %s, Got %s",
						group, items[j].Category)
				}
			}
		}

		// 测试 min / max / count / avg / sum 操作。
		for i := range result {
			min := &ItemTest{} // 用于存储最小值
			max := &ItemTest{} // 用于存储最大值

			var group string // 用于存储当前分组的名称

			// 从当前分组中提取分组名称。
			result[i].Group(&group)

			// 计算当前分组中 "ID" 字段的最小值和最大值。
			result[i].Min("ID", min)
			result[i].Max("ID", max)

			// 计算当前分组中 "ID" 字段的平均值和总和。
			avg := result[i].Avg("ID")
			sum := result[i].Sum("ID")

			// 根据分组名称判断并检查计算结果是否正确。
			switch group {
			case "animal":
				// 检查 "animal" 分组的最小值是否正确。
				if !min.equal(&testData[2]) {
					t.Fatalf("Expected animal min value of %v Got %v", testData[2], min)
				}
				// 检查 "animal" 分组的最大值是否正确。
				if !max.equal(&testData[14]) {
					t.Fatalf("Expected animal max value of %v Got %v", testData[14], max)
				}

				// 检查 "animal" 分组的计数是否正确。
				if result[i].Count() != 7 {
					t.Fatalf("Expected animal count of %d got %d", 7, result[i].Count())
				}

				// 检查 "animal" 分组的平均值是否正确。
				if avg != 6.142857142857143 {
					t.Fatalf("Expected animal AVG of %v got %v", 6.142857142857143, avg)
				}

				// 检查 "animal" 分组的总和是否正确。
				if sum != 43 {
					t.Fatalf("Expected animal SUM of %v got %v", 43, sum)
				}

			case "food":
				// 检查 "food" 分组的最小值是否正确。
				if !min.equal(&testData[7]) {
					t.Fatalf("Expected food min value of %v Got %v", testData[7], min)
				}
				// 检查 "food" 分组的最大值是否正确。
				if !max.equal(&testData[15]) {
					t.Fatalf("Expected food max value of %v Got %v", testData[15], max)
				}

				// 检查 "food" 分组的计数是否正确。
				if result[i].Count() != 5 {
					t.Fatalf("Expected food count of %d got %d", 5, result[i].Count())
				}

				// 检查 "food" 分组的平均值是否正确。
				if avg != 9.2 {
					t.Fatalf("Expected food AVG of %v got %v", 9.2, avg)
				}

				// 检查 "food" 分组的总和是否正确。
				if sum != 46 {
					t.Fatalf("Expected food SUM of %v got %v", 46, sum)
				}

			case "vehicle":
				// 检查 "vehicle" 分组的最小值是否正确。
				if !min.equal(&testData[0]) {
					t.Fatalf("Expected vehicle min value of %v Got %v", testData[0], min)
				}
				// 检查 "vehicle" 分组的最大值是否正确。
				if !max.equal(&testData[11]) {
					t.Fatalf("Expected vehicle max value of %v Got %v", testData[11], max)
				}

				// 检查 "vehicle" 分组的计数是否正确。
				if result[i].Count() != 5 {
					t.Fatalf("Expected vehicle count of %d got %d", 5, result[i].Count())
				}

				// 检查 "vehicle" 分组的平均值是否正确。
				if avg != 3.8 {
					t.Fatalf("Expected vehicle AVG of %v got %v", 3.8, avg)
				}

				// 检查 "vehicle" 分组的总和是否正确。
				if sum != 19 {
					t.Fatalf("Expected vehicle SUM of %v got %v", 19, sum)
				}
			default:
				// 如果分组名称未在上述 case 中处理，记录错误信息并使测试失败。
				t.Fatalf(fmt.Sprintf("Unaccounted for grouping: %s", group))
			}
		}

	})
}

// TestFindAggregateMultipleGrouping 测试使用 FindAggregate 方法根据 "Category" 和 "Color" 字段对数据进行多重分组和聚合操作。
func TestFindAggregateMultipleGrouping(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 调用 FindAggregate 方法，根据 "Category" 和 "Color" 字段对数据进行多重分组。
		result, err := store.FindAggregate(&ItemTest{}, nil, "Category", "Color")

		// 如果在调用 FindAggregate 时出现错误，则记录错误信息并使测试失败。
		if err != nil {
			t.Fatalf("Error finding aggregate data from badgerhold: %s", err)
		}

		// 检查返回的分组数量是否等于预期的 7 个分组，如果不相符，则测试失败。
		if len(result) != 7 {
			t.Fatalf("Wrong number of groupings.  Wanted %d got %d", 7, len(result))
		}

		// 遍历每个分组，检查分组内的所有项是否正确属于该分组。
		for i := range result {
			var items []*ItemTest // 用于存储分组内的项
			var category string   // 用于存储当前分组的类别
			var color string      // 用于存储当前分组的颜色

			// 从当前分组中提取项和分组的类别、颜色。
			result[i].Reduction(&items)
			result[i].Group(&category, &color)

			// 检查每个项的类别和颜色是否与分组名称一致。
			for j := range items {
				if items[j].Category != category || items[j].Color != color {
					t.Fatalf("Reduction item is not in the proper grouping.  Wanted %s - %s, Got %s - %s",
						category, color, items[j].Category, items[j].Color)
				}
			}
		}

		// 测试 min / max / count / avg / sum 操作。
		for i := range result {
			min := &ItemTest{} // 用于存储最小值
			max := &ItemTest{} // 用于存储最大值

			var category string // 用于存储当前分组的类别
			var color string    // 用于存储当前分组的颜色

			// 从当前分组中提取分组的类别和颜色。
			result[i].Group(&category, &color)

			// 计算当前分组中 "ID" 字段的最小值和最大值。
			result[i].Min("ID", min)
			result[i].Max("ID", max)

			// 计算当前分组中 "ID" 字段的平均值和总和。
			avg := result[i].Avg("ID")
			sum := result[i].Sum("ID")

			// 根据分组名称判断并检查计算结果是否正确。
			group := category + "-" + color

			switch group {
			case "animal-":
				if !min.equal(&testData[2]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[2], min)
				}
				if !max.equal(&testData[14]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[14], max)
				}

				if result[i].Count() != 6 {
					t.Fatalf("Expected %s count of %d got %d", group, 6, result[i].Count())
				}

				if avg != 7 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 7, avg)
				}

				if sum != 42 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 42, sum)
				}
			case "animal-blue":
				if !min.equal(&testData[5]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[5], min)
				}
				if !max.equal(&testData[5]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[5], max)
				}

				if result[i].Count() != 1 {
					t.Fatalf("Expected %s count of %d got %d", group, 1, result[i].Count())
				}

				if avg != 1 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 1, avg)
				}

				if sum != 1 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 1, sum)
				}
			case "food-":
				if !min.equal(&testData[7]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[7], min)
				}
				if !max.equal(&testData[15]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[15], max)
				}

				if result[i].Count() != 4 {
					t.Fatalf("Expected %s count of %d got %d", group, 4, result[i].Count())
				}

				if avg != 9.25 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 9.25, avg)
				}

				if sum != 37 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 37, sum)
				}
			case "food-orange":
				if !min.equal(&testData[10]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[10], min)
				}
				if !max.equal(&testData[10]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[10], max)
				}

				if result[i].Count() != 1 {
					t.Fatalf("Expected %s count of %d got %d", group, 1, result[i].Count())
				}

				if avg != 9 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 9, avg)
				}

				if sum != 9 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 9, sum)
				}
			case "vehicle-":
				if !min.equal(&testData[0]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[0], min)
				}
				if !max.equal(&testData[3]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[3], max)
				}

				if result[i].Count() != 3 {
					t.Fatalf("Expected %s count of %d got %d", group, 3, result[i].Count())
				}

				if avg != 1.3333333333333333 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 1.3333333333333333, avg)
				}

				if sum != 4 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 4, sum)
				}
			case "vehicle-orange":
				if !min.equal(&testData[6]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[6], min)
				}
				if !max.equal(&testData[6]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[6], max)
				}

				if result[i].Count() != 1 {
					t.Fatalf("Expected %s count of %d got %d", group, 1, result[i].Count())
				}

				if avg != 5 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 5, avg)
				}

				if sum != 5 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 5, sum)
				}
			case "vehicle-pink":
				if !min.equal(&testData[11]) {
					t.Fatalf("Expected %s min value of %v Got %v", group, testData[11], min)
				}
				if !max.equal(&testData[11]) {
					t.Fatalf("Expected %s max value of %v Got %v", group, testData[11], max)
				}

				if result[i].Count() != 1 {
					t.Fatalf("Expected %s count of %d got %d", group, 1, result[i].Count())
				}

				if avg != 10 {
					t.Fatalf("Expected %s AVG of %v got %v", group, 10, avg)
				}

				if sum != 10 {
					t.Fatalf("Expected %s SUM of %v got %v", group, 10, sum)
				}
			default:
				t.Fatalf(fmt.Sprintf("Unaccounted for grouping: %s", group))

			}
		}
	})
}

// TestFindAggregateGroupPointerPanic 测试在没有指针传递给 Group 方法时，是否会引发 panic。
func TestFindAggregateGroupPointerPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running group without a pointer did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		group := ""

		// 尝试在没有指针的情况下调用 Group 方法，应引发 panic。
		result[0].Group(group)
	})
}

// TestFindAggregateGroupLenPanic 测试在传递错误数量的分组字段时，是否会引发 panic。
func TestFindAggregateGroupLenPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running group with wrong number of groupings did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		group := ""
		group2 := ""

		// 尝试在传递错误数量的分组字段时调用 Group 方法，应引发 panic。
		result[0].Group(&group, &group2)
	})
}

// TestFindAggregateReductionPointerPanic 测试在没有指针传递给 Reduction 方法时，是否会引发 panic。
func TestFindAggregateReductionPointerPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Reduction without a pointer did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		var items []ItemTest

		// 尝试在没有指针的情况下调用 Reduction 方法，应引发 panic。
		result[0].Reduction(items)
	})
}

// TestFindAggregateSortInvalidFieldPanic 测试在对不存在的字段进行排序时，是否会引发 panic。
func TestFindAggregateSortInvalidFieldPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Sort on a non-existent field did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		// 尝试对不存在的字段进行排序，应引发 panic。
		result[0].Sort("BadField")
	})
}

// TestFindAggregateSortLowerFieldPanic 测试在对小写字段进行排序时，是否会引发 panic。
func TestFindAggregateSortLowerFieldPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Sort on a lower case field did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		// 尝试对小写字段进行排序，应引发 panic。
		result[0].Sort("category")
	})
}

// TestFindAggregateMaxPointerPanic 测试在没有指针传递给 Max 方法时，是否会引发 panic。
func TestFindAggregateMaxPointerPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Max without a pointer did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		var items ItemTest

		// 尝试在没有指针的情况下调用 Max 方法，应引发 panic。
		result[0].Max("Category", items)
	})
}

// TestFindAggregateMaxPointerNilPanic 测试在传递 nil 指针给 Max 方法时，是否会引发 panic。
func TestFindAggregateMaxPointerNilPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Max on a nil pointer did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		var items *ItemTest

		// 尝试在传递 nil 指针的情况下调用 Max 方法，应引发 panic。
		result[0].Max("Category", items)
	})
}

// TestFindAggregateMinPointerPanic 测试在没有指针传递给 Min 方法时，是否会引发 panic。
func TestFindAggregateMinPointerPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Min without a pointer did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		var items ItemTest

		// 尝试在没有指针的情况下调用 Min 方法，应引发 panic。
		result[0].Min("Category", items)
	})
}

// TestFindAggregateMinPointerNilPanic 测试在传递 nil 指针给 Min 方法时，是否会引发 panic。
func TestFindAggregateMinPointerNilPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Min on a nil pointer did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		var items *ItemTest

		// 尝试在传递 nil 指针的情况下调用 Min 方法，应引发 panic。
		result[0].Min("Category", items)
	})
}

// TestFindAggregateBadSumFieldPanic 测试在对错误的字段进行 Sum 操作时，是否会引发 panic。
func TestFindAggregateBadSumFieldPanic(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 延迟处理 recover 以捕获 panic，如果没有发生 panic 则测试失败。
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Running Sum on a bad field did not panic!")
			}
		}()

		// 调用 FindAggregate 方法，根据 "Category" 字段对数据进行分组。
		result, _ := store.FindAggregate(&ItemTest{}, nil, "Category")

		// 尝试对不存在的字段进行 Sum 操作，应引发 panic。
		result[0].Sum("BadField")
	})
}

// TestFindAggregateBadGroupField 测试在使用错误的字段进行分组时，是否会返回错误。
func TestFindAggregateBadGroupField(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 尝试根据不存在的字段进行分组，应返回错误。
		_, err := store.FindAggregate(&ItemTest{}, nil, "BadField")
		if err == nil {
			t.Fatalf("FindAggregate didn't fail when grouped by a bad field.")
		}
	})
}

// TestFindAggregateWithNoResult 测试在查询无结果时，是否会正确处理。
func TestFindAggregateWithNoResult(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 进行查询，期望查询无结果。
		result, err := store.FindAggregate(&ItemTest{}, badgerhold.Where("Name").
			Eq("Never going to match on this"), "Category")
		if err != nil {
			t.Fatalf("FindAggregate failed when the query produced no results")
		}

		// 检查结果是否为空。
		if len(result) != 0 {
			t.Fatalf("Incorrect result.  Wanted 0 got %d", len(result))
		}
	})
}

// TestFindAggregateWithNoGroupBy 测试在没有分组字段时，是否会正确处理。
func TestFindAggregateWithNoGroupBy(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入测试数据到 store 中。
		insertTestData(t, store)

		// 进行无分组字段的查询，应返回一个分组结果。
		result, err := store.FindAggregate(&ItemTest{}, nil)
		if err != nil {
			t.Fatalf("FindAggregate failed when there was no groupBy ")
		}

		// 检查结果是否只有一个分组。
		if len(result) != 1 {
			t.Fatalf("Incorrect result.  Wanted 1 got %d", len(result))
		}
	})
}
