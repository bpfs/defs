package files

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"errors"

	"github.com/bpfs/defs/utils/logger"
	"golang.org/x/crypto/ripemd160"
)

// PrivateKeyToPublicKeyHash 通过私钥生成公钥哈希
//
// 参数:
//   - privateKey *ecdsa.PrivateKey: 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 生成的公钥哈希
//   - bool: 操作是否成功
func PrivateKeyToPublicKeyHash(privateKey *ecdsa.PrivateKey) ([]byte, bool) {
	// 检查私钥是否为空
	if privateKey == nil {
		logger.Error("私钥不能为空")
		return nil, false
	}

	// 从私钥中提取公钥
	publicKey := ExtractPublicKey(privateKey)

	// 对公钥进行哈希处理并返回结果
	return HashPublicKey(publicKey), true
}

// PrivateKeyBytesToPublicKeyHash 通过私钥字节生成公钥哈希
//
// 参数:
//   - privateKeyBytes []byte: 输入的私钥字节
//
// 返回值:
//   - []byte: 生成的公钥哈希
//   - error: 如果在处理过程中发生错误，返回相应的错误信息
func PrivateKeyBytesToPublicKeyHash(privateKeyBytes []byte) ([]byte, error) {
	// 检查私钥是否为空
	if privateKeyBytes == nil {
		logger.Error("私钥不能为空")
		return nil, errors.New("私钥不能为空")
	}

	// 将私钥字节反序列化为ECDSA私钥
	privateKey, err := UnmarshalPrivateKey(privateKeyBytes)
	if err != nil {
		logger.Errorf("反序列化私钥失败: %v", err)
		return nil, err
	}

	// 使用现有的PrivateKeyToPublicKeyHash方法生成公钥哈希
	pubKeyHash, success := PrivateKeyToPublicKeyHash(privateKey)
	if !success {
		logger.Error("生成公钥哈希失败")
		return nil, errors.New("生成公钥哈希失败")
	}

	return pubKeyHash, nil
}

// ExtractPublicKey 从ECDSA私钥中提取公钥
//
// 参数:
//   - privateKey *ecdsa.PrivateKey: 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 提取的公钥的字节表示
func ExtractPublicKey(privateKey *ecdsa.PrivateKey) []byte {
	// 使用椭圆曲线的MarshalCompressed方法将公钥转换为压缩格式的字节序列
	// return elliptic.MarshalCompressed(privateKey.Curve, privateKey.X, privateKey.Y)
	return elliptic.Marshal(privateKey.Curve, privateKey.X, privateKey.Y)
}

// HashPublicKey 将公钥字节进行SHA-256和RIPEMD-160双重哈希
//
// 参数:
//   - pubKeyBytes []byte: 公钥的字节表示
//
// 返回值:
//   - []byte: 公钥的双重哈希值
func HashPublicKey(pubKeyBytes []byte) []byte {
	// 对公钥字节进行SHA-256哈希
	shaHash := sha256.Sum256(pubKeyBytes)

	// 创建RIPEMD-160哈希器
	ripeHasher := ripemd160.New()

	// 将SHA-256哈希结果写入RIPEMD-160哈希器
	ripeHasher.Write(shaHash[:])

	// 计算并返回RIPEMD-160哈希值
	return ripeHasher.Sum(nil)
}

// MarshalPublicKey 将ECDSA公钥序列化为字节表示
//
// 参数:
//   - publicKey ecdsa.PublicKey: 输入的ECDSA公钥
//
// 返回值:
//   - []byte: 公钥的字节序列
func MarshalPublicKey(publicKey ecdsa.PublicKey) []byte {
	// 将公钥转换为压缩格式的字节序列
	return elliptic.MarshalCompressed(publicKey.Curve, publicKey.X, publicKey.Y)
}

// UnmarshalPublicKey 将字节序列反序列化为ECDSA公钥
//
// 参数:
//   - pubKeyBytes []byte: 公钥的字节表示
//
// 返回值:
//   - ecdsa.PublicKey: 反序列化后的ECDSA公钥
//   - error: 失败时的错误信息
func UnmarshalPublicKey(pubKeyBytes []byte) (ecdsa.PublicKey, error) {
	// 使用P-256曲线反序列化公钥
	x, y := elliptic.Unmarshal(elliptic.P256(), pubKeyBytes)
	if x == nil || y == nil {
		err := errors.New("无效的公钥字节")
		logger.Errorf("无效的公钥字节: %v", err)
		return ecdsa.PublicKey{}, err
	}
	// 返回反序列化后的公钥
	return ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

// MarshalPrivateKey 将ECDSA私钥序列化为字节表示
//
// 参数:
//   - privateKey *ecdsa.PrivateKey: 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 私钥的字节序列
//   - error: 失败时的错误信息
func MarshalPrivateKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	// 检查私钥是否为空
	if privateKey == nil {
		err := errors.New("无效的私钥")
		logger.Errorf("无效的私钥: %v", err)
		return nil, err
	}
	// 将私钥序列化为ASN.1 DER格式
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		logger.Errorf("%v", err)
		return nil, err
	}
	return privateKeyBytes, nil
}

// UnmarshalPrivateKey 将字节序列反序列化为ECDSA私钥
//
// 参数:
//   - privKeyBytes []byte: 私钥的字节表示
//
// 返回值:
//   - *ecdsa.PrivateKey: 反序列化后的ECDSA私钥
//   - error: 失败时的错误信息
func UnmarshalPrivateKey(privKeyBytes []byte) (*ecdsa.PrivateKey, error) {
	// 检查私钥字节是否为空
	if len(privKeyBytes) == 0 {
		err := errors.New("无效的私钥字节")
		logger.Errorf("无效的私钥字节: %v", err)
		return nil, err
	}

	// 解析ASN.1 DER格式的私钥
	privateKey, err := x509.ParseECPrivateKey(privKeyBytes)
	if err != nil {
		logger.Errorf("解析私钥失败: %v", err)
		return nil, err
	}
	return privateKey, nil
}
