package uploads

import (
	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/utils/logger"
)

// CreateFileAssetRecord 创建一个新的文件资产记录
// 参数:
//   - db: *badgerhold.Store 数据库实例
//   - fileRecord: *pb.UploadFileRecord 上传文件记录信息
//
// 返回值:
//   - error: 如果创建过程中发生错误，返回错误信息；否则返回 nil
func CreateFileAssetRecord(db *badgerhold.Store, fileRecord *pb.UploadFileRecord) error {
	// 从P2PKH脚本中提取公钥哈希
	pubKeyHash, err := script.ExtractPubKeyHashFromScript(fileRecord.FileSecurity.P2PkhScript)
	if err != nil {
		logger.Errorf("从P2PKH脚本提取公钥哈希失败: %v", err)
		return err
	}

	// 创建文件资产记录对象
	assetRecord := &pb.FileAssetRecord{
		FileId:      fileRecord.FileId,               // 文件唯一标识符
		Sha256Hash:  fileRecord.FileMeta.Sha256Hash,  // 文件内容的SHA256哈希值
		Name:        fileRecord.FileMeta.Name,        // 文件名称
		Size_:       fileRecord.FileMeta.Size_,       // 文件大小(字节)
		Extension:   fileRecord.FileMeta.Extension,   // 文件扩展名
		ContentType: fileRecord.FileMeta.ContentType, // 文件MIME类型
		PubkeyHash:  pubKeyHash,                      // 所有者的公钥哈希
		ParentId:    0,                               // 父目录ID(0表示根目录)
		Type:        0,                               // 文件类型(0表示普通文件)
		Labels:      "",                              // 文件标签(空字符串表示无标签)
		IsShared:    false,                           // 共享状态标志
		ShareAmount: 0,                               // 文件共享次数
		UploadTime:  fileRecord.StartedAt,            // 文件上传开始时间
		ModTime:     fileRecord.FinishedAt,           // 文件上传完成时间
	}

	// 创建文件资产存储实例
	store := database.NewFileAssetStore(db)

	// 保存文件资产记录到数据库
	if err := store.CreateFileAsset(assetRecord); err != nil {
		logger.Errorf("保存文件资产记录到数据库失败: %v", err)
		return err
	}

	return nil
}
