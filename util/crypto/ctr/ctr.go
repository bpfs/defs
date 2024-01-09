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

// Encrypt 使用AES-CTR模式和给定的密钥对明文进行加密
func Encrypt(key, plaintext []byte) ([]byte, error) {
	// 创建cipher.Block实例
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建cipher.Block失败: " + err.Error())
	}

	// 创建随机的初始向量(IV)
	iv := make([]byte, aes.BlockSize)
	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		return nil, fmt.Errorf("生成初始向量(IV)失败: " + err.Error())
	}

	// 创建CTR模式的加密器
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

// Decrypt 使用AES-CTR模式和给定的密钥对密文进行解密
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	// 创建cipher.Block实例
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建cipher.Block失败: " + err.Error())
	}

	// 检查密文长度是否符合要求
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("非法的密文格式")
	}

	// 提取初始向量和实际的密文
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	// 创建CTR模式的解密器
	stream := cipher.NewCTR(block, iv)

	// 进行解密
	plaintext := make([]byte, len(ciphertext))
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}
