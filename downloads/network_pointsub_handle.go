package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/segment"
	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/gogo/protobuf/proto"
)

// handleRequestSegment 处理请求文件片段的请求
// 参数:
//   - params: RegisterDownloadPointSubProtocolParams 类型，包含注册所需的所有依赖项
//
// 返回值:
//   - func([]byte) ([]byte, error): 处理请求片段的函数
func handleRequestSegment(params RegisterDownloadPointSubProtocolParams) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		// 解析请求
		request := new(pb.SegmentContentRequest)
		if err := request.Unmarshal(data); err != nil {
			logger.Errorf("解析请求失败: %v", err)
			return nil, err
		}

		logger.Infof("收到片段请求: segmentID=%s", request.SegmentId)

		// 创建 FileSegmentStorageStore 实例
		store := database.NewFileSegmentStorageSqlStore(params.DB.SqliteDB)

		// 获取文件片段存储记录
		segmentStorage, err := store.GetFileSegmentStorage(request.SegmentId)
		if err != nil {
			logger.Errorf("获取文件片段存储记录失败: %v", err)
			return nil, err
		}
		// 构建文件元数据
		fileMeta := &pb.FileMeta{
			Name:        segmentStorage.Name,
			Extension:   segmentStorage.Extension,
			Size_:       segmentStorage.Size_,
			ContentType: segmentStorage.ContentType,
			Sha256Hash:  segmentStorage.Sha256Hash,
		}

		// 构建片段存储路径
		subDir := filepath.Join(paths.GetSlicePath(), params.Host.ID().String(), request.FileId)

		// 打开片段文件
		file, err := os.Open(filepath.Join(subDir, segmentStorage.SegmentId))
		if err != nil {
			logger.Errorf("打开文件失败: %v", err)
			return nil, err
		}
		defer file.Close()

		// 定义需要读取的片段类型
		segmentTypes := []string{"SEGMENTID", "SEGMENTINDEX", "SEGMENTCONTENT", "SIGNATURE"}

		// 读取文件片段
		segmentResults, _, err := segment.ReadFileSegments(file, segmentTypes)
		if err != nil {
			logger.Errorf("读取文件片段失败: %v", err)
			return nil, err
		}

		// 获取并验证片段ID
		id, exists := segmentResults["SEGMENTID"]
		if !exists {
			logger.Error("片段ID不存在")
			return nil, fmt.Errorf("片段ID不存在")
		}

		// 获取并验证片段索引
		index, exists := segmentResults["SEGMENTINDEX"]
		if !exists {
			logger.Error("片段索引不存在")
			return nil, fmt.Errorf("片段索引不存在")
		}

		// 创建类型解码器
		codec := segment.NewTypeCodec()

		// 解码片段ID
		idDecode, err := codec.Decode(id.Data)
		if err != nil {
			logger.Errorf("解码片段ID失败: %v", err)
			return nil, err
		}

		// 解码片段索引
		indexDecode, err := codec.Decode(index.Data)
		if err != nil {
			logger.Errorf("解码片段索引失败: %v", err)
			return nil, err
		}

		// 验证片段标识和索引
		if !reflect.DeepEqual(idDecode, segmentStorage.SegmentId) || !reflect.DeepEqual(indexDecode, segmentStorage.SegmentIndex) {
			logger.Errorf("文件片段标识或索引不匹配")
			return nil, fmt.Errorf("文件片段标识或索引不匹配")
		}

		// 获取并验证片段内容
		content, exists := segmentResults["SEGMENTCONTENT"]
		if !exists {
			logger.Error("片段内容不存在")
			return nil, fmt.Errorf("片段内容不存在")
		}

		// 解码片段内容
		contentDecodeDecode, err := codec.Decode(content.Data)
		if err != nil {
			logger.Errorf("解码片段内容失败: %v", err)
			return nil, err
		}

		// 获取并验证签名
		signature, exists := segmentResults["SIGNATURE"]
		if !exists {
			logger.Error("签名不存在")
			return nil, fmt.Errorf("签名不存在")
		}

		// 解码签名
		signatureDecode, err := codec.Decode(signature.Data)
		if err != nil {
			logger.Errorf("解码签名失败: %v", err)
			return nil, err
		}

		// 反序列化切片表
		var wrapper database.SliceTableWrapper
		if err := proto.Unmarshal(segmentStorage.SliceTable, &wrapper); err != nil {
			return nil, err
		}

		// 构建切片表映射
		sliceTable := make(map[int64]*pb.HashTable)
		for _, entry := range wrapper.Entries {
			value := entry.Value
			sliceTable[entry.Key] = &value
		}

		// 构建响应对象
		response := &pb.SegmentContentResponse{
			TaskId:         request.TaskId,
			FileId:         request.FileId,
			FileMeta:       fileMeta,
			P2PkScript:     segmentStorage.P2PkScript,
			SegmentId:      segmentStorage.SegmentId,
			SegmentIndex:   segmentStorage.SegmentIndex,
			Crc32Checksum:  segmentStorage.Crc32Checksum,
			SegmentContent: contentDecodeDecode.([]byte),
			EncryptionKey:  segmentStorage.EncryptionKey,
			Signature:      signatureDecode.([]byte),
			SliceTable:     sliceTable,
		}

		// 序列化响应
		responseData, err := response.Marshal()
		if err != nil {
			logger.Errorf("序列化响应失败: %v", err)
			return nil, err
		}

		logger.Infof("成功发送片段: segmentID=%s, size=%d", request.SegmentId, len(response.SegmentContent))
		return responseData, nil
	}
}
