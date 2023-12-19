package defs

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/bpfs/defs/filestore"

	"github.com/bpfs/defs/eventbus"

	"github.com/bpfs/dep2p"
	"github.com/dgraph-io/ristretto"
	"github.com/sirupsen/logrus"
)

// 流互斥锁
var streamMutex sync.Mutex

// 文件上传请求(检查)
type FileUploadRequestCheckPayload struct {
	AssetID    string    // 文件资产的唯一标识(外部标识)
	UploadTime time.Time // 上传时间
}

// 用于刷新上传的通道
type uploadChan struct {
	AssetID     string   // 文件资产的唯一标识(外部标识)
	SliceHash   string   // 文件片段的哈希值(外部标识)
	TotalPieces int      // 文件总片数
	Index       int      // 文件片段的索引(该片段在文件中的顺序位置)
	Pid         []string // 节点ID

}

// Upload 上传新文件
// path: 文件路径
func (fs *FS) Upload(path string) (*struct {
	AssetID  string    // 文件资产的唯一标识(外部标识)
	FileHash string    // 文件内容的哈希值(内部标识)
	Name     string    // 文件的基本名称
	Size     int64     // 常规文件的长度（以字节为单位）
	ModTime  time.Time // 修改时间
	FileType string    // 文件类型或格式
}, error) {
	var fileInfo *FileInfo
	var err error

	switch fs.opt.storageMode {
	// 文件模式
	case FileMode:

	// 切片模式
	case SliceMode:

	// 纠删码(大小)模式
	case RS_Size:
		// 从文件路径中读取并分片文件，使用指定的数据分片和奇偶分片
		fileInfo, err = fs.readFileWithShards(path, fs.opt.dataShards, fs.opt.parityShards)
		if err != nil {
			return nil, err
		}

	// 纠删码(比例)模式
	case RS_Proportion:
		// 从文件路径中读取并分片文件，根据分片大小和奇偶比例来确定数据分片和奇偶分片
		fileInfo, err = fs.readFileWithSizeAndRatio(path, fs.opt.shardSize, fs.opt.parityRatio)
		if err != nil {
			return nil, err
		}

	// 禁用纠删码
	default:

	}

	// 查询制定的文件资产是否存在
	if fs.db.SelectOneAssetID(fileInfo.assetID) {
		return nil, fmt.Errorf("文件资产已存在")

	}

	// 创建文件处理函数
	if err := fs.createFileWith(fileInfo, "", "", nil); err != nil {
		return nil, err
	}

	return &struct {
		AssetID  string
		FileHash string
		Name     string
		Size     int64
		ModTime  time.Time
		FileType string
	}{
		AssetID:  fileInfo.assetID,
		FileHash: fileInfo.fileHash,
		Name:     fileInfo.name,
		Size:     fileInfo.size,
		ModTime:  fileInfo.modTime,
		FileType: fileInfo.fileType,
	}, nil
}

// 创建文件处理函数
func (fs *FS) createFileWith(f *FileInfo, owned, customName string, metadata map[string]string) error {
	// 添加一个新的上传任务到内存池
	if err := fs.pool.AddUploadTask(f.assetID, len(f.SliceList())); err != nil {
		return err
	}

	// 检查并用文件的哈希值创建文件夹
	fpath := path.Join(UploadPath, f.assetID)
	if err := checkAndMkdir(fpath); err != nil {
		return err
	}

	// 获取文件上传检查事件总线
	bus := fs.registry.GetEventBus(EventFileUploadCheck)
	if bus == nil {
		return fmt.Errorf("无法获取文件上传检查事件总线")
	}
	bus.Publish(EventFileUploadCheck, f.assetID, f.dataShards, f.parityShards, f.uploadTime)

	// 本地文件片段
	if err := fs.localFileFragments(f, fpath, owned, customName, metadata); err != nil {
		return err
	}

	// 返回资产
	return nil
}

// localFileFragments 本地文件片段
func (fs *FS) localFileFragments(f *FileInfo, fpath, owned, customName string, metadata map[string]string) error {
	// 获取文件片段上传事件总线
	bus := fs.registry.GetEventBus(EventFileSliceUpload)
	if bus == nil {
		return fmt.Errorf("无法获取文件检查事件总线")
	}

	for k, slice := range f.SliceList() {
		paused, err := fs.pool.IsUploadTaskPaused(f.assetID)
		if err != nil {
			return err
		}
		if paused {
			return fmt.Errorf("文件 %s 已暂停上传", f.assetID)
		}

		// 文件片段存储为本地文件
		if err := fs.sliceLocalFileHandle(f, slice, fpath, owned, customName, metadata); err != nil {
			return err
		}

		// 文件片段上传事件
		bus.Publish(EventFileSliceUpload, f.assetID, slice.SliceHash(), len(f.SliceList()), k+1)
	}

	return nil
}

