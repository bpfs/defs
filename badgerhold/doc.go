/*
badgerhold 是一个基于 Badger DB 之上的索引和查询层。它的目的是提供一种简单、持久的 Go 类型存储和检索方法。Badger DB 是一个嵌入式键值存储，而 badgerhold 提供了一个更高级的接口，适用于 Badger 的常见用例。

# Go 类型

badgerhold 直接处理 Go 类型。插入数据时，您可以直接传递结构体。当查询数据时，您需要传递一个指向要返回类型的切片的指针。默认情况下，使用 Gob 编码。
您可以在同一个 DB 文件中存储多种不同的类型，并且它们（及其索引）会被分别存储。

示例代码展示了如何插入数据和查询数据：

	err := store.Insert(1234, Item{
		Name:    "Test Name",
		Created: time.Now(),
	})

	var result []Item

	err := store.Find(&result, query)

# 索引

badgerhold 会自动为结构体中标记为 badgerholdIndex 的字段创建索引。例如：

	type Item struct {
		ID       int
		Name     string
		Category string `badgerholdIndex:"Category"`
		Created  time.Time
	}

查询时，第一个指定的字段（如果存在索引）将用作索引。

# 查询

查询是对一组字段应用的链式条件。例如：

	badgerhold.Where("Name").Eq("John Doe").And("DOB").Lt(time.Now())

这个查询语句表示查找 Name 为 "John Doe" 且 DOB 小于当前时间的记录。

# 总结

badgerhold 提供了一种高效且易于使用的方法来在 Badger DB 之上进行复杂的查询和数据操作，并自动处理了索引的创建和使用。
*/
package badgerhold

/**

基础功能

1. 数据插入（Insert）:
	将结构体数据插入到数据库中，支持自动生成主键或使用自定义键。

2. 数据查询（Find/Get）:
	支持根据主键查询单条数据（Get）。
	支持使用复杂条件查询多条数据（Find），包括等于、不等于、大于、小于、范围、逻辑与、逻辑或等查询条件。

3. 数据更新（Update）:
	根据主键更新已存在的结构体数据。

4. 数据删除（Delete）:
	根据主键删除数据。
	支持批量删除符合条件的数据。


高级功能

1. 索引支持（Indexing）:
	支持在结构体的字段上创建索引，以加快查询速度。
	支持对单个或多个字段创建复合索引。

2. 批量操作（Batching）:
	支持批量插入、更新或删除操作，以提高性能，尤其适用于大规模数据操作。

3. 事务支持（Transactions）:
	提供事务支持，可以在事务中执行多个操作，确保这些操作的原子性（要么全部成功，要么全部失败）。

4. 自定义编码/解码（Custom Encoding/Decoding）:
	允许开发者自定义结构体的序列化和反序列化方式，支持加密或特殊序列化格式。

5. 自动迁移（Auto Migration）:
	支持在结构体字段发生变化时，自动处理数据结构的迁移。

6. TTL（Time-to-Live）支持:
	允许为数据项设置生存时间（TTL），超时后数据会自动被删除。

7. 数据导入/导出（Data Import/Export）:
	支持将数据库中的数据导出到文件，也支持从备份文件中导入数据，方便数据备份和迁移。

8. 查询条件构造器（Query Builder）:
	提供丰富的查询条件构造器，支持组合多个条件构建复杂查询。
	支持各种比较运算符（等于、不等于、大于、小于等），逻辑运算符（与、或、非），以及范围查询。


其他功能

1. 多种存储模式（Storage Modes）:
	支持通过多种模式（如全局存储、会话存储等）对数据进行存储和访问。

2. 高级查询功能（Advanced Querying）:
	提供一些高级查询功能，如聚合查询、排序、分页等。

3. 键值数据存储（Key-Value Storage）:
	支持直接存储和访问简单的键值对数据。

4. 类型安全（Type Safety）:
	在操作数据时提供类型安全检查，避免运行时类型错误。

5. 灵活的选项配置（Flexible Options Configuration）:
	提供多种配置选项，以自定义数据库的行为和性能。

6. 嵌入式数据库（Embedded Database）:
	badgerhold 基于 Badger，是一种高性能的嵌入式数据库，适合在单机环境下使用。

7. 易于使用的 API:
	提供简单易用的 API，适合开发者快速上手并集成到应用中。

8. 内置数据迁移工具（Built-in Data Migration Tools）:
	支持数据模式变更时的自动或手动迁移，确保数据一致性。


性能与优化

1. 高性能数据存取:
	基于 Badger 提供的 LSM 树结构，badgerhold 在读写性能上具有良好的表现。

2. 内存优化:
	提供优化内存使用的配置选项，适用于低内存环境。

3. 并发支持:
	支持并发读写操作，确保在多线程环境下的数据一致性和性能。

*/

