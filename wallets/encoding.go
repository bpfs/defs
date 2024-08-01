// encoding.go
// 格式转换和编码

package wallets

import "github.com/bpfs/defs/base58"

// EncodeBase58 将数据编码为Base58格式
// 参数:
//   - data ([]byte): 输入的数据
//
// 返回值:
//   - string: Base58编码后的字符串
func EncodeBase58(data []byte) string {
	return base58.Encode(data)
}

// DecodeBase58 从Base58格式解码数据
// 参数:
//   - data (string): Base58编码的字符串
//
// 返回值:
//   - []byte: 解码后的数据
func DecodeBase58(data string) []byte {
	return base58.Decode(data)
}
