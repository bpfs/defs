// 格式化

package defs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

const (
	FileHeader = "\x89PNGA\r\n\x1a\n" // 自定义文件头
	Version    = 1                    // 版本控制
)

// 块的偏移位置和长度，用于随机访问
var ChunkOffsetLength = make(map[string]int64)

// WriteChunk 写入块到缓冲区
func WriteChunk(buffer *bytes.Buffer, chunkType string, data []byte, key []byte) error {
	// length := uint32(len(data))
	// MakeTable 返回一个由指定多项式构造的表。 本表的内容不得修改。
	crcTable := crc32.MakeTable(crc32.IEEE)

	encryptedData := data
	var err error

	if len(key) > 0 {
		// 数据加密
		encryptedData, err = encryptData(data, key)
		if err != nil {
			return err
		}
	}

	// 数据压缩
	compressedData, err := compressData(encryptedData)
	if err != nil {
		return err
	}

	length := uint32(len(compressedData))

	// 记录块的偏移位置和长度，用于随机访问
	ChunkOffsetLength[chunkType] = int64(buffer.Len())

	// 写入块长度
	// Write 将数据的二进制表示形式写入 w 中。 数据必须是固定大小的值或固定大小值的切片，或者指向此类数据的指针。 布尔值编码为一个字节：1 表示 true，0 表示 false。 写入 w 的字节使用指定的字节顺序进行编码，并从数据的连续字段中读取。 编写结构时，将为具有空白 (_) 字段名称的字段写入零值。
	if err := binary.Write(buffer, binary.BigEndian, length); err != nil {
		return err
	}

	// 写入块类型
	// WriteString 与 Write 类似，但写入字符串 s 的内容而不是字节切片。
	if _, err := buffer.WriteString(chunkType); err != nil {
		return err
	}

	// 写入压缩和加密后的数据
	// Write 将 b 中的 len(b) 个字节写入文件。 它返回写入的字节数和错误（如果有）。 当 n != len(b) 时，Write 返回非零错误。
	if _, err := buffer.Write(compressedData); err != nil {
		return err
	}

	// CRC32校验
	// Checksum 使用表中表示的多项式返回数据的 CRC-32 校验和。
	crcValue := crc32.Checksum(append([]byte(chunkType), compressedData...), crcTable)
	// Write 将数据的二进制表示形式写入 w 中。
	if err := binary.Write(buffer, binary.BigEndian, crcValue); err != nil {
		return err
	}

	return nil
}

// ReadChunk 读取块内容
func ReadChunk(file *os.File, chunkType string, key []byte) (string, []byte, error) {
	// 根据块类型查找偏移和长度
	offset, ok := ChunkOffsetLength[chunkType]
	if !ok {
		return "", nil, fmt.Errorf("块类型不存在")
	}

	// 定位到块开始位置
	// Seek 将文件上的下一个读取或写入的偏移量设置为偏移量，根据来源进行解释：0 表示相对于文件的原点，1 表示相对于当前偏移量，2 表示相对于末尾。
	file.Seek(offset, io.SeekStart)

	var length uint32

	// 读取块长度
	// Read 将结构化二进制数据从 r 读取到 data 中。
	if err := binary.Read(file, binary.BigEndian, &length); err != nil {
		return "", nil, err
	}

	// 读取块类型
	chunkTypeBytes := make([]byte, 4)
	// ReadFull 将 r 中的 len(buf) 个字节准确读取到 buf 中。 它返回复制的字节数，如果读取的字节数较少，则返回错误。 仅当未读取任何字节时，错误才会为 EOF。 如果在读取部分而非全部字节后发生 EOF，ReadFull 将返回 ErrUnexpectedEOF。 返回时，n == len(buf) 当且仅当 err == nil 时。 如果 r 在读取至少 len(buf) 个字节后返回错误，则该错误将被丢弃。
	if _, err := io.ReadFull(file, chunkTypeBytes); err != nil {
		return "", nil, err
	}

	// 读取块数据
	compressedData := make([]byte, length)
	if _, err := io.ReadFull(file, compressedData); err != nil {
		return "", nil, err
	}

	// 校验CRC32
	var crcValue uint32
	if err := binary.Read(file, binary.BigEndian, &crcValue); err != nil {
		return "", nil, err
	}

	// MakeTable 返回一个由指定多项式构造的表。 本表的内容不得修改。
	crcTable := crc32.MakeTable(crc32.IEEE)
	// Checksum 使用表中表示的多项式返回数据的 CRC-32 校验和。
	if crcValue != crc32.Checksum(append(chunkTypeBytes, compressedData...), crcTable) {
		return "", nil, fmt.Errorf("CRC32校验失败")
	}

	// 数据解压和解密
	encryptedData, err := decompressData(compressedData)
	if err != nil {
		return "", nil, err
	}

	data := encryptedData
	if len(key) > 0 {
		data, err = decryptData(encryptedData, key)
		if err != nil {
			return "", nil, err
		}
	}

	return string(chunkTypeBytes), data, nil
}

// func main() {
// 	// 生成一个RSA密钥对
// 	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
// 	if err != nil {
// 		panic(err)
// 	}
// 	publicKey := &privateKey.PublicKey

// 	// 待签名的数据
// 	data := []byte("Hello, world!")

// 	// 签名数据
// 	signature, err := signData(privateKey, data)
// 	if err != nil {
// 		panic(err)
// 	}

// 	// 验证签名
// 	isValid := verifySignature(publicKey, data, signature)
// 	if isValid {
// 		fmt.Println("The signature is valid.")
// 	} else {
// 		fmt.Println("The signature is NOT valid.")
// 	}
// }
