package shamir

import (
	"bytes"
	"encoding/gob"
	"math/big"
)

// ShamirScheme 封装Shamir秘密共享方案的配置信息
type ShamirScheme struct {
	TotalShares int      // 总份额数
	Threshold   int      // 需要恢复秘密的最小份额数
	Prime       *big.Int // 用于运算的素数
}

// NewShamirScheme 创建一个新的ShamirScheme实例
// 参数：
//   - totalShares: int 总份额数
//   - threshold: int 需要恢复秘密的最小份额数
//   - prime: *big.Int 用于运算的素数
//
// 返回值：
//   - *ShamirScheme 新创建的ShamirScheme实例
func NewShamirScheme(totalShares, threshold int, prime *big.Int) *ShamirScheme {
	return &ShamirScheme{
		TotalShares: totalShares, // 初始化总份额数
		Threshold:   threshold,   // 初始化恢复秘密的最小份额数
		Prime:       prime,       // 初始化用于运算的素数
	}
}

// GenerateShares 生成含有固定份额的秘密份额
// func (s *ShamirScheme) GenerateShares(secret []byte) ([][]byte, error) {
// 	shares, err := GenerateStandardShares(secret, s.TotalShares, s.Threshold)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// 打印所有生成的份额
// 	logrus.Printf("生成的所有密钥分片:\n")
// 	for i, share := range shares {
// 		logrus.Printf("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
// 	}

// 	// 将分享转换为[][]byte格式
// 	sharesBytes := make([][]byte, len(shares))
// 	for i, share := range shares {
// 		sharesBytes[i] = append(share[0].Bytes(), share[1].Bytes()...)
// 	}

// 	return sharesBytes, nil
// }

// // RecoverSecretFromShares 从给定的份额恢复秘密
// func (s *ShamirScheme) RecoverSecretFromShares(sharesBytes ...[]byte) ([]byte, error) {
// 	shares := make([][2]*big.Int, len(sharesBytes))
// 	for i, bytes := range sharesBytes {
// 		// 假设每个big.Int都是固定长度，例如根据素数的长度
// 		byteLen := 1
// 		xBytes := bytes[:byteLen]
// 		yBytes := bytes[byteLen:]

// 		shares[i] = [2]*big.Int{new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes)}
// 	}

// 	// 打印用于恢复的份额，确保它们正确
// 	logrus.Printf("用于恢复的密钥分片:\n")
// 	for i, share := range shares {
// 		logrus.Printf("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
// 	}

// 	return RecoverSecret(shares, s.Prime)
// }

// GenerateShares 方法改进，使用 gob 编码
// 参数：
//   - secret: []byte 要分割的秘密
//
// 返回值：
//   - [][]byte 生成的秘密份额
//   - error 可能的错误
func (s *ShamirScheme) GenerateShares(secret []byte) ([][]byte, error) {
	// 使用GenerateStandardShares生成秘密份额
	shares, err := GenerateStandardShares(secret, s.TotalShares, s.Threshold, s.Prime)
	if err != nil {
		return nil, err
	}

	// 打印所有生成的份额
	// logrus.Printf("生成的所有密钥分片:\n")
	// for i, share := range shares {
	// 	logrus.Printf("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
	// }

	// 对每个份额进行gob编码
	var sharesBytes [][]byte
	for _, share := range shares {
		var buf bytes.Buffer
		encoder := gob.NewEncoder(&buf)
		if err := encoder.Encode(share); err != nil {
			return nil, err
		}
		sharesBytes = append(sharesBytes, buf.Bytes())
	}

	return sharesBytes, nil
}

// RecoverSecretFromShares 方法改进，使用 gob 解码
// 参数：
//   - sharesBytes: ...[]byte 用于恢复秘密的份额集合
//
// 返回值：
//   - []byte 恢复的秘密
//   - error 可能的错误
func (s *ShamirScheme) RecoverSecretFromShares(sharesBytes ...[]byte) ([]byte, error) {
	// 解码gob编码的份额
	var shares [][2]*big.Int
	for _, byte := range sharesBytes {
		var share [2]*big.Int
		buf := bytes.NewBuffer(byte)
		decoder := gob.NewDecoder(buf)
		if err := decoder.Decode(&share); err != nil {
			return nil, err
		}

		shares = append(shares, share)
	}

	// 打印用于恢复的份额，确保它们正确
	// logrus.Printf("用于恢复的密钥分片:\n")
	// for i, share := range shares {
	// 	logrus.Printf("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
	// }

	// 使用RecoverSecret从份额中恢复秘密
	return RecoverSecret(shares, s.Prime)
}
