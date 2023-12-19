package defs

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

/**
安全性:
	如果你需要完整性和/或来源验证，GCM通常是更好的选择。
易用性:
	GCM更难以错误地使用，因为它为你处理了更多的安全性问题（例如认证）。
性能:
	如果硬件支持AES-NI指令集，GCM通常更快。
*/

/**
AES-GCM
认证加密（Authenticated Encryption）: 提供数据的加密（保密性）、完整性和来源验证。
速度: 通常来说，AES-GCM在硬件支持下运行得非常快。
标准随机数长度（Nonce Size）: gcm.NonceSize() 用于生成随机数（Nonce）。
附加数据（AAD）: 支持无需加密但需要验证其完整性的附加数据。
*/

// encryptData 使用给定的密钥对数据进行加密
func encryptData(data, key []byte) ([]byte, error) {
	// 创建新的cipher.Block实例，基于AES算法
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建cipher.Block失败: " + err.Error())
	}

	// 创建GCM模式的cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM模式失败: " + err.Error())
	}

	// 创建一个nonce（仅用一次的随机数）
	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, fmt.Errorf("生成nonce失败: " + err.Error())
	}

	// 使用GCM模式进行加密
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decryptData 使用给定的密钥对数据进行解密
func decryptData(ciphertext, key []byte) ([]byte, error) {
	// 创建新的cipher.Block实例
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建cipher.Block失败: " + err.Error())
	}

	// 创建GCM模式的cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM模式失败: " + err.Error())
	}

	// 检查密文长度是否符合要求
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("非法的密文格式")
	}

	// 提取nonce和实际的密文
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	// 使用GCM模式进行解密
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败: " + err.Error())
	}

	return plaintext, nil
}

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
