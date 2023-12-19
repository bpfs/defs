package defs

import (
	"fmt"
)

// 文件下载请求(清单)
type FileDownloadRequestChecklistPayload struct {
	AssetID string // 文件资产的唯一标识(外部标识)
}

// 文件下载请求(内容)
type FileDownloadRequestContentPayload struct {
	AssetID   string // 文件资产的唯一标识(外部标识)
	SliceHash string // 待下载的切片哈希
	Index     int    // 文件片段的索引(该片段在文件中的顺序位置)
}

// 文件下载响应(清单)
type FileDownloadResponseChecklistPayload struct {
	AssetID         string            // 文件资产的唯一标识(外部标识)
	FileHash        string            // 文件内容的哈希值(内部标识)
	Name            string            // 文件的基本名称
	Size            int64             // 常规文件的长度(以字节为单位)
	SliceTable      map[int]HashTable // 文件片段的哈希表
	AvailableSlices map[int]string    // 本地存储的文件片段信息
}

// 文件下载响应(内容)
type FileDownloadResponseContentPayload struct {
	AssetID      string // 文件资产的唯一标识(外部标识)
	SliceHash    string // 下载的切片哈希
	Index        int    // 文件片段的索引(该片段在文件中的顺序位置)
	SliceContent []byte // 切片内容
}

// 用于刷新下载的通道
type downloadChan struct {
	AssetID     string // 文件资产的唯一标识(外部标识)
	SliceHash   string // 文件片段的哈希值(外部标识)
	TotalPieces int    // 文件总片数（数据片段和纠删码片段的总数）
	Index       int    // 文件片段的索引(该片段在文件中的顺序位置)
}

// Download 下载文件
// 暂停后重新下载，需要输入文件ID用于解密，不落盘
// assetID        string         // 文件资产的唯一标识(外部标识)
// fileHash       string         // 文件内容的哈希值(内部标识)
func (fs *FS) Download(assetID string, fileHash ...string) error {

	// 添加一个新的下载任务
	var err error
	// 如果提供了 fileHash 参数，则传递给 AddDownloadTask
	if len(fileHash) > 0 {
		err = fs.pool.AddDownloadTask(assetID, fileHash[0])
	} else {
		err = fs.pool.AddDownloadTask(assetID)
	}
	if err != nil {
		return err
	}

	// 无法获取文件下载开始事件总线
	bus := fs.registry.GetEventBus(EventFileDownloadStart)
	if bus == nil {
		return fmt.Errorf("无法获取文件下载开始事件总线")
	}
	bus.Publish(EventFileDownloadStart, assetID)

	return nil
}

// TODO:ContinueDownloading 继续下载文件
func (fs *FS) ContinueDownloading(assetID string, fileHash ...string) error {
	return nil
}

// TODO:PauseDownloading 停止下载文件
func (fs *FS) PauseDownloading(assetID string, fileHash ...string) error {
	return nil
}

// TODO:DeleteDownloading 删除下载文件
func (fs *FS) DeleteDownloading(assetID string, fileHash ...string) error {
	return nil
}

// SendDownloadInfo 向下载通道发送信息
func SendDownloadInfo(downloadChans chan *downloadChan, assetID, sliceHash string, totalPieces, index int) {
	downloadInfo := &downloadChan{
		AssetID:     assetID,
		SliceHash:   sliceHash,
		TotalPieces: totalPieces,
		Index:       index,
	}
	downloadChans <- downloadInfo
}

// GetDownloadChannel 返回下载通道
func (fs *FS) GetDownloadChannel() chan *downloadChan {
	return fs.downloadChan
}

/**

downloadChan := fs.GetDownloadChannel()

// 监听下载通道
go func() {
    for info := range downloadChan {
        // 处理下载信息
        fmt.Printf("下载信息: %+v\n", info)
    }
}()

*/
