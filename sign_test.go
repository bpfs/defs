package defs

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSignData(t *testing.T) {
	// 生成密钥对
	seedData := []byte("your_seed_data_here")
	privateKey, publicKey, err := GenerateKeysFromSeed(seedData, 2048)
	if err != nil {
		t.Error("Error generating keys:", err)
		return
	}

	// 待签名数据
	data := []byte("Hello, World!")

	// 签名
	signature, err := signData(privateKey, data)
	if err != nil {
		t.Error("Error signing data:", err)
		return
	}

	// 验证签名
	isVerified := verifySignature(publicKey, data, signature)
	if isVerified {
		logrus.Printf("Signature verified.")
	} else {
		t.Error("Signature verification failed.")
	}
}