// 基础功能的示例代码及详细注释
// 以下是 badgerhold 的基础功能示例代码，包括数据插入、查询、更新和删除的操作，并为每一行代码提供了详细的注释。
// 功能总结
// 插入（Insert）: 使用 Insert 方法将数据插入到数据库中，可以使用自动生成的序列号或手动设置主键。
// 查询（Get/Find）: 使用 Get 根据主键查询单条记录，使用 Find 根据条件查询多条记录。
// 更新（Update）: 使用 Update 方法根据主键更新已存在的记录。
// 删除（Delete）: 使用 Delete 方法根据主键删除单条记录，使用 DeleteMatching 方法批量删除符合条件的记录。

/**

1. 数据插入（Insert）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	type Person struct {
		ID   int    `badgerholdKey:"ID"`  // 定义 ID 为主键，使用 badgerholdKey 标签指定
		Name string // 人员的名字
		Age  int    // 人员的年龄
	}

	func main() {
		// 打开 Badgerhold 数据库，使用默认配置
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil { // 检查数据库打开时是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}
		defer store.Close() // 确保在 main 函数结束时关闭数据库连接

		// 创建一个 Person 实例
		person := &Person{
			ID:   1,          // 设置 ID，虽然使用了自定义键，这里也手动设置
			Name: "Alice",    // 设置 Name 字段
			Age:  30,         // 设置 Age 字段
		}

		// 将 Person 实例插入到 Badgerhold 数据库中
		err = store.Insert(badgerhold.NextSequence(), person)
		if err != nil { // 检查插入操作是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}

		logger.Println("Person successfully inserted") // 如果插入成功，记录日志
	}

*/

/**

2. 数据查询（Find/Get）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil { // 检查数据库打开时是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}
		defer store.Close() // 确保在 main 函数结束时关闭数据库连接

		// 单个查询（Get）示例，根据主键 ID 查询 Person
		var person Person
		err = store.Get(1, &person) // 使用主键 ID 查询记录
		if err != nil { // 检查查询操作是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}
		logger.Printf("Retrieved Person: %+v\n", person) // 输出查询到的 Person 实例

		// 查询所有年龄大于 25 的人员（Find）示例
		var persons []Person
		err = store.Find(&persons, badgerhold.Where("Age").Gt(25)) // 查询 Age 大于 25 的记录
		if err != nil { // 检查查询操作是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}
		logger.Printf("Persons older than 25: %+v\n", persons) // 输出查询到的所有符合条件的 Person 实例
	}


*/

/**

3. 数据更新（Update）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil { // 检查数据库打开时是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}
		defer store.Close() // 确保在 main 函数结束时关闭数据库连接

		// 更新 Person 实例
		person := &Person{
			ID:   1,          // 要更新的记录的主键 ID
			Name: "Alice",    // 更新后的 Name 字段值
			Age:  31,         // 更新后的 Age 字段值
		}

		err = store.Update(1, person) // 根据主键 ID 更新记录
		if err != nil { // 检查更新操作是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}

		logger.Println("Person successfully updated") // 如果更新成功，记录日志
	}

*/

/**

4. 数据删除（Delete）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil { // 检查数据库打开时是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}
		defer store.Close() // 确保在 main 函数结束时关闭数据库连接

		// 根据主键 ID 删除 Person 实例
		err = store.Delete(1, &Person{}) // 使用主键 ID 删除记录
		if err != nil { // 检查删除操作是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}

		logger.Println("Person successfully deleted") // 如果删除成功，记录日志

		// 批量删除所有年龄小于 30 的人员
		err = store.DeleteMatching(&Person{}, badgerhold.Where("Age").Lt(30)) // 删除 Age 小于 30 的所有记录
		if err != nil { // 检查批量删除操作是否出现错误
			logger.Fatal(err) // 如果有错误，记录并终止程序
		}

		logger.Println("Persons under 30 successfully deleted") // 如果批量删除成功，记录日志
	}

*/

