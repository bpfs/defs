// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试插入和更新操作。
// 测试内容：验证 Put 方法的正确性，该方法在不存在记录时插入新记录，存在时更新记录。

package badgerhold_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bpfs/defs/badgerhold"

	"github.com/dgraph-io/badger/v4"
)

// TestInsert 测试插入操作
func TestInsert(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义用于插入的键
		data := &ItemTest{
			Name:     "Test Name",     // 定义插入的数据项名称
			Category: "Test Category", // 定义插入的数据项类别
			Created:  time.Now(),      // 定义插入数据的创建时间
		}

		err := store.Insert(key, data) // 执行插入操作
		if err != nil {
			t.Fatalf("Error inserting data for test: %s", err) // 插入失败时记录错误并使测试失败
		}

		result := &ItemTest{} // 用于存储查询结果的变量

		err = store.Get(key, result) // 查询插入的数据
		if err != nil {
			t.Fatalf("Error getting data from badgerhold: %s", err) // 查询失败时记录错误并使测试失败
		}

		if !data.equal(result) { // 检查查询结果是否与插入的数据一致
			t.Fatalf("Got %v wanted %v.", result, data)
		}

		// 测试重复插入
		err = store.Insert(key, &ItemTest{
			Name:    "Test Name", // 定义重复插入的数据项名称
			Created: time.Now(),  // 定义重复插入的数据项创建时间
		})

		if err != badgerhold.ErrKeyExists { // 检查重复插入是否返回预期的错误
			t.Fatalf("Insert didn't fail! Expected %s got %s", badgerhold.ErrKeyExists, err)
		}

	})
}

// TestInsertReadTxn 测试在只读事务中插入数据
func TestInsertReadTxn(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义用于插入的键
		data := &ItemTest{
			Name:     "Test Name",     // 定义插入的数据项名称
			Category: "Test Category", // 定义插入的数据项类别
			Created:  time.Now(),      // 定义插入数据的创建时间
		}

		// 尝试在只读事务中插入数据
		err := store.Badger().View(func(tx *badger.Txn) error {
			return store.TxInsert(tx, key, data)
		})

		if err == nil { // 检查在只读事务中插入是否触发预期的错误
			t.Fatalf("Inserting into a read only transaction didn't fail!")
		}

	})
}

// TestUpdate 测试更新操作
func TestUpdate(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义用于更新的键
		data := &ItemTest{
			Name:     "Test Name",     // 定义插入的数据项名称
			Category: "Test Category", // 定义插入的数据项类别
			Created:  time.Now(),      // 定义插入数据的创建时间
		}

		// 尝试更新未插入的数据
		err := store.Update(key, data)
		if err != badgerhold.ErrNotFound { // 检查未插入的数据更新是否返回预期的错误
			t.Fatalf("Update without insert didn't fail! Expected %s got %s", badgerhold.ErrNotFound, err)
		}

		// 插入数据以进行后续更新操作
		err = store.Insert(key, data)
		if err != nil {
			t.Fatalf("Error creating data for update test: %s", err) // 插入失败时记录错误并使测试失败
		}

		result := &ItemTest{} // 用于存储查询结果的变量

		err = store.Get(key, result) // 查询插入的数据
		if err != nil {
			t.Fatalf("Error getting data from badgerhold: %s", err) // 查询失败时记录错误并使测试失败
		}

		if !data.equal(result) { // 检查查询结果是否与插入的数据一致
			t.Fatalf("Got %v wanted %v.", result, data)
		}

		// 定义更新后的数据
		update := &ItemTest{
			Name:     "Test Name Updated",     // 更新后的数据项名称
			Category: "Test Category Updated", // 更新后的数据项类别
			Created:  time.Now(),              // 更新后的数据创建时间
		}

		// 执行更新操作
		err = store.Update(key, update)

		if err != nil {
			t.Fatalf("Error updating data: %s", err) // 更新失败时记录错误并使测试失败
		}

		err = store.Get(key, result) // 查询更新后的数据
		if err != nil {
			t.Fatalf("Error getting data from badgerhold: %s", err) // 查询失败时记录错误并使测试失败
		}

		if !result.equal(update) { // 检查更新是否成功
			t.Fatalf("Update didn't complete.  Expected %v, got %v", update, result)
		}

	})
}

