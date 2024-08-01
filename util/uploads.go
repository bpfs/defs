package util

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/bpfs/defs/afero"
)

// GetContentType 获取 MIME 类型的方法
func GetContentType(file afero.File) (string, error) {
	// 读取文件的前512个字节
	buf := make([]byte, 512)
	_, err := file.Read(buf)
	if err != nil {
		return "", err
	}

	// 将文件指针重置到开头
	_, err = file.Seek(0, 0)
	if err != nil {
		return "", err
	}

	// 使用http.DetectContentType获取MIME类型
	contentType := http.DetectContentType(buf)
	return contentType, nil
}

// GetFileChecksum 计算文件的校验和的方法
func GetFileChecksum(file afero.File) ([]byte, error) {
	// 创建一个新的哈希器实例
	hasher := sha256.New()
	// 将文件内容写入哈希器
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	// 将文件指针重置到开头
	_, err := file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	// 计算并返回文件的SHA-256哈希值
	return hasher.Sum(nil), nil

	// checksum := hex.EncodeToString(hasher.Sum(nil))
	// return checksum, nil
}

// GenerateSecretFromPrivateKeyAndChecksum 使用私钥和文件校验和生成秘密
func GenerateSecretFromPrivateKeyAndChecksum(ownerPriv *ecdsa.PrivateKey, checksum []byte) ([]byte, error) {
	// 私钥的D值转换为字节序列
	privateKeyBytes := ownerPriv.D.Bytes()

	// 创建一个新的哈希器实例用于生成最终的秘密
	hasher := sha256.New()

	// 写入私钥的字节序列和文件校验和
	if _, err := hasher.Write(privateKeyBytes); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(checksum); err != nil {
		return nil, err
	}

	// 计算并返回秘密的SHA-256哈希值作为最终秘密
	return hasher.Sum(nil), nil
}

// GenerateFileID 生成用于文件的FileID
func GenerateFileID(checksum []byte) (string, error) {
	// 对生成的秘密进行再次哈希，以生成FileID
	hasher := sha256.New()
	if _, err := hasher.Write(checksum); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// GenerateSegmentID 生成用于文件片段的SegmentID
// func GenerateSegmentID(fileID string, index int) (string, error) {
// 	// 将文件ID和分片索引转换为字节并组合
// 	input := []byte(fmt.Sprintf("%s-%d", fileID, index))

// 	// 使用SHA-256对组合后的字节进行哈希，生成SegmentID
// 	hasher := sha256.New()
// 	if _, err := hasher.Write(input); err != nil {
// 		return "", err
// 	}

// 	// 将哈希值转换为十六进制字符串作为SegmentID
// 	return hex.EncodeToString(hasher.Sum(nil)), nil
// }
