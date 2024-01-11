package base58

import (
	"crypto/sha256"
	"errors"
)

// ErrChecksum 表示校验编码字符串的校验和不与校验和进行验证。
var ErrChecksum = errors.New("校验和错误")

// ErrInvalidFormat 表示校验编码字符串的格式无效。
var ErrInvalidFormat = errors.New("格式无效：版本和/或校验和字节丢失")

// checksum: 对数据执行两次SHA256哈希（sha256^2）后的前四个字节
func checksum(input []byte) (cksum [4]byte) {
	h := sha256.Sum256(input)
	h2 := sha256.Sum256(h[:])
	copy(cksum[:], h2[:4])
	return
}

// CheckEncode 前置一个版本字节并附加一个四字节校验和。
func CheckEncode(input []byte, version byte) string {
	b := make([]byte, 0, 1+len(input)+4)
	b = append(b, version)
	b = append(b, input...)
	cksum := checksum(b)
	b = append(b, cksum[:]...)
	return Encode(b)
}

// CheckDecode 解码使用 CheckEncode 编码的字符串并验证校验和。
func CheckDecode(input string) (result []byte, version byte, err error) {
	// 将修改后的 base58 字符串解码为字节切片
	decoded := Decode(input)
	if len(decoded) < 5 {
		return nil, 0, ErrInvalidFormat
	}

	// 检查并删除校验和
	version = decoded[0]
	var cksum [4]byte
	copy(cksum[:], decoded[len(decoded)-4:])
	if checksum(decoded[:len(decoded)-4]) != cksum {
		return nil, 0, ErrChecksum
	}
	payload := decoded[1 : len(decoded)-4]
	result = append(result, payload...)
	return
}
