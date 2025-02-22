package downloads

import (
	"github.com/bpfs/defs/v2/pb"
)

// getRequiredDataShards 获取所需的数据分片数量
// 参数:
//   - sliceTable: 哈希表切片映射
//
// 返回值:
//   - int: 所需的数据分片数量
func getRequiredDataShards(sliceTable map[int64]*pb.HashTable) int {
	dataShards := 0
	// 遍历切片表
	for _, slice := range sliceTable {
		// 如果不是RS编码，则增加数据分片计数
		if !slice.IsRsCodes {
			dataShards++
		}
	}
	// 返回数据分片数量
	return dataShards
}
