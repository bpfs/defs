package upload

import (
	"bytes"
	"crypto/md5"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/reedsolomon"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/util/crypto/gcm"
	sign "github.com/bpfs/defs/util/sign/rsa"
	"github.com/bpfs/defs/util/tempfile"
	"github.com/bpfs/defs/util/zip/gzip"
	"github.com/sirupsen/logrus"
)

// readSplit 读取分割
func readSplit(cache *ristretto.Cache, r io.Reader, fileInfo *core.FileInfo, fileHash []byte, capacity, dataShards, parityShards int64) error {
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

	privateKey, publicKey, err := sign.GenerateKeysFromSeed([]byte(fileInfo.GetFileKey()), 2048)
	if err != nil {
		return err
	}
	publicKeyBytes, err := sign.PublicKeyToBytes(publicKey)
	if err != nil {
		return err
	}
	// 构建P2PK脚本
	script, err := script.NewScriptBuilder().
		AddData(publicKeyBytes).   // 直接添加公钥
		AddOp(script.OP_CHECKSIG). // 添加检查签名操作
		Script()
	if err != nil {
		return err
	}
	fmt.Printf("十六进制脚本:\t%x\n", script)
	fileInfo.BuildP2pkScript(script) // 设置文件的 P2PK 脚本

	fileInfo.BuildSliceList(len(split)) // 提前设定大小

	// 第一步：加密文件片段并存储到缓存中，同时构建哈希表
	sliceHashes := make([]string, len(split))
	for index, content := range split {
		rc := index > int(dataShards)

		// 将文件的哈希值与文件片段的内容连接起来
		content = append(fileHash[:], content...)
		// 对数据先进行压缩再进行加密
		encryptedData, err := compressAndEncrypt([]byte(fileInfo.GetFileKey()), content)
		if err != nil {
			return err
		}

		// 计算并存储文件片段的哈希
		sliceHash := hex.EncodeToString(util.CalculateHash(content))
		sliceHashes[index] = sliceHash

		// TODO: 缓存测试
		// 将加密数据存储到 ristretto 缓存中
		// cache.Set(sliceHash, encryptedData, int64(len(encryptedData)))
		tempfile.Write(sliceHash, encryptedData)
		// 更新 FileInfo 的哈希表
		fileInfo.AddSliceTable(index, sliceHash, rc) // 向哈希表添加新的文件片段内容
	}

	// 第二步：生成签名
	for index, sliceHash := range sliceHashes {
		rc := index > int(dataShards)

		// 根据给定的私钥和数据生成签名
		signature, err := generateSignature(privateKey, publicKey, fileInfo.GetFileID(), fileInfo.GetSliceTable(), index, sliceHash, rc)
		if err != nil {
			return err
		}
		// 更新 FileInfo 的片段列表
		fileInfo.AddSliceList(core.BuildSliceInfo(index, sliceHash, signature))
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
func compressAndEncrypt(pk, data []byte) ([]byte, error) {
	// AES加密的密钥，长度需要是16、24或32字节
	key := md5.Sum(pk)

	// 数据加密
	encryptedData, err := gcm.EncryptData(data, key[:])
	if err != nil {
		return nil, fmt.Errorf("encrypt data failed: %v", err)
	}

	// 数据压缩
	compressedData, err := gzip.CompressData(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("compress data failed: %v", err)
	}

	// TODO：测试
	// 数据解压
	decompressData, err := gzip.DecompressData(compressedData)
	if err != nil {
		return nil, fmt.Errorf("decompress data failed: %v", err)
	}

	// 解密验证
	decrypted, err := gcm.DecryptData(decompressData, key[:])
	if err != nil {
		logrus.Errorf("解密失败: %v", err)
	}

	if !bytes.Equal(data, decrypted) {
		logrus.Errorf("原文和解密后的文本不匹配：原文 %d, 解密后 %d", len(data), len(decrypted))
	}

	return compressedData, nil
}

// generateSignature 根据给定的私钥和数据生成签名。
func generateSignature(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey, fileID string, sliceTable map[int]core.HashTable, index int, sliceHash string, rc bool) ([]byte, error) {
	st, err := json.Marshal(sliceTable)
	if err != nil {
		return nil, fmt.Errorf(": %v", err)
	}
	// 待签名数据
	merged, err := util.MergeFieldsForSigning(
		fileID,    // 文件的唯一标识
		st,        // 切片内容的哈希表
		index,     // 文件片段的索引
		sliceHash, // 文件片段的哈希值
		rc,        // 文件片段的是否为纠删码
	)
	if err != nil {
		return nil, fmt.Errorf("合并字段签名失败: %v", err)
	}

	// 签名
	signature, err := sign.SignData(privateKey, merged)
	if err != nil {
		return nil, fmt.Errorf(": %v", err)
	}

	return signature, nil
}
