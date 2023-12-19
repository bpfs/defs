// 切片

package defs

import (
	"bytes"
	"crypto/md5"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/dgraph-io/ristretto"
	"github.com/klauspost/reedsolomon"
)

// SliceInfo 描述了文件的一个切片信息
type SliceInfo struct {
	index     int    // 文件片段的索引(该片段在文件中的顺序位置)
	sliceHash string // 文件片段的哈希值(外部标识)
	rsCodes   bool   // 是否为纠删码
	/**
	数据签名字段：
		assetID			资产的唯一标识(外部标识)
		sliceTable		切片内容的哈希表
		sliceHash		文件片段的哈希值(外部标识)
		index			文件片段的索引(该片段在文件中的顺序位置)
		mode			文件片段的存储模式
		content			文件片段的内容(使用文件内容的哈希值进行加密)

	请求数据时将公钥发送出去，接收方先本地验签。验证无误后再通过网络回复给请求方
	数据流转时，也需要将公钥附上，对数据的验签可以避免错误数据在网络中流转，减少资源消耗
	*/
	signature []byte // 文件和文件片段的数据签名
}

// readSplit 读取分割
func (f *FileInfo) readSplit(cache *ristretto.Cache, r io.Reader, capacity, dataShards, parityShards int64) error {
	// 使用子函数读取数据到 buffer。
	buf, err := readIntoBuffer(r, capacity)
	if err != nil {
		return err
	}

	// 使用子函数进行编码和数据切分。
	split, err := prepareEncoderAndSplit(buf.Bytes(), dataShards, parityShards)
	if err != nil {
		return err
	}

	// 使用种子数据生成 RSA 密钥对
	privateKey, publicKey, err := GenerateKeysFromSeed([]byte(f.fileHash), 2048)
	if err != nil {
		return fmt.Errorf("生成密钥失败: %v", err)
	}
	// 将 publicKey 转换为 []byte
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return err
	}

	// 初始化文件信息
	f.assetID = hex.EncodeToString((CalculateHash([]byte(f.fileHash))))
	// f.assetID = string(calculateHash([]byte(f.fileHash)))
	// 文件所有者的公钥
	f.publicKey = publicKeyBytes
	// 切片内容的哈希表
	f.sliceTable = make(map[int]HashTable)
	// 切片列表
	f.sliceList = make([]SliceInfo, 0, len(split)) // 提前设定大小

	for k, v := range split {
		sliceHash := hex.EncodeToString(CalculateHash(v))
		// 对数据先进行压缩再进行加密
		// encryptedData, err := f.compressAndEncrypt(v)
		// if err != nil {
		// 	return err
		// }

		// 将加密数据存储到 ristretto 缓存中
		cache.Set(sliceHash, v, int64(len(v)))

		slice := SliceInfo{
			index:     k + 1,     // 文件片段的索引
			sliceHash: sliceHash, // 文件片段的哈希值
			rsCodes:   false,     // 是否为纠删码
		}

		if k > int(dataShards) {
			slice.rsCodes = true
		}

		// 根据给定的私钥和数据生成签名
		signature, err := f.generateSignature(privateKey, slice, v)
		if err != nil {
			return err
		}

		slice.signature = signature
		f.sliceList = append(f.sliceList, slice)
		f.sliceTable[slice.index] = HashTable{
			Hash:    slice.sliceHash,
			RsCodes: slice.rsCodes,
		}
	}

	return nil
}

// readIntoBuffer 从给定的 io.Reader 中读取数据到一个预分配大小的 bytes.Buffer。
func readIntoBuffer(r io.Reader, capacity int64) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(make([]byte, 0, capacity))
	_, err := buf.ReadFrom(r)
	if err != nil {
		return nil, fmt.Errorf("read from reader failed: %v", err)
	}
	return buf, nil
}

