// Package uploads 提供文件上传相关的功能实现
package uploads

import (
	"crypto/ecdsa"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/crypto/gcm"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/reedsolomon"
	sign "github.com/bpfs/defs/v2/sign/ecdsa"

	"github.com/bpfs/defs/v2/zip/gzip"
)

// NewFileSegment 创建并初始化一个新的 FileSegment 实例，提供分片的详细信息及其上传状态。
// 参数：
//   - db: *badgerhold.Store 数据库实例
//   - taskID: string 任务ID，用于标识上传任务
//   - fileID: string 文件ID，用于标识文件
//   - file: *os.File 待处理的文件对象
//   - pk: []byte 用于加密的公钥
//   - dataShards: int64 数据分片数量
//   - parityShards: int64 奇偶校验分片数量
//
// 返回值：
//   - error: 如果发生错误，返回错误信息
func NewFileSegment(db *badgerhold.Store, taskID string, fileID string, file *os.File, pk []byte, dataShards, parityShards int64) error {
	// 确保文件在函数结束时关闭
	defer file.Close()

	// 更新文件状态为编码中
	if err := UpdateUploadFileStatus(db, taskID, pb.UploadStatus_UPLOAD_STATUS_ENCODING); err != nil {
		logger.Errorf("更新文件状态为编码中失败,taskID:%s,err:%v", taskID, err)
		return err
	}

	// 创建Reed-Solomon编码器，用于生成纠删码
	enc, err := reedsolomon.NewFile(int(dataShards), int(parityShards))
	if err != nil {
		logger.Errorf("纠错码编码器初始化失败,dataShards:%d,parityShards:%d,err:%v", dataShards, parityShards, err)
		return err
	}

	// 使用SplitFile方法将文件分割成多个分片
	shardFiles, err := enc.SplitFile(file)
	if err != nil {
		logger.Errorf("分割数据失败,file:%s,err:%v", file.Name(), err)
		return err
	}

	// 对分片进行编码，生成奇偶校验分片
	if err := enc.EncodeFile(shardFiles); err != nil {
		logger.Errorf("编码数据分片失败,file:%s,err:%v", file.Name(), err)
		return err
	}

	// 初始化一个空的映射，用于存储分片索引和对应的HashTable实例
	hashTableMap := make(map[int64]*pb.HashTable)

	// 遍历处理每个分片
	for index, shard := range shardFiles {
		// 检查分片是否存在
		if shard == nil {
			logger.Errorf("分片不存在,index:%d", index)
			return fmt.Errorf("分片 %d 不存在", index)
		}

		// 获取分片文件大小
		size, err := files.GetFileSize(shard)
		if err != nil {
			logger.Errorf("获取分片大小失败,index:%d,err:%v", index, err)
			return err
		}

		// 计算分片的CRC32校验和
		checksum, err := files.GetFileCRC32(shard)
		if err != nil {
			logger.Errorf("计算分片校验和失败,index:%d,err:%v", index, err)
			return err
		}

		// 设置当前片段在文件中的索引位置,从0开始
		segmentIndex := int64(index)

		// 根据文件ID和分片索引生成唯一的分片ID
		segmentID, err := files.GenerateSegmentID(fileID, segmentIndex)
		if err != nil {
			logger.Errorf("生成文件片段的唯一标识失败,fileID:%s,segmentIndex:%d,err:%v", fileID, segmentIndex, err)
			return err
		}

		// 读取分片文件内容
		content, err := readSegmentContent(shard)
		if err != nil {
			logger.Errorf("读取文件片段内容失败,segmentID:%s,err:%v", segmentID, err)
			return err
		}

		// 对分片内容进行压缩和加密处理
		encryptedData, err := compressAndEncrypt(pk, content)
		if err != nil {
			logger.Errorf("压缩和加密文件分片失败,segmentID:%s,err:%v", segmentID, err)
			return err
		}

		// 判断当前分片是否为奇偶校验分片
		isRsCodes := segmentIndex >= dataShards

		// 在数据库中创建分片记录
		if err := CreateUploadSegmentRecord(
			db,            // 数据库实例
			taskID,        // 任务ID
			segmentID,     // 分片ID
			segmentIndex,  // 分片索引
			size,          // 分片大小
			checksum,      // 分片校验和
			encryptedData, // 加密后的分片内容
			isRsCodes,     // 是否为纠删码分片
			pb.SegmentUploadStatus_SEGMENT_UPLOAD_STATUS_PENDING, // 初始状态为待上传
		); err != nil {
			logger.Errorf("创建分片记录失败,taskID:%s,segmentID:%s,err:%v", taskID, segmentID, err)
			return err
		}

		// 创建并填充HashTable实例
		hashTableMap[segmentIndex] = &pb.HashTable{
			SegmentId:     segmentID,    // 分片唯一标识
			SegmentIndex:  segmentIndex, // 分片索引位置
			Crc32Checksum: checksum,     // CRC32校验和
			IsRsCodes:     isRsCodes,    // 是否为纠删码分片
		}
		// 清理分片内容
		shard = nil
		encryptedData = nil
	}

	// 更新文件记录的分片表和状态
	if err := UpdateUploadFileHashTable(db, taskID, hashTableMap); err != nil {
		logger.Errorf("更新文件状态和分片表失败,taskID:%s,err:%v", taskID, err)
		return err
	}
	// 清理HashTable映射
	hashTableMap = nil
	runtime.GC() // 强制触发垃圾回收
	return nil
}

