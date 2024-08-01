// key_management.go
// 密钥管理

package wallets

import (
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
	versionByte = byte(0x00)      // 全局版本字节
	curve       = elliptic.P256() // 全局椭圆曲线
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
//   - *ecdsa.PrivateKey: 生成的私钥
//   - []byte: 公钥的字节表示
//   - error: 失败时的错误信息
func GenerateECDSAKeyPair(salt []byte, password []byte, iterations, keyLength int, useSHA512 bool) (*ecdsa.PrivateKey, []byte, error) {
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
		logrus.Errorf("[%s] 生成主钱包失败: %v", debug.WhereAmI(), err)
		return nil, nil, err
	}

	// 构造ECDSA私钥
	privateKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
		},
		D: new(big.Int).SetBytes(masterKey.Key),
	}

	// 计算公钥
	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(masterKey.Key)

	// 将公钥转换为字节表示，使用非压缩形式
	pubKey := elliptic.MarshalCompressed(privateKey.PublicKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)

	// 返回私钥和公钥字节
	return privateKey, pubKey, nil
}

// ExtractPublicKey 从ECDSA私钥中提取公钥
// 参数:
//   - privateKey (*ecdsa.PrivateKey): 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 提取的公钥的字节表示
func ExtractPublicKey(privateKey *ecdsa.PrivateKey) []byte {
	// 将私钥中的公钥转换为字节序列
	return elliptic.MarshalCompressed(privateKey.Curve, privateKey.X, privateKey.Y)
}

// MarshalPublicKey 将ECDSA公钥序列化为字节表示
// 参数:
//   - publicKey (ecdsa.PublicKey): 输入的ECDSA公钥
//
// 返回值:
//   - []byte: 公钥的字节序列
func MarshalPublicKey(publicKey ecdsa.PublicKey) []byte {
	// 将公钥转换为字节序列
	return elliptic.MarshalCompressed(publicKey.Curve, publicKey.X, publicKey.Y)
}

// UnmarshalPublicKey 将字节序列反序列化为ECDSA公钥
// 参数:
//   - pubKeyBytes ([]byte): 公钥的字节表示
//
// 返回值:
//   - ecdsa.PublicKey: 反序列化后的ECDSA公钥
//   - error: 失败时的错误信息
func UnmarshalPublicKey(pubKeyBytes []byte) (ecdsa.PublicKey, error) {
	// 使用全局曲线
	x, y := elliptic.Unmarshal(curve, pubKeyBytes)
	if x == nil || y == nil {
		err := errors.New("无效的公钥字节")
		logrus.Errorf("[%s] 无效的公钥字节: %v", debug.WhereAmI(), err)
		return ecdsa.PublicKey{}, err
	}
	// 返回反序列化后的公钥
	return ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// MarshalPrivateKey 将ECDSA私钥序列化为字节表示
// 参数:
//   - privateKey (*ecdsa.PrivateKey): 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 私钥的字节序列
//   - error: 失败时的错误信息
func MarshalPrivateKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	if privateKey == nil || privateKey.D == nil {
		err := errors.New("无效的私钥")
		logrus.Errorf("[%s] 无效的私钥: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return privateKey.D.Bytes(), nil
}

// UnmarshalPrivateKey 将字节序列反序列化为ECDSA私钥
// 参数:
//   - privKeyBytes ([]byte): 私钥的字节表示
//
// 返回值:
//   - *ecdsa.PrivateKey: 反序列化后的ECDSA私钥
//   - error: 失败时的错误信息
func UnmarshalPrivateKey(privKeyBytes []byte) (*ecdsa.PrivateKey, error) {
	if len(privKeyBytes) == 0 {
		err := errors.New("无效的私钥字节")
		logrus.Errorf("[%s] 无效的私钥字节: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// PEM解码
	privBlock, _ := pem.Decode(privKeyBytes)
	if privBlock == nil {
		err := errors.New("私钥PEM解码失败")
		logrus.Errorf("[%s] 私钥PEM解码失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 解析ASN.1 DER格式的私钥
	privateKey, err := x509.ParseECPrivateKey(privBlock.Bytes)
	if err != nil {
		logrus.Errorf("[%s] 解析私钥失败: %v", debug.WhereAmI(), err)
		return nil, err
	}
	return privateKey, nil
}
