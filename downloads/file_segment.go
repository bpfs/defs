package downloads

import (
	"crypto/md5"
	"fmt"
	"hash/crc32"

	"github.com/bpfs/defs/v2/crypto/gcm"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/script"
	ecdsa_ "github.com/bpfs/defs/v2/sign/ecdsa"

	"github.com/bpfs/defs/v2/zip/gzip"
)

// VerifySegmentSignature 验证片段签名
// 参数:
//   - p: 片段内容响应对象,包含需要验证的数据和签名
//
// 返回值:
//   - error: 验证失败返回错误,验证成功返回nil
//
// 功能:
//   - 验证片段内容的签名是否有效
//   - 使用ECDSA算法进行签名验证
//   - 验证签名数据的完整性和真实性
func VerifySegmentSignature(p *pb.SegmentContentResponse) error {
	// 构造签名数据对象,包含需要验证的数据字段
	signatureData := &pb.SignatureData{
		FileId:        p.FileId,                                           // 文件ID
		ContentType:   p.FileMeta.ContentType,                             // 内容类型
		Sha256Hash:    p.FileMeta.Sha256Hash,                              // SHA256哈希
		SliceTable:    files.ConvertSliceTableToSortedSlice(p.SliceTable), // 排序后的分片表
		SegmentId:     p.SegmentId,                                        // 分片ID
		SegmentIndex:  p.SegmentIndex,                                     // 分片索引
		Crc32Checksum: p.Crc32Checksum,                                    // CRC32校验和
		EncryptedData: p.SegmentContent,                                   // 加密数据
	}

	// 从P2PK脚本中提取ECDSA公钥
	pubKey, err := script.ExtractPubKeyFromP2PKScriptToECDSA(p.P2PkScript)
	if err != nil {
		logger.Errorf("从P2PK脚本提取公钥失败: %v", err)
		return err
	}

	// 序列化签名数据为字节数组
	merged, err := signatureData.Marshal()
	if err != nil {
		logger.Errorf("序列化数据失败: %v", err)
		return err
	}

	// 计算序列化数据的MD5哈希
	hash := md5.Sum(merged)
	merged = hash[:]

	// 使用ECDSA验证签名
	valid, err := ecdsa_.VerifySignature(pubKey, merged, p.Signature)
	if err != nil || !valid {
		logger.Errorf("验证签名失败: %v", err)
		return err
	}

	return nil
}

// DecompressAndDecryptSegmentContent 解压并解密片段内容
// 参数:
//   - shareOne: 第一个密钥分片
//   - shareTwo: 第二个密钥分片
//   - compressedData: 压缩并加密的数据内容
//
// 返回值:
//   - []byte: 解压并解密后的原始数据
//   - error: 解压或解密失败时返回错误信息
//
// 功能:
//   - 对压缩的加密数据进行解压缩
//   - 使用密钥分片恢复解密密钥
//   - 使用AES-GCM模式解密数据
//   - 返回解密后的原始数据
func DecompressAndDecryptSegmentContent(shareOne, shareTwo []byte, compressedData []byte) ([]byte, error) {
	// 解压缩加密数据
	decompressedData, err := gzip.DecompressData(compressedData)
	if err != nil {
		logger.Errorf("解压加密数据失败: %v", err)
		return nil, err
	}

	// 使用密钥分片恢复原始密钥
	decryptionKey, err := files.RecoverSecretFromShares(shareOne, shareTwo)
	if err != nil {
		logger.Errorf("从密钥分片恢复密钥失败: %v", err)
		return nil, err
	}

	// 计算密钥的MD5哈希作为AES密钥
	aesKey := md5.Sum(decryptionKey)

	// 使用AES-GCM解密数据
	plaintext, err := gcm.DecryptData(decompressedData, aesKey[:])
	if err != nil {
		logger.Errorf("AES-GCM解密数据失败: %v", err)
		return nil, err
	}

	return plaintext, nil
}

// VerifySegmentChecksum 验证片段校验和
// 参数:
//   - content: 需要验证的内容数据
//   - expectedChecksum: 期望的校验和值
//
// 返回值:
//   - error: 校验和不匹配返回错误,匹配返回nil
//
// 功能:
//   - 计算内容的CRC32校验和
//   - 验证计算的校验和与期望值是否匹配
//   - 确保数据完整性
func VerifySegmentChecksum(content []byte, expectedChecksum uint32) error {
	// 计算内容的CRC32校验和
	actualChecksum := crc32.ChecksumIEEE(content)

	// 比较计算的校验和与期望值
	if actualChecksum != expectedChecksum {
		logger.Errorf("校验和验证失败: 期望值=%d, 实际值=%d", expectedChecksum, actualChecksum)
		return fmt.Errorf("校验和不匹配")
	}

	return nil
}
