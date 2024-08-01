package shamir

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestGenerateShares(t *testing.T) {
	// 定义或生成prime
	prime, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	scheme := NewShamirScheme(5, 3, prime)

	// 秘密和固定份额
	secret := []byte("Hello, Shamir Secret Sharing!")

	// 生成份额
	shares, err := scheme.GenerateShares(secret)
	if err != nil {
		panic(err)
	}

	for i, share := range shares {
		logrus.Printf("#%d: %s\n", i, hex.EncodeToString(share))
	}

	// 使用前三个份额恢复秘密
	recoveredSecret, err := scheme.RecoverSecretFromShares(shares[0], shares[1], shares[2])
	if err != nil {
		panic(err)
	}

	logrus.Println("Recovered secret:", string(recoveredSecret))
}
