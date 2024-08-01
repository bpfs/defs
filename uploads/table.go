package uploads

// HashTable 描述分片的校验和是否属于纠删码
type HashTable struct {
	Checksum  []byte // 分片的校验和，用于校验分片数据的完整性和一致性
	IsRsCodes bool   // 标记该分片是否使用了纠删码技术，用于数据的恢复和冗余
}

// NewHashTable 创建并初始化一个映射，映射的键是分片的索引，值是HashTable实例。
// 它用于描述每个分片的哈希值和是否使用了纠删码技术。
func NewHashTable(segments map[int]*FileSegment, dataShards int64) map[int]*HashTable {
	// 初始化一个空的映射，用于存储分片索引和对应的HashTable实例。
	hashTableMap := make(map[int]*HashTable)

	// 遍历所有分片。
	for index, segment := range segments {
		// 检查当前分片是否为数据分片还是奇偶校验（纠删码）分片。
		isRsCodes := index >= int(dataShards) // 如果索引大于等于数据分片的数量，则为纠删码分片。

		// 创建HashTable实例并填充数据。
		hashTableMap[index] = &HashTable{
			Checksum:  segment.Checksum, // 分片的校验和
			IsRsCodes: isRsCodes,        // 标记是否为纠删码分片
		}
	}

	// 返回填充好的映射。
	return hashTableMap
}
