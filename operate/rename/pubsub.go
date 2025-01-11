package rename

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/fscfg"

	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/pubsub"
	"go.uber.org/fx"
)

const (
	version = "1.0.0" // 协议版本号,用于标识PubSub协议的版本
)

// Topic 定义了文件重命名操作的主题类型
// 用于标识不同类型的PubSub消息主题
type Topic int

// 定义文件重命名操作的主题类型常量
const (
	PubSubRenameFileSegmentRequestTopic Topic = iota // 重命名文件请求主题：用于修改文件名的请求消息
)

// topicStrings 将文件重命名操作的主题枚举映射到对应的字符串值
// key: Topic类型的主题枚举值
// value: 对应的字符串格式主题名称
var topicStrings = map[Topic]string{
	PubSubRenameFileSegmentRequestTopic: fmt.Sprintf("defs@pubsub/Rename/renameFileSegment/request/%s", version), // 重命名文件请求的主题字符串
}

// String 将Topic转换为对应的字符串表示
// 参数:
//   - t: Topic类型的主题枚举值
//
// 返回值:
//   - string: 返回主题对应的字符串名称
func (t Topic) String() string {
	return topicStrings[t]
}

// AllowedTopics 定义了系统支持的所有文件重命名操作主题
// 用于管理和限制系统中允许使用的PubSub主题
var AllowedTopics = []Topic{
	PubSubRenameFileSegmentRequestTopic, // 重命名：允许修改文件名的主题
}

// RegisterPubsubProtocolInput 定义了注册PubsubProtocol所需的输入参数
// 使用fx.In标记用于依赖注入
type RegisterPubsubProtocolInput struct {
	fx.In

	Ctx  context.Context    // 全局上下文，用于管理整个应用的生命周期
	Opt  *fscfg.Options     // 文件存储配置选项，包含系统运行所需的各种参数
	DB   *database.DB       // 本地数据存储实例，用于持久化数据
	FS   afero.Afero        // 文件系统接口，提供文件操作能力
	Host host.Host          // libp2p网络主机实例，用于P2P网络通信
	NPS  *pubsub.NodePubSub // 发布订阅系统，用于处理P2P网络中的消息订阅和发布
}

// RegisterRenamePubsubProtocol 注册文件重命名相关的PubSub协议处理器
// 参数:
//   - lc: fx.Lifecycle 应用生命周期管理器，用于管理协议处理器的生命周期
//   - input: RegisterPubsubProtocolInput 注册所需的输入参数，包含所有必要的依赖项
//
// 返回值:
//   - error: 如果注册过程中出现错误，返回相应的错误信息；成功则返回nil
func RegisterRenamePubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 添加生命周期钩子，用于在应用启动时注册处理器
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 订阅重命名文件请求主题，设置消息处理函数
			if err := input.NPS.SubscribeWithTopic(PubSubRenameFileSegmentRequestTopic.String(), func(res *pubsub.Message) {
				HandleRenameFileSegmentRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.NPS, res)
			}, true); err != nil {
				logger.Errorf("订阅重命名文件请求主题失败: topic=%s, err=%v", PubSubRenameFileSegmentRequestTopic.String(), err)
				return err
			}
			return nil
		},
	})
}
