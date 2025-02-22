// Package uploads 提供了文件上传相关的功能
package uploads

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

// Topic 定义了允许的主题类型
type Topic int

// 定义主题类型常量
const (
	PubSubDeleteFileSegmentRequestTopic Topic = iota // 删除文件切片请求主题
)

// topicStrings 将 Topic 枚举映射到对应的字符串值
var topicStrings = map[Topic]string{
	PubSubDeleteFileSegmentRequestTopic: fmt.Sprintf("defs@pubsub/upload/deleteFileSegment/request/%s", version),
}

// String 将Topic转换为对应的字符串表示
// 返回值:
//   - string: 主题对应的字符串
func (t Topic) String() string {
	return topicStrings[t]
}

// AllowedTopics 定义了系统支持的所有主题列表
var AllowedTopics = []Topic{
	PubSubDeleteFileSegmentRequestTopic, // 删除文件切片请求主题
}

// RegisterPubsubProtocolInput 定义了注册PubsubProtocol所需的输入参数
type RegisterPubsubProtocolInput struct {
	fx.In

	Ctx    context.Context    // 全局上下文，用于管理整个应用的生命周期
	Opt    *fscfg.Options     // 文件存储配置选项
	DB     *database.DB       // 本地数据存储实例
	FS     afero.Afero        // 文件系统接口
	Host   host.Host          // libp2p网络主机实例
	NPS    *pubsub.NodePubSub // 发布订阅系统
	Upload *UploadManager     // 上传管理器实例
}

// RegisterUploadPubsubProtocol 注册所有上传相关的PubSub协议处理器
// 参数:
//   - lc: fx.Lifecycle 应用生命周期管理器
//   - input: RegisterPubsubProtocolInput 注册所需的输入参数
//
// 返回值:
//   - error: 如果注册过程中出现错误，返回相应的错误信息
func RegisterUploadPubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 添加生命周期钩子
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 订阅删除文件切片请求主题
			if err := input.NPS.SubscribeWithTopic(PubSubDeleteFileSegmentRequestTopic.String(), func(res *pubsub.Message) {
				// 处理删除文件切片请求
				if err := HandleDeleteFileSegmentRequestPubSub(input.Ctx, input.Opt, input.DB, input.FS, input.NPS, input.Upload, res); err != nil {
					logger.Errorf("处理删除文件切片请求失败: topic=%s, err=%v", PubSubDeleteFileSegmentRequestTopic.String(), err)
				}
			}, true); err != nil {
				logger.Errorf("订阅删除文件切片请求主题失败: topic=%s, err=%v", PubSubDeleteFileSegmentRequestTopic.String(), err)
				return err
			}

			return nil
		},
	})
}
