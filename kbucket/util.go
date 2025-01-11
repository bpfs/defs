package kbucket

import (
	"crypto/subtle"
	"fmt"

	"github.com/bpfs/defs/v2/kbucket/keyspace"
	"github.com/minio/sha256-simd"

	"github.com/dep2p/libp2p/core/peer"
)

// ErrLookupFailure 表示路由表查询未返回任何结果时的错误。这不是预期行为。
var ErrLookupFailure = fmt.Errorf("failed to find any peer in table")

// ID 是一个在 XORKeySpace 中的 DHT ID 的类型。
// 类型 dht.ID 表示其内容已从 peer.ID 或 util.Key 进行了哈希处理。这统一了键空间。
type ID []byte

// less 比较两个 ID 的大小。
// 参数:
//   - other: 要比较的另一个 ID
//
// 返回值:
//   - bool: 如果当前 ID 小于 other，则返回 true；否则返回 false
func (id ID) less(other ID) bool {
	// 将当前 ID 转换为 XORKeySpace 中的 keyspace.Key 类型
	a := keyspace.Key{Space: keyspace.XORKeySpace, Bytes: id}
	// 将另一个 ID 转换为 XORKeySpace 中的 keyspace.Key 类型
	b := keyspace.Key{Space: keyspace.XORKeySpace, Bytes: other}
	// 调用 keyspace.Key 的 Less() 方法比较两个键的大小
	return a.Less(b)
}

// xor 对两个 ID 进行异或运算。
// 参数:
//   - a: 第一个 ID
//   - b: 第二个 ID
//
// 返回值:
//   - ID: 两个 ID 异或运算的结果
func xor(a, b ID) ID {
	// 使用工具包中的 XOR 函数进行异或运算并转换为 ID 类型
	return ID(XOR(a, b))
}

// CommonPrefixLen 计算两个 ID 的公共前缀长度。
// 参数:
//   - a: 第一个 ID
//   - b: 第二个 ID
//
// 返回值:
//   - int: 两个 ID 的公共前缀长度（以位为单位）
func CommonPrefixLen(a, b ID) int {
	// 先对两个 ID 进行异或运算，然后计算结果中前导零的数量
	return keyspace.ZeroPrefixLen(XOR(a, b))
}

// ConvertPeerID 通过哈希处理 Peer ID（Multihash）创建一个 DHT ID。
// 参数:
//   - id: 要转换的 peer.ID
//
// 返回值:
//   - ID: 转换后的 DHT ID
func ConvertPeerID(id peer.ID) ID {
	// 对 peer.ID 的字节表示进行 SHA-256 哈希计算
	hash := sha256.Sum256([]byte(id))
	// 返回哈希结果作为 DHT ID
	return hash[:]
}

// ConvertKey 通过哈希处理本地键（字符串）创建一个 DHT ID。
// 参数:
//   - id: 要转换的字符串键
//
// 返回值:
//   - ID: 转换后的 DHT ID
func ConvertKey(id string) ID {
	// 对字符串键的字节表示进行 SHA-256 哈希计算
	hash := sha256.Sum256([]byte(id))
	// 返回哈希结果作为 DHT ID
	return hash[:]
}

// Closer 判断两个节点中哪个更接近目标键。
// 参数:
//   - a: 第一个节点的 Peer ID
//   - b: 第二个节点的 Peer ID
//   - key: 目标键
//
// 返回值:
//   - bool: 如果节点 a 比节点 b 更接近键 key，则返回 true；否则返回 false
func Closer(a, b peer.ID, key string) bool {
	// 将节点 a 的 Peer ID 转换为 DHT ID
	aid := ConvertPeerID(a)
	// 将节点 b 的 Peer ID 转换为 DHT ID
	bid := ConvertPeerID(b)
	// 将键 key 转换为 DHT ID
	tgt := ConvertKey(key)
	// 计算节点 a 与键 key 的距离
	adist := xor(aid, tgt)
	// 计算节点 b 与键 key 的距离
	bdist := xor(bid, tgt)

	// 判断节点 a 与键 key 的距离是否比节点 b 与键 key 的距离更近
	return adist.less(bdist)
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