// 高级功能的示例代码
// 以下是 badgerhold 高级功能的示例代码，包括索引支持、批量操作、事务支持、自定义编码/解码、自动迁移、TTL 支持、数据导入/导出以及查询条件构造器的用法。
// 总结
// 索引支持：通过在结构体字段上创建索引，加速查询。
// 批量操作：在单个事务内批量插入、更新或删除数据，以提高性能。
// 事务支持：确保多个操作的原子性，操作要么全部成功，要么全部失败。
// 自定义编码/解码：开发者可以定制数据的序列化与反序列化逻辑。
// 自动迁移：数据结构变更时，badgerhold 自动处理迁移。
// TTL 支持：设置数据的生存时间，自动删除过期数据。
// 数据导入/导出：方便进行数据备份和迁移。
// 查询条件构造器：构建复杂查询，支持多种条件组合。

/**

1. 索引支持（Indexing）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	type Product struct {
		ID       int     `badgerholdKey:"ID"`      // 定义 ID 为主键
		Name     string  `badgerholdIndex:"Name"`  // 为 Name 字段创建索引
		Category string  `badgerholdIndex:"Category"` // 为 Category 字段创建索引
		Price    float64 // 产品价格
	}

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入数据
		store.Insert(badgerhold.NextSequence(), &Product{ID: 1, Name: "Laptop", Category: "Electronics", Price: 999.99})
		store.Insert(badgerhold.NextSequence(), &Product{ID: 2, Name: "Phone", Category: "Electronics", Price: 499.99})

		// 基于索引字段查询
		var products []Product
		err = store.Find(&products, badgerhold.Where("Name").Eq("Laptop").And(badgerhold.Where("Category").Eq("Electronics")))
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Found products: %+v\n", products) // 输出查询结果
	}

*/

/**

2. 批量操作（Batching）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 创建批量操作
		batch := store.Batched()

		// 批量插入
		products := []Product{
			{ID: 3, Name: "Tablet", Category: "Electronics", Price: 299.99},
			{ID: 4, Name: "Monitor", Category: "Electronics", Price: 199.99},
		}

		for _, product := range products {
			err := batch.Insert(badgerhold.NextSequence(), &product)
			if err != nil {
				logger.Fatal(err)
			}
		}

		// 提交批量操作
		err = batch.Commit()
		if err != nil {
			logger.Fatal(err)
		}

		logger.Println("Batch insert completed")
	}

*/

/**

3. 事务支持（Transactions）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		err = store.Badger().Update(func(txn *badgerhold.Tx) error {
			// 在事务中插入一条记录
			err := txn.Insert(badgerhold.NextSequence(), &Product{ID: 5, Name: "Keyboard", Category: "Accessories", Price: 49.99})
			if err != nil {
				return err
			}

			// 在事务中更新一条记录
			err = txn.Update(5, &Product{ID: 5, Name: "Mechanical Keyboard", Category: "Accessories", Price: 69.99})
			if err != nil {
				return err
			}

			// 事务中的所有操作成功完成后，提交事务
			return nil
		})

		if err != nil {
			logger.Fatal(err)
		}

		logger.Println("Transaction successfully committed")
	}

*/

/**

4. 自定义编码/解码（Custom Encoding/Decoding）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	// 定义一个需要自定义编码/解码的结构体
	type CustomData struct {
		ID   int    `badgerholdKey:"ID"`
		Data []byte
	}

	// 自定义编码方法
	func (c CustomData) Marshal() ([]byte, error) {
		// 在这里实现自定义的编码逻辑，例如加密数据
		encryptedData := customEncrypt(c.Data)
		return encryptedData, nil
	}

	// 自定义解码方法
	func (c *CustomData) Unmarshal(data []byte) error {
		// 在这里实现自定义的解码逻辑，例如解密数据
		decryptedData := customDecrypt(data)
		c.Data = decryptedData
		return nil
	}

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入带有自定义编码的数据
		customData := &CustomData{ID: 1, Data: []byte("Sensitive Data")}
		err = store.Insert(badgerhold.NextSequence(), customData)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Println("Custom encoded data inserted")
	}

*/