// TestUpdateReadTxn 测试在只读事务中更新数据
func TestUpdateReadTxn(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义用于更新的键
		data := &ItemTest{
			Name:     "Test Name",     // 定义更新的数据项名称
			Category: "Test Category", // 定义更新的数据项类别
			Created:  time.Now(),      // 定义更新数据的创建时间
		}

		// 尝试在只读事务中更新数据
		err := store.Badger().View(func(tx *badger.Txn) error {
			return store.TxUpdate(tx, key, data)
		})

		if err == nil { // 检查在只读事务中更新是否触发预期的错误
			t.Fatalf("Updating into a read only transaction didn't fail!")
		}

	})
}

// TestUpsert 测试插入或更新操作
func TestUpsert(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义用于插入或更新的键
		data := &ItemTest{
			Name:     "Test Name",     // 定义插入的数据项名称
			Category: "Test Category", // 定义插入的数据项类别
			Created:  time.Now(),      // 定义插入数据的创建时间
		}

		// 执行插入或更新操作
		err := store.Upsert(key, data)
		if err != nil {
			t.Fatalf("Error upserting data: %s", err) // 插入或更新失败时记录错误并使测试失败
		}

		result := &ItemTest{} // 用于存储查询结果的变量

		err = store.Get(key, result) // 查询插入的数据
		if err != nil {
			t.Fatalf("Error getting data from badgerhold: %s", err) // 查询失败时记录错误并使测试失败
		}

		if !data.equal(result) { // 检查查询结果是否与插入的数据一致
			t.Fatalf("Got %v wanted %v.", result, data)
		}

		// 定义更新后的数据
		update := &ItemTest{
			Name:     "Test Name Updated",     // 更新后的数据项名称
			Category: "Test Category Updated", // 更新后的数据项类别
			Created:  time.Now(),              // 更新后的数据创建时间
		}

		// 再次执行插入或更新操作
		err = store.Upsert(key, update)

		if err != nil {
			t.Fatalf("Error updating data: %s", err) // 插入或更新失败时记录错误并使测试失败
		}

		err = store.Get(key, result) // 查询更新后的数据
		if err != nil {
			t.Fatalf("Error getting data from badgerhold: %s", err) // 查询失败时记录错误并使测试失败
		}

		if !result.equal(update) { // 检查插入或更新是否成功
			t.Fatalf("Upsert didn't complete.  Expected %v, got %v", update, result)
		}
	})
}

// TestUpsertReadTxn 测试在只读事务中执行 Upsert 操作时是否会产生错误。
func TestUpsertReadTxn(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保测试环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey"   // 定义一个键 "testKey"。
		data := &ItemTest{ // 创建一个 ItemTest 对象，包含测试数据。
			Name:     "Test Name",
			Category: "Test Category",
			Created:  time.Now(),
		}

		// 尝试在一个只读事务中执行 Upsert 操作，预期该操作会失败。
		err := store.Badger().View(func(tx *badger.Txn) error {
			return store.TxUpsert(tx, key, data) // 在只读事务中尝试 Upsert 操作。
		})

		// 检查是否未产生错误，如果没有错误，测试失败，因为在只读事务中不应允许 Upsert 操作。
		if err == nil {
			t.Fatalf("Updating into a read only transaction didn't fail!")
		}
	})
}

