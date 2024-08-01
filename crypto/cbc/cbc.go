package cbc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// Encrypt 使用 AES-CBC 模式和给定的密钥对明文进行加密
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
		return nil, err // 返回错误信息
	}

	// 对明文进行 PKCS7 填充，以使其长度为块大小的整数倍
	plaintext = pkcs7Padding(plaintext, aes.BlockSize)

	// 创建随机的初始向量 (IV)
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err // 返回错误信息
	}

	// 创建 CBC 模式的加密器
	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)

	// 使用 CBC 模式进行加密
	mode.CryptBlocks(ciphertext, plaintext)

	// 将初始向量和密文拼接在一起，返回结果
	return append(iv, ciphertext...), nil
}

// Decrypt 使用 AES-CBC 模式和给定的密钥对密文进行解密
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
		return nil, err // 返回错误信息
	}

	// 检查密文长度是否符合要求
	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("密文太短") // 返回错误信息
	}

	// 分离初始向量和密文
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	// 检查密文长度是否为块大小的整数倍
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("密文的长度不是块大小的整数倍") // 返回错误信息
	}

	// 创建 CBC 模式的解密器
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)

	// 使用 CBC 模式进行解密
	mode.CryptBlocks(plaintext, ciphertext)

	// 去除 PKCS7 填充，返回结果
	return pkcs7Unpadding(plaintext), nil
}

// pkcs7Padding 对明文进行 PKCS7 填充
// 参数：
//   - plaintext: []byte 需要填充的明文数据。
//   - blockSize: int 块大小。
//
// 返回值：
//   - []byte: 填充后的明文数据。
func pkcs7Padding(plaintext []byte, blockSize int) []byte {
	// 计算需要填充的字节数
	padding := blockSize - len(plaintext)%blockSize
	// 创建填充字节
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	// 将填充字节追加到明文数据后，返回结果
	return append(plaintext, padtext...)
}

// pkcs7Unpadding 移除明文的 PKCS7 填充
// 参数：
//   - plaintext: []byte 需要移除填充的明文数据。
//
// 返回值：
//   - []byte: 移除填充后的明文数据。
func pkcs7Unpadding(plaintext []byte) []byte {
	// 获取最后一个字节的值，作为填充字节数
	length := len(plaintext)
	unpadding := int(plaintext[length-1])
	// 移除填充字节，返回结果
	return plaintext[:(length - unpadding)]
}
