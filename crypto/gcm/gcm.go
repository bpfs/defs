package gcm

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

// EncryptData 使用给定的密钥对数据进行加密
// 参数：
//   - data: []byte 需要加密的数据。
//   - key: []byte 用于加密的密钥。
//
// 返回值：
//   - []byte: 加密后的数据。
//   - error: 如果发生错误，返回错误信息。
func EncryptData(data, key []byte) ([]byte, error) {
	// 创建新的 cipher.Block 实例，基于 AES 算法
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 cipher.Block 失败: %v", err)
	}

	// 创建 GCM 模式的 cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 模式失败: %v", err)
	}

	// 创建一个 nonce（仅用一次的随机数）
	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %v", err)
	}

	// 使用 GCM 模式进行加密
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// DecryptData 使用给定的密钥对数据进行解密
// 参数：
//   - ciphertext: []byte 需要解密的密文。
//   - key: []byte 用于解密的密钥。
//
// 返回值：
//   - []byte: 解密后的明文数据。
//   - error: 如果发生错误，返回错误信息。
func DecryptData(ciphertext, key []byte) ([]byte, error) {
	// 创建新的 cipher.Block 实例
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 cipher.Block 失败: %v", err)
	}

	// 创建 GCM 模式的 cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 模式失败: %v", err)
	}

	// 检查密文长度是否符合要求
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("非法的密文格式")
	}

	// 提取 nonce 和实际的密文
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	// 使用 GCM 模式进行解密
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %v", err)
	}

	return plaintext, nil
}
