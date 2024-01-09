package registry

import (
	"context"

	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/delete"
	"github.com/bpfs/defs/core/download"
	"github.com/bpfs/defs/core/edit"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/search"
	"github.com/bpfs/defs/core/upload"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

type RegisterPubsubProtocolInput struct {
	fx.In
	Ctx          context.Context         // 全局上下文
	Opt          *opts.Options           // 文件存储选项配置
	P2P          *dep2p.DeP2P            // DeP2P网络主机
	PubSub       *pubsub.DeP2PPubSub     // DeP2P网络订阅
	DB           *sqlites.SqliteDB       // sqlite数据库服务
	UploadChan   chan *core.UploadChan   // 用于刷新上传的通道
	DownloadChan chan *core.DownloadChan // 用于刷新下载的通道
	SearchChan   chan *search.SearchChan // 用于刷新搜索的通道

	Registry *eventbus.EventRegistry // 事件总线
	Cache    *ristretto.Cache        // 缓存实例
	Pool     *pool.MemoryPool        // 内存池
}

// RegisterPubsubProtocol 注册订阅
func RegisterPubsubProtocol(lc fx.Lifecycle, input RegisterPubsubProtocolInput) {
	// 文件上传请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubFileUploadRequestTopic, func(res *streams.RequestMessage) {
		upload.HandleFileUploadRequestPubSub(input.P2P, input.PubSub, res)
	}, true); err != nil {
		logrus.Errorf("注册文件上传请求失败：%v \n", err)
	}

	// 文件上传响应主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubFileUploadResponseTopic, func(res *streams.RequestMessage) {
		upload.HandleFileUploadResponsePubSub(input.P2P, input.PubSub, input.Pool, res)
	}, true); err != nil {
		logrus.Errorf("注册文件上传响应失败：%v \n", err)
	}

	// 文件下载请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubFileDownloadRequestTopic, func(res *streams.RequestMessage) {
		download.HandleFileDownloadRequestPubSub(input.P2P, input.PubSub, res)
	}, true); err != nil {
		logrus.Errorf("注册文件下载请求失败：%v \n", err)
	}

	// 文件下载响应主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubFileDownloadResponseTopic, func(res *streams.RequestMessage) {
		download.HandleFileDownloadResponsePubSub(input.P2P, input.PubSub, input.DB, input.DownloadChan, input.Registry, input.Pool, res)
	}, true); err != nil {
		logrus.Errorf("注册文件下载响应失败：%v \n", err)
	}

	// 文件删除请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubFileDeleteRequestTopic, func(res *streams.RequestMessage) {
		delete.HandleFileDeleteRequestPubSub(input.P2P, input.PubSub, res)
	}, true); err != nil {
		logrus.Errorf("注册文件删除请求失败：%v \n", err)
	}

	// 文件修改请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubFileEditRequestTopic, func(res *streams.RequestMessage) {
		edit.HandleFileEditRequestPubSub(input.P2P, input.PubSub, input.DB, res)
	}, true); err != nil {
		logrus.Errorf("注册文件修改请求失败：%v \n", err)
	}

	// 文件新增共享请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubAddSharedRequestTopic, func(res *streams.RequestMessage) {
		edit.HandleAddSharedRequestPubSub(input.Opt, input.P2P, input.PubSub, input.DB, res)
	}, true); err != nil {
		logrus.Errorf("注册文件新增共享请求失败：%v \n", err)
	}

	// 新增搜索请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubAddSearchRequestTopic, func(res *streams.RequestMessage) {
		search.HandleAddSearchRequestPubSub(input.P2P, input.PubSub, input.DB, res)
	}, true); err != nil {
		logrus.Errorf("注册新增搜索请求失败：%v \n", err)
	}

	// 新增搜索请求主题
	if err := input.PubSub.SubscribeWithTopic(config.PubsubAddSearchResponseTopic, func(res *streams.RequestMessage) {
		search.HandleAddSearchResponsePubSub(input.P2P, input.PubSub, input.DB, input.SearchChan, res)
	}, true); err != nil {
		logrus.Errorf("注册新增搜索请求失败：%v \n", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}