// TestUpdateMatching 测试使用 UpdateMatching 方法根据查询条件更新数据。
func TestUpdateMatching(t *testing.T) {
	// 遍历所有测试用例。
	for _, tst := range testResults {
		t.Run(tst.name, func(t *testing.T) {
			// 使用 testWrap 包裹测试逻辑。
			testWrap(t, func(store *badgerhold.Store, t *testing.T) {
				// 插入测试数据。
				insertTestData(t, store)

				// 调用 UpdateMatching 方法，根据查询条件更新数据。
				err := store.UpdateMatching(&ItemTest{}, tst.query, func(record interface{}) error {
					update, ok := record.(*ItemTest)
					if !ok { // 检查数据类型是否匹配。
						return fmt.Errorf("Record isn't the correct type!  Wanted Itemtest, got %T",
							record)
					}

					// 更新字段。
					update.UpdateField = "updated"
					update.UpdateIndex = "updated index"

					return nil
				})

				// 如果更新操作出错，测试失败。
				if err != nil {
					t.Fatalf("Error updating data from badgerhold: %s", err)
				}

				var result []ItemTest
				// 查找更新后的数据，验证更新是否成功。
				err = store.Find(&result, badgerhold.Where("UpdateIndex").Eq("updated index").
					And("UpdateField").Eq("updated"))
				if err != nil {
					t.Fatalf("Error finding result after update from badgerhold: %s", err)
				}

				// 检查更新后的数据条数是否与预期相符。
				if len(result) != len(tst.result) {
					if testing.Verbose() {
						t.Fatalf("Find result count after update is %d wanted %d.  Results: %v",
							len(result), len(tst.result), result)
					}
					t.Fatalf("Find result count after update is %d wanted %d.", len(result),
						len(tst.result))
				}

				// 验证每一条更新后的数据是否符合预期。
				for i := range result {
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

// TestIssue14 测试更新数据后，旧索引是否仍然存在。
func TestIssue14(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey"   // 定义一个键 "testKey"。
		data := &ItemTest{ // 创建一个包含测试数据的 ItemTest 对象。
			Name:     "Test Name",
			Category: "Test Category",
			Created:  time.Now(),
		}
		// 插入测试数据。
		err := store.Insert(key, data)
		if err != nil {
			t.Fatalf("Error creating data for update test: %s", err)
		}

		// 创建一个更新后的 ItemTest 对象。
		update := &ItemTest{
			Name:     "Test Name Updated",
			Category: "Test Category Updated",
			Created:  time.Now(),
		}

		// 执行更新操作。
		err = store.Update(key, update)
		if err != nil {
			t.Fatalf("Error updating data: %s", err)
		}

		var result []ItemTest
		// 尝试根据旧索引查找记录。
		err = store.Find(&result, badgerhold.Where("Category").Eq("Test Category"))
		if err != nil {
			t.Fatalf("Error retrieving query result for TestIssue14: %s", err)
		}

		// 验证旧索引是否已被删除。
		if len(result) != 0 {
			t.Fatalf("Old index still exists after update.  Expected %d got %d!", 0, len(result))
		}
	})
}

// TestIssue14Upsert 测试 Upsert 操作后，旧索引是否仍然存在。
func TestIssue14Upsert(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey"   // 定义一个键 "testKey"。
		data := &ItemTest{ // 创建一个包含测试数据的 ItemTest 对象。
			Name:     "Test Name",
			Category: "Test Category",
			Created:  time.Now(),
		}
		// 插入测试数据。
		err := store.Insert(key, data)
		if err != nil {
			t.Fatalf("Error creating data for update test: %s", err)
		}

		// 创建一个更新后的 ItemTest 对象。
		update := &ItemTest{
			Name:     "Test Name Updated",
			Category: "Test Category Updated",
			Created:  time.Now(),
		}

		// 执行 Upsert 操作。
		err = store.Upsert(key, update)
		if err != nil {
			t.Fatalf("Error updating data: %s", err)
		}

		var result []ItemTest
		// 尝试根据旧索引查找记录。
		err = store.Find(&result, badgerhold.Where("Category").Eq("Test Category"))
		if err != nil {
			t.Fatalf("Error retrieving query result for TestIssue14: %s", err)
		}

		// 验证旧索引是否已被删除。
		if len(result) != 0 {
			t.Fatalf("Old index still exists after update.  Expected %d got %d!", 0, len(result))
		}
	})
}

// TestIssue14UpdateMatching 测试使用 UpdateMatching 操作后，旧索引是否仍然存在。
func TestIssue14UpdateMatching(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey"   // 定义一个键 "testKey"。
		data := &ItemTest{ // 创建一个包含测试数据的 ItemTest 对象。
			Name:     "Test Name",
			Category: "Test Category",
			Created:  time.Now(),
		}
		// 插入测试数据。
		err := store.Insert(key, data)
		if err != nil {
			t.Fatalf("Error creating data for update test: %s", err)
		}

		// 执行 UpdateMatching 操作，根据 "Name" 字段更新数据。
		err = store.UpdateMatching(&ItemTest{}, badgerhold.Where("Name").Eq("Test Name"),
			func(record interface{}) error {
				update, ok := record.(*ItemTest)
				if !ok { // 检查数据类型是否正确。
					return fmt.Errorf("Record isn't the correct type!  Wanted Itemtest, got %T", record)
				}

				// 更新 Category 字段。
				update.Category = "Test Category Updated"

				return nil
			})

		if err != nil {
			t.Fatalf("Error updating data: %s", err)
		}

		var result []ItemTest
		// 尝试根据旧索引查找记录。
		err = store.Find(&result, badgerhold.Where("Category").Eq("Test Category"))
		if err != nil {
			t.Fatalf("Error retrieving query result for TestIssue14: %s", err)
		}

		// 验证旧索引是否已被删除。
		if len(result) != 0 {
			t.Fatalf("Old index still exists after update.  Expected %d got %d!", 0, len(result))
		}
	})
}

// TestInsertSequence 测试使用自动递增序列作为键进行数据插入的功能。
func TestInsertSequence(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保测试环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {

		// 定义一个带有 badgerholdKey 标记的结构体，用于测试自动递增序列作为键的功能。
		type SequenceTest struct {
			Key uint64 `badgerholdKey:"Key"` // 定义 Key 字段为 badgerholdKey 类型，使用自动生成的序列值。
		}

		// 插入 10 条数据，键为自动递增的序列值。
		for i := 0; i < 10; i++ {
			err := store.Insert(badgerhold.NextSequence(), &SequenceTest{})
			if err != nil {
				// 如果插入失败，记录错误并使测试失败。
				t.Fatalf("Error inserting data for sequence test: %s", err)
			}
		}

		var result []SequenceTest // 用于存储查询结果。

		// 查询所有插入的数据。
		err := store.Find(&result, nil)
		if err != nil {
			// 如果查询失败，记录错误并使测试失败。
			t.Fatalf("Error getting data from badgerhold: %s", err)
		}

		// 检查每条数据的键是否按预期顺序排列。
		for i := 0; i < 10; i++ {
			if i != int(result[i].Key) {
				// 如果键不匹配预期值，记录错误并使测试失败。
				t.Fatalf("Sequence is not correct.  Wanted %d, got %d", i, result[i].Key)
			}
		}
	})
}

