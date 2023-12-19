// 文件

package defs

import (
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/spf13/afero"
)

// FileInfo 描述一个文件
type FileInfo struct {
	assetID      string            // 文件资产的唯一标识(外部标识)
	fileHash     string            // 文件内容的哈希值(内部标识)
	name         string            // 文件的基本名称
	size         int64             // 常规文件的长度(以字节为单位)
	modTime      time.Time         // 修改时间(非文件修改时间)
	uploadTime   time.Time         // 上传时间
	dataShards   int64             // 数据片段的数量
	parityShards int64             // 奇偶校验片段的数量
	fileType     string            // 文件类型或格式
	publicKey    []byte            // 文件所有者的公钥
	sliceTable   map[int]HashTable // 切片内容的哈希表
	sliceList    []SliceInfo       // 切片列表
}

type HashTable struct {
	Hash    string // 文件片段的哈希值
	RsCodes bool   // 是否为纠删码
}

// readFile 从文件路径中读取文件
func (fs *FS) readFile(filename string) (*FileInfo, error) {
	afero := afero.NewOsFs()
	f, err := afero.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fileInfo, _, err := populateFileInfo(f, fs.opt.maxBufferSize) // 填充文件信息
	if err != nil {
		return nil, err
	}

	return fileInfo, nil
}

// readFileWithShards 从文件路径中读取并分片文件，使用指定的数据分片和奇偶分片
func (fs *FS) readFileWithShards(filename string, dataShards, parityShards int64) (*FileInfo, error) {
	afero := afero.NewOsFs()
	f, err := afero.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fileInfo, size, err := populateFileInfo(f, fs.opt.maxBufferSize) // 填充文件信息
	if err != nil {
		return nil, err
	}

	if size < int64(dataShards+parityShards) {
		return nil, fmt.Errorf("文件过小，无法满足分片要求")
	}

	fileInfo.dataShards = dataShards     // 数据片段的数量
	fileInfo.parityShards = parityShards // 奇偶校验片段的数量

	return fileInfo.finalizeFileRead(fs.cache, f, size, fs.opt.defaultBufSize)
}

// readFileWithSizeAndRatio 从文件路径中读取并分片文件，根据分片大小和奇偶比例来确定数据分片和奇偶分片
func (fs *FS) readFileWithSizeAndRatio(filename string, shardSize int64, parityRatio float64) (*FileInfo, error) {
	afero := afero.NewOsFs()

	// 打开文件
	f, err := afero.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fileInfo, size, err := populateFileInfo(f, fs.opt.maxBufferSize)
	if err != nil {
		return nil, err
	}

	if size < shardSize {
		return nil, fmt.Errorf("文件过小，小于要求的分片大小")
	}

	// 计算数据分片和奇偶分片数量
	totalShards := math.Ceil(float64(size) / float64(shardSize))     // 回大于或等于 x 的最小整数值
	fileInfo.dataShards = int64(totalShards / (1 + parityRatio))     // 数据片段的数量
	fileInfo.parityShards = int64(totalShards) - fileInfo.dataShards // 奇偶校验片段的数量

	return fileInfo.finalizeFileRead(fs.cache, f, size, fs.opt.defaultBufSize)
}

// populateFileInfo 填充文件信息
func populateFileInfo(f afero.File, maxBufferSize int64) (*FileInfo, int64, error) {
	fileInfo := new(FileInfo)
	var size int64

	if fi, err := f.Stat(); err == nil {
		// 文件的基本名称
		fileInfo.name = fi.Name()
		// 常规文件的长度（以字节为单位）
		fileInfo.size = fi.Size()
		// 修改时间
		fileInfo.modTime = fi.ModTime()
		// 上传时间
		fileInfo.uploadTime = time.Now()
		// 文件类型或格式
		fileInfo.fileType = strings.TrimPrefix(filepath.Ext(fi.Name()), ".")

		size = fi.Size() // 常规文件的大小（以字节为单位）
		if size > maxBufferSize {
			return nil, size, fmt.Errorf("文件的大小 %d 不可大于 %d", size, maxBufferSize)
		}
	} else {
		return nil, 0, err
	}

	fileHash, err := calculateFileHash(f)
	if err != nil {
		return nil, 0, err
	}
	// 文件内容的哈希值
	fileInfo.fileHash = hex.EncodeToString(fileHash)

	_, _ = f.Seek(0, io.SeekStart)

	return fileInfo, size, nil
}

