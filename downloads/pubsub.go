// package downloads_ 提供了文件下载相关的功能
package downloads

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/pointsub"
	"github.com/dep2p/pubsub"
	npspb "github.com/dep2p/pubsub/pb"
	"go.uber.org/fx"
)

// Topic 定义了允许的主题类型
type Topic int

// 定义主题类型常量
const (
	PubSubFileInfoRequestTopic          Topic = iota // 文件信息请求主题
	PubSubDownloadManifestRequestTopic               // 下载清单请求主题
	PubSubDownloadManifestResponseTopic              // 下载清单响应主题
	PubSubDownloadContentRequestTopic                // 下载内容请求主题
)

// topicStrings 将 Topic 枚举映射到对应的字符串值
var topicStrings = map[Topic]string{
	PubSubFileInfoRequestTopic:          fmt.Sprintf("defs@pubsub/download/fileinfo/request/%s", version),
	PubSubDownloadManifestRequestTopic:  fmt.Sprintf("defs@pubsub/download/manifest/request/%s", version),
	PubSubDownloadManifestResponseTopic: fmt.Sprintf("defs@pubsub/download/manifest/response/%s", version),
	PubSubDownloadContentRequestTopic:   fmt.Sprintf("defs@pubsub/download/content/request/%s", version),
}

// String 将Topic转换为对应的字符串表示
// 返回值:
//   - string: 主题对应的字符串
//
// 功能:
//   - 将Topic枚举值转换为对应的字符串格式
//   - 用于主题订阅和发布时的标识
func (t Topic) String() string {
	return topicStrings[t]
}

// AllowedTopics 定义了系统支持的所有主题列表
var AllowedTopics = []Topic{
	PubSubFileInfoRequestTopic,          // 文件信息请求主题
	PubSubDownloadManifestRequestTopic,  // 下载清单请求主题
	PubSubDownloadManifestResponseTopic, // 下载清单响应主题
	PubSubDownloadContentRequestTopic,   // 下载内容请求主题
}

// RegisterPubsubProtocolInput 定义了注册PubsubProtocol所需的输入参数
type RegisterPubsubProtocolInput struct {
	fx.In

	Ctx  context.Context    // 全局上下文，用于管理整个应用的生命周期
	Opt  *fscfg.Options     // 文件存储配置选项
	DB   *database.DB       // 本地数据存储实例
	FS   afero.Afero        // 文件系统接口
	Host host.Host          // libp2p网络主机实例
	PS   *pointsub.PointSub // 点对点传输实例
	// RoutingTable *kbucket.RoutingTable // 路由表，用于管理对等节点和路由
	NPS      *pubsub.NodePubSub // 发布订阅系统
	Download *DownloadManager   // 下载管理器实例
}

// RegisterDownloadPubsubProtocol 注册所有下载相关的PubSub协议处理器
// 参数:
//   - lc: 应用生命周期管理器
//   - input: 注册所需的输入参数
//
// 返回值:
//   - error: 注册失败返回错误,成功返回nil
//
// 功能:
//   - 订阅文件信息请求主题
//   - 订阅索引清单请求主题
//   - 订阅片段内容请求主题
//   - 订阅索引清单响应主题
//   - 为每个主题注册对应的处理函数
func RegisterDownloadPubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 添加生命周期钩子
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 订阅文件信息请求主题
			if err := input.NPS.SubscribeWithTopic(PubSubFileInfoRequestTopic.String(), func(res *pubsub.Message) {
				HandleFileInfoRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.Host, input.NPS, input.Download, res)
			}, true); err != nil {
				logger.Errorf("订阅文件信息请求主题失败: %v", err)
				return err
			}

			// 订阅索引清单请求主题
			if err := input.NPS.SubscribeWithTopic(PubSubDownloadManifestRequestTopic.String(), func(res *pubsub.Message) {
				HandleDownloadManifestRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.NPS, input.Download, res)
			}, true); err != nil {
				logger.Errorf("订阅索引清单请求主题失败: %v", err)
				return err
			}

			// 订阅片段内容请求主题
			if err := input.NPS.SubscribeWithTopic(PubSubDownloadContentRequestTopic.String(), func(res *pubsub.Message) {
				HandleDownloadContentRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.NPS, input.Download, res)
			}, true); err != nil {
				logger.Errorf("订阅片段内容请求主题失败: %v", err)
				return err
			}

			// 订阅索引清单响应主题
			if err := input.NPS.SubscribeWithTopic(PubSubDownloadManifestResponseTopic.String(), func(res *pubsub.Message) {
				HandleDownloadManifestResponsePubSub(input.Ctx, input.Opt, input.NPS, input.DB, input.FS, input.Download, res)
			}, true); err != nil {
				logger.Errorf("订阅索引清单响应主题失败: %v", err)
				return err
			}

			return nil
		},
	})
}

// ReplyToMessage 用于回复收到的消息
// 参数:
//   - ctx: 上下文
//   - topic: 主题实例
//   - messageID: 原始消息ID
//   - replyData: 回复的数据内容
//
// 返回值:
//   - error: 回复失败返回错误,成功返回nil
//
// 功能:
//   - 构造回复消息
//   - 发布回复消息到指定主题
//   - 记录错误日志
func ReplyToMessage(ctx context.Context, topic *pubsub.Topic, messageID string, replyData []byte) error {
	// 发布回复消息
	if err := topic.Publish(ctx, replyData, pubsub.WithMessageMetadata(messageID, npspb.MessageMetadata_RESPONSE)); err != nil {
		logger.Errorf("发布回复消息失败: %v", err)
		return err
	}

	return nil
}
