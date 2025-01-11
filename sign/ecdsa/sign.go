package ecdsa

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"math/big"

	logging "github.com/dep2p/log"
)

var logger = logging.Logger("ecdsa")

// SignData 使用ECDSA私钥为数据签名
// 参数:
//   - privateKey: ECDSA私钥
//   - data: 待签名的数据
//
// 返回值:
//   - []byte: ASN.1格式的签名
//   - error: 错误信息
func SignData(privateKey *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	// 计算数据的SHA256哈希值
	hashed := sha256.Sum256(data)

	// 使用私钥对哈希值进行签名
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hashed[:])
	if err != nil {
		logger.Error("签名失败", "错误", err)
		return nil, err
	}

	// 将r和s值编码为ASN.1格式的签名
	signature, err := marshalECDSASignature(r, s)
	if err != nil {
		logger.Error("签名编码失败", "错误", err)
		return nil, err
	}

	// logger.Info("签名成功")
	return signature, nil
}

// VerifySignature 使用ECDSA公钥验证数据的签名
// 参数:
//   - publicKey: ECDSA公钥
//   - data: 原始数据
//   - signature: ASN.1格式的签名
//
// 返回值:
//   - bool: 签名是否有效
//   - error: 错误信息
func VerifySignature(publicKey *ecdsa.PublicKey, data []byte, signature []byte) (bool, error) {
	// 计算数据的SHA256哈希值
	hashed := sha256.Sum256(data)

	// 解码ASN.1格式的签名
	r, s, err := unmarshalECDSASignature(signature)
	if err != nil {
		logger.Error("签名解码失败", "错误", err)
		return false, err
	}

	// 验证签名
	valid := ecdsa.Verify(publicKey, hashed[:], r, s)
	if valid {
		logger.Info("签名验证成功")
	} else {
		logger.Warn("签名验证失败")
	}
	return valid, nil
}

// marshalECDSASignature 将ECDSA签名的r和s值编码为ASN.1格式
// 参数:
//   - r, s: ECDSA签名的r和s值
//
// 返回值:
//   - []byte: ASN.1格式的签名
//   - error: 错误信息
func marshalECDSASignature(r, s *big.Int) ([]byte, error) {
	return asn1.Marshal(ecdsaSignature{r, s})
}

// unmarshalECDSASignature 将ASN.1格式的签名解码为ECDSA的r和s值
// 参数:
//   - signature: ASN.1格式的签名
//
// 返回值:
//   - *big.Int: r值
//   - *big.Int: s值
//   - error: 错误信息
func unmarshalECDSASignature(signature []byte) (*big.Int, *big.Int, error) {
	var sig ecdsaSignature
	_, err := asn1.Unmarshal(signature, &sig)
	if err != nil {
		logger.Error("签名解码失败", "错误", err)
		return nil, nil, err
	}

	// 检查r和s值是否为正数
	if sig.R.Sign() <= 0 || sig.S.Sign() <= 0 {
		err := errors.New("ECDSA 签名包含零或负值")
		logger.Error("无效的签名值", "错误", err)
		return nil, nil, err
	}

	return sig.R, sig.S, nil
}

// ecdsaSignature 是用于ASN.1编码的ECDSA签名结构
type ecdsaSignature struct {
	R, S *big.Int
}
