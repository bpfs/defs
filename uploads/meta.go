package uploads

import (
	"crypto/ecdsa"
	"fmt"
	"math"
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/defs/wallets"
	"github.com/sirupsen/logrus"
)

// FileMeta 代表文件的基本元数据信息
// 它为文件上传提供了必要的描述信息，如文件大小、类型等
type FileMeta struct {
	FileID      string // 文件唯一标识，用于在系统内部唯一区分文件
	Name        string // 文件名，包括扩展名，描述文件的名称
	Extension   string // 文件的扩展名
	Size        int64  // 文件大小，单位为字节，描述文件的总大小
	ContentType string // MIME类型，表示文件的内容类型，如"text/plain"
	Checksum    []byte // 文件的校验和，用于在上传前后验证文件的完整性和一致性
}

// NewFileMeta 创建并初始化一个新的 FileMeta 实例，提供文件的基本元数据信息。
// 参数：
//   - file: afero.File 文件对象。
//   - privateKey: *ecdsa.PrivateKey ECDSA 私钥，用于生成文件ID。
//
// 返回值：
//   - *FileMeta: 新创建的 FileMeta 实例，包含文件的基本元数据。
//   - error: 如果发生错误，返回错误信息。
func NewFileMeta(file afero.File, privateKey *ecdsa.PrivateKey) (*FileMeta, error) {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		logrus.Errorf("[%s] 获取文件信息失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 获取文件名
	name := fileInfo.Name()

	// 获取文件的 MIME 类型
	contentType, err := util.GetContentType(file)
	if err != nil {
		logrus.Errorf("[%s] 获取 MIME 类型失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 计算文件的校验和
	checksum, err := util.GetFileChecksum(file)
	if err != nil {
		logrus.Errorf("[%s] 计算文件校验和失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 提取私钥对应的公钥
	publicKeyBytes := wallets.ExtractPublicKey(privateKey)
	// 将公钥和校验和拼接生成文件ID
	combined := append(publicKeyBytes, checksum...)
	// 使用校验和生成文件的唯一标识
	fileID, err := util.GenerateFileID(combined)
	if err != nil {
		logrus.Errorf("[%s] 生成文件 ID 失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 返回文件元数据信息
	return &FileMeta{
		FileID:      fileID,             // 文件唯一标识
		Name:        name,               // 文件名
		Extension:   filepath.Ext(name), // 文件扩展名
		Size:        fileInfo.Size(),    // 文件大小
		ContentType: contentType,        // MIME 类型
		Checksum:    checksum,           // 文件的校验和
	}, nil
}

// CalculateShards 根据文件大小和存储选项计算数据分片和奇偶校验分片的数量。
// 参数：
//   - opt: *opts.Options 存储选项，包含存储模式和其他参数。
//
// 返回值：
//   - int64: 数据分片数。
//   - int64: 奇偶校验分片数。
//   - error: 如果发生错误，返回错误信息。
func (meta *FileMeta) CalculateShards(opt *opts.Options) (int64, int64, error) {
	// 初始化数据分片和奇偶校验分片数量
	var dataShards, parityShards int64 = 1, 0

	// 根据存储模式计算分片数量
	switch opt.GetStorageMode() {
	case opts.FileMode:
		if meta.Size > opt.GetMaxSliceSize() {
			// 文件大于最大片段的大小时，自动切换至切片模式
			dataShards = int64(math.Ceil(float64(meta.Size) / float64(opt.GetShardSize())))
		}

	case opts.SliceMode:
		if meta.Size < opt.GetMinSliceSize() {
			// 文件小于最小片段的大小时，保持文件模式
			break
		}
		totalShards := math.Ceil(float64(meta.Size) / float64(opt.GetShardSize()))
		dataShards = int64(totalShards)

	case opts.RS_Size:
		// 纠删码(大小)模式，直接使用用户定义的数据和奇偶校验分片数量
		dataShards = opt.GetDataShards()
		parityShards = opt.GetParityShards()

	case opts.RS_Proportion:
		// 纠删码(比例)模式，根据比例计算数据和奇偶校验分片数量
		totalShards := math.Ceil(float64(meta.Size) / float64(opt.GetShardSize()))
		dataShards = int64(float64(totalShards) / (1 + opt.GetParityRatio()))
		parityShards = int64(totalShards) - dataShards

	default:
		return 0, 0, fmt.Errorf("不支持的存储模式")
	}

	// 返回数据分片数和奇偶校验分片数
	return dataShards, parityShards, nil
}
