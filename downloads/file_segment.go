package downloads

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"

	"compress/gzip" // 使用标准库的gzip包

	"github.com/bpfs/defs/v2/crypto/gcm"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/script"
	ecdsa_ "github.com/bpfs/defs/v2/sign/ecdsa"
)

// VerifySegmentSignature 验证片段签名
// 参数:
//   - p: 片段内容响应对象,包含需要验证的数据和签名
//
// 返回值:
//   - error: 验证失败返回错误,验证成功返回nil
//
// 功能:
//   - 验证片段内容的签名是否有效
//   - 使用ECDSA算法进行签名验证
//   - 验证签名数据的完整性和真实性
func VerifySegmentSignature(p *pb.SegmentContentResponse) error {
	// 构造签名数据对象,包含需要验证的数据字段
	signatureData := &pb.SignatureData{
		FileId:        p.FileId,                                           // 文件ID
		ContentType:   p.FileMeta.ContentType,                             // 内容类型
		Sha256Hash:    p.FileMeta.Sha256Hash,                              // SHA256哈希
		SliceTable:    files.ConvertSliceTableToSortedSlice(p.SliceTable), // 排序后的分片表
		SegmentId:     p.SegmentId,                                        // 分片ID
		SegmentIndex:  p.SegmentIndex,                                     // 分片索引
		Crc32Checksum: p.Crc32Checksum,                                    // CRC32校验和
		EncryptedData: p.SegmentContent,                                   // 加密数据
	}

	// 从P2PK脚本中提取ECDSA公钥
	pubKey, err := script.ExtractPubKeyFromP2PKScriptToECDSA(p.P2PkScript)
	if err != nil {
		logger.Errorf("从P2PK脚本提取公钥失败: %v", err)
		return err
	}

	// 序列化签名数据为字节数组
	merged, err := signatureData.Marshal()
	if err != nil {
		logger.Errorf("序列化数据失败: %v", err)
		return err
	}

	// 计算序列化数据的MD5哈希
	hash := md5.Sum(merged)
	merged = hash[:]

	// 使用ECDSA验证签名
	valid, err := ecdsa_.VerifySignature(pubKey, merged, p.Signature)
	if err != nil || !valid {
		logger.Errorf("验证签名失败: %v", err)
		return err
	}

	return nil
}

// ProcessFunc 定义处理函数类型
type ProcessFunc func([]byte) ([]byte, error)

// 定义全局变量
var (
	// 与加密时相同的块大小
	processChunkSize = 1024 * 1024 // 1MB chunks

	globalPool     *ResourcePool
	globalPoolOnce sync.Once
)

// DecompressContext 解压缩上下文
type DecompressContext struct {
	buffer *bytes.Buffer
}

// ResourcePool 资源池结构体
type ResourcePool struct {
	streamBufferPool sync.Pool
	decompressPool   sync.Pool
}

// Global 获取全局资源池实例
// 返回值:
//   - *ResourcePool: 全局资源池实例
//
// 功能:
//   - 创建并返回一个全局资源池实例
//   - 使用sync.Once确保实例只被创建一次
func Global() *ResourcePool {
	globalPoolOnce.Do(func() {
		globalPool = &ResourcePool{
			streamBufferPool: sync.Pool{
				New: func() interface{} {
					return make([]byte, processChunkSize)
				},
			},
			decompressPool: sync.Pool{
				New: func() interface{} {
					return &DecompressContext{
						buffer: bytes.NewBuffer(make([]byte, 0, processChunkSize)),
					}
				},
			},
		}
	})
	return globalPool
}

// GetStreamBuffer 获取流缓冲区
// 返回值:
//   - []byte: 获取的流缓冲区
//
// 功能:
//   - 从资源池中获取一个预分配的流缓冲区
//   - 使用sync.Pool管理缓冲区的生命周期

func (p *ResourcePool) GetStreamBuffer() []byte {
	return p.streamBufferPool.Get().([]byte)
}

