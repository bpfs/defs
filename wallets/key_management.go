// key_management.go
// 密钥管理

package wallets

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"hash"
	"math/big"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
	"github.com/tyler-smith/go-bip32"
	"golang.org/x/crypto/pbkdf2"
)

// 全局版本字节和曲线
var (
	versionByte = byte(0x00)  // 全局版本字节
	curve       = ecdh.P256() // 全局椭圆曲线
)

// GenerateECDSAKeyPair 从给定的种子生成ECDSA密钥对
// 参数:
//   - salt ([]byte): 加密操作的盐值
//   - password ([]byte): 用于生成密钥的密码
//   - iterations (int): PBKDF2算法的迭代次数
//   - keyLength (int): 生成的密钥长度
//   - useSHA512 (bool): 是否使用SHA512哈希函数
//
// 返回值:
//   - *ecdh.PrivateKey: 生成的私钥
//   - []byte: 公钥的字节表示
//   - error: 失败时的错误信息
func GenerateECDSAKeyPair(salt []byte, password []byte, iterations, keyLength int, useSHA512 bool) (*ecdh.PrivateKey, []byte, error) {
	// 根据useSHA512选择合适的哈希函数
	var hashFunc func() hash.Hash
	if useSHA512 {
		hashFunc = sha512.New
	} else {
		hashFunc = sha256.New
	}

	// 组合固定前缀和盐值
	combined := append([]byte("BPFS"), password...)

	// 使用PBKDF2算法生成强密钥
	// *** 这里不考虑Key的入参，因为早期salt,password位置颠倒，无法修正。
	key := pbkdf2.Key(salt, combined, iterations, keyLength, hashFunc)

	// 使用生成的密钥生成主钱包
	masterKey, err := bip32.NewMasterKey(key)
	if err != nil {
		logrus.Errorf("[%s] 生成主钱包失败: %v", utils.WhereAmI(), err)
		return nil, nil, err
	}

	// 使用crypto/ecdh生成ECDH私钥
	privateKey, err := ecdh.P256().NewPrivateKey(masterKey.Key)
	if err != nil {
		logrus.Errorf("[%s] 生成私钥失败: %v", utils.WhereAmI(), err)
		return nil, nil, err
	}

	// 从私钥派生公钥
	publicKey := privateKey.PublicKey()

	// 将公钥转换为字节表示，使用非压缩形式
	pubKey := publicKey.Bytes()

	// 返回私钥和公钥字节
	return privateKey, pubKey, nil
}

// ExtractPublicKey 从ECDSA私钥中提取公钥
// 参数:
//   - privateKey (*ecdh.PrivateKey): 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 提取的公钥的字节表示
func ExtractPublicKey(privateKey *ecdh.PrivateKey) []byte {
	// 从私钥派生公钥并转换为字节序列
	publicKey := privateKey.PublicKey()
	return publicKey.Bytes()
}

// MarshalPublicKey 将ECDSA公钥序列化为字节表示
// 参数:
//   - publicKey (ecdh.PublicKey): 输入的ECDH公钥
//
// 返回值:
//   - []byte: 公钥的字节序列
func MarshalPublicKey(publicKey ecdh.PublicKey) []byte {
	// 将公钥转换为字节序列
	return publicKey.Bytes()
}

// UnmarshalPublicKey 将字节序列反序列化为ECDSA公钥
// 参数:
//   - pubKeyBytes ([]byte): 公钥的字节表示
//
// 返回值:
//   - *ecdh.PublicKey: 反序列化后的ECDH公钥
//   - error: 失败时的错误信息
func UnmarshalPublicKey(pubKeyBytes []byte) (*ecdh.PublicKey, error) {
	// 从字节序列反序列化为公钥
	publicKey, err := curve.NewPublicKey(pubKeyBytes)
	if err != nil {
		logrus.Errorf("[%s] 无效的公钥字节: %v", utils.WhereAmI(), err)
		return &ecdh.PublicKey{}, err
	}

	return publicKey, nil
}

// MarshalPrivateKey 将ECDSA私钥序列化为字节表示
// 参数:
//   - privateKey (*ecdh.PrivateKey): 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 私钥的字节序列
//   - error: 失败时的错误信息
func MarshalPrivateKey(privateKey *ecdh.PrivateKey) ([]byte, error) {
	if privateKey == nil {
		err := errors.New("无效的私钥")
		logrus.Errorf("[%s] 无效的私钥: %v", utils.WhereAmI(), err)
		return nil, err
	}

	// 直接返回私钥的字节序列
	return privateKey.Bytes(), nil
}

// UnmarshalPrivateKey 将字节序列反序列化为ECDSA私钥
// 参数:
//   - privKeyBytes ([]byte): 私钥的字节表示
//
// 返回值:
//   - *ecdh.PrivateKey: 反序列化后的ECDSA私钥
//   - error: 失败时的错误信息
func UnmarshalPrivateKey(privKeyBytes []byte) (*ecdh.PrivateKey, error) {
	if len(privKeyBytes) == 0 {
		err := errors.New("无效的私钥字节")
		logrus.Errorf("[%s] 无效的私钥字节: %v", utils.WhereAmI(), err)
		return nil, err
	}

	privateKey, err := curve.NewPrivateKey(privKeyBytes)
	if err != nil {
		logrus.Errorf("[%s] 解析私钥失败: %v", utils.WhereAmI(), err)
		return nil, err
	}
	return privateKey, nil
}