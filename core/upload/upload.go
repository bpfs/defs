package upload

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"path"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/defs/util/tempfile"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/kbucket"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sirupsen/logrus"
)

// Upload 上传新文件
func Upload(
	ctx context.Context, // 全局上下文
	opt *opts.Options, // 文件存储选项配置
	// p2p *dep2p.DeP2P, // DeP2P网络主机
	// pubsub *pubsub.DeP2PPubSub, // DeP2P网络订阅
	db *sqlites.SqliteDB, // sqlite数据库服务
	// uploadChan chan *core.UploadChan, // 用于刷新上传的通道
	// downloadChan chan *core.DownloadChan, // 用于刷新下载的通道
	registry *eventbus.EventRegistry, // 事件总线
	cache *ristretto.Cache, // 缓存实例
	pool *pool.MemoryPool, // 内存池

	path string, // 文件路径
	ownerPriv *ecdsa.PrivateKey, // 所有者的私钥
) (*struct {
	FileID     string    // 文件的唯一标识
	FileKey    string    // 文件的密钥
	Name       string    // 文件的名称
	Size       int64     // 文件的长度（以字节为单位）
	UploadTime time.Time // 上传时间
	ModTime    time.Time // 修改时间
	FileType   string    // 文件类型或格式
}, error) {
	afero := afero.NewOsFs()
	file, err := afero.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if ownerPriv == nil {
		ownerPriv = opt.GetDefaultOwnerPriv() // 获取默认所有者的私钥
		if ownerPriv == nil {
			return nil, fmt.Errorf("文件所有者的私钥不可为空")
		}
	}
	pubKeyHash, err := script.GetPubKeyHashFromPrivKey(ownerPriv) // 从ECDSA私钥中提取公钥哈希
	if err != nil {
		return nil, err
	}

	fileInfo, fileHash, err := readFile(opt, cache, file, pubKeyHash)
	if err != nil {
		return nil, err
	}

	// 查询制定的文件是否存在
	if sqlite.SelectOneFileID(db, fileInfo.GetFileID()) {
		return nil, fmt.Errorf("所有者对应的文件已存在")
	}

	var dataShards int64 = 1   // 文件片段的总数
	var parityShards int64 = 0 // 奇偶校验片段的总数
	// 获取存储模式
	switch opt.GetStorageMode() {
	// 文件模式
	case opts.FileMode:
		// 文件大于最大片段的大小时，自动切换至'切片模式'
		if fileInfo.GetSize() > opt.GetMaxSliceSize() {
			dataShards = int64(math.Ceil(float64(fileInfo.GetSize()) / float64(opt.GetShardSize()))) // 返回大于或等于 x 的最小整数值
			break
		}

	// 切片模式
	case opts.SliceMode:
		// 文件小于最小片段的大小时，自动切换至'文件模式'
		if fileInfo.GetSize() < opt.GetMinSliceSize() {
			break
		}

		totalShards := math.Ceil(float64(fileInfo.GetSize()) / float64(opt.GetShardSize())) // 返回大于或等于 x 的最小整数值
		dataShards = int64(totalShards)

	// 纠删码(大小)模式
	case opts.RS_Size:
		dataShards = opt.GetDataShards()     // 获取奇偶校验片段的数量
		parityShards = opt.GetParityShards() // 获取奇偶校验片段的数量

	// 纠删码(比例)模式
	case opts.RS_Proportion:
		// 计算数据分片和奇偶分片数量
		totalShards := math.Ceil(float64(fileInfo.GetSize()) / float64(opt.GetShardSize())) // 回大于或等于 x 的最小整数值

		dataShards = int64(totalShards / (1 + opt.GetParityRatio())) // 数据片段的数量
		parityShards = int64(totalShards) - dataShards               // 奇偶校验片段的数量

	default:
		return nil, fmt.Errorf("不支持的存储模式")
	}

	if err := readSplit(cache, file, fileInfo, fileHash, fileInfo.GetSize()+opt.GetDefaultBufSize(), dataShards, parityShards); err != nil {
		return nil, err
	}

	// 创建文件处理函数
	if err := createFileWith(pool, registry, cache, fileInfo, "", "", nil, dataShards, parityShards); err != nil {
		return nil, err
	}

	return &struct {
		FileID     string    // 文件的唯一标识
		FileKey    string    // 文件的密钥
		Name       string    // 文件的名称
		Size       int64     // 文件的长度（以字节为单位）
		UploadTime time.Time // 上传时间
		ModTime    time.Time // 修改时间
		FileType   string    // 文件类型或格式
	}{
		FileID:     fileInfo.GetFileID(),
		FileKey:    fileInfo.GetFileKey(),
		Name:       fileInfo.GetName(),
		Size:       fileInfo.GetSize(),
		UploadTime: fileInfo.GetUploadTime(),
		ModTime:    fileInfo.GetModTime(),
		FileType:   fileInfo.GetFileType(),
	}, nil
}