// TestInsertSequenceSetKey 测试在插入时设置键值的功能，确保键字段能够正确设置。
func TestInsertSequenceSetKey(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保测试环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {

		// 定义一个结构体，键字段使用 badgerholdKey 标记，类型为 uint64。
		type InsertSequenceSetKeyTest struct {
			Key uint64 `badgerholdKey:"Key"` // 键字段，使用自动生成的序列值。
		}

		// 插入 10 条数据，并检查键字段是否正确设置。
		for i := 0; i < 10; i++ {
			st := InsertSequenceSetKeyTest{} // 创建一个新的结构体实例。
			if st.Key != 0 {
				// 如果键的初始值不是 0，测试失败。
				t.Fatalf("Zero value of test data should be 0")
			}
			err := store.Insert(badgerhold.NextSequence(), &st)
			if err != nil {
				// 如果插入失败，记录错误并使测试失败。
				t.Fatalf("Error inserting data for sequence test: %s", err)
			}
			if int(st.Key) != i {
				// 如果插入后键值不匹配预期值，测试失败。
				t.Fatalf("Inserted data's key field was not updated as expected.  Wanted %d, got %d",
					i, st.Key)
			}
		}
	})
}

// TestInsertSetKey 测试在插入时手动设置键值的功能，检查不同情况下的行为。
func TestInsertSetKey(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保测试环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {

		// 定义一个结构体，键字段使用 badgerholdKey 标记，类型为 uint。
		type TestInsertSetKey struct {
			Key uint `badgerholdKey:"Key"` // 键字段，类型为 uint。
		}

		// 测试用例 1: 有效的键设置。
		t.Run("Valid", func(t *testing.T) {
			st := TestInsertSetKey{} // 创建一个新的结构体实例。
			key := uint(123)         // 定义一个 uint 类型的键值。
			err := store.Insert(key, &st)
			if err != nil {
				// 如果插入失败，记录错误并使测试失败。
				t.Fatalf("Error inserting data for key set test: %s", err)
			}
			if st.Key != key {
				// 如果键字段未正确设置，测试失败。
				t.Fatalf("Key was not set.  Wanted %d, got %d", key, st.Key)
			}
		})

		// 测试用例 2: 使用值传递而非引用传递，键字段无法设置。
		t.Run("NotSettable", func(t *testing.T) {
			st := TestInsertSetKey{}     // 创建一个新的结构体实例。
			key := uint(234)             // 定义一个 uint 类型的键值。
			err := store.Insert(key, st) // 使用值传递插入数据。
			if err != nil {
				// 如果插入失败，记录错误并使测试失败。
				t.Fatalf("Error inserting data for key set test: %s", err)
			}
			if st.Key != 0 {
				// 如果键字段被错误设置，测试失败。
				t.Fatalf("Key was set incorrectly.  Wanted %d, got %d", 0, st.Key)
			}
		})

		// 测试用例 3: 键字段非零值的情况。
		t.Run("NonZero", func(t *testing.T) {
			key := uint(456)               // 定义一个 uint 类型的键值。
			st := TestInsertSetKey{424242} // 创建一个键字段非零的结构体实例。
			err := store.Insert(key, &st)
			if err != nil {
				// 如果插入失败，记录错误并使测试失败。
				t.Fatalf("Error inserting data for key set test: %s", err)
			}
			if st.Key != 424242 {
				// 如果键字段未正确保留，测试失败。
				t.Fatalf("Key was set incorrectly.  Wanted %d, got %d", 424242, st.Key)
			}
		})

		// 测试用例 4: 键类型不匹配的情况。
		t.Run("TypeMismatch", func(t *testing.T) {
			key := int(789)               // 定义一个 int 类型的键值。
			st := TestInsertSetKey{}      // 创建一个新的结构体实例。
			err := store.Insert(key, &st) // 尝试插入类型不匹配的键。
			if err != nil {
				// 如果插入失败，记录错误并使测试失败。
				t.Fatalf("Error inserting data for key set test: %s", err)
			}
			if st.Key != 0 {
				// 如果键字段被错误设置，测试失败。
				t.Fatalf("Key was not set.  Wanted %d, got %d", 0, st.Key)
			}
		})
	})
}