func (fileInfo *FileInfo) finalizeFileRead(cache *ristretto.Cache, f io.Reader, n, defaultBufSize int64) (*FileInfo, error) {
	// 读取分割
	if err := fileInfo.readSplit(cache, f, n+defaultBufSize, fileInfo.dataShards, fileInfo.parityShards); err != nil {
		return nil, err
	}
	return fileInfo, nil
}

// AssetID 资产的唯一标识
func (fi *FileInfo) AssetID() string {
	return fi.assetID
}

// Name 文件的基本名称
func (fi *FileInfo) Name() string {
	return fi.name
}

// Size 常规文件的长度（以字节为单位）
func (fi *FileInfo) Size() int64 {
	return fi.size
}

// ModTime 修改时间
func (fi *FileInfo) ModTime() time.Time {
	return fi.modTime
}

// UploadTime 上传时间
func (fi *FileInfo) UploadTime() time.Time {
	return fi.uploadTime
}

// FileHash 文件内容的哈希值
func (fi *FileInfo) FileHash() string {
	return fi.fileHash
}

// fileType 文件类型或格式
func (fi *FileInfo) FileType() string {
	return fi.fileType
}

// publicKey 文件所有者的公钥
// func (fi *FileInfo) PublicKey() *rsa.PublicKey {
func (fi *FileInfo) PublicKey() []byte {
	return fi.publicKey
}

// DataShards 数据分片
func (fi *FileInfo) DataShards() int64 {
	return fi.dataShards
}

// ParityShards 奇偶分片
func (fi *FileInfo) ParityShards() int64 {
	return fi.parityShards
}

// SliceTable 切片内容的哈希表
func (fi *FileInfo) SliceTable() map[int]HashTable {
	return fi.sliceTable
}

// Slice 切片列表
func (fi *FileInfo) SliceList() []SliceInfo {
	return fi.sliceList
}

// func ReadFile(filename string, dataShards int, parityShards int) (*FileInfo, error) {
// 	fs := afero.NewOsFs()

// 	// Open 打开一个文件，返回该文件或错误（如果发生）。
// 	f, err := fs.Open(filename)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer f.Close()

// 	// FileInfo 会准确地告诉我们要读多少内容，这是一个很好但不确定的赌注，所以让我们尝试一下，但要做好答案错误的准备。
// 	var n int64

// 	fileInfo := new(FileInfo)

// 	if fi, err := f.Stat(); err == nil {
// 		fileInfo.name = fi.Name()       // 文件的基本名称
// 		fileInfo.size = fi.Size()       // 常规文件的长度（以字节为单位）； 系统依赖于其他人
// 		fileInfo.modTime = fi.ModTime() // 修改时间

// 		// 不要预先分配巨大的缓冲区，以防万一。
// 		if size := fi.Size(); size < 1e9 {
// 			n = size
// 		}
// 	}

// 	// 计算文件的hash
// 	hash, err := calculateFileHash(f)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// EncodeToString 返回 src 的十六进制编码。
// 	fileInfo.hash = hex.EncodeToString(hash)
// 	fileInfo.dataShards = dataShards
// 	fileInfo.parityShards = parityShards

// 	// 重置文件阅读器
// 	_, _ = f.Seek(0, io.SeekStart)

// 	if err := fileInfo.readSplit(f, n+bytes.MinRead, fileInfo.dataShards, fileInfo.parityShards); err != nil {
// 		return nil, err
// 	}

// 	return fileInfo, err

// }
