package uploads

import (
	"crypto/ecdsa"
	"crypto/md5"
	"fmt"
	"path"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/segment"
	sign "github.com/bpfs/defs/sign/ecdsa"
	"github.com/sirupsen/logrus"

	"github.com/bpfs/defs/crypto/gcm"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/tempfile"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/defs/zip/gzip"
)

// sliceLocalFileHandle 文件片段存储为本地文件
func sliceLocalFileHandle(task *UploadTask) error {
	// 将文件大小转换为 []byte
	sizeByte, err := util.ToBytes[int64](task.File.Size)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}
	// 将文件上传的开始时间戳转换为 []byte
	uploadTimeByte, err := util.ToBytes[int64](task.File.StartedAt)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 定义共享状态，默认为 false，表示不共享
	shared := false
	// 将 shared 转换为 []byte
	sharedByte, err := util.ToBytes[bool](shared)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 编码文件片段的哈希表
	sliceTableBytes, err := util.EncodeToBytes(task.File.SliceTable)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	encryptionKey := task.File.Security.EncryptionKey[1]

	// 文件分片信息
	for index, s := range task.File.Segments {
		if task.Status == StatusPaused { // 上传任务的当前状态:已暂停，则退出
			break
		}

		// 如果文件片段尚未准备好，则进行处理
		if s.Status != SegmentStatusNotReady {
			continue // 否则，退出
		}

		// 将 Index 转换为 []byte
		indexByte, err := util.ToBytes[int](index)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}

		// 文件片段的唯一标识
		segmentID := s.SegmentID

		// 读取文件片段的缓存信息
		content, err := tempfile.Read(segmentID)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}

		// 对文件片段的数据先进行压缩再进行加密
		encryptedData, err := compressAndEncrypt(task.File.Security.Secret, content)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}

		data := map[string][]byte{
			"FILEID":          []byte(task.File.FileID),       // 写入文件的唯一标识
			"NAME":            []byte(task.File.Name),         // 写入文件的名称
			"SIZE":            sizeByte,                       // 写入文件的长度
			"CONTENTTYPE":     []byte(task.File.ContentType),  // MIME类型
			"CHECKSUM":        task.File.Checksum,             // 文件的校验和
			"UPLOADTIME":      uploadTimeByte,                 // 写入文件的上传时间
			"P2PKHSCRIPT":     task.File.Security.P2PKHScript, // 写入文件的 P2PKH 脚本
			"P2PKSCRIPT":      task.File.Security.P2PKScript,  // 写入文件的 P2PK 脚本
			"SLICETABLE":      sliceTableBytes,                // 写入文件片段的哈希表
			"SEGMENTID":       []byte(segmentID),              // 写入文件片段的唯一标识
			"INDEX":           indexByte,                      // 写入文件片段的索引
			"SEGMENTCHECKSUM": s.Checksum,                     // 写入分片的校验和
			"CONTENT":         encryptedData,                  // 写入文件片段的内容(加密)
			"ENCRYPTIONKEY":   encryptionKey,                  // 文件加密密钥
			"SIGNATURE":       nil,                            // 写入文件和文件片段的数据签名
			"SHARED":          sharedByte,                     // 写入文件共享状态(私有)
			"VERSION":         []byte(opts.Version),           // 版本
		}

		// 根据给定的私钥和已经是[]byte的数据直接生成签名
		signature, err := generateSignature(task.File.Security.PrivateKey,
			data["FILEID"],          // 写入文件的唯一标识
			data["CONTENTTYPE"],     // MIME类型
			data["CHECKSUM"],        // 文件的校验和
			data["SLICETABLE"],      // 文件片段的哈希表
			data["SEGMENTID"],       // 文件片段的唯一标识
			data["INDEX"],           // 文件片段的索引
			data["SEGMENTCHECKSUM"], // 分片的校验和
			data["CONTENT"],         // 文件片段的内容(加密)
		)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}
		// 将生成的签名写入data中的"SIGNATURE"字段
		data["SIGNATURE"] = signature

		// 构建文件路径
		slicePath := path.Join(task.File.TempStorage, segmentID)

		// 调用 WriteFileSegment 方法来创建新文件并写入数据
		if err := segment.WriteFileSegment(slicePath, data); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}

		// 修改分片大小
		s.Size = len(encryptedData)

		// 设置文件片段的状态为待上传
		s.SetStatusPending()

		// 删除缓存的文件片段
		// TODO: 上传完成后，才删除缓存的文件片段
		// tempfile.Delete(segmentID)

	}

	return nil
}

// compressAndEncrypt 对数据先进行压缩再进行加密。
func compressAndEncrypt(pk, data []byte) ([]byte, error) {
	// AES加密的密钥，长度需要是16、24或32字节
	key := md5.Sum(pk)

	// 数据加密
	encryptedData, err := gcm.EncryptData(data, key[:])
	if err != nil {
		return nil, fmt.Errorf("加密数据时失败: %v", err)
	}

	// 数据压缩
	compressedData, err := gzip.CompressData(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("压缩数据时失败: %v", err)
	}

	// // TODO：测试
	// // 数据解压
	// decompressData, err := gzip.DecompressData(compressedData)
	// if err != nil {
	// 	return nil, fmt.Errorf("解压数据时失败: %v", err)
	// }

	// // 解密验证
	// decrypted, err := gcm.DecryptData(decompressData, key[:])
	// if err != nil {
	// 	logrus.Errorf("解密数据时失败: %v", err)
	// 	return nil, fmt.Errorf("解密数据时失败: %v", err)
	// }

	// if !bytes.Equal(data, decrypted) {
	// 	logrus.Errorf("原文和解密后的文本不匹配：原文 %d, 解密后 %d", len(data), len(decrypted))
	// 	return nil, fmt.Errorf("原文和解密后的文本不匹配：原文 %d, 解密后 %d", len(data), len(decrypted))
	// }

	return compressedData, nil
}

// generateSignature 根据给定的私钥和数据生成签名。
// fileID,          // 文件的唯一标识
// contentType,     // MIME类型
// checksum,        // 文件的校验和
// st,              // 文件片段的哈希表
// segmentID,       // 文件片段的唯一标识
// index,           // 分片索引
// segmentChecksum, // 分片的校验和
// encryptedData,	// 文件片段的内容(加密)
func generateSignature(privateKey *ecdsa.PrivateKey, fileId []byte,
	contentType []byte,
	checksum []byte,
	sliceTable []byte,
	index []byte,
	segmentID []byte,
	segmentsChecksum []byte, content []byte) ([]byte, error) {
	// 待签名数据
	merged, err := util.MergeFieldsForSigning(fileId, contentType, checksum, sliceTable, index, segmentID, segmentsChecksum, content)
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

// generateSignature 根据给定的私钥和已经是[]byte的数据直接生成签名。
// func generateSignature(privateKey *rsa.PrivateKey, data map[string][]byte) ([]byte, error) {
// 	// 创建一个字节切片，用于存储所有待签名数据
// 	var merged []byte

// 	// 按照特定的顺序或根据某种逻辑组合data中的值
// 	// 注意：这里假设data中"SIGNATURE"字段为空，因此在合并时跳过"SIGNATURE"字段
// 	for key, value := range data {
// 		if key != "SIGNATURE" { // 跳过"SIGNATURE"字段
// 			merged = append(merged, value...)
// 		}
// 	}

// 	// 计算合并数据的哈希值
// 	hashed := sha256.Sum256(merged)

// 	// 使用私钥对哈希值进行签名
// 	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
// 	if err != nil {
// 		return nil, fmt.Errorf("生成签名失败: %v", err)
// 	}

// 	return signature, nil
// }
