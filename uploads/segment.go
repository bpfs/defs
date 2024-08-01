package uploads

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/reedsolomon"
	"github.com/bpfs/defs/tempfile"
	"github.com/bpfs/defs/util"
	"github.com/sirupsen/logrus"
)

// FileSegment 描述一个文件分片的详细信息及其上传状态
// 文件被分割成多个片段独立上传，以支持大文件的高效传输和断点续传
type FileSegment struct {
	Index     int                 // 分片索引，表示该片段在文件中的顺序
	SegmentID string              // 文件片段的唯一标识
	Size      int                 // 分片大小，单位为字节，描述该片段的数据量大小
	Checksum  []byte              // 分片的校验和，用于验证该片段的完整性和一致性
	IsRsCodes bool                // 是否是纠删码片段
	Status    SegmentUploadStatus // 分片的上传状态，描述该片段的上传进度和结果
}

// NewFileSegment 创建并初始化一个新的 FileSegment 实例，提供分片的详细信息及其上传状态。
// 参数：
//   - opt: *opts.Options 文件存储选项。
//   - r: io.Reader 文件读取器。
//   - fileID: string 文件唯一标识。
//   - capacity: int64 缓冲区容量。
//   - dataShards: int64 数据分片数。
//   - parityShards: int64 奇偶校验分片数。
//
// 返回值：
//   - map[int]*FileSegment: 文件分片的映射。
//   - error: 如果发生错误，返回错误信息。
func NewFileSegment(opt *opts.Options, r io.Reader, fileID string, capacity, dataShards, parityShards int64) (map[int]*FileSegment, error) {
	// 使用子函数读取数据到 buffer。
	buf, err := readIntoBuffer(r, capacity)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 创建一个新编码器并将其初始化为您要使用的数据分片和奇偶校验分片的数量。
	enc, err := reedsolomon.New(int(dataShards), int(parityShards))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, fmt.Errorf("纠错码编码器初始化时失败: %v", err)
	}

	// 将数据切片分割为提供给编码器的分片数量，并在必要时创建空奇偶校验分片。
	shards, err := enc.Split(buf.Bytes())
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, fmt.Errorf("分割数据时失败: %v", err)
	}

	// 对一组数据分片进行奇偶校验编码。 输入是“分片”，其中包含数据分片，后跟奇偶校验分片。
	if err := enc.Encode(shards); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, fmt.Errorf("编码数据分片时失败: %v", err)
	}

	// 创建FileSegment实例映射
	segments := make(map[int]*FileSegment)
	for index, shard := range shards {
		hasher := sha256.New()
		_, err := hasher.Write(shard)
		if err != nil {
			return nil, fmt.Errorf("计算分片校验和时失败: %v", err)
		}

		// 生成分片ID
		segmentID, err := util.GenerateSegmentID(fileID, index)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return nil, fmt.Errorf("生成文件片段的唯一标识时失败: %v", err)
		}

		// 创建 FileSegment 实例
		segment := &FileSegment{
			Index:     index,                 // 分片索引
			SegmentID: segmentID,             // 文件片段的唯一标识
			Size:      len(shard),            // 分片大小
			Checksum:  hasher.Sum(nil),       // 分片的校验和
			IsRsCodes: false,                 // 是否是纠删码片段
			Status:    SegmentStatusNotReady, // 初始化时，所有分片状态为尚未准备好
		}

		// 设置是否为纠删码分片
		if index >= int(dataShards) {
			segment.IsRsCodes = true
		}

		// 添加到分片映射中
		segments[index] = segment

		// 将文件片段存储到临时存储中
		tempfile.Write(segmentID, shard)
	}

	return segments, nil
}

// readIntoBuffer 从给定的 io.Reader 中读取数据到一个预分配大小的 bytes.Buffer。
func readIntoBuffer(r io.Reader, capacity int64) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(make([]byte, 0, capacity))
	_, err := buf.ReadFrom(r)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, fmt.Errorf("从阅读器读取时失败: %v", err)
	}
	return buf, nil
}

// SetStatusNotReady 设置文件片段的状态为尚未准备好
func (segment *FileSegment) SetStatusNotReady() {
	segment.Status = SegmentStatusNotReady
}

// SetStatusPending 设置文件片段的状态为待上传
func (segment *FileSegment) SetStatusPending() {
	segment.Status = SegmentStatusPending
}

// SetStatusUploading 设置文件片段的状态为上传中
func (segment *FileSegment) SetStatusUploading() {
	segment.Status = SegmentStatusUploading
}

// SetStatusCompleted 设置文件片段的状态为已完成
func (segment *FileSegment) SetStatusCompleted() {
	segment.Status = SegmentStatusCompleted
	// 删除缓存的文件片段
	tempfile.Delete(segment.SegmentID)
}

// SetStatusFailed 设置文件片段的状态为失败
func (segment *FileSegment) SetStatusFailed() {
	segment.Status = SegmentStatusFailed
}