// TestAlternateTags 测试在使用自定义标记（tag）时的插入和查询功能。
func TestAlternateTags(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保测试环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		type TestAlternate struct {
			Key  uint64 `badgerhold:"key"`   // 使用自定义标记 "key" 作为键字段。
			Name string `badgerhold:"index"` // 使用自定义标记 "index" 作为索引字段。
		}
		item := TestAlternate{
			Name: "TestName", // 设置 Name 字段为 "TestName"。
		}

		key := uint64(123) // 定义一个 uint64 类型的键值。
		err := store.Insert(key, &item)
		if err != nil {
			// 如果插入失败，记录错误并使测试失败。
			t.Fatalf("Error inserting data for alternate tag test: %s", err)
		}

		// 检查插入后的键字段是否正确设置。
		if item.Key != key {
			t.Fatalf("Key was not set.  Wanted %d, got %d", key, item.Key)
		}

		var result []TestAlternate // 用于存储查询结果。

		// 使用自定义索引字段 "Name" 进行查询。
		err = store.Find(&result, badgerhold.Where("Name").Eq(item.Name).Index("Name"))
		if err != nil {
			// 如果查询失败，记录错误并使测试失败。
			t.Fatalf("Query on alternate tag index failed: %s", err)
		}

		// 检查查询结果是否为 1 条记录。
		if len(result) != 1 {
			t.Fatalf("Expected 1 got %d", len(result))
		}
	})
}

