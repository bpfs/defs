// Package files 提供文件相关的操作功能,包括密钥分片生成和恢复等功能
package files

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"github.com/bpfs/defs/shamir"
	"github.com/bpfs/defs/utils/logger"
)

// 定义 Shamir 秘密共享方案相关常量
const (
	totalShares = 3 // 总份额数,表示要生成的密钥分片总数
	threshold   = 2 // 恢复阈值,表示恢复原始密钥所需的最小分片数量
)

// GenerateKeyShares 生成密钥分片
//
// 参数:
//   - ownerPriv *ecdsa.PrivateKey: 文件所有者的私钥,用于生成初始秘密
//   - fileIdentifier string: 文件的唯一标识符,用于生成特定于文件的密钥分片
//
// 返回值:
//   - [][]byte: 生成的密钥分片列表,每个元素为一个密钥分片
//   - error: 生成过程中的错误信息,如果成功则为nil
func GenerateKeyShares(ownerPriv *ecdsa.PrivateKey, fileIdentifier string) ([][]byte, error) {
	// 验证私钥参数不能为空
	if ownerPriv == nil {
		return nil, fmt.Errorf("私钥不能为空")
	}
	// 验证文件标识符不能为空
	if fileIdentifier == "" {
		return nil, fmt.Errorf("文件标识符不能为空")
	}

	// 设置 Shamir 方案使用的素数,这是一个标准的椭圆曲线参数
	prime, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)

	// 创建 Shamir 方案实例,用于生成和恢复密钥分片
	scheme := shamir.NewShamirScheme(totalShares, threshold, prime)

	// 使用私钥和文件标识符生成初始秘密
	secret, err := GenerateSecretFromPrivateKeyAndChecksum(ownerPriv, []byte(fileIdentifier))
	if err != nil {
		logger.Errorf("生成秘密失败: %v", err)
		return nil, err
	}

	// 使用 Shamir 方案生成密钥分片
	shares, err := scheme.GenerateShares(secret)
	if err != nil {
		logger.Errorf("生成共享密钥失败: %v", err)
		return nil, err
	}

	// 验证是否成功生成分片
	if len(shares) == 0 {
		return nil, fmt.Errorf("生成共享密钥时失败")
	}

	// 验证生成的分片数量是否符合预期
	if len(shares) != totalShares {
		logger.Errorf("生成的密钥数量不对: 应该是%v，实际是%v", totalShares, len(shares))
		return nil, fmt.Errorf("生成的密钥数量不对")
	}

	return shares, nil
}

// RecoverSecretFromShares 从密钥分片中恢复原始密钥
//
// 参数:
//   - shareOne []byte: 第一个密钥分片,用于恢复原始密钥
//   - shareTwo []byte: 第二个密钥分片,用于恢复原始密钥
//
// 返回值:
//   - []byte: 恢复的原始密钥数据
//   - error: 恢复过程中的错误信息,如果成功则为nil
func RecoverSecretFromShares(shareOne, shareTwo []byte) ([]byte, error) {
	// 初始化 Shamir 方案的素数参数
	prime, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)

	// 创建 Shamir 方案实例,用于恢复原始密钥
	scheme := shamir.NewShamirScheme(totalShares, threshold, prime)

	// 使用两个分片恢复原始密钥
	decryptionKey, err := scheme.RecoverSecretFromShares(shareOne, shareTwo)
	if err != nil {
		logger.Errorf("使用密钥分片恢复原始密钥失败: %v", err)
		return nil, err
	}

	return decryptionKey, nil
}

// GenerateSecretFromPrivateKeyAndChecksum 使用私钥和文件校验和生成秘密
//
// 参数:
//   - ownerPriv *ecdsa.PrivateKey: 文件所有者的私钥,用于生成秘密
//   - checksum []byte: 文件的校验和,用于生成特定于文件的秘密
//
// 返回值:
//   - []byte: 生成的秘密字节数组
//   - error: 生成过程中的错误信息,如果成功则为nil
func GenerateSecretFromPrivateKeyAndChecksum(ownerPriv *ecdsa.PrivateKey, checksum []byte) ([]byte, error) {
	// 验证私钥参数不能为空
	if ownerPriv == nil {
		logger.Error("私钥不能为空")
		return nil, errors.New("私钥不能为空")
	}

	// 验证校验和不能为空
	if len(checksum) == 0 {
		logger.Error("校验和不能为空")
		return nil, errors.New("校验和不能为空")
	}

	// 将私钥序列化为字节数组
	privKeyBytes, err := MarshalPrivateKey(ownerPriv)
	if err != nil {
		logger.Errorf("序列化私钥失败: %v", err)
		return nil, err
	}

	// 创建 SHA256 哈希器实例
	hasher := sha256.New()

	// 写入私钥数据到哈希器
	if _, err := hasher.Write(privKeyBytes); err != nil {
		logger.Errorf("写入私钥字节失败: %v", err)
		return nil, err
	}

	// 写入校验和数据到哈希器
	if _, err := hasher.Write(checksum); err != nil {
		logger.Errorf("写入校验和失败: %v", err)
		return nil, err
	}

	// 计算最终的哈希值作为秘密
	secret := hasher.Sum(nil)

	return secret, nil
}