// prepareEncoderAndSplit 初始化 Reed-Solomon 编码器并对数据进行切分和编码。
func prepareEncoderAndSplit(data []byte, dataShards, parityShards int64) ([][]byte, error) {
	enc, err := reedsolomon.New(int(dataShards), int(parityShards))
	if err != nil {
		return nil, fmt.Errorf("initialize reedsolomon encoder failed: %v", err)
	}

	split, err := enc.Split(data)
	if err != nil {
		return nil, fmt.Errorf("split data failed: %v", err)
	}

	if err := enc.Encode(split); err != nil {
		return nil, fmt.Errorf("encode data shards failed: %v", err)
	}

	return split, nil
}

// compressAndEncrypt 对数据先进行压缩再进行加密。
func (f *FileInfo) compressAndEncrypt(data []byte) ([]byte, error) {
	// 数据压缩
	compressedData, err := compressData(data)
	if err != nil {
		return nil, fmt.Errorf("compress data failed: %v", err)
	}

	// AES加密的密钥，长度需要是16、24或32字节
	key := md5.Sum([]byte(f.fileHash))

	// 数据加密
	encryptedData, err := encryptData(compressedData, key[:])
	if err != nil {
		return nil, fmt.Errorf("encrypt data failed: %v", err)
	}

	return encryptedData, nil
}

// generateSignature 根据给定的私钥和数据生成签名。
func (f *FileInfo) generateSignature(privateKey *rsa.PrivateKey, slice SliceInfo, content []byte) ([]byte, error) {
	// 待签名数据
	merged, err := MergeFieldsForSigning(
		f.assetID,       // 文件资产的唯一标识
		f.sliceTable,    // 切片内容的哈希表
		slice.index,     // 文件片段的索引
		slice.sliceHash, // 文件片段的哈希值
		slice.rsCodes,   // 文件片段的存储模式
		content,         // 文件片段的内容(使用文件内容的哈希值进行加密)
	)
	if err != nil {
		return nil, fmt.Errorf("merge fields for signing failed: %v", err)
	}

	// 签名
	signature, err := signData(privateKey, merged)
	if err != nil {
		return nil, fmt.Errorf("sign data failed: %v", err)
	}

	return signature, nil
}

// SliceHash 资产的唯一标识(外部标识)
func (sl *SliceInfo) SliceHash() string {
	return sl.sliceHash
}

// Index 文件片段的索引(该片段在文件中的顺序位置)
func (sl *SliceInfo) Index() int {
	return sl.index
}

// RSCodes 文件片段是否为纠删码
func (sl *SliceInfo) RSCodes() bool {
	return sl.rsCodes
}

// Content 文件片段的内容(使用文件内容的哈希值进行加密)
// func (sl *SliceInfo) Content() []byte {
// 	return sl.content
// }

// Signature 文件和文件片段的数据签名
func (sl *SliceInfo) Signature() []byte {
	return sl.signature
}

// readAll 从 r 读取直到出现错误或 EOF，并返回从分配有指定容量的内部缓冲区读取的数据。
// func readAll(r io.Reader, capacity int64) (b []byte, err error) {
// 	// NewBuffer 使用 buf 作为初始内容创建并初始化一个新的 Buffer。
// 	buf := bytes.NewBuffer(make([]byte, 0, capacity))
// 	// 如果缓冲区溢出，我们将得到 bytes.ErrTooLarge。
// 	// 将其作为错误返回。 任何其他恐慌仍然存在。
// 	defer func() {
// 		e := recover()
// 		if e == nil {
// 			return
// 		}
// 		if panicErr, ok := e.(error); ok && panicErr == bytes.ErrTooLarge {
// 			err = panicErr
// 		} else {
// 			panic(e)
// 		}
// 	}()

// 	// ReadFrom 从 r 读取数据直到 EOF 并将其附加到缓冲区，根据需要增加缓冲区。
// 	_, err = buf.ReadFrom(r)
// 	return buf.Bytes(), err
// }