// readSegmentContent 读取文件片段的内容
// 参数：
//   - file: *os.File 文件片段对象
//
// 返回值：
//   - []byte: 读取的文件内容
//   - error: 如果发生错误，返回错误信息
func readSegmentContent(file *os.File) ([]byte, error) {
	// 获取文件片段的大小信息
	segmentInfo, err := file.Stat()
	if err != nil {
		logger.Errorf("获取文件片段信息失败,file:%s,err:%v", file.Name(), err)
		return nil, err
	}

	// 根据文件大小创建缓冲区
	content := make([]byte, segmentInfo.Size())

	// 将文件指针重置到开始位置
	if _, err := file.Seek(0, 0); err != nil {
		logger.Errorf("重置文件片段指针失败,file:%s,err:%v", file.Name(), err)
		return nil, err
	}

	// 读取文件内容到缓冲区
	n, err := file.Read(content)
	if err != nil {
		logger.Errorf("读取文件片段内容失败,file:%s,err:%v", file.Name(), err)
		return nil, err
	}

	// 验证读取的字节数是否正确
	if int64(n) != segmentInfo.Size() {
		logger.Errorf("读取的字节数与文件片段大小不匹配,file:%s,read:%d,size:%d", file.Name(), n, segmentInfo.Size())
		return nil, fmt.Errorf("读取的字节数 (%d) 与文件片段大小 (%d) 不匹配", n, segmentInfo.Size())
	}

	return content, nil
}

// compressAndEncrypt 对数据先进行加密再进行压缩
// 参数:
//   - pk: []byte 用于生成加密密钥的公钥
//   - data: []byte 需要加密和压缩的原始数据
//
// 返回值:
//   - []byte: 加密并压缩后的数据
//   - error: 如果在加密或压缩过程中发生错误，返回相应的错误信息
func compressAndEncrypt(pk, data []byte) ([]byte, error) {
	// 使用MD5对公钥进行哈希，生成16字节的AES密钥
	key := md5.Sum(pk)

	// 使用GCM模式和生成的密钥对数据进行加密
	encryptedData, err := gcm.EncryptData(data, key[:])
	if err != nil {
		logger.Errorf("加密数据失败,err:%v", err)
		return nil, err
	}

	// 对加密后的数据进行GZIP压缩
	compressedData, err := gzip.CompressData(encryptedData)
	if err != nil {
		logger.Errorf("压缩数据失败,err:%v", err)
		return nil, err
	}

	// // TODO：测试代码，用于验证加密解密过程
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
	encryptedData = nil
	return compressedData, nil
}

// generateSignature 使用私钥对 SignatureData 进行签名
// 参数:
//   - privateKey: *ecdsa.PrivateKey ECDSA私钥，用于生成签名
//   - data: *pb.SignatureData 需要签名的数据结构
//
// 返回值:
//   - []byte: 生成的签名数据
//   - error: 如果在签名过程中发生错误，返回相应的错误信息
func generateSignature(privateKey *ecdsa.PrivateKey, data *pb.SignatureData) ([]byte, error) {
	// 将 SignatureData 结构体序列化为字节数组
	dataBytes, err := data.Marshal()
	if err != nil {
		logger.Errorf("序列化SignatureData失败,err:%v", err)
		return nil, err
	}

	// 对序列化后的数据计算MD5哈希
	hash := md5.Sum(dataBytes)
	merged := hash[:]

	// 打印哈希值的十六进制表示，用于调试
	logger.Infof("===> %v", hex.EncodeToString(merged))

	// 使用ECDSA私钥对哈希值进行签名
	signature, err := sign.SignData(privateKey, merged)
	if err != nil {
		logger.Errorf("签名数据失败,err:%v", err)
		return nil, err
	}

	return signature, nil
}
