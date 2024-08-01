package downloads

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/network"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

var (
	// 文件下载请求清单(请求)
	PubSubDownloadChecklistRequestTopic = fmt.Sprintf("defs@pubsub/download/checklist/request/%s", version)

	// 文件下载请求清单（回应）
	PubSubDownloadChecklistResponseTopic = fmt.Sprintf("defs@pubsub/download/checklist/response/%s", version)
)

type RegisterPubsubProtocolInput struct {
	fx.In
	Ctx      context.Context     // 全局上下文
	Opt      *opts.Options       // 文件存储选项配置
	Afe      afero.Afero         // 文件系统接口
	P2P      *dep2p.DeP2P        // DeP2P网络主机
	PubSub   *pubsub.DeP2PPubSub // DeP2P网络订阅
	Download *DownloadManager    // 管理所有下载任务
}

// RegisterPubsubProtocol 注册订阅
func RegisterPubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 文件下载请求主题
	if err := input.PubSub.SubscribeWithTopic(PubSubDownloadChecklistRequestTopic, func(res *streams.RequestMessage) {
		HandleFileDownloadRequestPubSub(input.Opt, input.Afe, input.P2P, input.PubSub, res)
	}, true); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)

	}

	// 文件下载回应主题
	if err := input.PubSub.SubscribeWithTopic(PubSubDownloadChecklistResponseTopic, func(res *streams.RequestMessage) {
		HandleFileDownloadResponsePubSub(input.P2P, input.PubSub, input.Download, res)
	}, true); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}

// SegmentListRequest 描述请求文件片段清单的参数
type SegmentListRequest struct {
	TaskID       string            // 任务唯一标识
	FileID       string            // 文件唯一标识
	UserPubHash  []byte            // 用户的公钥哈希
	SegmentNodes map[int][]peer.ID // 文件片段所在节点
}

// HandleFileDownloadRequestPubSub 处理文件下载请求
func HandleFileDownloadRequestPubSub(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, res *streams.RequestMessage) {
	// 获取请求方地址
	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return
	}

	switch res.Message.Type {
	// 请求清单
	case "requestList":
		// 文件下载请求(清单)
		payload := new(SegmentListRequest)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return
		}

		logrus.Infof("[ %s ]请求[ %s ]的索引清单", res.Message.Sender, payload.FileID)

		// 从指定文件中读取一个或多个段
		segmentList, err := processSlice(opt, afe, p2p, payload.FileID, payload.UserPubHash)
		if err != nil || segmentList == nil {
			if err != nil {
				logrus.Errorf("[%s]从指定文件中读取一个或多个段时失败: %v", debug.WhereAmI(), err)
			}
			return
		}

		// 文件下载响应(清单)
		responseChecklistPayload := FileDownloadResponseChecklistPayload{
			TaskID:          payload.TaskID,              // 任务唯一标识
			FileID:          segmentList.fileID,          // 文件的唯一标识
			Name:            segmentList.name,            // 文件名
			Size:            segmentList.size,            // 文件大小
			ContentType:     segmentList.contentType,     // MIME类型
			SliceTable:      segmentList.sliceTable,      // 文件片段的哈希表
			AvailableSlices: segmentList.availableSlices, // 本地存储的文件片段信息
		}

		// 本地存储的文件片段信息为空
		if len(responseChecklistPayload.AvailableSlices) == 0 {
			logrus.Errorf("[%s]本地存储的文件片段信息为空", debug.WhereAmI())
			return
		}

		localNodeID := p2p.Host().ID()
		// 检查是否需要发送响应
		if !shouldSendResponse(localNodeID, payload.SegmentNodes, responseChecklistPayload.AvailableSlices) {
			logrus.Info("所有片段信息都已存在，无需发送响应")
			return
		}

		usePubSub := false

		// 发送获取片段到目标节点
		network.StreamMutex.Lock()
		res, err := network.SendStream(p2p, StreamDownloadChecklistResponseProtocol, "", receiver, responseChecklistPayload)
		// network.StreamMutex.Unlock()
		if err != nil || res == nil || res.Code != 200 {
			if res != nil && res.Code == 6604 {
				logrus.Warnf("[%s]发送获取片段到目标节点时，下载任务不存在", debug.WhereAmI())
				return // 直接退出
			}

			usePubSub = true // 标记需要使用订阅发送

			if err != nil {
				logrus.Errorf("[%s]发送获取片段到目标节点时失败: %v", debug.WhereAmI(), err)
			} else {
				logrus.Errorf("[%s]发送获取片段到目标节点时返回错误码: %d", debug.WhereAmI(), res.Code)
			}
		}

		if usePubSub {
			// 发送响应清单(使用订阅回复)
			if err := network.SendPubSub(p2p, pubsub, PubSubDownloadChecklistResponseTopic, "responseList", receiver, responseChecklistPayload); err != nil {
				logrus.Errorf("发送下载订阅回复失败[%s]", debug.WhereAmI())
				return
			}
		}

	default:
		return
	}
}