// 生成切片文件数据
// func GenerateSliceFile(name string, size int64, modTime time.Time, hash string) (*bytes.Buffer, error) {
// 	buffer := &bytes.Buffer{}

// 	// 写入文件头和版本信息
// 	if _, err := buffer.WriteString(FileHeader); err != nil {
// 		return nil, err
// 	}
// 	if err := binary.Write(buffer, binary.BigEndian, byte(Version)); err != nil {
// 		return nil, err
// 	}

// 	// 写入文件名称
// 	if err := writeChunk(buffer, "NAME", []byte(name), nil); err != nil {
// 		return nil, err
// 	}

// 	sizeData, err := int64ToBytes(size)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// 写入文件大小
// 	if err := writeChunk(buffer, "SIZE", sizeData, nil); err != nil {
// 		return nil, err
// 	}

// 	modTimeData, err := timeToBytes(modTime)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// 写入修改时间
// 	if err := writeChunk(buffer, "MODTIME", modTimeData, nil); err != nil {
// 		return nil, err
// 	}

// 	// 写入文件Hash
// 	if err := writeChunk(buffer, "HASH", []byte(hash), nil); err != nil {
// 		return nil, err
// 	}

// 	return buffer, nil
// }

// UpdateSegmentCommon 是一个通用的函数，用于更新给定类型的数据段。
// 它接受一个 io.WriterAt 和 io.ReaderAt 接口，这样可以同时处理 *os.File 和 *bytes.Buffer。
// func UpdateSegmentCommon(writerAt io.WriterAt, readerAt io.ReaderAt, segmentType string, newData []byte, xref *FileXref) error {
// 	xref.mu.Lock()
// 	defer xref.mu.Unlock()

// 	entry, ok := xref.XrefTable[segmentType]
// 	if !ok {
// 		return fmt.Errorf("segment not found in xref table")
// 	}

// 	oldLength := entry.Length
// 	newLength := uint32(len(newData))

// 	// 内容长度相同，直接覆盖
// 	if oldLength == newLength {
// 		if _, err := writerAt.WriteAt(newData, entry.Offset); err != nil {
// 			return err
// 		}
// 		return nil
// 	}

// 	// 需要移动的字节
// 	movingBytes := make([]byte, oldLength)
// 	if _, err := readerAt.ReadAt(movingBytes, entry.Offset); err != nil {
// 		return err
// 	}

// 	// 新数据比旧数据短
// 	if newLength < oldLength {
// 		diff := oldLength - newLength

// 		// 写入新数据
// 		if _, err := writerAt.WriteAt(newData, entry.Offset); err != nil {
// 			return err
// 		}

// 		// 移动后续内容
// 		if _, err := writerAt.WriteAt(movingBytes[newLength:], entry.Offset+int64(newLength)); err != nil {
// 			return err
// 		}
// 	} else { // 新数据比旧数据长
// 		diff := newLength - oldLength

// 		// 移动后续内容
// 		if _, err := writerAt.WriteAt(movingBytes, entry.Offset+int64(diff)); err != nil {
// 			return err
// 		}

// 		// 写入新数据
// 		if _, err := writerAt.WriteAt(newData, entry.Offset); err != nil {
// 			return err
// 		}
// 	}

// 	// 更新 xref 表中的数据长度
// 	entry.Length = newLength
// 	xref.XrefTable[segmentType] = entry
// 	return nil
// }

// // UpdateSegmentInFile 用于更新文件中的数据段
// func UpdateSegmentInFile(file *os.File, segmentType string, newData []byte, xref *FileXref) error {
// 	return UpdateSegmentCommon(file, file, segmentType, newData, xref)
// }

// // UpdateSegmentInBuffer 用于更新缓冲区中的数据段
// func UpdateSegmentInBuffer(buffer *bytes.Buffer, segmentType string, newData []byte, xref *FileXref) error {
// 	reader := bytes.NewReader(buffer.Bytes())
// 	return UpdateSegmentCommon(buffer, reader, segmentType, newData, xref)
// }
