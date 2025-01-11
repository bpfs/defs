package uploads

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/bpfs/defs/files"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/pb"
)

// NewFileMeta 创建并初始化一个新的 FileMeta 实例，提供文件的基本元数据信息
// 参数：
//   - file: *os.File 文件对象，用于读取文件信息
//
// 返回值：
//   - *pb.FileMeta: 新创建的 FileMeta 实例，包含文件的基本元数据
//   - error: 如果发生错误，返回错误信息
func NewFileMeta(f *os.File) (*pb.FileMeta, error) {
	// 获取文件名
	name, err := files.GetFileName(f)
	if err != nil {
		logger.Errorf("获取文件名失败: %v, 文件: %s", err, f.Name())
		return nil, err
	}

	// 获取文件扩展名
	ext := filepath.Ext(name) // 获取文件扩展名，包含点号
	if ext != "" {
		ext = ext[1:]                            // 移除扩展名前的点号
		name = strings.TrimSuffix(name, "."+ext) // 从文件名中移除扩展名部分
	}

	// 获取文件大小
	size, err := files.GetFileSize(f)
	if err != nil {
		logger.Errorf("获取文件大小失败: %v, 文件: %s", err, f.Name())
		return nil, err
	}

	// 获取MIME类型
	mimeType, err := files.GetFileMIME(f)
	if err != nil {
		logger.Errorf("获取MIME类型失败: %v, 文件: %s", err, f.Name())
		return nil, err
	}

	// 获取CRC32校验和
	// crc32Sum, err := GetFileCRC32(f)
	// if err != nil {
	// 	logger.Errorf("计算CRC32校验和失败: %v", err)
	// 	return nil, err
	// }

	// 获取SHA256哈希值
	sha256Hash, err := files.GetFileSHA256(f)
	if err != nil {
		logger.Errorf("计算SHA256哈希值失败: %v, 文件: %s", err, f.Name())
		return nil, err
	}

	// 获取修改时间
	modTime, err := files.GetFileModTime(f)
	if err != nil {
		logger.Errorf("获取文件修改时间失败: %v, 文件: %s", err, f.Name())
		return nil, err
	}

	// 构造并返回FileMeta对象
	meta := &pb.FileMeta{
		Name:        name,           // 文件原始名称,不包含扩展名
		Extension:   ext,            // 文件扩展名,不包含点号(.)
		Size_:       size,           // 文件总大小,单位:字节
		ContentType: mimeType,       // MIME类型,用于标识文件格式
		Sha256Hash:  sha256Hash,     // 文件内容的SHA256哈希值,用于校验文件完整性
		ModifiedAt:  modTime.Unix(), // 文件最后修改的Unix时间戳
	}

	return meta, nil
}

// CalculateShards 根据文件大小和存储选项计算数据分片和奇偶校验分片的数量
// 参数：
//   - size: int64 文件大小，单位为字节
//   - opt: *fscfg.Options 存储选项，包含存储模式和其他参数
//
// 返回值：
//   - int64: 数据分片数
//   - int64: 奇偶校验分片数
//   - error: 如果发生错误，返回错误信息
func CalculateShards(size int64, opt *fscfg.Options) (int64, int64, error) {
	// 初始化数据分片和奇偶校验分片数量
	var dataShards, parityShards int64 = 1, 0

	// 根据存储模式计算分片数量
	switch opt.GetStorageMode() {
	case fscfg.FileMode:
		// 文件模式：当文件大于最大片段大小时，自动切换至切片模式
		if size > opt.GetMaxSliceSize() {
			dataShards = int64(math.Ceil(float64(size) / float64(opt.GetShardSize())))
		}

	case fscfg.SliceMode:
		// 切片模式：当文件小于最小片段大小时，保持文件模式
		if size < opt.GetMinSliceSize() {
			break
		}
		// 计算总分片数并设置为数据分片数
		totalShards := math.Ceil(float64(size) / float64(opt.GetShardSize()))
		dataShards = int64(totalShards)

	case fscfg.RS_Size:
		// 纠删码(大小)模式：使用用户定义的数据和奇偶校验分片数量
		dataShards = opt.GetDataShards()
		parityShards = opt.GetParityShards()

	case fscfg.RS_Proportion:
		// 纠删码(比例)模式：根据比例计算数据和奇偶校验分片数量
		totalShards := math.Ceil(float64(size) / float64(opt.GetShardSize()))
		dataShards = int64(float64(totalShards) / (1 + opt.GetParityRatio()))
		if dataShards < 1 {
			dataShards = 1
		}
		parityShards = int64(totalShards) - dataShards
		if parityShards < 1 {
			parityShards = 1
		}

	default:
		// 不支持的存储模式
		logger.Errorf("不支持的存储模式: %v", opt.GetStorageMode())
		return 0, 0, fmt.Errorf("不支持的存储模式: %v", opt.GetStorageMode())
	}

	return dataShards, parityShards, nil
}
