package keyspace

import (
	"bytes"
	"crypto/subtle"
	"math/big"
	"math/bits"

	sha256 "github.com/minio/sha256-simd"
)

// XORKeySpace 是一个基于异或操作的键空间实现。
// 它使用 SHA-256 哈希对标识符进行规范化，并使用异或(XOR)操作计算键之间的距离。
var XORKeySpace = &xorKeySpace{}

// 确保 xorKeySpace 实现了 KeySpace 接口
var _ KeySpace = XORKeySpace

// xorKeySpace 实现了基于异或操作的键空间
type xorKeySpace struct{}

// Key 将原始标识符转换为此空间中的键。
//
// 参数:
//   - id: 要转换的原始标识符字节切片
//
// 返回值:
//   - Key: 转换后的键对象，包含原始标识符、键空间引用和规范化后的字节表示
func (s *xorKeySpace) Key(id []byte) Key {
	logger.Debugf("将标识符转换为键: %x", id)
	hash := sha256.Sum256(id) // 使用 SHA-256 对输入标识符进行哈希
	key := hash[:]            // 将哈希结果转换为字节切片
	logger.Debugf("标识符哈希结果: %x", key)
	return Key{ // 返回包含所有相关信息的键对象
		Space:    s,   // 键空间引用
		Original: id,  // 原始标识符
		Bytes:    key, // 规范化后的字节表示
	}
}

// Equal 判断两个键在此键空间中是否相等。
//
// 参数:
//   - k1: 第一个键
//   - k2: 第二个键
//
// 返回值:
//   - bool: 如果两个键相等则返回 true，否则返回 false
func (s *xorKeySpace) Equal(k1, k2 Key) bool {
	logger.Debugf("比较键的相等性: %x 和 %x", k1.Bytes, k2.Bytes)
	equal := bytes.Equal(k1.Bytes, k2.Bytes) // 通过比较规范化后的字节表示判断相等性
	logger.Debugf("键比较结果: %v", equal)
	return equal
}

// Distance 计算两个键在此键空间中的距离。
//
// 参数:
//   - k1: 第一个键
//   - k2: 第二个键
//
// 返回值:
//   - *big.Int: 表示两个键之间距离的大整数
func (s *xorKeySpace) Distance(k1, k2 Key) *big.Int {
	logger.Debugf("计算键之间的距离: %x 和 %x", k1.Bytes, k2.Bytes)
	k3 := XOR(k1.Bytes, k2.Bytes)      // 对两个键的字节表示进行异或操作
	dist := big.NewInt(0).SetBytes(k3) // 将异或结果转换为大整数
	logger.Debugf("键距离计算结果: %v", dist)
	return dist // 返回表示距离的大整数
}

// Less 比较两个键的大小关系。
//
// 参数:
//   - k1: 第一个键
//   - k2: 第二个键
//
// 返回值:
//   - bool: 如果 k1 小于 k2 则返回 true，否则返回 false
func (s *xorKeySpace) Less(k1, k2 Key) bool {
	logger.Debugf("比较键的大小: %x 和 %x", k1.Bytes, k2.Bytes)
	less := bytes.Compare(k1.Bytes, k2.Bytes) < 0 // 通过比较规范化后的字节表示判断大小关系
	logger.Debugf("键大小比较结果: %v", less)
	return less
}

// ZeroPrefixLen 计算字节切片中开头连续零位的数量。
//
// 参数:
//   - id: 要计算的字节切片
//
// 返回值:
//   - int: 开头连续零位的数量
func ZeroPrefixLen(id []byte) int {
	logger.Debugf("计算字节切片的零前缀长度: %x", id)
	for i, b := range id { // 遍历字节切片中的每个字节
		if b != 0 { // 如果当前字节不为零
			result := i*8 + bits.LeadingZeros8(uint8(b)) // 返回之前的零字节数 * 8 加上当前字节中开头的零位数
			logger.Debugf("零前缀长度计算结果: %d", result)
			return result
		}
	}
	result := len(id) * 8 // 如果所有字节都是零，返回总位数
	logger.Debugf("零前缀长度计算结果(全零): %d", result)
	return result
}

// XOR 对两个字节切片执行异或运算，返回结果切片。
// 参数:
//   - a: 第一个字节切片
//   - b: 第二个字节切片
//
// 返回值:
//   - []byte: 异或运算后的结果切片
func XOR(a, b []byte) []byte {
	_ = b[len(a)-1] // 保持与之前相同的行为，但这看起来像一个bug

	c := make([]byte, len(a))
	subtle.XORBytes(c, a, b)
	return c
}
