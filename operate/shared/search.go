package shared

import (
	"context"
	"sync"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/fscfg"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
	"github.com/dep2p/pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SearchSubscription 定义检索订阅结构
// 用于管理文件检索的订阅状态和通信
type SearchSubscription struct {
	ch   chan *pb.ResponseSearchFileSegmentPubSub // 用于接收检索响应的通道
	err  error                                    // 存储可能发生的错误
	done chan struct{}                            // 用于取消订阅的信号通道
	mu   sync.Mutex                               // 用于保护结构体字段的互斥锁
}

// Next 返回订阅中的下一个检索响应
// 参数:
//   - ctx: 上下文，用于控制操作的生命周期
//
// 返回值:
//   - *pb.ResponseSearchFileSegmentPubSub: 检索响应数据
//   - error: 可能发生的错误
func (sub *SearchSubscription) Next(ctx context.Context) (*pb.ResponseSearchFileSegmentPubSub, error) {
	select {
	case resp, ok := <-sub.ch: // 从通道接收响应
		if !ok {
			return nil, sub.err // 通道关闭时返回存储的错误
		}
		return resp, nil // 返回接收到的响应
	case <-ctx.Done(): // 上下文取消时返回错误
		return nil, ctx.Err()
	}
}

// Cancel 取消订阅
// 关闭done通道以通知相关goroutine停止工作
func (sub *SearchSubscription) Cancel() {
	sub.mu.Lock()         // 加锁保护并发访问
	defer sub.mu.Unlock() // 确保解锁

	if sub.done != nil {
		close(sub.done) // 关闭done通道
		sub.done = nil  // 避免重复关闭
	}
}

// newSearchSubscription 创建新的检索订阅实例
// 返回值:
//   - *SearchSubscription: 新创建的检索订阅对象
func newSearchSubscription() *SearchSubscription {
	return &SearchSubscription{
		ch:   make(chan *pb.ResponseSearchFileSegmentPubSub, 16), // 创建带缓冲的响应通道
		done: make(chan struct{}),                                // 创建用于取消的通道
	}
}

// 全局订阅管理器
var (
	subscriptions   = make(map[string]*SearchSubscription) // 存储文件ID到订阅对象的映射
	subscriptionsMu sync.RWMutex                           // 保护subscriptions map的读写锁
)

// RequestSearchFileSegmentPubSub 发起检索文件请求
// 参数:
//   - ctx: 上下文，用于控制操作的生命周期
//   - host: libp2p主机实例，用于网络通信
//   - nps: 节点发布订阅系统实例
//   - fileID: 要检索的文件ID
//
// 返回值:
//   - *SearchSubscription: 用于接收检索响应的订阅对象
//   - error: 可能发生的错误
func RequestSearchFileSegmentPubSub(
	ctx context.Context,
	host host.Host,
	nps *pubsub.NodePubSub,
	fileID string,
) (*SearchSubscription, error) {
	// 创建新的订阅
	sub := newSearchSubscription()

	// 注册订阅到全局管理器
	subscriptionsMu.Lock()
	subscriptions[fileID] = sub
	subscriptionsMu.Unlock()

	// 启动goroutine清理订阅资源
	go func() {
		<-sub.done // 等待取消信号
		subscriptionsMu.Lock()
		delete(subscriptions, fileID) // 从管理器中移除订阅
		close(sub.ch)                 // 关闭响应通道
		subscriptionsMu.Unlock()
	}()

	// 获取本地节点的地址信息
	addrInfo := peer.AddrInfo{
		ID:    host.ID(),
		Addrs: host.Addrs(),
	}

	// 序列化地址信息
	addrInfoBytes, err := addrInfo.MarshalJSON()
	if err != nil {
		logger.Errorf("序列化 AddrInfo 失败: %v", err)
		sub.Cancel()
		return nil, err
	}

	// 构造检索请求数据
	request := &pb.RequestSearchFileSegmentPubSub{
		FileId:   fileID,
		AddrInfo: addrInfoBytes,
	}

	// 序列化请求数据
	data, err := request.Marshal()
	if err != nil {
		logger.Errorf("序列化请求数据失败: %v", err)
		sub.Cancel()
		return nil, err
	}

	// 获取发布主题
	topic, err := nps.GetTopic(PubSubSearchFileSegmentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取主题失败: %v", err)
		sub.Cancel()
		return nil, err
	}

	// 发布检索请求到网络
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发送消息失败: %v", err)
		sub.Cancel()
		return nil, err
	}

	return sub, nil
}