// HandleFileDownloadResponsePubSub 处理文件下载响应
func HandleFileDownloadResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, download *DownloadManager, res *streams.RequestMessage) {
	switch res.Message.Type {
	// 响应清单
	case "responseList":
		payload := new(FileDownloadResponseChecklistPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return
		}

		logrus.Infof("[ %s ]使用订阅消息回复了[ %s ]的索引清单", res.Message.Sender, payload.FileID)

		// logrus.Printf("TaskID: %s", payload.TaskID)
		// logrus.Printf("FileID: %s", payload.FileID)
		// logrus.Printf("Name: %s", payload.Name)
		// logrus.Printf("Size: %d", payload.Size)
		// logrus.Printf("ContentType: %s", payload.ContentType)
		// logrus.Printf("SliceTable: %v", payload.SliceTable)
		// logrus.Printf("AvailableSlices: %v", payload.AvailableSlices)

		task, ok := download.Tasks[payload.TaskID]
		if !ok {
			return
		}
		// 解析发送者点节点id
		receiver, err := peer.Decode(res.Message.Sender)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return
		}

		// 更新下载任务中特定片段的节点信息
		task.UpdateDownloadPieceInfo(payload, receiver)

	default:
		return
	}
}

// segmentListResult 定义了从片段文件读取的结果数据结构。
type segmentListResult struct {
	fileID          string             // 文件唯一标识
	name            string             // 文件名
	size            int64              // 文件大小
	contentType     string             // MIME类型
	checksum        []byte             // 文件的校验和
	shared          bool               // 文件的共享状态
	p2pkhScript     []byte             // P2PKH 脚本
	sliceTable      map[int]*HashTable // 文件片段的哈希表
	availableSlices []int              // 本地存储的文件片段信息
}