// sliceLocalFileHandle 文件片段存储为本地文件
func (fs *FS) sliceLocalFileHandle(f *FileInfo, s SliceInfo, fpath, owned, customName string, metadata map[string]string) error {
	xref := NewFileXref()

	// 使用文件的哈希值创建一个新文件
	slice, err := os.Create(path.Join(fpath, s.SliceHash()))
	if err != nil {
		return err
	}

	// 将 size 转换为 []byte
	sizeByte, err := ToBytes[int64](f.Size())
	if err != nil {
		return err
	}
	// 将 modTime、uploadTime 转换为 []byte
	modTimeByte, err := ToBytes[int64](f.ModTime().Unix())
	if err != nil {
		return err
	}
	uploadTimeByte, err := ToBytes[int64](f.UploadTime().Unix())
	if err != nil {
		return err
	}
	// 将 Index 转换为 []byte
	indexByte, err := ToBytes[int](s.Index())
	if err != nil {
		return err
	}
	// 定义共享状态，默认为 false，表示不共享
	shared := false
	// 将 shared 转换为 []byte
	sharedByte, err := ToBytes[bool](shared)
	if err != nil {
		return err
	}

	// 编码
	sliceTableBytes, err := EncodeToBytes(f.SliceTable())
	if err != nil {
		logrus.Errorf("[sendStream] 编码失败:\t%v", err)
		return err
	}

	value, found := fs.cache.Get(s.SliceHash())
	if !found {
		return fmt.Errorf("文件缓存异常")
	}

	data := map[string][]byte{
		"ASSETID":    []byte(f.AssetID()),   // 写入资产的唯一标识
		"NAME":       []byte(f.Name()),      // 写入文件的基本名称
		"SIZE":       sizeByte,              // 写入文件的长度
		"MODTIME":    modTimeByte,           // 写入文件的修改时间
		"UPLOADTIME": uploadTimeByte,        // 写入文件的上传时间
		"PUBLICKEY":  f.PublicKey(),         // 写入文件所有者的公钥
		"SLICETABLE": sliceTableBytes,       // 写入文件片段的哈希表
		"SLICEHASH":  []byte(s.SliceHash()), // 写入文件片段的哈希值
		"INDEX":      indexByte,             // 写入文件片段的索引
		"CONTENT":    value.([]byte),        // 写入文件片段的内容(加密)
		"SIGNATURE":  []byte(s.Signature()), // 写入文件和文件片段的数据签名
		"SHARED":     sharedByte,            // 写入文件共享状态
		"VERSION":    []byte("0.0.1"),       // 版本
	}

	// 批量将段写入文件
	if err := WriteSegmentsToFile(slice, data, xref); err != nil {
		return err
	}
	// 保存 xref 表并关闭文件
	if err := SaveAndClose(slice, xref); err != nil {
		return err
	}

	fs.cache.Del(s.SliceHash()) // 删除缓存项

	return nil
}

// SendFileSliceToNetwork 发送文件片段至网络
// assetID		文件资产的唯一标识(外部标识)
// sliceHash	文件片段的哈希值(外部标识)
// totalPieces	文件片段的总量
// current		当前序列
func SendFileSliceToNetwork(opt *Options, p2p *dep2p.DeP2P, uploadChan chan *uploadChan, registry *eventbus.EventRegistry, cache *ristretto.Cache, pool *MemoryPool, assetID, sliceHash string, totalPieces, current int) error {
	// 在流操作之前获取互斥锁
	streamMutex.Lock()

	// 新建文件存储
	fs, err := filestore.NewFileStore(UploadPath)
	if err != nil {
		return err
	}

	// 根据路径+切片名字获取数据
	sliceByte, err := fs.Read(assetID, sliceHash)
	if err != nil {
		// 切片读取失败
		// 更新切片为上传失败
		logrus.Errorf("Read 错误:\t%v\n%s\n%s\n\n", err, assetID, sliceHash)
		return err
	}

	// 发送至节点
	res, pid, err := sendSlice(opt, p2p, sliceHash, sliceByte)
	if err != nil {
		logrus.Errorf("sendNode 错误:\t%v\n", err)
		return err
	}

	logrus.Print("[响应数据]")
	logrus.Printf("Code:\t%d\n", res.Code)
	logrus.Printf("Msg:\t\t%s\n", res.Msg)
	logrus.Printf("Data:\t%s\n", res.Data)
	// 上传失败
	if res.Code != 200 {
		return err
	}

	uploadPieceInfo := &UploadPieceInfo{
		Index:  current,                    // 文件片段的序列号
		PeerID: []string{string(res.Data)}, // 节点的host地址
	}
	// 用于更新特定文件资产片段的信息
	pool.UpdateUploadPieceInfo(assetID, sliceHash, uploadPieceInfo)

	// 向上传通道发送信息
	go func() {
		var receiverPeers []string
		receiverPeers = append(receiverPeers, pid.String())
		SendUploadInfo(uploadChan, assetID, sliceHash, totalPieces, current, receiverPeers)
	}()

	return nil
}

// SendUploadInfo 向上传通道发送信息
func SendUploadInfo(uploadChans chan *uploadChan, assetID, sliceHash string, totalPieces, index int, peerIDs []string) {
	uploadInfo := &uploadChan{
		AssetID:     assetID,
		SliceHash:   sliceHash,
		TotalPieces: totalPieces,
		Index:       index,
		Pid:         peerIDs,
	}
	uploadChans <- uploadInfo
}

// GetUploadChannel 返回上传通道
func (fs *FS) GetUploadChannel() chan *uploadChan {
	return fs.uploadChan
}

/**

uploadChan := fs.GetUploadChannel()

// 监听上传通道
go func() {
    for info := range uploadChan {
        // 处理上传信息
        fmt.Printf("上传信息: %+v\n", info)
    }
}()

*/
