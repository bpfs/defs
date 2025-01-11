// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试获取单个记录的功能。
// 测试内容：验证 Get 方法的正确性，确保根据键值能够正确获取对应的数据。

package badgerhold_test

import (
	"testing"
	"time"

	"github.com/bpfs/defs/v2/badgerhold"

	"github.com/dgraph-io/badger/v4"
)

// TestGet 测试 Get 方法从存储中获取数据的行为。
func TestGet(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义要插入和获取数据的键
		data := &ItemTest{
			Name:    "Test Name", // 定义测试数据的 Name 字段
			Created: time.Now(),  // 定义测试数据的 Created 字段为当前时间
		}
		// 插入数据到 store 中，如果出错则记录错误并使测试失败
		err := store.Insert(key, data)
		if err != nil {
			t.Fatalf("Error creating data for get test: %s", err)
		}

		result := &ItemTest{} // 定义存储获取结果的变量

		// 从 store 中获取数据，如果出错则记录错误并使测试失败
		err = store.Get(key, result)
		if err != nil {
			t.Fatalf("Error getting data from badgerhold: %s", err)
		}

		// 检查获取到的数据是否与插入的数据相等，如果不相等则记录错误并使测试失败
		if !data.equal(result) {
			t.Fatalf("Got %v wanted %v.", result, data)
		}
	})
}

// TestIssue36 测试不同结构体定义方式对自增键 ID 的影响。
func TestIssue36(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		// 定义多个结构体，使用不同的 tag 定义自增键 ID
		type Tag1 struct {
			ID uint64 `badgerholdKey` // 使用 `badgerholdKey` 定义主键
		}

		type Tag2 struct {
			ID uint64 `badgerholdKey:"Key"` // 使用 `badgerholdKey:"Key"` 定义主键
		}

		type Tag3 struct {
			ID uint64 `badgerhold:"key"` // 使用 `badgerhold:"key"` 定义主键
		}

		type Tag4 struct {
			ID uint64 `badgerholdKey:""` // 使用 `badgerholdKey:""` 定义主键
		}

		// 插入并验证 Tag1 的数据
		data1 := []*Tag1{{}, {}, {}} // 定义数据
		for i := range data1 {
			ok(t, store.Insert(badgerhold.NextSequence(), data1[i])) // 插入数据
			equals(t, uint64(i), data1[i].ID)                        // 验证 ID 是否按预期递增
		}

		// 插入并验证 Tag2 的数据
		data2 := []*Tag2{{}, {}, {}} // 定义数据
		for i := range data2 {
			ok(t, store.Insert(badgerhold.NextSequence(), data2[i])) // 插入数据
			equals(t, uint64(i), data2[i].ID)                        // 验证 ID 是否按预期递增
		}

		// 插入并验证 Tag3 的数据
		data3 := []*Tag3{{}, {}, {}} // 定义数据
		for i := range data3 {
			ok(t, store.Insert(badgerhold.NextSequence(), data3[i])) // 插入数据
			equals(t, uint64(i), data3[i].ID)                        // 验证 ID 是否按预期递增
		}

		// 插入并验证 Tag4 的数据
		data4 := []*Tag4{{}, {}, {}} // 定义数据
		for i := range data4 {
			ok(t, store.Insert(badgerhold.NextSequence(), data4[i])) // 插入数据
			equals(t, uint64(i), data4[i].ID)                        // 验证 ID 是否按预期递增
		}

		// 测试 Get 方法，验证是否能够正确获取数据
		for i := range data1 {
			get1 := &Tag1{}
			ok(t, store.Get(data1[i].ID, get1))
			equals(t, data1[i], get1)
		}

		for i := range data2 {
			get2 := &Tag2{}
			ok(t, store.Get(data2[i].ID, get2))
			equals(t, data2[i], get2)
		}

		for i := range data3 {
			get3 := &Tag3{}
			ok(t, store.Get(data3[i].ID, get3))
			equals(t, data3[i], get3)
		}

		for i := range data4 {
			get4 := &Tag4{}
			ok(t, store.Get(data4[i].ID, get4))
			equals(t, data4[i], get4)
		}

		// 测试 Find 方法，验证是否能够根据 ID 查找到正确的记录
		for i := range data1 {
			var find1 []*Tag1
			ok(t, store.Find(&find1, badgerhold.Where(badgerhold.Key).Eq(data1[i].ID)))
			assert(t, len(find1) == 1, "incorrect rows returned")
			equals(t, find1[0], data1[i])
		}

		for i := range data2 {
			var find2 []*Tag2
			ok(t, store.Find(&find2, badgerhold.Where(badgerhold.Key).Eq(data2[i].ID)))
			assert(t, len(find2) == 1, "incorrect rows returned")
			equals(t, find2[0], data2[i])
		}

		for i := range data3 {
			var find3 []*Tag3
			ok(t, store.Find(&find3, badgerhold.Where(badgerhold.Key).Eq(data3[i].ID)))
			assert(t, len(find3) == 1, "incorrect rows returned")
			equals(t, find3[0], data3[i])
		}

		for i := range data4 {
			var find4 []*Tag4
			ok(t, store.Find(&find4, badgerhold.Where(badgerhold.Key).Eq(data4[i].ID)))
			assert(t, len(find4) == 1, "incorrect rows returned")
			equals(t, find4[0], data4[i])
		}
	})
}

// TestTxGetBadgerError 测试 TxGet 方法在使用已经丢弃的事务时的行为。
func TestTxGetBadgerError(t *testing.T) {
	// 使用 testWrap 包裹测试逻辑，确保每次测试前后环境的一致性。
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		key := "testKey" // 定义要插入和获取数据的键
		data := &ItemTest{
			Name:    "Test Name", // 定义测试数据的 Name 字段
			Created: time.Now(),  // 定义测试数据的 Created 字段为当前时间
		}
		// 插入数据到 store 中，如果出错则记录错误并使测试失败
		err := store.Insert(key, data)
		if err != nil {
			t.Fatalf("Error creating data for TxGet test: %s", err)
		}

		txn := store.Badger().NewTransaction(false) // 创建新的只读事务
		txn.Discard()                               // 丢弃事务

		result := &ItemTest{} // 定义存储获取结果的变量
		// 使用已经丢弃的事务获取数据，期望返回错误
		err = store.TxGet(txn, key, result)
		if err != badger.ErrDiscardedTxn {
			t.Fatalf("TxGet didn't fail! Expected %s got %s", badger.ErrDiscardedTxn, err)
		}
	})
}
