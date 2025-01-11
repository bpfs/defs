// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// 目的：测试与数据库连接和操作相关的功能。
// 测试内容：包括数据库的打开与关闭、存储选项的设置、以及与底层 BadgerDB 的交互。

package badgerhold_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/bpfs/defs/v2/badgerhold"
)

// TestOpen 测试打开和关闭数据库存储的行为。
func TestOpen(t *testing.T) {
	opt := testOptions()               // 获取测试配置选项
	store, err := badgerhold.Open(opt) // 打开数据库存储
	if err != nil {
		t.Fatalf("Error opening %s: %s", opt.Dir, err) // 如果打开数据库失败，记录错误信息并使测试失败
	}

	if store == nil {
		t.Fatalf("store is null!") // 检查 store 是否为空，若为空则测试失败
	}

	err = store.Close() // 关闭数据库存储
	if err != nil {
		t.Fatal(err) // 如果关闭数据库失败，记录错误信息并使测试失败
	}
	err = os.RemoveAll(opt.Dir) // 删除数据库文件夹
	if err != nil {
		t.Fatal(err) // 如果删除文件夹失败，记录错误信息并使测试失败
	}
}

// TestBadger 测试从 badgerhold.Store 中获取 Badger 实例。
func TestBadger(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		b := store.Badger() // 获取 Badger 实例
		if b == nil {
			t.Fatalf("Badger is null in badgerhold") // 如果获取的 Badger 实例为空，则测试失败
		}
	})
}

// TestAlternateEncoding 测试使用不同的编码器和解码器。
func TestAlternateEncoding(t *testing.T) {
	opt := testOptions()               // 获取测试配置选项
	opt.Encoder = json.Marshal         // 使用 JSON 作为编码器
	opt.Decoder = json.Unmarshal       // 使用 JSON 作为解码器
	store, err := badgerhold.Open(opt) // 打开数据库存储

	if err != nil {
		t.Fatalf("Error opening %s: %s", opt.Dir, err) // 如果打开数据库失败，记录错误信息并使测试失败
	}

	defer os.RemoveAll(opt.Dir) // 在测试结束后删除数据库文件夹
	defer store.Close()         // 在测试结束后关闭数据库存储

	insertTestData(t, store) // 插入测试数据到数据库

	tData := testData[3] // 获取测试数据

	var result []ItemTest

	store.Find(&result, badgerhold.Where(badgerhold.Key).Eq(tData.Key)) // 查找测试数据

	if len(result) != 1 {
		if testing.Verbose() {
			t.Fatalf("Find result count is %d wanted %d.  Results: %v", len(result), 1, result) // 检查返回的结果数量是否正确
		}
		t.Fatalf("Find result count is %d wanted %d.", len(result), 1) // 如果结果数量不正确，测试失败
	}

	if !result[0].equal(&tData) {
		t.Fatalf("Results not equal! Wanted %v, got %v", tData, result[0]) // 检查返回的结果是否与预期相符
	}
}

// TestGetUnknownType 测试获取一个未知类型的数据。
func TestGetUnknownType(t *testing.T) {
	opt := testOptions()               // 获取测试配置选项
	store, err := badgerhold.Open(opt) // 打开数据库存储
	if err != nil {
		t.Fatalf("Error opening %s: %s", opt.Dir, err) // 如果打开数据库失败，记录错误信息并使测试失败
	}

	defer os.RemoveAll(opt.Dir) // 在测试结束后删除数据库文件夹
	defer store.Close()         // 在测试结束后关闭数据库存储

	type test struct {
		Test string
	}

	var result test
	err = store.Get("unknownKey", &result) // 尝试获取一个不存在的键的数据
	if err != badgerhold.ErrNotFound {
		t.Errorf("Expected error of type ErrNotFound, not %T", err) // 检查是否返回了预期的错误
	}
}

// ItemWithStorer 自定义的结构体类型，用于测试 storer 接口。
type ItemWithStorer struct{ Name string }

// Type 返回存储类型名称
func (i *ItemWithStorer) Type() string { return "Item" }

