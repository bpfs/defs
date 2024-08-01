package uploads

import (
	"crypto/ecdsa"
	"path"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/shamir"
	"github.com/bpfs/defs/util"
	"github.com/sirupsen/logrus"
)

// UploadFile 包含待上传文件的详细信息及其分片信息
// 它是上传任务的核心部分，定义了文件如何被处理和上传
type UploadFile struct {
	FileMeta                         // 文件的元数据，如文件名、大小、类型等
	Segments    map[int]*FileSegment // 文件分片信息，详细描述了文件如何被分割为多个片段进行上传
	TempStorage string               // 文件的临时存储位置，用于保存文件分片的临时数据
	Security    *FileSecurity        // 文件的安全和加密相关信息
	SliceTable  map[int]*HashTable   // 文件片段的哈希表，记录每个片段的哈希值，支持纠错和数据完整性验证
	StartedAt   int64                // 文件上传的开始时间戳，记录任务开始执行的时间点
	FinishedAt  int64                // 文件上传的完成时间戳，记录任务完成执行的时间点
}

// NewUploadFile 创建并初始化一个新的 UploadFile 实例。
// 参数：
//   - opt: *opts.Options 文件存储选项。
//   - ownerPriv: *ecdsa.PrivateKey 文件所有者的私钥。
//   - file: afero.File 文件对象。
//   - scheme: *shamir.ShamirScheme Shamir 秘钥共享方案。
//
// 返回值：
//   - *UploadFile: 新创建的 UploadFile 实例。
//   - error: 如果发生错误，返回错误信息。
func NewUploadFile(opt *opts.Options, ownerPriv *ecdsa.PrivateKey, file afero.File, scheme *shamir.ShamirScheme) (*UploadFile, error) {
	// 生成FileMeta实例
	fileMeta, err := NewFileMeta(file, ownerPriv)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 使用文件所有者的私钥和FileID生成秘密
	secret, err := util.GenerateSecretFromPrivateKeyAndChecksum(ownerPriv, []byte(fileMeta.FileID))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 检查并用文件的哈希值创建文件夹
	tempStorage := path.Join(paths.GetRootPath(), paths.GetUploadPath(), fileMeta.FileID)
	// 检查文件夹是否存在，不存在则新建
	if err := util.CheckAndMkdir(tempStorage); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 创建并初始化一个新的FileSecurity实例，封装了文件的安全和权限相关的信息
	fileSecurity, err := NewFileSecurity(ownerPriv, file, scheme, secret)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 根据文件大小和存储选项计算数据分片和奇偶校验分片的数量
	dataShards, parityShards, err := fileMeta.CalculateShards(opt)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	capacity := fileMeta.Size + opt.GetDefaultBufSize()

	// 创建并初始化一个新的FileSegment实例，提供分片的详细信息及其上传状态
	segments, err := NewFileSegment(opt, file, fileMeta.FileID, capacity, dataShards, parityShards)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 创建并初始化一个映射，映射的键是分片的索引，值是HashTable实例。
	sliceTable := NewHashTable(segments, dataShards)

	u := &UploadFile{
		Segments:    segments,           // 文件分片信息
		TempStorage: tempStorage,        // 文件的临时存储位置
		Security:    fileSecurity,       // 文件的安全和加密相关信息
		SliceTable:  sliceTable,         // 文件片段的哈希表
		StartedAt:   time.Now().Unix(),  // 文件上传的开始时间戳
		FinishedAt:  time.Time{}.Unix(), // 文件上传的完成时间戳
	}
	u.FileID = fileMeta.FileID           // 文件唯一标识，用于在系统内部唯一区分文件
	u.Name = fileMeta.Name               // 文件名，包括扩展名，描述文件的名称
	u.Extension = fileMeta.Extension     // 文件的扩展名
	u.Size = fileMeta.Size               // 文件大小，单位为字节，描述文件的总大小
	u.ContentType = fileMeta.ContentType // MIME类型，表示文件的内容类型，如"text/plain"
	u.Checksum = fileMeta.Checksum       // 文件的校验和，用于在上传前后验证文件的完整性和一致性

	return u, nil
}