/**

5. 自动迁移（Auto Migration）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	type User struct {
		ID    int    `badgerholdKey:"ID"`
		Name  string
		Email string // 新增的字段
	}

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入旧的结构体数据
		store.Insert(badgerhold.NextSequence(), &User{ID: 1, Name: "Alice"})

		// 此时增加了 Email 字段，badgerhold 会自动迁移
		var users []User
		err = store.Find(&users, nil)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Users after migration: %+v\n", users)
	}

*/

/**

6. TTL（Time-to-Live）支持

	package main

	import (
		"log"
		"time"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入带有 TTL 的数据
		user := &User{ID: 2, Name: "Bob", Email: "bob@example.com"}
		err = store.TTLInsert(badgerhold.NextSequence(), user, time.Hour) // 数据在 1 小时后自动删除
		if err != nil {
			logger.Fatal(err)
		}

		logger.Println("TTL data inserted")
	}

*/

/**

7. 数据导入/导出（Data Import/Export）

	package main

	import (
		"log"
		"os"
		"github.com/timshannon/badgerhold"
	)

	func exportData(store *badgerhold.Store, filename string) {
		f, err := os.Create(filename)
		if err != nil {
			logger.Fatal(err)
		}
		defer f.Close()

		err = store.Backup(f)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Println("Data exported to", filename)
	}

	func importData(store *badgerhold.Store, filename string) {
		f, err := os.Open(filename)
		if err != nil {
			logger.Fatal(err)
		}
		defer f.Close()

		err = store.Load(f)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Println("Data imported from", filename)
	}

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 导出数据到文件
		exportData(store, "backup.db")

		// 从文件中导入数据
		importData(store, "backup.db")
	}

*/

/**

8. 查询条件构造器（Query Builder）

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入一些示例数据
		store.Insert(badgerhold.NextSequence(), &User{ID: 3, Name: "Charlie", Email: "charlie@example.com"})
		store.Insert(badgerhold.NextSequence(), &User{ID: 4, Name: "Dave", Email: "dave@example.com"})

		// 使用查询条件构造器查找所有 Name 为 "Charlie" 或 "Dave" 的用户
		var users []User
		err = store.Find(&users, badgerhold.Where("Name").Eq("Charlie").Or(badgerhold.Where("Name").Eq("Dave")))
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Users found: %+v\n", users)
	}

*/

// 其他功能的示例代码
// 以下是 badgerhold 中关于多种存储模式、高级查询功能、键值数据存储、类型安全、灵活选项配置、嵌入式数据库、易于使用的 API 和内置数据迁移工具的示例代码。
// 功能总结
// 多种存储模式：支持全局和会话存储模式。
// 高级查询功能：支持聚合查询、排序和分页等高级功能。
// 键值数据存储：支持直接存储和访问简单的键值对数据。
// 类型安全：提供类型安全检查，避免运行时的类型错误。
// 灵活的选项配置：可根据需求自定义数据库配置。
// 嵌入式数据库：适用于单机环境的高性能嵌入式数据库。
// 易于使用的 API：提供简洁易用的接口。
// 内置数据迁移工具：支持自动和手动的数据迁移，确保数据一致性。

/**

1. 多种存储模式（Storage Modes）

badgerhold 支持通过多种模式（如全局存储、会话存储等）对数据进行存储和访问。以下示例展示了如何在不同的模式下使用 badgerhold。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		// 默认模式：全局存储模式
		globalStore, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer globalStore.Close()

		// 在全局存储模式下存储数据
		globalStore.Insert(badgerhold.NextSequence(), &Person{ID: 1, Name: "Alice", Age: 30})

		// 自定义模式：会话存储模式
		sessionOptions := badgerhold.DefaultOptions
		sessionOptions.Dir = "./session_data"
		sessionOptions.ValueDir = "./session_data"
		sessionStore, err := badgerhold.Open(sessionOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer sessionStore.Close()

		// 在会话存储模式下存储数据
		sessionStore.Insert(badgerhold.NextSequence(), &Person{ID: 2, Name: "Bob", Age: 25})

		logger.Println("Data stored in both global and session modes")
	}

*/

