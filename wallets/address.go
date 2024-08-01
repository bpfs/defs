// address.go
// 地址生成与解析

package wallets

import (
	"crypto/ecdsa"
	"errors"

	"github.com/bpfs/defs/base58"
	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// PrivateKeyToPublicKeyHash 通过私钥生成公钥哈希
// 参数:
//   - privateKey (*ecdh.PrivateKey): 私钥
//
// 返回值:
//   - []byte: 公钥哈希
//   - bool: 是否成功
func PrivateKeyToPublicKeyHash(privateKey *ecdh.PrivateKey) ([]byte, bool) {
	if privateKey == nil {
		logrus.Errorf("[%s] 私钥不能为空", debug.WhereAmI())
		return nil, false
	}
	publicKey := ExtractPublicKey(privateKey)
	return HashPublicKey(publicKey), true
}

// PrivateKeyToAddress 通过私钥生成钱包地址
// 参数:
//   - privateKey (*ecdh.PrivateKey): 私钥
//
// 返回值:
//   - string: 钱包地址
//   - bool: 是否成功
func PrivateKeyToAddress(privateKey *ecdh.PrivateKey) (string, bool) {
	if privateKey == nil {
		logrus.Errorf("[%s] 私钥不能为空", debug.WhereAmI())
		return "", false
	}
	publicKey := ExtractPublicKey(privateKey)
	return PublicKeyBytesToAddress(publicKey)
}

// PublicKeyToPublicKeyHash 通过公钥生成公钥哈希
// 参数:
//   - publicKey (ecdh.PublicKey): 公钥
//
// 返回值:
//   - []byte: 公钥哈希
//   - bool: 是否成功
func PublicKeyToPublicKeyHash(publicKey ecdh.PublicKey) ([]byte, bool) {
	pubKeyBytes := MarshalPublicKey(publicKey)
	if !IsValidPublicKey(pubKeyBytes) {
		logrus.Errorf("[%s] 无效的公钥", debug.WhereAmI())
		return nil, false
	}
	return HashPublicKey(pubKeyBytes), true
}

// PublicKeyToAddress 通过公钥生成钱包地址
// 参数:
//   - publicKey (ecdh.PublicKey): 公钥
//
// 返回值:
//   - string: 钱包地址
//   - bool: 是否成功
func PublicKeyToAddress(publicKey ecdh.PublicKey) (string, bool) {
	pubKeyBytes := MarshalPublicKey(publicKey)
	return PublicKeyBytesToAddress(pubKeyBytes)
}

// PublicKeyBytesToPublicKeyHash 通过公钥字节生成公钥哈希
// 参数:
//   - pubKeyBytes ([]byte): 公钥字节
//
// 返回值:
//   - []byte: 公钥哈希
//   - bool: 是否成功
func PublicKeyBytesToPublicKeyHash(pubKeyBytes []byte) ([]byte, bool) {
	if !IsValidPublicKey(pubKeyBytes) {
		logrus.Errorf("[%s] 无效的公钥字节", debug.WhereAmI())
		return nil, false
	}
	return HashPublicKey(pubKeyBytes), true
}

// PublicKeyBytesToAddress 通过公钥字节生成钱包地址
// 参数:
//   - pubKeyBytes ([]byte): 公钥字节
//
// 返回值:
//   - string: 钱包地址
//   - bool: 是否成功
func PublicKeyBytesToAddress(pubKeyBytes []byte) (string, bool) {
	if !IsValidPublicKey(pubKeyBytes) {
		logrus.Errorf("[%s] 无效的公钥字节", debug.WhereAmI())
		return "", false
	}
	pubKeyHash := HashPublicKey(pubKeyBytes)
	return PublicKeyHashToAddress(pubKeyHash)
}

// PublicKeyHashToAddress 通过公钥哈希生成钱包地址
// 参数:
//   - pubKeyHash ([]byte): 公钥哈希
//
// 返回值:
//   - string: 钱包地址
//   - bool: 是否成功
func PublicKeyHashToAddress(pubKeyHash []byte) (string, bool) {
	if !IsValidPublicKeyHash(pubKeyHash) {
		logrus.Errorf("[%s] 无效的公钥哈希字节", debug.WhereAmI())
		return "", false
	}
	return base58.CheckEncode(pubKeyHash, versionByte), true
}

// AddressToPublicKeyHash 通过钱包地址获取公钥哈希
// 参数:
//   - address (string): 钱包地址
//
// 返回值:
//   - []byte: 公钥哈希
//   - error: 失败时的错误信息
func AddressToPublicKeyHash(address string) ([]byte, error) {
	// 检查钱包地址是否有效
	if !IsValidAddress(address) {
		err := errors.New("无效的钱包地址")
		logrus.Errorf("[%s] 无效的钱包地址: %v", debug.WhereAmI(), err)
		return nil, err
	}
	payload, version, err := base58.CheckDecode(address)
	if err != nil {
		logrus.Errorf("[%s] 地址解码失败: %v", debug.WhereAmI(), err)
		return nil, err
	}
	if version != versionByte {
		err := errors.New("版本字节不匹配")
		logrus.Errorf("[%s] 版本字节不匹配: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return payload, nil
}

// IsValidAddress 检查钱包地址是否有效
// 参数:
//   - address (string): 钱包地址
//
// 返回值:
//   - bool: 地址是否有效的布尔值
func IsValidAddress(address string) bool {
	_, version, err := base58.CheckDecode(address)
	return err == nil && version == versionByte
}

// IsValidPublicKeyHash 检查公钥哈希是否有效
// 参数:
//   - pubKeyHash ([]byte): 公钥哈希
//
// 返回值:
//   - bool: 公钥哈希是否有效的布尔值
func IsValidPublicKeyHash(pubKeyHash []byte) bool {
	return len(pubKeyHash) == 20
}

// IsValidPublicKey 检查公钥字节是否有效
// 参数:
//   - pubKeyBytes ([]byte): 公钥字节
//
// 返回值:
//   - bool: 公钥是否有效的布尔值
func IsValidPublicKey(pubKeyBytes []byte) bool {
	_, err := UnmarshalPublicKey(pubKeyBytes)
	return err == nil
}

/////

// PublicKeyToAddress 从ECDSA公钥生成对应的钱包地址
// 参数:
//   - publicKey ([]byte): 输入的公钥字节
//
// 返回值:
//   - string: 生成的钱包地址
// func PublicKeyToAddress(publicKey []byte) string {
// 	if !isValidPublicKey(publicKey) {
// 		logrus.Errorf("[%s] 无效的公钥字节", debug.WhereAmI())
// 		return ""
// 	}
// 	pubKeyHash := HashPublicKey(publicKey)
// 	address := base58.CheckEncode(pubKeyHash, versionByte)
// 	return address
// }

// PublicKeyHashToAddress 从公钥哈希生成钱包地址
// 参数:
//   - pubKeyHash ([]byte): 公钥的哈希值
//
// 返回值:
//   - string: 生成的钱包地址
// func PublicKeyHashToAddress(pubKeyHash []byte) string {
// 	if !isValidPublicKeyHash(pubKeyHash) {
// 		logrus.Errorf("[%s] 无效的公钥哈希字节", debug.WhereAmI())
// 		return ""
// 	}
// 	address := base58.CheckEncode(pubKeyHash, versionByte)
// 	return address
// }

// AddressToPublicKeyHash 从Base58编码的地址中提取公钥哈希
// 参数:
//   - address (string): 输入的Base58编码的钱包地址
//
// 返回值:
//   - []byte: 从地址中提取的公钥哈希
//   - error: 失败时的错误信息
// func AddressToPublicKeyHash(address string) ([]byte, error) {
// 	if !ValidateAddress(address) {
// 		err := errors.New("无效的钱包地址")
// 		logrus.Errorf("[%s] 无效的钱包地址: %v", debug.WhereAmI(), err)
// 		return nil, err
// 	}
// 	payload, version, err := base58.CheckDecode(address)
// 	if err != nil {
// 		logrus.Errorf("[%s] 地址解码失败: %v", debug.WhereAmI(), err)
// 		return nil, err
// 	}
// 	if version != versionByte {
// 		err := errors.New("版本字节不匹配")
// 		logrus.Errorf("[%s] 版本字节不匹配: %v", debug.WhereAmI(), err)
// 		return nil, err
// 	}
// 	return payload, nil
// }

// ValidateAddress 检查地址是否有效
// 参数:
//   - address (string): 输入的钱包地址
//
// 返回值:
//   - bool: 地址是否有效的布尔值
// func ValidateAddress(address string) bool {
// 	_, version, err := base58.CheckDecode(address)
// 	return err == nil && version == versionByte
// }

// isValidPublicKey 验证公钥是否有效
// 参数:
//   - pubKeyBytes ([]byte): 输入的公钥字节
//
// 返回值:
//   - bool: 公钥是否有效的布尔值
// func isValidPublicKey(pubKeyBytes []byte) bool {
// 	_, err := UnmarshalPublicKey(pubKeyBytes)
// 	return err == nil
// }

// isValidPublicKeyHash 验证公钥哈希是否有效
// 参数:
//   - pubKeyHashBytes ([]byte): 输入的公钥哈希字节
//
// 返回值:
//   - bool: 公钥哈希是否有效的布尔值
// func isValidPublicKeyHash(pubKeyHashBytes []byte) bool {
// 	return len(pubKeyHashBytes) == 20
// }
