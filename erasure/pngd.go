package erasure

// 在这个示例中，我进行了以下扩展：

// 在文件头后面添加了一个版本字段（Version），用于版本控制。
// 使用一个全局的map（chunkOffsetLength）来存储每个块的偏移和长度，这样就可以实现随机访问特定的块。
// 添加了一个名为META的块，用于存储元数据。
// 这样，您就有了一个更加全面和健壮的自定义文件格式。请注意，这仍然是一个非常基础的示例，可能还需要进一步的扩展和优化。

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
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
var chunkOffsetLength = make(map[string]int64)

func compressData(data []byte) ([]byte, error) {
	// 数据压缩

	var compressedData bytes.Buffer
	// 	NewWriter 返回一个新的 Writer。 对返回的 writer 的写入被压缩并写入 w。
	// 完成后，调用者有责任在 Writer 上调用 Close。 写入可能会被缓冲，并且在关闭之前不会被刷新。
	// 希望设置 Writer.Header 中字段的调用者必须在第一次调用 Write、Flush 或 Close 之前执行此操作。
	w := gzip.NewWriter(&compressedData)
	// Write 将 p 的压缩形式写入底层 io.Writer。 在 Writer 关闭之前，压缩字节不一定会被刷新。
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	// Close 通过将所有未写入的数据刷新到底层 io.Writer 并写入 GZIP 页脚来关闭 Writer。 它不会关闭底层的 io.Writer。
	if err := w.Close(); err != nil {
		return nil, err
	}
	// Bytes 返回长度为 b.Len() 的切片，其中保存缓冲区的未读部分。
	// 该切片仅在下一次缓冲区修改之前有效（即，仅在下一次调用 Read、Write、Reset 或 Truncate 等方法之前）。
	// 切片至少在下一次缓冲区修改之前对缓冲区内容进行别名，因此对切片的立即更改将影响将来读取的结果。
	return compressedData.Bytes(), nil
}

func decompressData(data []byte) ([]byte, error) {
	// 数据解压

	// NewReader 创建一个新的 Reader 来读取给定的 reader。 如果 r 没有同时实现 io.ByteReader，则解压缩器可能会从 r 读取超出需要的数据。
	// 完成后，调用者有责任在 Reader 上调用 Close。
	// Reader.Header 字段在返回的 Reader 中有效。
	//
	// 	NewBuffer 使用 buf 作为初始内容创建并初始化一个新的 Buffer。 新的 Buffer 取得了 buf 的所有权，并且调用者在这次调用之后不应该使用 buf。
	// NewBuffer的目的是准备一个Buffer来读取现有的数据。 它还可用于设置内部写入缓冲区的初始大小。 为此，buf 应该具有所需的容量，但长度为零。
	// 在大多数情况下，new(Buffer)（或者只是声明一个 Buffer 变量）足以初始化一个 Buffer。
	r, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	// ReadAll 从 r 读取直到出现错误或 EOF，然后返回读取的数据。 成功的调用返回 err == nil，而不是 err == EOF。 因为 ReadAll 被定义为从 src 读取直到 EOF，所以它不会将 Read 中的 EOF 视为要报告的错误。
	decompressedData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decompressedData, nil
}

func encryptData(data, key []byte) ([]byte, error) {
	// 数据加密

	// NewCipher 创建并返回一个新的 cipher.Block。 key 参数应该是 AES 密钥，可以是 16、24 或 32 字节，以选择 AES-128、AES-192 或 AES-256。
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, len(data))
	// NewCTR 返回一个 Stream，该 Stream 在计数器模式下使用给定的块进行加密/解密。 iv 的长度必须与Block 的块大小相同。
	stream := cipher.NewCTR(block, make([]byte, block.BlockSize()))
	// 	XORKeyStream 将给定切片中的每个字节与密码密钥流中的一个字节进行异或。 dst 和 src 必须完全重叠或根本不重叠。
	// 如果 len(dst) < len(src)，XORKeyStream 应该会出现恐慌。 传递大于 src 的 dst 是可以接受的，在这种情况下，XORKeyStream 将仅更新 dst[:len(src)] 而不会触及 dst 的其余部分。
	// 对 XORKeyStream 的多次调用的行为就像在单次运行中传递 src 缓冲区的串联一样。 也就是说，Stream 会维护状态并且不会在每次 XORKeyStream 调用时重置。
	stream.XORKeyStream(ciphertext, data)
	return ciphertext, nil
}

func decryptData(data, key []byte) ([]byte, error) {
	// 数据解密
	return encryptData(data, key) // 在AES CTR模式下，解密和加密是相同的操作
}

func writeChunk(file *os.File, chunkType string, data []byte, key []byte) error {
	// length := uint32(len(data))
	// MakeTable 返回一个由指定多项式构造的表。 本表的内容不得修改。
	crcTable := crc32.MakeTable(crc32.IEEE)

	// 数据加密
	encryptedData, err := encryptData(data, key)
	if err != nil {
		return err
	}

	// 数据压缩
	compressedData, err := compressData(encryptedData)
	if err != nil {
		return err
	}

	length := uint32(len(compressedData))

	// 记录块的偏移位置和长度，用于随机访问
	// Seek 将文件上的下一个读取或写入的偏移量设置为偏移量，根据来源进行解释：0 表示相对于文件的原点，1 表示相对于当前偏移量，2 表示相对于末尾。 它返回新的偏移量和错误（如果有）。 未指定使用 O_APPEND 打开的文件上的 Seek 行为。
	chunkOffsetLength[chunkType], err = file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// 写入块长度
	// Write 将数据的二进制表示形式写入 w 中。 数据必须是固定大小的值或固定大小值的切片，或者指向此类数据的指针。 布尔值编码为一个字节：1 表示 true，0 表示 false。 写入 w 的字节使用指定的字节顺序进行编码，并从数据的连续字段中读取。 编写结构时，将为具有空白 (_) 字段名称的字段写入零值。
	if err := binary.Write(file, binary.BigEndian, length); err != nil {
		return err
	}

	// 写入块类型
	// WriteString 与 Write 类似，但写入字符串 s 的内容而不是字节切片。
	if _, err := file.WriteString(chunkType); err != nil {
		return err
	}

	// 写入压缩和加密后的数据
	// Write 将 b 中的 len(b) 个字节写入文件。 它返回写入的字节数和错误（如果有）。 当 n != len(b) 时，Write 返回非零错误。
	if _, err := file.Write(compressedData); err != nil {
		return err
	}

	// CRC32校验
	// Checksum 使用表中表示的多项式返回数据的 CRC-32 校验和。
	crcValue := crc32.Checksum(append([]byte(chunkType), compressedData...), crcTable)
	if err := binary.Write(file, binary.BigEndian, crcValue); err != nil {
		return err
	}

	return nil
}

