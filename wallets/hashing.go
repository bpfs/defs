// hashing.go
// 哈希计算

package wallets

import (
	"crypto/sha256"

	"golang.org/x/crypto/ripemd160"
)

// HashPublicKey 将公钥字节进行SHA-256和RIPEMD-160双重哈希
// 参数:
//   - pubKeyBytes ([]byte): 公钥的字节表示
//
// 返回值:
//   - []byte: 公钥的哈希值
func HashPublicKey(pubKeyBytes []byte) []byte {
	// 对公钥字节进行SHA-256哈希
	shaHash := sha256.Sum256(pubKeyBytes)
	// 创建RIPEMD-160哈希器
	ripeHasher := ripemd160.New()
	// 写入SHA-256哈希结果到RIPEMD-160哈希器
	ripeHasher.Write(shaHash[:])
	// 返回RIPEMD-160哈希值
	return ripeHasher.Sum(nil)
}

// DoubleSHA256 对数据执行两次SHA-256哈希
// 参数:
//   - data ([]byte): 需要哈希的数据
//
// 返回值:
//   - []byte: 哈希值
func DoubleSHA256(data []byte) []byte {
	// 对输入数据进行第一次SHA-256哈希
	firstSHA := sha256.Sum256(data)
	// 对第一次哈希结果进行第二次SHA-256哈希
	secondSHA := sha256.Sum256(firstSHA[:])
	// 返回第二次SHA-256哈希结果
	return secondSHA[:]
}
