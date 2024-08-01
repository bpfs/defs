package downloads

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// AsyncDownload 表示需要异步下载的文件片段信息
type AsyncDownload struct {
	downloadMaximumSize int64          // 下载最大回复大小
	receiver            peer.ID        // 接收方ID
	taskID              string         // 任务唯一标识
	fileID              string         // 文件唯一标识
	segments            map[int]string // 剩余未处理的文件片段索引和唯一标识的映射
}

// handleAsyncDownload 处理异步下载
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统操作对象
//   - p2p: *dep2p.DeP2P P2P网络对象
//   - asyncDownload: AsyncDownload 异步下载请求
func (manager *DownloadManager) handleAsyncDownload(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, a *AsyncDownload) {
	reply := &StreamGetSliceToLocalResponse{
		SegmentInfo: make(map[int][]byte), // 初始化文件片段的索引和内容的映射
	}

	currentSize := 0 // 当前回复的总大小

	// 处理普通下载的文件片段
	segmentContent, err := processRegularSegments(opt, afe, p2p, a.fileID, a.segments, a.downloadMaximumSize, currentSize)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return
	}

	// 直接 reply.SegmentInfo = segmentInfo 赋值是可以的，因为 map 是引用类型。
	// 这意味着当你赋值 segmentInfo 到 reply.SegmentInfo 时，实际上是引用同一个底层数据结构，所以修改 segmentInfo 的内容会反映在 reply.SegmentInfo 中。
	//
	// 不过，为了确保代码的健壮性和可读性，我们可以通过拷贝 segmentInfo 到 reply.SegmentInfo 来防止意外的修改。

	// 将 segmentInfo 拷贝到 reply.SegmentInfo
	for k, v := range segmentContent {
		reply.SegmentInfo[k] = v
	}

	// 内容为空则直接退出
	// 原因是因为reply里面不是指针，虽然processRegularSegments传入到这个里面了但是因为不是指针所以改变不了这个值，
	// 我在processRegularSegments 里面打印了是有值的。你看下，目前还没改动，你看看咋搞搞。
	if len(reply.SegmentInfo) == 0 {
		return
	}

	// 向指定的节点发送文件片段
	res, err := RequestStreamAsyncDownload(p2p, a.receiver, a.taskID, a.fileID, reply.SegmentInfo)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return
	}

	if res == nil {
		return
	}

	// 如果有剩余未处理的文件片段，将其传递给 DownloadManager 进行异步下载
	if len(res.Segments) > 0 {
		manager.ReceivePendingSegments(opt.GetDownloadMaximumSize(), a.receiver.String(), a.taskID, a.fileID, res.Segments)
	}
}

// localHandleAsyncDownload 本地处理异步下载文件片段
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - download: *DownloadManager 管理所有下载任务
//   - requesterAddress string 目标节点的 ID
//   - taskID: string 任务唯一标识
//   - segmentInfo: map[int][]byte 文件片段的索引和内容的映射
//
// 返回值：
//   - int32: 状态码
//   - string: 状态信息
func localHandleAsyncDownload(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	download *DownloadManager,
	requesterAddress string,
	taskID string,
	segmentInfo map[int][]byte,
) (*StreamAsyncDownloadResponse, error) {
	task, exists := download.Tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("任务不存在")
	}

	receiver, err := peer.Decode(requesterAddress)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 处理下载文件片段的回复信息
	ok, err := processSegmentInfo(opt, afe, p2p, task, receiver, segmentInfo, download.DownloadChan)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 已触发合并操作
	if ok {
		return nil, nil
	}

	reply := &StreamAsyncDownloadResponse{
		// 获取特定节点下的待下载文件片段索引和唯一标识的映射
		Segments: task.File.GetPendingSegmentsForNode(receiver),
	}

	return reply, nil
}