// PutStreamBuffer 归还流缓冲区
// 参数:
//   - buf: 需要归还的流缓冲区
//
// 功能:
//   - 将流缓冲区归还到资源池中
//   - 使用sync.Pool管理缓冲区的生命周期
func (p *ResourcePool) PutStreamBuffer(buf []byte) {
	p.streamBufferPool.Put(buf)
}

// GetDecompressContext 获取解压缩上下文
// 返回值:
//   - *DecompressContext: 获取的解压缩上下文
//
// 功能:
//   - 从资源池中获取一个预分配的解压缩上下文
//   - 使用sync.Pool管理上下文的实例
func (p *ResourcePool) GetDecompressContext() *DecompressContext {
	return p.decompressPool.Get().(*DecompressContext)
}

// PutDecompressContext 归还解压缩上下文
// 参数:
//   - ctx: 需要归还的解压缩上下文
//
// 功能:
//   - 将解压缩上下文归还到资源池中
//   - 使用sync.Pool管理上下文的实例
func (p *ResourcePool) PutDecompressContext(ctx *DecompressContext) {
	p.decompressPool.Put(ctx)
}

// DecryptPipeline 解密管道
type DecryptPipeline struct {
	reader     io.Reader
	writer     io.Writer
	processors []ProcessFunc
}

// Process 流式处理
// 返回值:
//   - error: 如果处理失败，返回错误信息
func (p *DecryptPipeline) Process() error {
	// 读取整个加密的数据
	data, err := io.ReadAll(p.reader)
	if err != nil {
		logger.Errorf("读取加密数据失败: %v", err)
		return err
	}

	// 对整个数据进行处理
	var processed []byte = data
	for _, proc := range p.processors {
		processed, err = proc(processed)
		if err != nil {
			logger.Errorf("处理数据失败: %v", err)
			return err
		}
	}

	// 写入解密后的数据
	if _, err := p.writer.Write(processed); err != nil {
		logger.Errorf("写入解密数据失败: %v", err)
		return err
	}

	return nil
}

// decryptChunk 解密块
func decryptChunk(key []byte) ProcessFunc {
	return func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			logger.Warn("收到空数据块，跳过解密")
			return data, nil
		}

		// 添加输入数据的详细日志
		// logger.Infof("开始解密数据块: 大小=%d bytes", len(data))
		// logger.Infof("使用的解密密钥: %s", hex.EncodeToString(key))

		// 计算AES密钥
		aesKey := md5.Sum(key)
		// logger.Infof("计算得到的AES密钥: %s", hex.EncodeToString(aesKey[:]))

		// 添加数据块前16字节的日志(如果存在)，用于验证GCM nonce
		// if len(data) >= 16 {
		// 	logger.Infof("数据块前16字节: %s", hex.EncodeToString(data[:16]))
		// }

		// 解密数据
		decryptedData, err := gcm.DecryptData(data, aesKey[:])
		if err != nil {
			logger.Errorf("解密数据失败: %v", err)
			return nil, err
		}

		// 添加解密结果的日志
		// logger.Infof("数据解密完成: 加密大小=%d bytes, 解密后大小=%d bytes",
		// 	len(data), len(decryptedData))

		return decryptedData, nil
	}
}

// decompressChunk 解压块
func decompressChunk() ProcessFunc {
	return func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			logger.Warn("收到空数据，跳过解压")
			return data, nil
		}

		ctx := Global().GetDecompressContext()
		defer Global().PutDecompressContext(ctx)

		ctx.buffer.Reset()
		reader := bytes.NewReader(data)

		decompressor, err := gzip.NewReader(reader)
		if err != nil {
			logger.Errorf("创建解压缩reader失败: %v", err)
			return nil, err
		}
		defer decompressor.Close()

		_, err = io.Copy(ctx.buffer, decompressor)
		if err != nil {
			logger.Errorf("解压数据失败: %v", err)
			return nil, err
		}

		// logger.Infof("解压完成: 压缩大小=%d bytes, 解压后大小=%d bytes",
		// 	len(data), ctx.buffer.Len())

		return ctx.buffer.Bytes(), nil
	}
}