/**

2. 高级查询功能（Advanced Querying）

badgerhold 提供了高级查询功能，例如聚合查询、排序和分页。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入示例数据
		store.Insert(badgerhold.NextSequence(), &Person{ID: 1, Name: "Alice", Age: 30})
		store.Insert(badgerhold.NextSequence(), &Person{ID: 2, Name: "Bob", Age: 25})
		store.Insert(badgerhold.NextSequence(), &Person{ID: 3, Name: "Charlie", Age: 35})

		// 排序查询：按年龄排序
		var people []Person
		err = store.Find(&people, badgerhold.Where("Age").Ge(25).SortBy("Age"))
		if err != nil {
			logger.Fatal(err)
		}
		logger.Printf("Sorted people by age: %+v\n", people)

		// 分页查询：获取第 1 页，每页 2 条数据
		err = store.Find(&people, badgerhold.Where("Age").Ge(25).Skip(0).Limit(2))
		if err != nil {
			logger.Fatal(err)
		}
		logger.Printf("Paged people: %+v\n", people)
	}

*/

/**

3. 键值数据存储（Key-Value Storage）

badgerhold 允许直接存储和访问简单的键值对数据。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 存储简单的键值对
		err = store.Insert("name", "Alice")
		if err != nil {
			logger.Fatal(err)
		}

		// 读取键值对
		var name string
		err = store.Get("name", &name)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Retrieved value for key 'name': %s\n", name)
	}

*/

/**

4. 类型安全（Type Safety）

badgerhold 在操作数据时提供类型安全检查，避免运行时的类型错误。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	type User struct {
		ID   int
		Name string
		Age  int
	}

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入 User 类型数据
		user := &User{ID: 1, Name: "Alice", Age: 30}
		err = store.Insert(badgerhold.NextSequence(), user)
		if err != nil {
			logger.Fatal(err)
		}

		// 类型安全的查询：确保查询到的类型与插入时的类型一致
		var retrievedUser User
		err = store.Get(1, &retrievedUser)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Retrieved User: %+v\n", retrievedUser)
	}

*/

/**

5. 灵活的选项配置（Flexible Options Configuration）

badgerhold 提供多种配置选项，以自定义数据库的行为和性能。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		// 自定义 Badgerhold 配置
		options := badgerhold.DefaultOptions
		options.Dir = "./custom_data"
		options.ValueDir = "./custom_data"
		options.SyncWrites = false // 禁用同步写入以提高写入性能（可能导致数据丢失）

		store, err := badgerhold.Open(options)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入示例数据
		store.Insert(badgerhold.NextSequence(), &Person{ID: 1, Name: "Alice", Age: 30})

		logger.Println("Data stored with custom configuration")
	}

*/

/**

6. 嵌入式数据库（Embedded Database）

badgerhold 基于 Badger，是一种高性能的嵌入式数据库，适合在单机环境下使用。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		// 打开一个嵌入式数据库实例
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入和查询数据
		store.Insert(badgerhold.NextSequence(), &Person{ID: 1, Name: "Alice", Age: 30})

		var person Person
		err = store.Get(1, &person)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Retrieved Person from embedded database: %+v\n", person)
	}

*/

/**

7. 易于使用的 API

badgerhold 提供简单易用的 API，适合开发者快速上手并集成到应用中。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	func main() {
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入数据
		store.Insert(badgerhold.NextSequence(), &Person{ID: 1, Name: "Alice", Age: 30})

		// 查询数据
		var person Person
		err = store.Get(1, &person)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Easy-to-use API example: %+v\n", person)
	}

*/

/**

8. 内置数据迁移工具（Built-in Data Migration Tools）

badgerhold 支持数据模式变更时的自动或手动迁移，确保数据一致性。

	package main

	import (
		"log"
		"github.com/timshannon/badgerhold"
	)

	type OldPerson struct {
		ID   int    `badgerholdKey:"ID"`
		Name string
	}

	type NewPerson struct {
		ID    int    `badgerholdKey:"ID"`
		Name  string
		Email string // 新增字段
	}

	func main() {
		// 打开数据库
		store, err := badgerhold.Open(badgerhold.DefaultOptions)
		if err != nil {
			logger.Fatal(err)
		}
		defer store.Close()

		// 插入旧的数据结构
		store.Insert(badgerhold.NextSequence(), &OldPerson{ID: 1, Name: "Alice"})

		// 读取时会自动迁移到新的数据结构
		var newPerson NewPerson
		err = store.Get(1, &newPerson)
		if err != nil {
			logger.Fatal(err)
		}

		logger.Printf("Migrated Person: %+v\n", newPerson)
	}

*/