// processSlice 从指定文件中读取一个或多个段，将其赋值给回传参数。
func processSlice(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, fileID string, userPubHash []byte) (*segmentListResult, error) {
	subDir := filepath.Join(paths.GetSlicePath(), p2p.Host().ID().String(), fileID)

	// 检查目录是否存在
	exists, err := afero.DirExists(afe, subDir)
	if err != nil {
		logrus.Errorf("[%s]检查目录是否存在时失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 目录不存在
	if !exists {
		return nil, nil
	}

	// 列出指定子目录中的所有文件
	slices, err := afero.ListFileNamesRecursively(afe, subDir)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 无所需的文件片段则直接退出
	if len(slices) == 0 {
		return nil, nil
	}

	segmentList := &segmentListResult{
		fileID: fileID, // 文件唯一标识
	}

SLICESLOOP:
	for _, segmentID := range slices {
		// logrus.Printf("文件片段的唯一标识: %s", segmentID)

		// 打开文件片段

		sliceFile, err := util.OpenFile(opt, afe, subDir, segmentID)
		if err != nil {
			logrus.Errorf("[%s]打开文件片段 %s 时失败: %v", debug.WhereAmI(), segmentID, err)
			continue
		}
		defer sliceFile.Close()

		// 定义需要读取的段类型
		segmentTypes := []string{
			"SEGMENTID", // 文件片段的唯一标识
			"INDEX",     // 分片索引
		}

		if segmentList.fileID == "" {
			segmentTypes = append(segmentTypes, "FILEID") // 文件唯一标识
		}
		if segmentList.name == "" {
			segmentTypes = append(segmentTypes, "NAME") // 文件名
		}
		if segmentList.size == 0 {
			segmentTypes = append(segmentTypes, "SIZE") // 文件大小
		}
		if segmentList.contentType == "" {
			segmentTypes = append(segmentTypes, "CONTENTTYPE") // MIME类型
		}
		if len(segmentList.checksum) == 0 {
			segmentTypes = append(segmentTypes, "SHARED") // 文件的校验和
		}
		if !segmentList.shared {
			segmentTypes = append(segmentTypes, "SHARED") // 文件共享状态
		}
		if len(segmentList.p2pkhScript) == 0 {
			segmentTypes = append(segmentTypes, "P2PKHSCRIPT") // P2PKH 脚本
		}
		if segmentList.sliceTable == nil {
			segmentTypes = append(segmentTypes, "SLICETABLE") // 文件片段的哈希表
		}
		// 从指定文件中读取一个或多个段
		segmentResults, _, err := segment.ReadFileSegments(sliceFile, segmentTypes)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			continue
		}

		indexSegment, exists := segmentResults["INDEX"]
		if !exists {
			// 出现任何错误，立即继续下一个 sliceHash
			continue
		}

		index, err := util.FromBytes[int32](indexSegment.Data)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			continue
		}

		// 处理每个段的结果
		for segmentType, result := range segmentResults {
			if result.Error != nil {
				// 出现任何错误，立即继续下一个 sliceHash
				continue SLICESLOOP
			}

			switch segmentType {
			// 文件的唯一标识
			case "FILEID":
				if fileID != string(result.Data) {
					continue SLICESLOOP
				}

				// 文件片段的唯一标识
			case "SEGMENTID":
				if segmentID != string(result.Data) {
					continue SLICESLOOP
				}

			// 文件的名称
			case "NAME":
				segmentList.name = string(result.Data)

			// 文件的长度
			case "SIZE":
				if segmentList.size, err = util.FromBytes[int64](result.Data); err != nil {
					logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
					continue SLICESLOOP
				}

			// MIME类型
			case "CONTENTTYPE":
				segmentList.contentType = string(result.Data)

			// 文件的校验和
			case "CHECKSUM":
				segmentList.checksum = result.Data

			// 文件共享状态
			case "SHARED":
				shared, err := util.FromBytes[bool](result.Data)
				if err != nil {
					logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
					continue SLICESLOOP
				}
				segmentList.shared = shared

			// 文件的 P2PKH 脚本
			case "P2PKHSCRIPT":
				segmentList.p2pkhScript = result.Data

			// 文件片段的哈希表
			case "SLICETABLE":
				var sliceTable map[int]*HashTable
				if err := util.DecodeFromBytes(result.Data, &sliceTable); err != nil {
					logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
					continue SLICESLOOP
				}
				segmentList.sliceTable = sliceTable
			}
		}

		// 共享与权限校验
		if !segmentList.shared {
			// 验证脚本中所有者的公钥哈希
			if !script.VerifyScriptPubKeyHash(segmentList.p2pkhScript, userPubHash) {
				continue SLICESLOOP
			}
		}

		// logrus.Printf("第 %d 片 %s 添加成功", index, segmentID)
		segmentList.availableSlices = append(segmentList.availableSlices, int(index))
	}

	return segmentList, nil
}

// shouldSendResponse 检查是否需要发送响应
func shouldSendResponse(localNodeID peer.ID, segmentNodes map[int][]peer.ID, availableSlices []int) bool {
	// 如果 segmentNodes 为空，则直接返回 true
	if len(segmentNodes) == 0 {
		return true
	}

	// 获取本地节点拥有的片段索引
	localNodeSlices := findSegmentIndexesForPeer(localNodeID, segmentNodes)

	// 将本地存储的片段信息转换为map，以便比较
	availableSlicesMap := make(map[int]bool)
	for _, slice := range availableSlices {
		availableSlicesMap[slice] = true
	}

	// 比较本地存储的片段信息和请求方的片段信息
	for _, slice := range localNodeSlices {
		if !availableSlicesMap[slice] {
			return true // 如果有任何一个片段不匹配，则需要发送响应
		}
	}

	return false // 所有片段都匹配，无需发送响应
}

// findSegmentIndexesForPeer 查找指定peer.ID在SegmentNodes映射中的所有片段索引
func findSegmentIndexesForPeer(peerID peer.ID, segmentNodes map[int][]peer.ID) []int {
	var indexes []int // 存储匹配的片段索引
	// 遍历SegmentNodes映射
	for index, peers := range segmentNodes {
		for _, id := range peers {
			if id == peerID {
				indexes = append(indexes, index) // 添加匹配的索引
				break                            // 已找到当前索引，跳过剩余的ID检查
			}
		}
	}
	return indexes
}
