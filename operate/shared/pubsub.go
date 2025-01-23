package shared

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/fscfg"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/pubsub"
	"go.uber.org/fx"
)

const (
	version = "1.0.0" // 协议版本号，用于区分不同版本的PubSub协议
)

// Topic 定义了文件共享操作的主题类型
type Topic int

// 定义文件共享操作的主题类型常量
const (
	PubSubSetFileSegmentRequestTopic     Topic = iota // 设置共享文件请求主题：用于设置文件的共享状态
	PubSubSearchFileSegmentRequestTopic               // 检索共享文件请求主题：用于发起文件检索
	PubSubSearchFileSegmentResponseTopic              // 检索共享文件响应主题：用于返回检索结果
)

// topicStrings 将文件共享操作的主题枚举映射到对应的字符串值
// 每个主题字符串包含：协议名称、操作类型、请求/响应类型和版本号
var topicStrings = map[Topic]string{
	PubSubSetFileSegmentRequestTopic:     fmt.Sprintf("defs@pubsub/Shared/setFileSegment/request/%s", version),
	PubSubSearchFileSegmentRequestTopic:  fmt.Sprintf("defs@pubsub/Shared/searchFileSegment/request/%s", version),
	PubSubSearchFileSegmentResponseTopic: fmt.Sprintf("defs@pubsub/Shared/searchFileSegment/response/%s", version),
}

// String 将Topic转换为对应的字符串表示
// 返回值:
//   - string: 主题对应的字符串
func (t Topic) String() string {
	return topicStrings[t]
}

// AllowedTopics 定义了系统支持的所有文件共享操作主题
var AllowedTopics = []Topic{
	PubSubSetFileSegmentRequestTopic,     // 设置共享：允许设置文件的共享属性
	PubSubSearchFileSegmentRequestTopic,  // 检索请求：允许搜索网络中的共享文件
	PubSubSearchFileSegmentResponseTopic, // 检索响应：处理检索结果
}

// RegisterPubsubProtocolInput 定义了注册PubsubProtocol所需的输入参数
type RegisterPubsubProtocolInput struct {
	fx.In

	Ctx  context.Context    // 全局上下文，用于管理整个应用的生命周期
	Opt  *fscfg.Options     // 文件存储配置选项
	DB   *database.DB       // 本地数据存储实例
	FS   afero.Afero        // 文件系统接口
	Host host.Host          // libp2p网络主机实例
	NPS  *pubsub.NodePubSub // 发布订阅系统
}

// RegisterSharedPubsubProtocol 注册文件共享相关的PubSub协议处理器
// 参数:
//   - lc: fx.Lifecycle 应用生命周期管理器
//   - input: RegisterPubsubProtocolInput 注册所需的输入参数
func RegisterSharedPubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 添加生命周期钩子
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 订阅设置共享文件请求主题
			if err := input.NPS.SubscribeWithTopic(PubSubSetFileSegmentRequestTopic.String(), func(res *pubsub.Message) {
				HandleSetFileSegmentRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.NPS, res)
			}, true); err != nil {
				logger.Errorf("订阅设置共享文件请求主题失败: topic=%s, err=%v", PubSubSetFileSegmentRequestTopic.String(), err)
				return err
			}

			// 订阅检索共享文件请求主题
			if err := input.NPS.SubscribeWithTopic(PubSubSearchFileSegmentRequestTopic.String(), func(res *pubsub.Message) {
				HandleSearchFileSegmentRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.NPS, res)
			}, true); err != nil {
				logger.Errorf("订阅检索共享文件请求主题失败: topic=%s, err=%v", PubSubSearchFileSegmentRequestTopic.String(), err)
				return err
			}

			// 订阅检索共享文件响应主题
			if err := input.NPS.SubscribeWithTopic(PubSubSearchFileSegmentResponseTopic.String(), func(res *pubsub.Message) {
				HandleSearchFileSegmentResponsePubSub(input.Ctx, res)
			}, true); err != nil {
				logger.Errorf("订阅检索共享文件响应主题失败: topic=%s, err=%v", PubSubSearchFileSegmentResponseTopic.String(), err)
				return err
			}

			return nil
		},
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}
