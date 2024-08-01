package base58

import (
	"testing"

	"github.com/sirupsen/logrus"
)

// 此示例演示如何解码修改后的 Base58 编码数据。
func TestDecode(t *testing.T) {
	// 解码修改后的 Base58 编码数据的示例。
	encoded := "25JnwSn7XKfNQ"
	decoded := Decode(encoded)

	// 显示解码后的数据。
	logrus.Println("Decoded Data:", string(decoded))

	// Output:
	// Decoded Data: Test data
}

// 此示例演示如何使用修改后的 base58 编码方案对数据进行编码。
func TestEncode(t *testing.T) {
	// 使用修改后的 Base58 编码方案对示例数据进行编码。
	data := []byte("Test data")
	encoded := Encode(data)

	// 显示编码数据。
	logrus.Println("Encoded Data:", encoded)

	// Output:
	// Encoded Data: 25JnwSn7XKfNQ
}

// 此示例演示如何解码 Base58Check 编码数据。
func TestCheckDecode(t *testing.T) {
	// 解码示例 Base58Check 编码数据。
	encoded := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	decoded, version, err := CheckDecode(encoded)
	if err != nil {
		logrus.Println(err)
		return
	}

	// 显示解码后的数据。
	logrus.Printf("Decoded data: %x\n", decoded)
	logrus.Println("Version Byte:", version)

	// Output:
	// Decoded data: 62e907b15cbf27d5425399ebf6f0fb50ebb88f18
	// Version Byte: 0
}

// 此示例演示如何使用 Base58Check 编码方案对数据进行编码。
func TestCheckEncode(t *testing.T) {
	// 使用 Base58Check 编码方案对示例数据进行编码。
	data := []byte("Test data")
	encoded := CheckEncode(data, 0)

	// 显示编码数据。
	logrus.Println("Encoded Data:", encoded)

	// Output:
	// Encoded Data: 182iP79GRURMp7oMHDU
}
