package downloads

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/bpfs/defs/files"
	"github.com/bpfs/defs/files/tempfile"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
)

// generateTempFileKey 生成临时文件的唯一键
// 参数:
//   - fileId: 文件ID
//   - segmentIndex: 片段索引
//
// 返回值:
//   - string: 生成的临时文件唯一键
func generateTempFileKey(fileId string, segmentIndex int64) string {
	// 将文件ID和片段索引组合，然后进行哈希处理
	data := fmt.Sprintf("%s-%d", fileId, segmentIndex)
	// 计算SHA256哈希值
	hash := sha256.Sum256([]byte(data))
	// 将哈希值转换为十六进制字符串并返回
	return hex.EncodeToString(hash[:])
}

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

// tempFileExists 检查临时文件是否存在并返回其大小
// 参数:
//   - tempFileKey: 临时文件的唯一键
//
// 返回值:
//   - bool: 文件是否存在
//   - int64: 文件大小（如果存在）
//   - error: 可能发生的错误
func tempFileExists(tempFileKey string) (bool, int64, error) {
	// 检查文件是否存在并获取文件信息
	fileInfo, exists, err := tempfile.CheckFileExistsAndInfo(tempFileKey)
	// 如果发生错误，返回错误信息
	if err != nil {
		return false, 0, err
	}
	// 如果文件不存在，返回 false 和 0
	if !exists {
		return false, 0, nil
	}
	// 如果文件存在，返回 true 和文件大小
	return true, fileInfo.Size(), nil
}

// verifyChecksum 验证数据的校验和
// 参数:
//   - content: 需要验证的数据
//   - expectedChecksum: 预期的CRC32校验和
//
// 返回值:
//   - bool: 如果校验和匹配返回true，否则返回false
func verifyChecksum(content []byte, expectedChecksum uint32) bool {
	// 计算内容的CRC32校验和
	calculatedChecksum := files.GetBytesCRC32(content)

	// 记录校验和比较日志
	logger.Info("校验和比较",
		"预期校验和", expectedChecksum,
		"计算校验和", calculatedChecksum)

	// 返回校验和是否匹配
	return calculatedChecksum == expectedChecksum
}