// Indexes 返回自定义的索引映射
func (i *ItemWithStorer) Indexes() map[string]badgerhold.Index {
	return map[string]badgerhold.Index{
		"Name": {
			IndexFunc: func(_ string, value interface{}) ([]byte, error) {
				v := value.(*ItemWithStorer).Name
				return badgerhold.DefaultEncode(v)
			},
			Unique: false, // 非唯一索引
		},
	}
}

// TestIssue115 测试多次 Upsert 操作在自定义索引情况下的行为。
func TestIssue115(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		item := &ItemWithStorer{"Name"}
		for i := 0; i < 2; i++ {
			err := store.Upsert("key", item) // 执行 Upsert 操作
			if err != nil {
				t.Fatal(err) // 如果操作失败，记录错误并使测试失败
			}
		}
	})
}

// TestIssue70TypePrefixCollision 测试不同类型在使用同一键时的行为。
func TestIssue70TypePrefixCollision(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {

		type TestStruct struct {
			Value int
		}

		type TestStructCollision struct {
			Value int
		}

		for i := 0; i < 5; i++ {
			ok(t, store.Insert(i, TestStruct{Value: i}))          // 插入 TestStruct 数据
			ok(t, store.Insert(i, TestStructCollision{Value: i})) // 插入 TestStructCollision 数据
		}

		query := badgerhold.Where(badgerhold.Key).In(0, 1, 2, 3, 4)
		var results []TestStruct
		ok(t, store.Find(
			&results,
			query, // 根据键查找数据
		))

		equals(t, 5, len(results)) // 检查返回的结果数量是否正确
	})
}

// TestIssue71IndexByCustomName 测试使用自定义索引名称的行为。
func TestIssue71IndexByCustomName(t *testing.T) {
	testWrap(t, func(store *badgerhold.Store, t *testing.T) {
		type Person struct {
			Name     string
			Division string `badgerholdIndex:"IdxDivision"`
		}

		record := Person{Name: "test", Division: "testDivision"}

		ok(t, store.Insert(1, record)) // 插入测试数据

		var result []Person
		ok(t, store.Find(&result, badgerhold.Where("Division").Eq(record.Division).Index("IdxDivision"))) // 根据自定义索引查找数据
	})
}

// testWrap 包装测试函数，确保每次测试前后环境的一致性。
func testWrap(t *testing.T, tests func(store *badgerhold.Store, t *testing.T)) {
	testWrapWithOpt(t, testOptions(), tests) // 使用默认配置执行测试
}

// testWrapWithOpt 包装测试函数，允许指定选项。
func testWrapWithOpt(t *testing.T, opt badgerhold.Options, tests func(store *badgerhold.Store, t *testing.T)) {
	var err error
	store, err := badgerhold.Open(opt) // 打开数据库存储
	if err != nil {
		t.Fatalf("Error opening %s: %s", opt.Dir, err) // 如果打开数据库失败，记录错误信息并使测试失败
	}

	if store == nil {
		t.Fatalf("store is null!") // 检查 store 是否为空，若为空则测试失败
	}

	tests(store, t)       // 执行测试函数
	store.Close()         // 关闭数据库存储
	os.RemoveAll(opt.Dir) // 删除数据库文件夹
}

// emptyLogger 定义了一个空日志记录器。
type emptyLogger struct{}

func (e emptyLogger) Errorf(msg string, data ...interface{})   {}
func (e emptyLogger) Infof(msg string, data ...interface{})    {}
func (e emptyLogger) Warningf(msg string, data ...interface{}) {}
func (e emptyLogger) Debugf(msg string, data ...interface{})   {}

// testOptions 返回测试用的默认配置选项。
func testOptions() badgerhold.Options {
	opt := badgerhold.DefaultOptions
	opt.Dir = tempdir() // 设置临时目录作为数据库目录
	opt.ValueDir = opt.Dir
	opt.Logger = emptyLogger{} // 使用空日志记录器
	return opt
}

// tempdir 返回一个临时目录路径。
func tempdir() string {
	name, err := ioutil.TempDir("", "badgerhold-")
	if err != nil {
		panic(err) // 如果创建临时目录失败，则直接 panic
	}
	return name
}

// assert 如果条件为 false 则使测试失败。
func assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok 如果 err 不为 nil 则使测试失败。
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals 如果 exp 不等于 act 则使测试失败。
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}
