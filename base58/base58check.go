package base58

import (
	"crypto/sha256"
	"errors"
)

// ErrChecksum 表示校验编码字符串的校验和不与校验和进行验证。
var ErrChecksum = errors.New("校验和错误")

// ErrInvalidFormat 表示校验编码字符串的格式无效。
var ErrInvalidFormat = errors.New("格式无效：版本和/或校验和字节丢失")

// checksum 对数据执行两次 SHA256 哈希（sha256^2）后的前四个字节。
// 参数：
//   - input: 输入的字节切片
//
// 返回：
//   - cksum: 校验和的前四个字节
func checksum(input []byte) (cksum [4]byte) {
	// 执行第一次 SHA256 哈希
	h := sha256.Sum256(input)
	// 执行第二次 SHA256 哈希
	h2 := sha256.Sum256(h[:])
	// 复制前四个字节到 cksum
	copy(cksum[:], h2[:4])
	return
}

// CheckEncode 前置一个版本字节并附加一个四字节校验和。
// 参数：
//   - input: 输入的字节切片
//   - version: 版本字节
//
// 返回：
//   - 使用 base58 编码的字符串
func CheckEncode(input []byte, version byte) string {
	// 创建一个字节切片，容量为 1+len(input)+4
	b := make([]byte, 0, 1+len(input)+4)
	// 添加版本字节
	b = append(b, version)
	// 添加输入数据
	b = append(b, input...)
	// 计算校验和
	cksum := checksum(b)
	// 添加校验和
	b = append(b, cksum[:]...)
	// 使用 base58 编码并返回
	return Encode(b)
}

// CheckDecode 解码使用 CheckEncode 编码的字符串并验证校验和。
// 参数：
//   - input: 使用 CheckEncode 编码的字符串
//
// 返回：
//   - result: 解码后的字节切片
//   - version: 版本字节
//   - err: 错误信息，如果有
func CheckDecode(input string) (result []byte, version byte, err error) {
	// 将 base58 字符串解码为字节切片
	decoded := Decode(input)
	// 检查解码后的长度是否小于 5
	if len(decoded) < 5 {
		return nil, 0, ErrInvalidFormat
	}

	// 检查并删除校验和
	version = decoded[0]
	var cksum [4]byte
	// 复制解码数据的最后四个字节到 cksum
	copy(cksum[:], decoded[len(decoded)-4:])
	// 验证校验和
	if checksum(decoded[:len(decoded)-4]) != cksum {
		return nil, 0, ErrChecksum
	}
	// 删除版本字节和校验和，获取有效负载
	payload := decoded[1 : len(decoded)-4]
	// 将有效负载复制到 result
	result = append(result, payload...)
	return
}
