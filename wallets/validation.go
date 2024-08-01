// key_management.go
// 高级验证

package wallets

import (
	"github.com/bpfs/defs/base58"
	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// VerifyAddressChecksum 验证地址的校验和是否正确
// 参数:
//   - address (string): 输入的Base58编码钱包地址
//
// 返回值:
//   - bool: 校验和是否正确的布尔值
func VerifyAddressChecksum(address string) bool {
	_, _, err := base58.CheckDecode(address)
	return err == nil
}

// IsAddressOfPublicKey 验证给定地址是否由指定的公钥生成
// 参数:
//   - address (string): 输入的Base58编码钱包地址
//   - publicKey ([]byte): 输入的公钥字节
//
// 返回值:
//   - bool: 地址是否由公钥生成的布尔值
func IsAddressOfPublicKey(address string, publicKey []byte) bool {
	// 检查公钥字节是否有效
	if !IsValidPublicKey(publicKey) {
		logrus.Errorf("[%s] 无效的公钥字节", debug.WhereAmI())
		return false
	}
	// 通过公钥字节生成公钥哈希
	generatedAddress, ok := PublicKeyBytesToAddress(publicKey)
	if !ok {
		return false
	}
	return address == generatedAddress
}

// IsAddressOfPublicKeyHash 验证给定地址是否与公钥哈希匹配
// 参数:
//   - address (string): 输入的Base58编码钱包地址
//   - publicKeyHash ([]byte): 输入的公钥哈希
//
// 返回值:
//   - bool: 地址是否与公钥哈希匹配的布尔值
func IsAddressOfPublicKeyHash(address string, publicKeyHash []byte) bool {
	// 检查公钥哈希是否有效
	if !IsValidPublicKeyHash(publicKeyHash) {
		logrus.Errorf("[%s] 无效的公钥哈希字节", debug.WhereAmI())
		return false
	}
	// 通过公钥哈希生成钱包地址
	generatedAddress, ok := PublicKeyHashToAddress(publicKeyHash)
	if !ok {
		return false
	}
	return address == generatedAddress
}
