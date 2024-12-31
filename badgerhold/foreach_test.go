// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试 ForEach 方法，遍历数据库中的每一个记录并执行指定操作。
// 测试内容：确保 ForEach 方法能够正确遍历记录并执行给定的回调函数。

package badgerhold_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/bpfs/defs/badgerhold"
)

// TestForEach 测试 ForEach 方法在遍历查询结果时的行为。
func TestForEach(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		insertTestData(t, store) // 插入测试数据

		// 遍历每个测试用例
		for _, tst := range testResults {
			t.Run(tst.name, func(t *testing.T) {
				count := 0 // 初始化计数器
				// 使用 ForEach 方法遍历查询结果
				err := store.ForEach(tst.query, func(record *ItemTest) error {
					count++ // 每遍历到一个结果，计数器加1

					found := false
					// 检查当前记录是否在预期结果中
					for i := range tst.result {
						if record.equal(&testData[tst.result[i]]) {
							found = true
							break
						}
					}

					// 如果当前记录不在预期结果中，返回错误
					if !found {
						if testing.Verbose() {
							return fmt.Errorf("%v was not found in the result set! Full results: %v",
								record, tst.result)
						}
						return fmt.Errorf("%v was not found in the result set!", record)
					}

					return nil // 返回 nil 表示处理正常
				})
				// 检查遍历的记录数是否与预期结果一致
				if count != len(tst.result) {
					t.Fatalf("ForEach count is %d wanted %d.", count, len(tst.result))
				}
				// 如果在遍历过程中出现错误，则记录错误并使测试失败
				if err != nil {
					t.Fatalf("Error during ForEach iteration: %s", err)
				}
			})
		}
	})
}

// TestIssue105ForEachKeys 测试 ForEach 方法在处理带有自增键的结构体时的行为。
func TestIssue105ForEachKeys(t *testing.T) {
	// 定义一个 Person 结构体，带有自增键 ID
	type Person struct {
		ID     uint64 `badgerhold:"key"` // 定义 ID 为主键
		Name   string
		Gender string
		Birth  time.Time
	}

	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 插入第一条记录，ID 应为 0
		data := &Person{Name: "tester1"}
		ok(t, store.Insert(badgerhold.NextSequence(), data))
		equals(t, uint64(0), data.ID)

		// 插入第二条记录，ID 应为 1
		data = &Person{Name: "tester2"}
		ok(t, store.Insert(badgerhold.NextSequence(), data))
		equals(t, uint64(1), data.ID)

		// 插入第三条记录，ID 应为 2
		data = &Person{Name: "tester3"}
		ok(t, store.Insert(badgerhold.NextSequence(), data))
		equals(t, uint64(2), data.ID)

		var id uint64 = 0 // 初始化 ID 计数器

		// 使用 ForEach 方法遍历查询结果，检查每个记录的 ID 是否按顺序递增
		ok(t, store.ForEach(nil, func(record *Person) error {
			assert(t, id == record.ID, record.Name+" incorrectly set key") // 检查 ID 是否正确
			id++                                                           // 递增 ID 计数器
			return nil                                                     // 返回 nil 表示处理正常
		}))
	})
}
