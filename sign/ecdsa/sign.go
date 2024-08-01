package ecdsa

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"math/big"
)

// SignData 使用ECDSA私钥为数据签名
func SignData(privateKey *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	hashed := sha256.Sum256(data)

	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hashed[:])
	if err != nil {
		return nil, err
	}

	// 将r和s值编码为ASN.1格式的签名
	signature, err := marshalECDSASignature(r, s)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// VerifySignature 使用ECDSA公钥验证数据的签名
func VerifySignature(publicKey *ecdsa.PublicKey, data []byte, signature []byte) (bool, error) {
	hashed := sha256.Sum256(data)

	r, s, err := unmarshalECDSASignature(signature)
	if err != nil {
		return false, err
	}

	valid := ecdsa.Verify(publicKey, hashed[:], r, s)
	return valid, nil
}

// marshalECDSASignature 将ECDSA签名的r和s值编码为ASN.1格式
func marshalECDSASignature(r, s *big.Int) ([]byte, error) {
	return asn1.Marshal(ecdsaSignature{r, s})
}

// unmarshalECDSASignature 将ASN.1格式的签名解码为ECDSA的r和s值
func unmarshalECDSASignature(signature []byte) (*big.Int, *big.Int, error) {
	var sig ecdsaSignature
	_, err := asn1.Unmarshal(signature, &sig)
	if err != nil {
		return nil, nil, err
	}

	if sig.R.Sign() <= 0 || sig.S.Sign() <= 0 {
		return nil, nil, errors.New("ECDSA 签名包含零或负值")
	}

	return sig.R, sig.S, nil
}

// ecdsaSignature 是用于ASN.1编码的ECDSA签名结构
type ecdsaSignature struct {
	R, S *big.Int
}
