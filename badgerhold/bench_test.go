// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：进行性能基准测试，用于评估 badgerhold 在不同操作下的性能表现。
// 测试内容：测试插入、查询、更新等操作的执行效率，比较使用索引和不使用索引时的性能差异。

package badgerhold_test

import (
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/bpfs/defs/badgerhold"

	"github.com/dgraph-io/badger/v4"
)

// BenchData 定义一个包含 ID 和 Category 字段的结构体，用于基准测试。
type BenchData struct {
	ID       int    // ID 是一个整数类型的字段，用于存储项的标识符。
	Category string // Category 是一个字符串类型的字段，用于存储项的类别。
}

// BenchDataIndexed 定义一个带有索引的结构体，用于基准测试，包含 ID 和带索引的 Category 字段。
type BenchDataIndexed struct {
	ID       int    // ID 是一个整数类型的字段，用于存储项的标识符。
	Category string `badgerholdIndex:"Category"` // Category 是一个字符串类型的字段，带有 badgerhold 索引。
}

// benchItem 是一个 BenchData 类型的变量，用于基准测试。
var benchItem = BenchData{
	ID:       30,              // ID 设置为 30。
	Category: "test category", // Category 设置为 "test category"。
}

// benchItemIndexed 是一个 BenchDataIndexed 类型的变量，用于基准测试。
var benchItemIndexed = BenchDataIndexed{
	ID:       30,              // ID 设置为 30。
	Category: "test category", // Category 设置为 "test category"。
}

// benchWrap 创建一个临时数据库用于测试，并在完成时关闭并清理。
func benchWrap(b *testing.B, options *badgerhold.Options, bench func(store *badgerhold.Store, b *testing.B)) {
	// 创建临时目录用于存储测试数据。
	tempDir, err := ioutil.TempDir("", "badgerhold_tests")
	if err != nil {
		b.Fatalf("Error opening %s: %s", tempDir, err)
	}
	// 在函数返回时移除临时目录，确保不会留下测试数据。
	defer os.RemoveAll(tempDir)

	// 如果没有提供选项，则使用默认选项。
	if options == nil {
		options = &badgerhold.DefaultOptions
	}

	// 设置数据库路径。
	options.Dir = tempDir
	options.ValueDir = tempDir

	// 打开数据库。
	store, err := badgerhold.Open(*options)
	if err != nil {
		b.Fatalf("Error opening %s: %s", tempDir, err)
	}

	// 在函数返回时关闭数据库。
	defer store.Close()

	// 确保数据库实例不为空。
	if store == nil {
		b.Fatalf("store is null!")
	}

	// 执行传入的基准测试函数。
	bench(store, b)
}

// idVal 是一个用于生成唯一 ID 的计数器。
var idVal uint64

// id 生成一个唯一的 ID。
func id() []byte {
	idVal++                              // 增加计数器。
	b := make([]byte, 8)                 // 创建一个长度为 8 的字节数组。
	binary.BigEndian.PutUint64(b, idVal) // 将计数器的值转换为大端序列，并存储在字节数组中。
	return b                             // 返回生成的 ID。
}

// BenchmarkRawInsert 基准测试直接插入编码后的数据。
func BenchmarkRawInsert(b *testing.B) {
	// 将 benchItem 编码为字节数组。
	data, err := badgerhold.DefaultEncode(benchItem)
	if err != nil {
		b.Fatalf("Error encoding data for raw benchmarking: %s", err)
	}

	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			// 在事务中插入数据。
			err = store.Badger().Update(func(tx *badger.Txn) error {
				return tx.Set(id(), data)
			})
			if err != nil {
				b.Fatalf("Error inserting raw bytes: %s", err)
			}
		}
	})
}

// BenchmarkNoIndexInsert 基准测试没有索引的插入操作。
func BenchmarkNoIndexInsert(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			// 插入数据到数据库中。
			err := store.Insert(id(), benchItem)
			if err != nil {
				b.Fatalf("Error inserting into store: %s", err)
			}
		}
	})
}

// BenchmarkIndexedInsert 基准测试带索引的插入操作。
func BenchmarkIndexedInsert(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			// 插入带索引的数据到数据库中。
			err := store.Insert(id(), benchItemIndexed)
			if err != nil {
				b.Fatalf("Error inserting into store: %s", err)
			}
		}
	})
}

// BenchmarkNoIndexUpsert 基准测试没有索引的更新插入操作。
func BenchmarkNoIndexUpsert(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			// 更新插入数据到数据库中。
			err := store.Upsert(id(), benchItem)
			if err != nil {
				b.Fatalf("Error inserting into store: %s", err)
			}
		}
	})
}

// BenchmarkIndexedUpsert 基准测试带索引的更新插入操作。
func BenchmarkIndexedUpsert(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			// 更新插入带索引的数据到数据库中。
			err := store.Upsert(id(), benchItemIndexed)
			if err != nil {
				b.Fatalf("Error inserting into store: %s", err)
			}
		}
	})
}