// DecompressAndDecryptSegmentContent 解密并解压片段内容
// 参数:
//   - shareOne: 第一个密钥分片
//   - shareTwo: 第二个密钥分片
//   - encryptedData: 压缩并加密的数据内容
//   - expectedChecksum: 期望的校验和值
//
// 返回值:
//   - []byte: 解密并解压后的原始数据
//   - error: 解密或解压失败时返回错误信息
//
// 功能:
//   - 使用密钥分片恢复原始密钥
//   - 计算密钥的MD5哈希作为AES密钥
//   - 先进行AES-GCM解密
//   - 再解压缩数据
func DecompressAndDecryptSegmentContent(shareOne, shareTwo []byte, encryptedData []byte, expectedChecksum uint32) ([]byte, error) {
	// 检查密钥分片
	// logger.Infof("密钥分片1: %x", shareOne)
	// logger.Infof("密钥分片2: %x", shareTwo)

	// 恢复密钥
	decryptionKey, err := files.RecoverSecretFromShares(shareOne, shareTwo)
	if err != nil {
		logger.Errorf("从密钥分片恢复密钥失败: %v", err)
		return nil, err
	}

	// 添加密钥验证日志
	// logger.Infof("恢复的原始密钥: %x", decryptionKey)
	// aesKey := md5.Sum(decryptionKey)
	// logger.Infof("计算的AES密钥: %x", aesKey[:])

	// 创建临时文件
	tempFile, err := os.CreateTemp("", "decrypt-*")
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return nil, err
	}
	defer os.Remove(tempFile.Name())

	// 写入加密数据到临时文件
	if _, err := tempFile.Write(encryptedData); err != nil {
		logger.Errorf("写入加密数据失败: %v", err)
		return nil, err
	}

	// 重置文件指针到开始位置
	if _, err := tempFile.Seek(0, 0); err != nil {
		logger.Errorf("重置文件指针失败: %v", err)
		return nil, err
	}

	// 创建解密缓冲区
	decryptBuffer := &bytes.Buffer{}

	// 先解密
	pipe := &DecryptPipeline{
		reader: tempFile,
		writer: decryptBuffer,
		processors: []ProcessFunc{
			decryptChunk(decryptionKey),
		},
	}

	// 执行解密处理
	if err := pipe.Process(); err != nil {
		logger.Errorf("解密处理失败: %v", err)
		return nil, err
	}

	// 计算解密后(压缩状态)的校验和
	decryptedData := decryptBuffer.Bytes()
	actualChecksum := crc32.ChecksumIEEE(decryptedData)
	if actualChecksum != expectedChecksum {
		logger.Errorf("校验和验证失败: 期望值=%d, 实际值=%d", expectedChecksum, actualChecksum)
		return nil, fmt.Errorf("校验和不匹配")
	}

	// 再解压
	decompressBuffer := &bytes.Buffer{}
	pipe = &DecryptPipeline{
		reader: bytes.NewReader(decryptedData),
		writer: decompressBuffer,
		processors: []ProcessFunc{
			decompressChunk(),
		},
	}

	if err := pipe.Process(); err != nil {
		logger.Errorf("解压处理失败: %v", err)
		return nil, err
	}

	return decompressBuffer.Bytes(), nil
}

// VerifySegmentChecksum 验证片段校验和
// 参数:
//   - content: 需要验证的内容数据
//   - expectedChecksum: 期望的校验和值
//
// 返回值:
//   - error: 校验和不匹配返回错误,匹配返回nil
//
// 功能:
//   - 计算内容的CRC32校验和
//   - 验证计算的校验和与期望值是否匹配
//   - 确保数据完整性
func VerifySegmentChecksum(content []byte, expectedChecksum uint32) error {
	// 计算内容的CRC32校验和
	actualChecksum := crc32.ChecksumIEEE(content)

	// 比较计算的校验和与期望值
	if actualChecksum != expectedChecksum {
		logger.Errorf("校验和验证失败: 期望值=%d, 实际值=%d", expectedChecksum, actualChecksum)
		return fmt.Errorf("校验和不匹配")
	}

	return nil
}