// processSegmentInfo 处理下载文件片段的回复信息
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - task: *DownloadTask 当前下载任务
//   - receiver: peer.ID 目标节点的 ID
//   - segmentInfo: map[int][]byte 文件片段的索引和内容的映射
//   - downloadChan: chan *DownloadChan 下载状态更新通道
//
// 返回值：
//   - error 错误信息
func processSegmentInfo(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	task *DownloadTask,
	receiver peer.ID,
	segmentInfo map[int][]byte,
	downloadChan chan *DownloadChan,
) (bool, error) {
	for index, sliceContent := range segmentInfo {
		// 下载任务的状态为下载完成
		if task.GetDownloadStatus() == StatusCompleted {
			return true, nil
		}

		subDir := filepath.Join(paths.GetDownloadPath(), p2p.Host().ID().String(), task.File.FileID)
		// 检查文件片段是否存在以及是否已经下载完成
		exists, isCompleted, err := task.File.IsSegmentCompleted(opt, afe, index, subDir)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return false, err
		}

		if !exists {
			logrus.Debugf("文件片段 %d 不存在\n", index)
			return false, nil
		}
		if isCompleted {
			logrus.Debugf("文件片段 %d 已下载完成\n", index)
			return false, nil
		}

		// 获取文件片段的唯一标识
		segmentID := task.File.GetSegmentID(index)
		// 写入本地文件
		if err := writeToLocalFile(opt, afe, p2p, task.Secret, task.File.FileID, segmentID, sliceContent); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return false, err
		}

		// 更新下载进度，并检查是否需要合并文件
		// ok := updateDownloadProgress(task, index)
		updateDownloadProgress(task, index)

		isComplete := task.Progress.All()

		// 对外通道
		go func() {
			downloadChan <- &DownloadChan{
				TaskID:           task.TaskID,                       // 任务唯一标识
				TotalPieces:      task.TotalPieces,                  // 文件总分片数
				DownloadProgress: task.File.DownloadCompleteCount(), // 已完成的数量
				IsComplete:       isComplete,                        // 是否下载完成
				SegmentID:        segmentID,                         // 文件片段的哈希值(外部标识)
				SegmentIndex:     index,                             // 分片索引，表示该片段在文件中的顺序
				SegmentSize:      len(sliceContent),                 // 文件片段大小，单位为字节
				UsesErasureCodes: task.File.IsRsCodes(index),        // 是否使用纠删码技术
				NodeID:           receiver,                          // 存储该文件片段的节点ID
				DownloadTime:     time.Now().UTC().Unix(),           // 下载完成时间的时间戳
			}
		}()

		if isComplete {
			return true, nil
		}
	}

	return false, nil
}

// ProcessDownloadRequest 处理下载请求
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - pub: *pubsub.DeP2PPubSub 网络订阅
//   - download: *DownloadManager 管理所有下载任务
//   - downloadMaximumSize: int64 下载最大回复大小
//   - taskID: string 任务唯一标识
//   - fileID: string 文件唯一标识
//   - prioritySegment: int 优先下载的文件片段索引
//   - segmentInfo: map[int]string 文件片段的索引和唯一标识的映射
//   - sender: string 请求方节点地址
//
// 返回值：
//   - *StreamGetSliceToLocalResponse 下载回复结构体
//   - error 错误信息
func ProcessDownloadRequest(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	pub *pubsub.DeP2PPubSub,
	download *DownloadManager,
	downloadMaximumSize int64,
	taskID string,
	fileID string,
	prioritySegment int,
	segmentInfo map[int]string,
	sender string,
) (*StreamGetSliceToLocalResponse, error) {
	reply := &StreamGetSliceToLocalResponse{
		SegmentInfo: make(map[int][]byte), // 初始化文件片段的索引和内容的映射
	}

	// 处理优先下载的文件片段
	currentSize, err := processPrioritySegment(opt, afe, p2p, downloadMaximumSize, fileID, prioritySegment, segmentInfo, reply.SegmentInfo)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 处理普通下载的文件片段
	segmentContent, err := processRegularSegments(opt, afe, p2p, fileID, segmentInfo, downloadMaximumSize, currentSize)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// 将 segmentInfo 拷贝到 reply.SegmentInfo
	for k, v := range segmentContent {
		reply.SegmentInfo[k] = v
	}

	remainingSegments := make(map[int]string) // 用于存储剩余未处理的文件片段

	// 保存所有剩余未处理的文件片段
	for index, segmentID := range segmentInfo {
		if _, exists := reply.SegmentInfo[index]; !exists {
			remainingSegments[index] = segmentID
		}
	}

	// 如果有剩余未处理的文件片段，将其传递给 DownloadManager 进行异步下载
	if len(remainingSegments) > 0 {
		download.ReceivePendingSegments(opt.GetDownloadMaximumSize(), sender, taskID, fileID, remainingSegments)
	}

	return reply, nil
}

