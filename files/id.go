package files

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

// GenerateTaskID 生成任务ID
// 使用时间戳、私钥和随机数生成一个唯一的taskID
//
// 参数:
//   - ownerPriv *ecdsa.PrivateKey: 所有者的私钥
//
// 返回值:
//   - string: 生成的taskID
//   - error: 处理过程中发生的任何错误
func GenerateTaskID(ownerPriv *ecdsa.PrivateKey) (string, error) {
	// 获取当前时间
	now := time.Now()
	// 生成一个0到999999之间的随机数
	randBigInt, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		logger.Errorf("生成随机数时出错: %v", err)
		return "", err
	}

	// 组合私钥的公钥X坐标、时间戳和随机数生成taskID
	taskID := fmt.Sprintf("%x-%d-%s", ownerPriv.PublicKey.X.Bytes(), now.Unix(), randBigInt.String())
	return taskID, nil
}

// GenerateFileID 生成文件的唯一标识
// 参数：
//   - privateKey: *ecdsa.PrivateKey ECDSA 私钥，用于生成文件ID
//   - checksum: []byte 文件的校验和
//
// 返回值：
//   - string: 生成的文件ID
//   - error: 如果发生错误，返回错误信息
func GenerateFileID(privateKey *ecdsa.PrivateKey, checksum []byte) (string, error) {
	// 提取私钥对应的公钥
	publicKeyBytes := elliptic.MarshalCompressed(privateKey.Curve, privateKey.X, privateKey.Y)
	// 将公钥和校验和拼接
	combined := append(publicKeyBytes, checksum...)

	// 对生成的秘密进行再次哈希，以生成FileID
	hasher := sha256.New()
	if _, err := hasher.Write(combined); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// GenerateSegmentID 生成用于文件片段的SegmentID
//
// 参数:
//   - fileID string: 文件的唯一标识符
//   - index int64: 文件片段的索引
//
// 返回值:
//   - string: 生成的SegmentID
//   - error: 处理过程中发生的任何错误
func GenerateSegmentID(fileID string, index int64) (string, error) {
	// 将文件ID和分片索引转换为字节并组合
	input := []byte(fmt.Sprintf("%s-%d", fileID, index))

	// 使用SHA-256对组合后的字节进行哈希，生成SegmentID
	hasher := sha256.New()
	if _, err := hasher.Write(input); err != nil {
		return "", err
	}

	// 将哈希值转换为十六进制字符串作为SegmentID
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
