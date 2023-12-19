package defs

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
	mathrand "math/rand"
)

// GenerateKeysFromSeed 使用种子数据生成 RSA 密钥对
func GenerateKeysFromSeed(seedData []byte, bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	seed := big.NewInt(0).SetBytes(seedData).Int64()
	mathrand.Seed(seed)
	customRand := mathrand.New(mathrand.NewSource(seed))
	privateKey, err := rsa.GenerateKey(customRand, bits)
	if err != nil {
		return nil, nil, err
	}
	publicKey := &privateKey.PublicKey
	return privateKey, publicKey, nil
}

// 使用RSA私钥为数据签名
func signData(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	// Sum256 返回数据的 SHA256 校验和。
	hashed := sha256.Sum256(data)
	// SignPKCS1v15 使用 RSA PKCS #1 v1.5 中的 RSASSA-PKCS1-V1_5-SIGN 计算散列签名。
	// 请注意，散列必须是使用给定散列函数对输入消息进行散列的结果。 如果哈希值为零，则直接对哈希值进行签名。 除了互操作性之外，这是不可取的。
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, err
	}
	return signature, nil
}

// 使用RSA公钥验证数据的签名
func verifySignature(publicKey *rsa.PublicKey, data []byte, signature []byte) bool {
	hashed := sha256.Sum256(data)
	// verifyPKCS1v15 验证 RSA PKCS #1 v1.5 签名。
	// hashed 是使用给定哈希函数对输入消息进行哈希处理的结果，sig 是签名。 返回零错误表明签名有效。 如果哈希值为零，则直接使用哈希值。 除了互操作性之外，这是不可取的。
	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
	return err == nil
}