// 文件上传请求(检查)
type FileUploadRequestCheckPayload struct {
	FileID     string    // 文件的唯一标识(外部标识)
	UploadTime time.Time // 上传时间
}

// SendFileSliceToNetwork 发送文件片段至网络
// fileID		文件的唯一标识(外部标识)
// sliceHash	文件片段的哈希值(外部标识)
// totalPieces	文件片段的总量
// current		当前序列
func SendFileSliceToNetwork(opt *opts.Options, p2p *dep2p.DeP2P, uploadChan chan *core.UploadChan, registry *eventbus.EventRegistry, cache *ristretto.Cache, pm *pool.MemoryPool, fileID, sliceHash string, totalPieces, current int) error {
	// 在流操作之前获取互斥锁
	network.StreamMutex.Lock()

	// 新建文件存储
	fs, err := afero.NewFileStore(paths.UploadPath)
	if err != nil {
		return err
	}

	// 根据路径+切片名字获取数据
	sliceByte, err := fs.Read(fileID, sliceHash)
	if err != nil {
		// 切片读取失败
		// 更新切片为上传失败
		logrus.Errorf("Read 错误:\t%v\n%s\n%s\n\n", err, fileID, sliceHash)
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

	uploadPieceInfo := &pool.UploadPieceInfo{
		Index:  current,                    // 文件片段的序列号
		PeerID: []string{string(res.Data)}, // 节点的host地址
	}
	// 用于更新特定文件片段的信息
	pm.UpdateUploadPieceInfo(fileID, sliceHash, uploadPieceInfo)

	// 向上传通道发送信息
	go func() {
		var receiverPeers []string
		receiverPeers = append(receiverPeers, pid.String())
		SendUploadInfo(uploadChan, fileID, sliceHash, totalPieces, current, receiverPeers)
	}()

	return nil
}

// SendUploadInfo 向上传通道发送信息
func SendUploadInfo(uploadChans chan *core.UploadChan, fileID, sliceHash string, totalPieces, index int, peerIDs []string) {
	uploadInfo := &core.UploadChan{
		FileID:      fileID,
		SliceHash:   sliceHash,
		TotalPieces: totalPieces,
		Index:       index,
		Pid:         peerIDs,
	}
	uploadChans <- uploadInfo
}

// sendSlice 向指定的 peer 发送文件片段
func sendSlice(opt *opts.Options, p2p *dep2p.DeP2P, sliceHash string, sliceByte []byte) (*streams.ResponseMessage, peer.ID, error) {
	for i := 0; ; {
		// 返回与给定 ID 最接近的 "count" 对等点的列表
		receiverPeers := p2p.RoutingTable(2).NearestPeers(kbucket.ConvertKey(sliceHash), i+1)

		// TODO:只是为了测试后面删除,因为没有失败在处理的逻辑
		if p2p.RoutingTable(2).Size() < int(opt.GetRoutingTableLow()) { // 路由表中连接的最小节点数量
			// 延时1秒后退出
			time.Sleep(1 * time.Second)
			continue
		}

		if len(receiverPeers) == i {
			// 应该放入本地后面再处理
			return nil, "", nil
		}
		if len(receiverPeers) == 0 {
			i++
			continue
		}
		if i >= len(receiverPeers) {
			i = len(receiverPeers) - 1
		}
		if i < 0 {
			i = 0
		}

		// 向指定的节点发流消息
		responseByte, err := network.SendStream(p2p, config.StreamFileSliceUploadProtocol, "", receiverPeers[i], sliceByte)
		if err != nil {
			i++
			continue
		}
		var response streams.ResponseMessage
		if err := response.Unmarshal(responseByte); err != nil {
			return nil, "", err
		}

		return &response, receiverPeers[i], nil
	}

}

// 创建文件处理函数
func createFileWith(pool *pool.MemoryPool, registry *eventbus.EventRegistry, cache *ristretto.Cache, fileInfo *core.FileInfo, owned, customName string, metadata map[string]string, dataShards, parityShards int64) error {
	// 添加一个新的上传任务到内存池
	if err := pool.AddUploadTask(fileInfo.GetFileID(), len(fileInfo.GetSliceList())); err != nil {
		return err
	}

	// 检查并用文件的哈希值创建文件夹
	fpath := path.Join(paths.UploadPath, fileInfo.GetFileID())
	if err := util.CheckAndMkdir(fpath); err != nil {
		return err
	}

	// 获取文件上传检查事件总线
	bus := registry.GetEventBus(config.EventFileUploadCheck)
	if bus == nil {
		return fmt.Errorf("无法获取文件上传检查事件总线")
	}

	// 文件上传检查事件
	bus.Publish(config.EventFileUploadCheck, fileInfo.GetFileID(), dataShards, parityShards, fileInfo.GetUploadTime())

	// 本地文件片段
	if err := localFileFragments(pool, registry, cache, fileInfo, fpath, owned, customName, metadata); err != nil {
		return err
	}

	return nil
}

// localFileFragments 本地文件片段
func localFileFragments(pool *pool.MemoryPool, registry *eventbus.EventRegistry, cache *ristretto.Cache, fileInfo *core.FileInfo, fpath, owned, customName string, metadata map[string]string) error {
	// 获取文件片段上传事件总线
	bus := registry.GetEventBus(config.EventFileSliceUpload)
	if bus == nil {
		return fmt.Errorf("无法获取文件检查事件总线")
	}

	for k, slice := range fileInfo.GetSliceList() {
		paused, err := pool.IsUploadTaskPaused(fileInfo.GetFileID())
		if err != nil {
			return err
		}
		if paused {
			return fmt.Errorf("文件 %s 已暂停上传", fileInfo.GetFileID())
		}

		// 文件片段存储为本地文件
		if err := sliceLocalFileHandle(cache, fileInfo, slice, fpath, owned, customName, metadata); err != nil {
			return err
		}

		// 文件片段上传事件
		bus.Publish(config.EventFileSliceUpload, fileInfo.GetFileID(), slice.GetSliceHash(), len(fileInfo.GetSliceList()), k+1)
	}

	return nil
}

// sliceLocalFileHandle 文件片段存储为本地文件
func sliceLocalFileHandle(cache *ristretto.Cache, fileInfo *core.FileInfo, s core.SliceInfo, fpath, owned, customName string, metadata map[string]string) error {
	// 将 size 转换为 []byte
	sizeByte, err := util.ToBytes[int64](fileInfo.GetSize())
	if err != nil {
		return err
	}
	// 将 modTime、uploadTime 转换为 []byte
	modTimeByte, err := util.ToBytes[int64](fileInfo.GetModTime().Unix())
	if err != nil {
		return err
	}
	uploadTimeByte, err := util.ToBytes[int64](fileInfo.GetUploadTime().Unix())
	if err != nil {
		return err
	}
	// 将 Index 转换为 []byte
	indexByte, err := util.ToBytes[int](s.GetIndex())
	if err != nil {
		return err
	}
	// 定义共享状态，默认为 false，表示不共享
	shared := false
	// 将 shared 转换为 []byte
	sharedByte, err := util.ToBytes[bool](shared)
	if err != nil {
		return err
	}

	// 编码
	sliceTableBytes, err := util.EncodeToBytes(fileInfo.GetSliceTable())
	if err != nil {
		logrus.Errorf("[sendStream] 编码失败:\t%v", err)
		return err
	}

	// TODO: 测试缓存
	// content, ok := cache.Get(s.GetSliceHash())
	// if !ok {
	// 	return fmt.Errorf("文件缓存异常")
	// }
	content, err := tempfile.Read(s.GetSliceHash())
	if err != nil {
		return err
	}

	data := map[string][]byte{
		"FILEID":      []byte(fileInfo.GetFileID()), // 写入文件的唯一标识
		"NAME":        []byte(fileInfo.GetName()),   // 写入文件的名称
		"SIZE":        sizeByte,                     // 写入文件的长度
		"MODTIME":     modTimeByte,                  // 写入文件的修改时间
		"UPLOADTIME":  uploadTimeByte,               // 写入文件的上传时间
		"P2PKHSCRIPT": fileInfo.GetP2pkhScript(),    // 写入文件的 P2PKH 脚本
		"P2PKSCRIPT":  fileInfo.GetP2pkScript(),     // 写入文件的 P2PK 脚本
		"SLICETABLE":  sliceTableBytes,              // 写入文件片段的哈希表
		"SLICEHASH":   []byte(s.GetSliceHash()),     // 写入文件片段的哈希值
		"INDEX":       indexByte,                    // 写入文件片段的索引
		// "CONTENT":     content.([]byte),             // 写入文件片段的内容(加密)
		"CONTENT":   content,                  // 写入文件片段的内容(加密)
		"SIGNATURE": []byte(s.GetSignature()), // 写入文件和文件片段的数据签名
		"SHARED":    sharedByte,               // 写入文件共享状态
		"VERSION":   []byte(opts.Version),     // 版本
	}

	// 构建文件路径
	filePath := path.Join(fpath, s.GetSliceHash())

	// 调用 WriteFileSegment 方法来创建新文件并写入数据
	if err := segment.WriteFileSegment(filePath, data); err != nil {
		return err
	}

	// cache.Del(s.GetSliceHash()) // 删除缓存项
	tempfile.Delete(s.GetSliceHash())

	return nil
}
