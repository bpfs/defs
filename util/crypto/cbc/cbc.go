package cbc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// Encrypt 使用AES-CBC模式和给定的密钥对明文进行加密
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 明文需要填充到块大小的整数倍
	plaintext = pkcs7Padding(plaintext, aes.BlockSize)

	// 创建初始向量
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	// 加密
	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)

	// 将初始向量和密文拼接
	return append(iv, ciphertext...), nil
}

// Decrypt 使用AES-CBC模式和给定的密钥对密文进行解密
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("密文太短")
	}

	// 分离初始向量和密文
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("密文的长度不是块大小的整数倍")
	}

	// 解密
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// 去除填充
	return pkcs7Unpadding(plaintext), nil
}

// pkcs7Padding 对明文进行PKCS7填充
func pkcs7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

// pkcs7Unpadding 移除明文的PKCS7填充
func pkcs7Unpadding(plaintext []byte) []byte {
	length := len(plaintext)
	unpadding := int(plaintext[length-1])
	return plaintext[:(length - unpadding)]
}
