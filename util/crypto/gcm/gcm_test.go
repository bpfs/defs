package gcm

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptDataWithAESGCM(t *testing.T) {
	key := []byte("mysecretkey12345") // 16字节的密钥
	plaintext := []byte("hello world")

	// 正常情况：加密和解密
	ciphertext, err := EncryptData(plaintext, key)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	decrypted, err := DecryptData(ciphertext, key)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("原文和解密后的文本不匹配：原文 %s, 解密后 %s", plaintext, decrypted)
	}

	// 错误情况1：密钥长度不正确
	wrongKey := []byte("wrongkey")
	_, err = EncryptData(plaintext, wrongKey)
	if err != nil {
		t.Fatalf("应当失败，因为密钥长度不正确")
	}

	// 错误情况2：解密使用了错误的密钥
	_, err = DecryptData(ciphertext, []byte("anotherwrongkey"))
	if err != nil {
		t.Fatalf("应当失败，因为解密使用了错误的密钥")
	}

	// 错误情况3：非法的密文格式
	_, err = DecryptData([]byte("short"), key)
	if err != nil {
		t.Fatalf("应当失败，因为密文格式不正确")
	}
}
