package uploads

import (
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/pb"
)

// handleSendingToNetwork 处理发送任务到网络的请求
// 参数:
//   - params: RegisterUploadPointSubProtocolParams 类型，包含注册所需的所有依赖项
//
// 返回值:
//   - func([]byte) ([]byte, error): 处理发送任务到网络的请求的函数
func handleSendingToNetwork(params RegisterUploadPointSubProtocolParams) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		// 解析请求载荷
		payload := new(pb.FileSegmentStorage)
		if err := payload.Unmarshal(data); err != nil {
			logger.Errorf("解码请求载荷失败: %v", err)
			return nil, err
		}

		logger.Infof("=====> SegmentId: %v", payload.SegmentId)

		// 复用 network_stream.go 中的函数
		if err := buildAndStoreFileSegment(payload, params.Host.ID().String()); err != nil {
			logger.Errorf("存储接收内容失败: %v", err)
			return nil, err
		}

		// 创建 FileSegmentStorageStore 实例
		store := database.NewFileSegmentStorageSqlStore(params.DB.SqliteDB)
		payloadSql, err := database.ToFileSegmentStorageSql(payload)
		if err != nil {
			logger.Errorf("将 FileSegmentStorage 转换为 FileSegmentStorageSql失败: %v", err)
			return nil, err
		}

		// 将文件片段存储记录保存到数据库
		if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
			logger.Errorf("保存文件片段存储记录失败: %v", err)
			return nil, err
		}

		return []byte("success"), nil
	}
}

// handleForwardToNetwork 处理转发任务到网络的请求
// 参数:
//   - params: RegisterUploadPointSubProtocolParams 类型，包含注册所需的所有依赖项
//
// 返回值:
//   - func([]byte) ([]byte, error): 处理转发任务到网络的请求的函数
func handleForwardToNetwork(params RegisterUploadPointSubProtocolParams) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		// 解析请求载荷
		payload := new(pb.FileSegmentStorage)
		if err := payload.Unmarshal(data); err != nil {
			logger.Errorf("解码请求载荷失败: %v", err)
			return nil, err
		}

		logger.Infof("转发=====> SegmentId: %v,内容%d", payload.SegmentId, len(payload.SegmentContent))

		// 复用 network_stream.go 中的函数
		if err := buildAndStoreFileSegment(payload, params.Host.ID().String()); err != nil {
			logger.Errorf("存储接收内容失败: %v", err)
			return nil, err
		}

		// 创建 FileSegmentStorageStore 实例
		store := database.NewFileSegmentStorageSqlStore(params.DB.SqliteDB)
		payloadSql, err := database.ToFileSegmentStorageSql(payload)
		if err != nil {
			logger.Errorf("将 FileSegmentStorage 转换为 FileSegmentStorageSql失败: %v", err)
			return nil, err
		}

		// 将文件片段存储记录保存到数据库
		if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
			logger.Errorf("保存文件片段存储记录失败: %v", err)
			return nil, err
		}

		return []byte("success"), nil
	}
}