// TestUniqueConstraint 测试在 badgerhold 中唯一约束的功能。
func TestUniqueConstraint(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保测试环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 定义一个结构体，包含一个带唯一约束的 Name 字段。
		type TestUnique struct {
			Key  uint64 `badgerhold:"key"`    // 键字段，使用自动生成的序列值。
			Name string `badgerhold:"unique"` // Name 字段，使用唯一约束。
		}

		item := &TestUnique{
			Name: "Tester Name", // 设置 Name 字段为 "Tester Name"。
		}

		// 插入一条记录，用于后续的唯一性测试。
		err := store.Insert(badgerhold.NextSequence(), item)
		if err != nil {
			// 如果插入失败，记录错误并使测试失败。
			t.Fatalf("Error inserting base record for unique testing: %s", err)
		}

		// 测试插入重复记录时是否触发唯一性约束。
		t.Run("Insert", func(t *testing.T) {
			err = store.Insert(badgerhold.NextSequence(), item)
			if err != badgerhold.ErrUniqueExists {
				t.Fatalf("Inserting duplicate record did not result in a unique constraint error: "+
					"Expected %s, Got %s", badgerhold.ErrUniqueExists, err)
			}
		})

		// 测试更新记录时是否触发唯一性约束。
		t.Run("Update", func(t *testing.T) {
			update := &TestUnique{
				Name: "Update Name", // 设置更新后的 Name 字段为 "Update Name"。
			}
			err = store.Insert(badgerhold.NextSequence(), update)
			if err != nil {
				t.Fatalf("Inserting record for update Unique testing failed: %s", err)
			}

			// 尝试将更新后的记录的 Name 字段改为已存在的唯一值。
			update.Name = item.Name

			err = store.Update(update.Key, update)
			if err != badgerhold.ErrUniqueExists {
				t.Fatalf("Duplicate record did not result in a unique constraint error: "+
					"Expected %s, Got %s", badgerhold.ErrUniqueExists, err)
			}
		})

		// 测试 Upsert 操作是否触发唯一性约束。
		t.Run("Upsert", func(t *testing.T) {
			update := &TestUnique{
				Name: "Upsert Name", // 设置更新后的 Name 字段为 "Upsert Name"。
			}
			err = store.Insert(badgerhold.NextSequence(), update)
			if err != nil {
				t.Fatalf("Inserting record for upsert Unique testing failed: %s", err)
			}

			// 尝试将 Upsert 的记录的 Name 字段改为已存在的唯一值。
			update.Name = item.Name

			err = store.Upsert(update.Key, update)
			if err != badgerhold.ErrUniqueExists {
				t.Fatalf("Duplicate record did not result in a unique constraint error: "+
					"Expected %s, Got %s", badgerhold.ErrUniqueExists, err)
			}
		})

		// 测试 UpdateMatching 操作是否触发唯一性约束。
		t.Run("UpdateMatching", func(t *testing.T) {
			update := &TestUnique{
				Name: "UpdateMatching Name", // 设置更新后的 Name 字段为 "UpdateMatching Name"。
			}
			err = store.Insert(badgerhold.NextSequence(), update)
			if err != nil {
				t.Fatalf("Inserting record for updatematching Unique testing failed: %s", err)
			}

			// 尝试通过 UpdateMatching 操作将记录的 Name 字段改为已存在的唯一值。
			err = store.UpdateMatching(TestUnique{}, badgerhold.Where(badgerhold.Key).Eq(update.Key),
				func(r interface{}) error {
					record, ok := r.(*TestUnique)
					if !ok {
						return fmt.Errorf("Record isn't the correct type!  Got %T", r)
					}

					record.Name = item.Name // 将 Name 字段改为已存在的唯一值。

					return nil
				})
			if err != badgerhold.ErrUniqueExists {
				t.Fatalf("Duplicate record did not result in a unique constraint error: "+
					"Expected %s, Got %s", badgerhold.ErrUniqueExists, err)
			}
		})

		// 测试删除记录后再插入相同的记录是否成功。
		t.Run("Delete", func(t *testing.T) {
			err = store.Delete(item.Key, TestUnique{})
			if err != nil {
				t.Fatalf("Error deleting record for unique testing %s", err)
			}

			err = store.Insert(badgerhold.NextSequence(), item)
			if err != nil {
				t.Fatalf("Error inserting duplicate record that has been previously removed: %s", err)
			}
		})
	})
}