// processPrioritySegment 处理优先下载的文件片段
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - downloadMaximumSize: int64 下载最大回复大小
//   - fileID: string 文件唯一标识
//   - prioritySegment: int 优先下载的文件片段索引
//   - segmentInfo: map[int]string 文件片段的索引和唯一标识的映射
//   - segmentMap: map[int][]byte 用于存储下载回复的映射
//
// 返回值：
//   - int 当前回复的总大小
//   - error 错误信息
func processPrioritySegment(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	downloadMaximumSize int64,
	fileID string,
	prioritySegment int,
	segmentInfo map[int]string,
	segmentMap map[int][]byte,
) (int, error) {
	currentSize := 0

	// 检查 segmentInfo 中是否包含 prioritySegment
	segmentID, exists := segmentInfo[prioritySegment]
	if !exists {
		return currentSize, fmt.Errorf("未请求文件片段索引") // 如果不包含优先片段，直接返回
	}

	subDir := filepath.Join(paths.GetSlicePath(), p2p.Host().ID().String(), fileID)
	priorityContent, err := util.Read(opt, afe, subDir, segmentID)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return 0, err
	}

	newSize := currentSize + len(priorityContent)
	if newSize <= int(downloadMaximumSize) {
		segmentMap[prioritySegment] = priorityContent // 添加优先片段到回复
		currentSize = newSize                         // 更新当前回复的总大小
	}
	// 从 SegmentInfo 中删除已处理的优先片段
	delete(segmentInfo, prioritySegment)

	return currentSize, nil
}

// processRegularSegments 处理普通下载的文件片段
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - fileID: string 文件唯一标识
//   - segmentInfo: map[int]string 文件片段的索引和唯一标识的映射
//   - downloadMaximumSize: int64 下载最大回复大小
//   - currentSize: int 当前回复的总大小
//
// 返回值：
//   - map[int][]byte: 文件片段内容映射
//   - error: 错误信息
func processRegularSegments(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	fileID string,
	segmentInfo map[int]string,
	downloadMaximumSize int64,
	currentSize int,
) (map[int][]byte, error) {
	// 初始化文件片段内容映射
	segmentMap := make(map[int][]byte)

	for index, segmentID := range segmentInfo {
		// 构建文件片段路径
		subDir := filepath.Join(paths.GetSlicePath(), p2p.Host().ID().String(), fileID)

		// 读取文件片段内容
		sliceContent, err := util.Read(opt, afe, subDir, segmentID)
		if err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			continue // 忽略单个读取错误，继续处理下一个片段
		}

		// 计算添加当前片段后的总大小
		newSize := currentSize + len(sliceContent)
		if newSize > int(downloadMaximumSize) {
			break // 如果超出最大大小限制，则退出循环
		}

		// 将片段内容存储到映射中
		segmentMap[index] = sliceContent
		// 更新当前回复的总大小
		currentSize = newSize
	}

	// 输出处理片段的数量
	return segmentMap, nil
}
