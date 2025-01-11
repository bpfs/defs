// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：提供项目功能的示例测试，通常用于生成文档中的示例代码。
// 测试内容：展示如何使用 badgerhold 的基本功能，包括插入、查询、删除等操作。

package badgerhold_test

import (
	"fmt"
	"os"
	"time"

	"github.com/bpfs/defs/badgerhold"
	logging "github.com/dep2p/log"
	"github.com/dgraph-io/badger/v4"
)

var logger = logging.Logger("badgerhold_test")

// Item 结构体表示一个项，其中包含 ID、Category（类别）和 Created（创建时间）。
// Category 字段通过 `badgerholdIndex` 标签在 badgerhold 中建立索引。
type Item struct {
	ID       int       // 项的唯一标识符
	Category string    `badgerholdIndex:"Category"` // 项的类别，使用 badgerholdIndex 建立索引
	Created  time.Time // 项的创建时间
}

// Example 函数展示了如何使用 badgerhold 插入数据并查询最近一个小时内创建的指定类别的项。
func Example() {
	// 定义一组待插入的数据
	data := []Item{
		{
			ID:       0,
			Category: "blue",                         // 设置类别为 blue
			Created:  time.Now().Add(-4 * time.Hour), // 设置创建时间为当前时间的 4 小时前
		},
		{
			ID:       1,
			Category: "red",                          // 设置类别为 red
			Created:  time.Now().Add(-3 * time.Hour), // 设置创建时间为当前时间的 3 小时前
		},
		{
			ID:       2,
			Category: "blue",                         // 设置类别为 blue
			Created:  time.Now().Add(-2 * time.Hour), // 设置创建时间为当前时间的 2 小时前
		},
		{
			ID:       3,
			Category: "blue",                            // 设置类别为 blue
			Created:  time.Now().Add(-20 * time.Minute), // 设置创建时间为当前时间的 20 分钟前
		},
	}

	// 创建一个临时目录，用于存储 badgerhold 数据库文件
	dir := tempdir()
	defer os.RemoveAll(dir) // 在函数结束时删除该临时目录

	// 配置 badgerhold 数据库选项
	options := badgerhold.DefaultOptions
	options.Dir = dir                      // 设置数据库目录
	options.ValueDir = dir                 // 设置值存储目录
	store, err := badgerhold.Open(options) // 打开 badgerhold 数据库
	defer store.Close()                    // 在函数结束时关闭数据库

	if err != nil {
		// 如果打开数据库失败，处理错误
		logger.Fatal(err)
	}

	// 在一个事务中插入数据
	err = store.Badger().Update(func(tx *badger.Txn) error {
		for i := range data { // 遍历每个数据项
			err := store.TxInsert(tx, data[i].ID, data[i]) // 将数据插入数据库
			if err != nil {
				return err // 如果插入失败，返回错误
			}
		}
		return nil // 事务成功完成，返回 nil
	})

	if err != nil {
		// 如果插入数据失败，处理错误
		logger.Fatal(err)
	}

	// 查询最近一个小时内创建的类别为 "blue" 的所有项
	var result []Item

	err = store.Find(&result, badgerhold.Where("Category").Eq("blue").And("Created").Ge(time.Now().UTC().Add(-1*time.Hour)))

	if err != nil {
		// 如果查询失败，处理错误
		logger.Fatal(err)
	}

	// 输出查询到的第一个结果的 ID
	fmt.Println("1", result[0].ID)
	// Output: 3
}