// TestIssue46ConcurrentIndexInserts 测试并发插入操作下的索引行为。
func TestIssue46ConcurrentIndexInserts(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		var wg sync.WaitGroup // 创建 WaitGroup 用于管理并发操作。

		// 对每个测试数据项进行并发插入操作。
		for i := range testData {
			wg.Add(1)
			go func(gt *testing.T, i int) {
				defer wg.Done()
				err := store.Insert(testData[i].Key, testData[i])
				if err != nil {
					gt.Fatalf("Error inserting test data for find test: %s", err)
				}
			}(t, i)
		}
		wg.Wait() // 等待所有并发操作完成。
	})
}

// TestIssue46ConcurrentIndexUpserts 测试并发 Upsert 操作下的索引行为。
func TestIssue46ConcurrentIndexUpserts(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		var wg sync.WaitGroup // 创建 WaitGroup 用于管理并发操作。

		// 对每个测试数据项进行并发 Upsert 操作。
		for i := range testData {
			wg.Add(1)
			go func(gt *testing.T, i int) {
				defer wg.Done()
				err := store.Upsert(testData[i].Key, testData[i])
				if err != nil {
					gt.Fatalf("Error inserting test data for find test: %s", err)
				}
			}(t, i)
		}
		wg.Wait() // 等待所有并发操作完成。
	})
}

// TestIssue46ConcurrentIndexUpdate 测试并发更新操作下的索引行为。
func TestIssue46ConcurrentIndexUpdate(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		insertTestData(t, store)
		var wg sync.WaitGroup // 创建 WaitGroup 用于管理并发操作。

		// 对每个测试数据项进行并发更新操作。
		for i := range testData {
			wg.Add(1)
			go func(gt *testing.T, i int) {
				defer wg.Done()
				err := store.Update(testData[i].Key, testData[i])
				if err != nil {
					gt.Fatalf("Error inserting test data for find test: %s", err)
				}
			}(t, i)
		}
		wg.Wait() // 等待所有并发操作完成。
	})
}

// TestIssue46ConcurrentIndexUpdateMatching 测试并发 UpdateMatching 操作下的索引行为。
func TestIssue46ConcurrentIndexUpdateMatching(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		insertTestData(t, store)
		var wg sync.WaitGroup // 创建 WaitGroup 用于管理并发操作。

		// 对每个测试数据项进行并发 UpdateMatching 操作。
		for i := range testData {
			wg.Add(1)
			go func(gt *testing.T, i int) {
				defer wg.Done()
				err := store.UpdateMatching(ItemTest{}, badgerhold.Where(badgerhold.Key).
					Eq(testData[i].Key), func(r interface{}) error {
					record, ok := r.(*ItemTest)
					if !ok {
						return fmt.Errorf("Record isn't the correct type!  Got %T", r)
					}
					record.Name = record.Name // 模拟更新操作，但不改变数据内容。

					return nil
				})
				if err != nil {
					gt.Fatalf("Error updating test data for test: %s", err)
				}
			}(t, i)
		}
		wg.Wait() // 等待所有并发操作完成。
	})
}

// CustomStorer 自定义数据结构，用于测试插入或更新时的索引错误处理。
type CustomStorer struct{ Name string }

// Type 返回自定义数据类型的名称。
func (i *CustomStorer) Type() string { return "CustomStorer" }

// Indexes 返回自定义数据结构的索引配置。
func (i *CustomStorer) Indexes() map[string]badgerhold.Index {
	return map[string]badgerhold.Index{
		"Name": {
			IndexFunc: func(_ string, value interface{}) ([]byte, error) {
				// 模拟索引函数错误，返回一个错误。
				return nil, errors.New("IndexFunc error")
			},
			Unique: false, // 索引是否唯一，设置为 false。
		},
	}
}

// TestInsertUpdateIndexError 测试在插入或更新时处理索引函数错误的行为。
func TestInsertUpdateIndexError(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		data := &CustomStorer{
			Name: "Test Name", // 设置 Name 字段为 "Test Name"。
		}
		err := store.Insert("testKey", data)
		if err == nil || err.Error() != "IndexFunc error" {
			// 如果插入操作没有失败或错误消息不匹配，记录错误并使测试失败。
			t.Fatalf("Insert didn't fail! Expected IndexFunc error got %s", err)
		}
	})
}
