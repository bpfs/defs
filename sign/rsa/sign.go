package rsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"log"
	"math/big"
	mathrand "math/rand"
)

// GenerateKeysFromSeed 使用种子数据生成 RSA 密钥对
// 参数:
//   - seedData: 用于生成密钥的种子数据
//   - bits: RSA 密钥的位数
//
// 返回值:
//   - *rsa.PrivateKey: 生成的私钥
//   - *rsa.PublicKey: 生成的公钥
//   - error: 如果生成过程中出现错误，返回相应的错误信息
func GenerateKeysFromSeed(seedData []byte, bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// 将种子数据转换为 int64 类型的种子
	seed := big.NewInt(0).SetBytes(seedData).Int64()
	// 使用种子创建自定义的随机数生成器
	customRand := mathrand.New(mathrand.NewSource(seed))
	// 使用自定义随机数生成器生成 RSA 私钥
	privateKey, err := rsa.GenerateKey(customRand, bits)
	if err != nil {
		log.Printf("生成 RSA 密钥对时出错: %v", err)
		return nil, nil, err
	}
	// 从私钥中获取公钥
	publicKey := &privateKey.PublicKey
	return privateKey, publicKey, nil
}

// PublicKeyToBytes 使用 x509 标准将公钥转换为字节
// 参数:
//   - pubKey: 要转换的 RSA 公钥
//
// 返回值:
//   - []byte: 转换后的公钥字节
//   - error: 如果转换过程中出现错误，返回相应的错误信息
func PublicKeyToBytes(pubKey *rsa.PublicKey) ([]byte, error) {
	// 使用 x509 标准将公钥转换为字节
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		log.Printf("将公钥转换为字节时出错: %v", err)
		return nil, err
	}
	return pubKeyBytes, nil
}

// SignData 使用 RSA 私钥为数据签名
// 参数:
//   - privateKey: 用于签名的 RSA 私钥
//   - data: 要签名的数据
//
// 返回值:
//   - []byte: 生成的签名
//   - error: 如果签名过程中出现错误，返回相应的错误信息
func SignData(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	// 计算数据的 SHA256 哈希值
	hashed := sha256.Sum256(data)
	// 使用 RSASSA-PKCS1-V1_5-SIGN 算法对哈希值进行签名
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		log.Printf("签名数据时出错: %v", err)
		return nil, err
	}
	return signature, nil
}

// VerifySignature 使用 RSA 公钥验证数据的签名
// 参数:
//   - publicKey: 用于验证签名的 RSA 公钥
//   - data: 原始数据
//   - signature: 要验证的签名
//
// 返回值:
//   - bool: 如果签名有效返回 true，否则返回 false
func VerifySignature(publicKey *rsa.PublicKey, data []byte, signature []byte) bool {
	// 计算数据的 SHA256 哈希值
	hashed := sha256.Sum256(data)
	// 验证 RSASSA-PKCS1-V1_5 签名
	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
	return err == nil
}
