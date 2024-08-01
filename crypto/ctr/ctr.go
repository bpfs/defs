package ctr

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

/**
AES-CTR
仅加密: 不提供完整性和来源验证，只提供数据加密。
速度: CTR模式也很快，但在没有硬件支持的情况下，可能没有GCM快。
初始向量（IV）: 使用 aes.BlockSize 长度的初始向量。
无附加数据（AAD）支持: 不支持附加数据。
*/

// Encrypt 使用 AES-CTR 模式和给定的密钥对明文进行加密
// 参数：
//   - key: []byte 用于加密的密钥。
//   - plaintext: []byte 需要加密的明文数据。
//
// 返回值：
//   - []byte: 加密后的数据，包括初始向量和密文。
//   - error: 如果发生错误，返回错误信息。
func Encrypt(key, plaintext []byte) ([]byte, error) {
	// 创建 cipher.Block 实例，基于 AES 算法
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 cipher.Block 失败: %v", err)
	}

	// 创建随机的初始向量 (IV)
	iv := make([]byte, aes.BlockSize)
	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		return nil, fmt.Errorf("生成初始向量 (IV) 失败: %v", err)
	}

	// 创建 CTR 模式的加密器
	stream := cipher.NewCTR(block, iv)

	// 进行加密
	ciphertext := make([]byte, len(plaintext))
	stream.XORKeyStream(ciphertext, plaintext)

	// 将初始向量和密文拼接在一起
	result := make([]byte, aes.BlockSize+len(ciphertext))
	copy(result, iv)
	copy(result[aes.BlockSize:], ciphertext)

	return result, nil
}

// Decrypt 使用 AES-CTR 模式和给定的密钥对密文进行解密
// 参数：
//   - key: []byte 用于解密的密钥。
//   - ciphertext: []byte 需要解密的密文数据，包括初始向量和密文。
//
// 返回值：
//   - []byte: 解密后的明文数据。
//   - error: 如果发生错误，返回错误信息。
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	// 创建 cipher.Block 实例，基于 AES 算法
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 cipher.Block 失败: %v", err)
	}

	// 检查密文长度是否符合要求
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("非法的密文格式")
	}

	// 提取初始向量和实际的密文
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	// 创建 CTR 模式的解密器
	stream := cipher.NewCTR(block, iv)

	// 进行解密
	plaintext := make([]byte, len(ciphertext))
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}
