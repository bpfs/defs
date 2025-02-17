package uploads

import (
	"fmt"
	"path/filepath"

	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/segment"
	"github.com/bpfs/defs/v2/utils/paths"
)

// buildAndStoreFileSegment 构建文件片段存储map并将其存储为文件
// 参数:
//   - payload: *pb.FileSegmentStorage 文件片段存储对象
//   - hostID: string 主机ID，用于构建文件路径
//
// 返回值:
//   - error: 如果在处理过程中发生错误，返回相应的错误信息
func buildAndStoreFileSegment(payload *pb.FileSegmentStorage, hostID string) error {
	// 构建文件片段存储map
	segmentMap, err := buildFileSegmentStorageMap(payload)
	if err != nil {
		logger.Errorf("构建文件片段存储map失败: %v", err)
		return err
	}

	// 设置文件存储的路径
	filePath := filepath.Join(paths.GetSlicePath(), hostID, payload.FileId, payload.SegmentId)

	// 使用segment.WriteFileSegment存储文件片段
	if err := segment.WriteFileSegment(filePath, segmentMap); err != nil {
		logger.Errorf("存储文件片段失败: %v", err)
		return err
	}

	return nil
}

// buildFileSegmentStorageMap 构建文件片段存储map
// 参数:
//   - payload: *pb.FileSegmentStorage 文件片段存储对象
//
// 返回值:
//   - map[string][]byte: 构建的map，key为字段名称的大写，值为对应内容的[]byte
//   - error: 如果在构建过程中发生错误，返回相应的错误信息
func buildFileSegmentStorageMap(payload *pb.FileSegmentStorage) (map[string][]byte, error) {
	result := make(map[string][]byte)
	codec := segment.NewTypeCodec()

	//////////////////// 基本文件信息 ////////////////////

	// 编码FileId
	fileId, err := codec.Encode(payload.FileId)
	if err != nil {
		logger.Errorf("编码 FileId 失败: %v", err)
		return nil, err
	}
	result["FILEID"] = fileId

	// 编码Name
	name, err := codec.Encode(payload.Name)
	if err != nil {
		logger.Errorf("编码 Name 失败: %v", err)
		return nil, err
	}
	result["NAME"] = name

	// 编码Extension
	extension, err := codec.Encode(payload.Extension)
	if err != nil {
		logger.Errorf("编码 Extension 失败: %v", err)
		return nil, err
	}
	result["EXTENSION"] = extension

	// 编码Size
	size, err := codec.Encode(payload.Size_)
	if err != nil {
		logger.Errorf("编码 Size 失败: %v", err)
		return nil, err
	}
	result["SIZE"] = size

	// 编码ContentType
	contentType, err := codec.Encode(payload.ContentType)
	if err != nil {
		logger.Errorf("编码 ContentType 失败: %v", err)
		return nil, err
	}
	result["CONTENTTYPE"] = contentType

	// 编码Sha256Hash
	sha256Hash, err := codec.Encode(payload.Sha256Hash)
	if err != nil {
		logger.Errorf("编码 Sha256Hash 失败: %v", err)
		return nil, err
	}
	result["SHA256HASH"] = sha256Hash

	// 编码UploadTime
	uploadTime, err := codec.Encode(payload.UploadTime)
	if err != nil {
		logger.Errorf("编码 UploadTime 失败: %v", err)
		return nil, err
	}
	result["UPLOADTIME"] = uploadTime

	//////////////////// 身份验证和安全相关 ////////////////////

	// 编码P2PkhScript
	p2pkhScript, err := codec.Encode(payload.P2PkhScript)
	if err != nil {
		logger.Errorf("编码 P2PkhScript 失败: %v", err)
		return nil, err
	}
	result["P2PKHSCRIPT"] = p2pkhScript

	// 编码P2PkScript
	p2pkScript, err := codec.Encode(payload.P2PkScript)
	if err != nil {
		logger.Errorf("编码 P2PkScript 失败: %v", err)
		return nil, err
	}
	result["P2PKSCRIPT"] = p2pkScript

	//////////////////// 分片信息 ////////////////////

	// 编码SliceTable
	if payload.SliceTable != nil {
		sliceTableBytes, err := files.SerializeSliceTable(payload.SliceTable)
		if err != nil {
			logger.Errorf("序列化 SliceTable 失败: %v", err)
			return nil, err
		}
		sliceTable, err := codec.Encode(sliceTableBytes)
		if err != nil {
			logger.Errorf("编码 SliceTable 失败: %v", err)
			return nil, err
		}
		result["SLICETABLE"] = sliceTable
	} else {
		logger.Error("文件哈希表为空")
		return nil, fmt.Errorf("文件哈希表为空")
	}

	//////////////////// 分片元数据 ////////////////////

	// 编码SegmentId
	segmentId, err := codec.Encode(payload.SegmentId)
	if err != nil {
		logger.Errorf("编码 SegmentId 失败: %v", err)
		return nil, err
	}
	result["SEGMENTID"] = segmentId

	// 编码SegmentIndex
	segmentIndex, err := codec.Encode(payload.SegmentIndex)
	if err != nil {
		logger.Errorf("编码 SegmentIndex 失败: %v", err)
		return nil, err
	}
	result["SEGMENTINDEX"] = segmentIndex

	// 编码Crc32Checksum
	crc32Checksum, err := codec.Encode(payload.Crc32Checksum)
	if err != nil {
		logger.Errorf("编码 Crc32Checksum 失败: %v", err)
		return nil, err
	}
	result["CRC32CHECKSUM"] = crc32Checksum

	//////////////////// 分片内容和加密 ////////////////////

	// 编码SegmentContent
	segmentContent, err := codec.Encode(payload.SegmentContent)
	if err != nil {
		logger.Errorf("编码 SegmentContent 失败: %v", err)
		return nil, err
	}
	result["SEGMENTCONTENT"] = segmentContent

	// 编码EncryptionKey
	encryptionKey, err := codec.Encode(payload.EncryptionKey)
	if err != nil {
		logger.Errorf("编码 EncryptionKey 失败: %v", err)
		return nil, err
	}
	result["ENCRYPTIONKEY"] = encryptionKey

	// 编码Signature
	signature, err := codec.Encode(payload.Signature)
	if err != nil {
		logger.Errorf("编码 Signature 失败: %v", err)
		return nil, err
	}
	result["SIGNATURE"] = signature

	//////////////////// 其他属性 ////////////////////

	// 编码Shared
	shared, err := codec.Encode(payload.Shared)
	if err != nil {
		logger.Errorf("编码 Shared 失败: %v", err)
		return nil, err
	}
	result["SHARED"] = shared

	// 编码Version
	version, err := codec.Encode(payload.Version)
	if err != nil {
		logger.Errorf("编码 Version 失败: %v", err)
		return nil, err
	}
	result["VERSION"] = version

	return result, nil
}