// HandleSearchFileSegmentResponsePubSub 处理检索响应
// 参数:
//   - ctx: 上下文，用于控制操作的生命周期
//   - res: 收到的pubsub消息
func HandleSearchFileSegmentResponsePubSub(
	ctx context.Context,
	res *pubsub.Message,
) {
	// 解析响应数据
	response := new(pb.ResponseSearchFileSegmentPubSub)
	if err := response.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析检索文件响应数据失败: %v", err)
		return
	}

	// 查找对应的订阅
	subscriptionsMu.RLock()
	sub, exists := subscriptions[response.FileId]
	subscriptionsMu.RUnlock()

	if !exists {
		return
	}

	// 发送响应到订阅通道
	select {
	case sub.ch <- response:
		logger.Infof("成功发送检索响应: fileID=%s", response.FileId)
	case <-sub.done:
		logger.Warn("订阅已取消")
	case <-ctx.Done():
		logger.Warn("上下文已取消")
	}
}

// HandleSearchFileSegmentRequestPubSub 处理检索共享文件请求
// 参数:
//   - ctx: 上下文，用于控制操作的生命周期
//   - opt: 文件系统配置选项
//   - db: 数据库实例
//   - fs: 文件系统接口
//   - nps: 节点发布订阅系统实例
//   - res: 收到的pubsub消息
func HandleSearchFileSegmentRequestPubSub(
	ctx context.Context,
	opt *fscfg.Options,
	db *database.DB,
	fs afero.Afero,
	nps *pubsub.NodePubSub,
	res *pubsub.Message,
) {
	// 解析请求数据
	request := new(pb.RequestSearchFileSegmentPubSub)
	if err := request.Unmarshal(res.Data); err != nil {
		logger.Errorf("解析检索文件请求数据失败: %v", err)
		return
	}

	// 获取文件片段存储实例
	store := database.NewFileSegmentStorageSqlStore(db.SqliteDB)

	// 查询共享文件信息
	fileSegment, found, err := store.GetSharedFileSegmentStorageByFileID(request.FileId)
	if err != nil {
		logger.Errorf("查询共享文件失败: %v", err)
		return
	}

	if !found {
		logger.Warnf("未找到共享文件: fileID=%s", request.FileId)
		return
	}

	// 构造响应数据
	response := &pb.ResponseSearchFileSegmentPubSub{
		FileId:      fileSegment.FileId,
		Name:        fileSegment.Name,
		Extension:   fileSegment.Extension,
		Size_:       fileSegment.Size_,
		ContentType: fileSegment.ContentType,
		UploadTime:  fileSegment.UploadTime,
	}

	// 序列化响应数据
	data, err := response.Marshal()
	if err != nil {
		logger.Errorf("序列化响应数据失败: %v", err)
		return
	}

	// 获取响应主题
	topic, err := nps.GetTopic(PubSubSearchFileSegmentResponseTopic.String())
	if err != nil {
		logger.Errorf("获取响应主题失败: %v", err)
		return
	}

	// 发布响应消息到网络
	if err := topic.Publish(ctx, data); err != nil {
		logger.Errorf("发布响应消息失败: %v", err)
		return
	}

	logger.Infof("成功响应文件检索请求: fileID=%s", request.FileId)
}