// BenchmarkNoIndexInsertJSON 基准测试使用 JSON 编码器的没有索引的插入操作。
func BenchmarkNoIndexInsertJSON(b *testing.B) {
	opt := badgerhold.DefaultOptions // 获取默认选项。
	opt.Encoder = json.Marshal       // 设置编码器为 JSON 编码器。
	opt.Decoder = json.Unmarshal     // 设置解码器为 JSON 解码器。

	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, &opt, func(store *badgerhold.Store, b *testing.B) {
		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			// 插入数据到数据库中。
			err := store.Insert(id(), benchItem)
			if err != nil {
				b.Fatalf("Error inserting into store: %s", err)
			}
		}
	})
}

// BenchmarkFindNoIndex 基准测试没有索引的查找操作。
func BenchmarkFindNoIndex(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		// 插入数据到数据库中用于查找测试。
		for i := 0; i < 3; i++ {
			for k := 0; k < 100; k++ {
				err := store.Insert(id(), benchItem)
				if err != nil {
					b.Fatalf("Error inserting benchmarking data: %s", err)
				}
			}
			err := store.Insert(id(), &BenchData{
				ID:       30,
				Category: "findCategory",
			})
			if err != nil {
				b.Fatalf("Error inserting benchmarking data: %s", err)
			}
		}

		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			var result []BenchData

			// 查找数据，基于 "Category" 字段。
			err := store.Find(&result, badgerhold.Where("Category").Eq("findCategory"))
			if err != nil {
				b.Fatalf("Error finding data in store: %s", err)
			}
		}
	})
}

// BenchmarkFindIndexed 基准测试带索引的查找操作。
func BenchmarkFindIndexed(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		// 插入数据到数据库中用于查找测试。
		for i := 0; i < 3; i++ {
			for k := 0; k < 100; k++ {
				err := store.Insert(id(), benchItemIndexed)
				if err != nil {
					b.Fatalf("Error inserting benchmarking data: %s", err)
				}
			}
			err := store.Insert(id(), &BenchDataIndexed{
				ID:       30,
				Category: "findCategory",
			})
			if err != nil {
				b.Fatalf("Error inserting benchmarking data: %s", err)
			}
		}

		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			var result []BenchDataIndexed

			// 查找带索引的数据，基于 "Category" 字段。
			err := store.Find(
				&result,
				badgerhold.Where("Category").Eq("findCategory").Index("Category"),
			)
			if err != nil {
				b.Fatalf("Error finding data in store: %s", err)
			}
		}
	})
}

// BenchmarkFindIndexedWithManyIndexValues 基准测试带多个索引值的查找操作。
func BenchmarkFindIndexedWithManyIndexValues(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		// 插入数据到数据库中用于查找测试。
		for i := 0; i < 3; i++ {
			for k := 0; k < 100; k++ {
				itemID := int(idVal) + 1
				item := BenchDataIndexed{
					ID:       itemID,
					Category: strconv.Itoa(itemID),
				}
				err := store.Insert(id(), item)
				if err != nil {
					b.Fatalf("Error inserting benchmarking data: %s", err)
				}
			}
			err := store.Insert(id(), &BenchDataIndexed{
				ID:       30,
				Category: "findCategory",
			})
			if err != nil {
				b.Fatalf("Error inserting benchmarking data: %s", err)
			}
		}

		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			var result []BenchDataIndexed

			// 查找带索引的数据，基于 "Category" 字段。
			err := store.Find(
				&result,
				badgerhold.Where("Category").Eq("findCategory").Index("Category"),
			)
			if err != nil {
				b.Fatalf("Error finding data in store: %s", err)
			}
		}
	})
}

// BenchmarkFindIndexedWithAdditionalCriteria 基准测试带附加条件的索引查找操作。
func BenchmarkFindIndexedWithAdditionalCriteria(b *testing.B) {
	// 使用 benchWrap 函数包装基准测试逻辑。
	benchWrap(b, nil, func(store *badgerhold.Store, b *testing.B) {
		// 插入数据到数据库中用于查找测试。
		for i := 0; i < 3; i++ {
			for k := 0; k < 100; k++ {
				itemID := int(idVal) + 1
				item := BenchDataIndexed{
					ID:       itemID,
					Category: strconv.Itoa(itemID),
				}
				err := store.Insert(id(), item)
				if err != nil {
					b.Fatalf("Error inserting benchmarking data: %s", err)
				}
			}
			err := store.Insert(id(), &BenchDataIndexed{
				ID:       30,
				Category: "findCategory",
			})
			if err != nil {
				b.Fatalf("Error inserting benchmarking data: %s", err)
			}
		}

		b.ResetTimer() // 重置基准测试计时器。

		for i := 0; i < b.N; i++ {
			var result []BenchDataIndexed

			// 查找带索引的数据，基于 "Category" 字段，并附加条件 "ID" 等于 30。
			err := store.Find(
				&result,
				badgerhold.Where("Category").In("findCategory").Index("Category").
					And("ID").Eq(30),
			)
			if err != nil {
				b.Fatalf("Error finding data in store: %s", err)
			}
		}
	})
}