func readChunk(file *os.File, chunkType string, key []byte) (string, []byte, error) {
	// 根据块类型查找偏移和长度
	offset, ok := chunkOffsetLength[chunkType]
	if !ok {
		return "", nil, fmt.Errorf("块类型不存在")
	}

	// 定位到块开始位置
	// Seek 将文件上的下一个读取或写入的偏移量设置为偏移量，根据来源进行解释：0 表示相对于文件的原点，1 表示相对于当前偏移量，2 表示相对于末尾。 它返回新的偏移量和错误（如果有）。 未指定使用 O_APPEND 打开的文件上的 Seek 行为。
	file.Seek(offset, io.SeekStart)

	var length uint32
	// 读取块长度
	// Read 将结构化二进制数据从 r 读取到 data 中。 数据必须是指向固定大小值或固定大小值切片的指针。 使用指定的字节顺序对从 r 读取的字节进行解码，并将其写入数据的连续字段。 解码布尔值时，零字节被解码为 false，任何其他非零字节被解码为 true。 读入结构体时，会跳过字段名称为空白 (_) 的字段的字段数据； 即，空白字段名称可用于填充。 读入结构时，必须导出所有非空白字段，否则读取可能会出现恐慌。
	// 仅当未读取任何字节时，错误才会为 EOF。 如果读取部分字节但不是全部字节后发生 EOF，则 Read 将返回 ErrUnexpectedEOF。
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
	// 校验和使用表中表示的多项式返回数据的 CRC-32 校验和。
	if crcValue != crc32.Checksum(append(chunkTypeBytes, compressedData...), crcTable) {
		return "", nil, fmt.Errorf("CRC32校验失败")
	}

	// 数据解压和解密
	encryptedData, err := decompressData(compressedData)
	if err != nil {
		return "", nil, err
	}
	data, err := decryptData(encryptedData, key)
	if err != nil {
		return "", nil, err
	}

	return string(chunkTypeBytes), data, nil
}

func Abcs() {
	// AES加密的密钥，长度需要是16、24或32字节
	key := []byte("mysecretpassword") // 改为长度为16的密钥

	// 创建新文件
	// Create 创建或截断指定文件。 如果文件已存在，则会被截断。 如果该文件不存在，则使用模式 0666（在 umask 之前）创建该文件。 如果成功，返回的 File 上的方法可用于 I/O； 关联的文件描述符的模式为 O_RDWR。 如果有错误，则其类型为 *PathError。
	file, err := os.Create("example.pnga")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// 写入文件头和版本信息
	// WriteString 与 Write 类似，但写入字符串 s 的内容而不是字节切片。
	if _, err := file.WriteString(FileHeader); err != nil {
		panic(err)
	}
	// Write 将数据的二进制表示形式写入 w 中。
	// 数据必须是固定大小的值或固定大小值的切片，或者指向此类数据的指针。
	// 布尔值编码为一个字节：1 表示 true，0 表示 false。
	// 写入 w 的字节使用指定的字节顺序进行编码，并从数据的连续字段中读取。
	// 编写结构时，将为具有空白 (_) 字段名称的字段写入零值。
	if err := binary.Write(file, binary.BigEndian, uint32(Version)); err != nil {
		panic(err)
	}

	// 写入IHDR块（图片信息）
	ihdrData := []byte{0x00, 0x01, 0x02, 0x03} // 模拟图片信息数据
	if err := writeChunk(file, "IHDR", ihdrData, key); err != nil {
		panic(err)
	}

	// 写入META块（元数据）
	metaData := []byte{0xAA, 0xBB, 0xCC, 0xDD} // 模拟元数据
	if err := writeChunk(file, "META", metaData, key); err != nil {
		panic(err)
	}

	// 重新打开文件进行读取
	file, err = os.Open("example.pnga")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// 读取并验证文件头
	header := make([]byte, len(FileHeader))
	if _, err := io.ReadFull(file, header); err != nil {
		panic(err)
	}
	if string(header) != FileHeader {
		fmt.Println("这不是一个有效的PNGA文件")
		return
	}

	var version uint32
	if err := binary.Read(file, binary.BigEndian, &version); err != nil {
		panic(err)
	}

	// 验证版本信息
	if version != Version {
		fmt.Printf("不支持的文件版本：%d\n", version)
		return
	}

	// 读取IHDR块
	chunkType, data, err := readChunk(file, "IHDR", key)
	if err != nil {
		panic(err)
	}
	fmt.Printf("块类型：%s，数据：%x\n", chunkType, data)

	// 读取META块
	chunkType, data, err = readChunk(file, "META", key)
	if err != nil {
		panic(err)
	}
	fmt.Printf("块类型：%s，数据：%x\n", chunkType, data)
}
