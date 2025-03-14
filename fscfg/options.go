package fscfg

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bpfs/defs/v2/utils/paths"
	"github.com/dep2p/pubsub"
)

const (
	DbFile  = "database.db"
	Version = "0.0.1"
)

// Option 定义了一个函数类型，用于配置 BPFS
type Option func(*Options) error

// 存储模式
type StorageMode int

const (
	FileMode      StorageMode = iota // 文件模式
	SliceMode                        // 切片模式，将文件分割成有限个切片(不使用纠删码)
	RS_Size                          // 纠删码(大小)模式
	RS_Proportion                    // 纠删码(比例)模式
)

// Options 是用于创建文件存储对象的参数
type Options struct {
	storageMode          StorageMode       // 存储模式
	defaultBufSize       int64             // 默认缓冲区大小
	maxBufferSize        int64             // 最大缓冲区大小
	maxSliceSize         int64             // 最大切片大小
	minSliceSize         int64             // 最小切片大小
	dataShards           int64             // 数据分片数量
	parityShards         int64             // 校验分片数量
	shardSize            int64             // 分片大小
	parityRatio          float64           // 校验比例
	maxConcurrentUploads int64             // 最大并发上传数量
	maxUploadRetries     int               // 上传失败时的最大重试次数
	defaultOwnerPriv     *ecdsa.PrivateKey // 默认所有者私钥
	defaultFileKey       string            // 默认文件密钥
	rootPath             string            // 根路径
	downloadPath         string            // 下载路径
	downloadMaximumSize  int64             // 最大下载大小
	maxRetries           int64             // 最大重试次数
	retryInterval        time.Duration     // 重试间隔
	localStorage         bool              // 是否使用本地存储
	routingTableLow      int64             // 路由表最小节点数
	maxXrefTable         int64             // 最大交叉引用表大小
	maxUploadSize        int64             // 最大上传大小
	minUploadSize        int64             // 最小上传大小
	maxPeersPerCpl       int               // 每个CPL值允许的最大节点数
	bucketSize           int               // 路由表桶的大小
	// PubSub 相关配置
	pubsubOptions          []pubsub.NodeOption // PubSub 配置选项列表
	minUploadServerNodes   int                 // 上传所需最小服务端节点数
	minDownloadServerNodes int                 // 下载所需最小服务端节点数
	nps                    *pubsub.NodePubSub  // 发布订阅系统实例
	maxConcurrentDownloads int64               // 最大并发下载数量
}

// DefaultOptions 设置一个推荐选项列表以获得良好的性能。
func DefaultOptions() *Options {
	return &Options{
		storageMode:    RS_Proportion, // 默认使用纠删码(比例)模式
		defaultBufSize: 1 << 12,       // 默认缓冲区大小为4KB
		maxBufferSize:  1 << 30,       // 最大缓冲区大小为1GB
		maxSliceSize:   1 << 25,       // 最大切片大小为32MB
		minSliceSize:   1 << 10,       // 最小切片大小为1KB
		// shardSize:            1 << 19,                     // 分片大小为512KB
		shardSize:            1 << 20,                     // 分片大小为1MB
		parityRatio:          0.3,                         // 校验比例为30%
		maxConcurrentUploads: 5,                           // 设置默认的最大并发上传数量
		maxUploadRetries:     3,                           // 设置默认的最大重试次数
		rootPath:             paths.GetRootPath(),         // 获取根路径
		downloadPath:         paths.DefaultDownloadPath(), // 获取默认下载路径
		downloadMaximumSize:  2 * 1024 * 1024,             // 最大下载大小为2MB
		maxRetries:           5,                           // 最大重试次数为5次
		retryInterval:        50 * time.Second,            // 重试间隔为50秒
		localStorage:         true,                        // 默认使用本地存储
		routingTableLow:      2,                           // 路由表最小节点数为2
		maxXrefTable:         10000,                       // 最大交叉引用表大小为10000
		//maxUploadSize:        10 << 30,                    // 最大上传大小为10GB
		maxUploadSize: 1 << 30, // 最大上传大小为1GB
		// minUploadSize:        1 << 20,                     // 最小上传大小为1MB
		minUploadSize:          1 << 19,               // 最小上传大小为512KB
		maxPeersPerCpl:         3,                     // CPL(Common Prefix Length)是指两个节点ID的共同前缀长度，用于衡量节点间的距离。每个CPL值最多允许3个节点，可以防止路由表被某个特定距离的节点占据，提高网络的多样性和稳定性
		bucketSize:             50,                    // 默认桶大小为50
		minUploadServerNodes:   1,                     // 默认上传所需最小服务端节点数为1
		minDownloadServerNodes: 1,                     // 默认下载所需最小服务端节点数为1
		nps:                    nil,                   // 默认为空,需要时再创建
		pubsubOptions:          []pubsub.NodeOption{}, // 默认空选项列表
		maxConcurrentDownloads: 5,                     // 设置默认的最大并发下载数量
	}
}

// ApplyOptions 应用给定的选项到 Options 对象
// 参数:
//   - opts: 可变参数,包含多个选项函数
//
// 返回值:
//   - error: 应用选项过程中的错误信息
func (opt *Options) ApplyOptions(opts ...Option) error {
	// 遍历所有选项函数
	for _, o := range opts {
		// 执行选项函数,如果出错则返回错误
		if err := o(opt); err != nil {
			return err
		}
	}
	return nil
}

// GetStorageMode 获取存储模式
// 返回值:
//   - StorageMode: 当前的存储模式
func (opt *Options) GetStorageMode() StorageMode {
	return opt.storageMode
}

// GetDefaultBufSize 获取常用缓冲区的大小
// 返回值:
//   - int64: 默认缓冲区大小(字节)
func (opt *Options) GetDefaultBufSize() int64 {
	return opt.defaultBufSize
}

// GetMaxBufferSize 获取最大缓冲的大小
// 返回值:
//   - int64: 最大缓冲区大小(字节)
func (opt *Options) GetMaxBufferSize() int64 {
	return opt.maxBufferSize
}

// GetMaxSliceSize 获取最大片段的大小
// 返回值:
//   - int64: 最大片段大小(字节)
func (opt *Options) GetMaxSliceSize() int64 {
	return opt.maxSliceSize
}

// GetMinSliceSize 获取最小片段的大小
// 返回值:
//   - int64: 最小片段大小(字节)
func (opt *Options) GetMinSliceSize() int64 {
	return opt.minSliceSize
}

// GetDataShards ��取文件数据片段的数量
// 返回值:
//   - int64: 数据片段数量
func (opt *Options) GetDataShards() int64 {
	return opt.dataShards
}

// GetParityShards 获取奇偶校验片段的数量
// 返回值:
//   - int64: 校验片段数量
func (opt *Options) GetParityShards() int64 {
	return opt.parityShards
}

// GetShardSize 获取文件片段的大小
// 返回值:
//   - int64: 片段大小(字节)
func (opt *Options) GetShardSize() int64 {
	return opt.shardSize
}

// GetParityRatio 获取奇偶校验片段占比
// 返回值:
//   - float64: 校验片段占比(0-1之间的小数)
func (opt *Options) GetParityRatio() float64 {
	return opt.parityRatio
}

// GetDefaultOwnerPriv 获取默认所有者私钥
// 返回值:
//   - *ecdsa.PrivateKey: 默认所有者的ECDSA私钥
func (opt *Options) GetDefaultOwnerPriv() *ecdsa.PrivateKey {
	return opt.defaultOwnerPriv
}

// GetDefaultFileKey 获取默认文件密钥
// 返回值:
//   - string: 默认文件加密密钥
func (opt *Options) GetDefaultFileKey() string {
	return opt.defaultFileKey
}

// GetRootPath 获取文件根路径
// 返回值:
//   - string: 文件系统根路径
func (opt *Options) GetRootPath() string {
	return opt.rootPath
}

// GetDownloadPath 获取下载路径
// 返回值:
//   - string: 文件下载保存路径
func (opt *Options) GetDownloadPath() string {
	return opt.downloadPath
}

// GetDownloadMaximumSize 获取下载最大回复大小
// 返回值:
//   - int64: 最大下载大小(字节)
func (opt *Options) GetDownloadMaximumSize() int64 {
	return opt.downloadMaximumSize
}

// GetMaxRetries 获取最大重试次数
// 返回值:
//   - int64: 操作失败时的最大重试次数
func (opt *Options) GetMaxRetries() int64 {
	return opt.maxRetries
}

// GetRetryInterval 获取重试间隔
// 返回值:
//   - time.Duration: 重试操作的时间间隔
func (opt *Options) GetRetryInterval() time.Duration {
	return opt.retryInterval
}

// GetLocalStorage 获取是否开启本地存储
// 返回值:
//   - bool: true表示使用本地存储,false表示不使用
func (opt *Options) GetLocalStorage() bool {
	return opt.localStorage
}

// GetRoutingTableLow 获取路由表中连接的最小节点数量
// 返回值:
//   - int64: 路由表最小节点数
func (opt *Options) GetRoutingTableLow() int64 {
	return opt.routingTableLow
}

// GetMaxXrefTable 获取Xref表中段的最大数量
// 返回值:
//   - int64: 交叉引用表最大条目数
func (opt *Options) GetMaxXrefTable() int64 {
	return opt.maxXrefTable
}

// GetMaxUploadSize 获取最大上传大小
// 返回值:
//   - int64: 单个文件最大上传大小(字节)
func (opt *Options) GetMaxUploadSize() int64 {
	return opt.maxUploadSize
}

// GetMinUploadSize 获取最小上传大小
// 返回值:
//   - int64: 单个文件最小上传大小(字节)
func (opt *Options) GetMinUploadSize() int64 {
	return opt.minUploadSize
}

// GetMaxConcurrentUploads 获取最大并发上传数量
// 返回值:
//   - int64: 允许同时进行的最大上传任务数
func (opt *Options) GetMaxConcurrentUploads() int64 {
	return opt.maxConcurrentUploads
}

// GetMaxUploadRetries 获取上传失败时的最大重试次数
// 返回值:
//   - int: 上传失败后的最大重试次数
func (opt *Options) GetMaxUploadRetries() int {
	return opt.maxUploadRetries
}

// GetMaxPeersPerCpl 获取每个CPL值允许的最大节点数
// 返回值:
//   - int: 每个CPL值允许的最大节点数,如未设置则返回默认值3
func (opt *Options) GetMaxPeersPerCpl() int {
	// 检查是否设置了有效值
	if opt.maxPeersPerCpl <= 0 {
		return 3 // 如果未设置或设置为非正数，返回默认值
	}
	return opt.maxPeersPerCpl
}

// GetBucketSize 获取路由表桶的大小
// 返回值:
//   - int: 路由表桶的大小,如未设置则返回默认值20
func (opt *Options) GetBucketSize() int {
	// 检查是否设置了有效值
	if opt.bucketSize <= 0 {
		return 20 // 如果未设置或设置为非正数，返回默认值
	}
	return opt.bucketSize
}

// GetPubSubOptions 获取所有 PubSub 配置选项
// 返回值:
//   - []pubsub.NodeOption: PubSub节点配置选项列表
func (opt *Options) GetPubSubOptions() []pubsub.NodeOption {
	return opt.pubsubOptions
}

// GetMinUploadServerNodes 获取上传所需最小服务端节点数
// 返回值:
//   - int: 上传操作所需的最小服务端节点数量
func (opt *Options) GetMinUploadServerNodes() int {
	return opt.minUploadServerNodes
}

// GetMinDownloadServerNodes 获取下载所需最小服务端节点数
// 返回值:
//   - int: 下载操作所需的最小服务端节点数量
func (opt *Options) GetMinDownloadServerNodes() int {
	return opt.minDownloadServerNodes
}

// GetNodePubSub 获取发布订阅系统实例
// 返回值:
//   - *pubsub.NodePubSub: 发布订阅系统实例指针
func (opt *Options) GetNodePubSub() *pubsub.NodePubSub {
	return opt.nps
}

// GetMaxConcurrentDownloads 获取最大并发下载数量
// 返回值:
//   - int64: 允许同时进行的最大下载任务数
func (opt *Options) GetMaxConcurrentDownloads() int64 {
	return opt.maxConcurrentDownloads
}

// WithPubSubOption 添加 PubSub 配置选项
// 参数:
//   - opt pubsub.NodeOption: 要添加的 PubSub 配置选项
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置 PubSub 选项
func WithPubSubOption(opt pubsub.NodeOption) Option {
	return func(o *Options) error {
		// 将新的 PubSub 选项追加到选项列表中
		o.pubsubOptions = append(o.pubsubOptions, opt)
		return nil
	}
}

// WithStorageMode 设置存储模式
// 参数:
//   - mode StorageMode: 要设置的存储模式
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置存储模式
func WithStorageMode(mode StorageMode) Option {
	return func(opt *Options) error {
		// 设置存储模式
		opt.storageMode = mode
		return nil
	}
}

// WithDefaultBufSize 设置常用缓冲区的大小
// 参数:
//   - size int64: 要设置的缓冲区大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置默认缓冲区大小
func WithDefaultBufSize(size int64) Option {
	return func(opt *Options) error {
		// 设置默认缓冲区大小
		opt.defaultBufSize = size
		return nil
	}
}

// WithMaxBufferSize 设置最大缓冲区的大小
// 参数:
//   - size int64: 要设置的最大缓冲区大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大缓冲区大小
func WithMaxBufferSize(size int64) Option {
	return func(opt *Options) error {
		// 设置最大缓冲区大小
		opt.maxBufferSize = size
		return nil
	}
}

// WithMaxSliceSize 设置最大片段的大小
// 参数:
//   - size int64: 要设置的最大片段大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大片段大小
func WithMaxSliceSize(size int64) Option {
	return func(opt *Options) error {
		// 检查片段大小是否超过上限(32MB)
		if size > 1<<25 {
			return fmt.Errorf("最大切片的大小 %d 不可大于 %d", size, 1<<25)
		}
		// 设置最大片段大小
		opt.maxSliceSize = size
		return nil
	}
}

// WithMinSliceSize 设置最小片段的大小
// 参数:
//   - size int64: 要设置的最小片段大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最小片段大小
func WithMinSliceSize(size int64) Option {
	return func(opt *Options) error {
		// 检查片段大小是否小于下限(1KB)
		if size < 1<<10 {
			return fmt.Errorf("最小切片的大小 %d 不可小于 %d", size, 1<<10)
		}
		// 设置最小片段大小
		opt.minSliceSize = size
		return nil
	}
}

// WithDataShards 设置文件数据片段的���量
// 参数:
//   - count int64: 要设置的数据片段数量
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置数据片段数量
func WithDataShards(count int64) Option {
	return func(opt *Options) error {
		// 设置数据片段数量
		opt.dataShards = count
		return nil
	}
}

// WithParityShards 设置奇偶校验片段的数量
// 参数:
//   - count int64: 要设置的校验片段数量
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置校验片段数量
func WithParityShards(count int64) Option {
	return func(opt *Options) error {
		// 设置校验片段数量
		opt.parityShards = count
		return nil
	}
}

// WithShardSize 设置文件片段的大小
// 参数:
//   - size int64: 要设置的片段大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置片段大小
func WithShardSize(size int64) Option {
	return func(opt *Options) error {
		// 设置片段大小
		opt.shardSize = size
		return nil
	}
}

// WithParityRatio 设置奇偶校段占比
// 参数:
//   - ratio float64: 要设置的校验片段占比
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置校验片段占比
func WithParityRatio(ratio float64) Option {
	return func(opt *Options) error {
		// 设置校验片段占��
		opt.parityRatio = ratio
		return nil
	}
}

// WithDefaultOwnerPriv 设置默认所有者私钥
// 参数:
//   - key *ecdsa.PrivateKey: 要设置的默认所有者私钥
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置默认所有者私钥
func WithDefaultOwnerPriv(key *ecdsa.PrivateKey) Option {
	return func(opt *Options) error {
		// 设置默认所有者私钥
		opt.defaultOwnerPriv = key
		return nil
	}
}

// WithDefaultFileKey 设置默认文件密钥
// 参数:
//   - key string: 要设置的默认文件密钥
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置默认文件密钥
func WithDefaultFileKey(key string) Option {
	return func(opt *Options) error {
		// 设置默认文件密钥
		opt.defaultFileKey = key
		return nil
	}
}

// WithRootPath 设置文件根路径
// 参数:
//   - path string: 要设置的根路径
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置根路径
func WithRootPath(path string) Option {
	return func(opt *Options) error {
		// 如果路径为空,则不进行设置
		if path == "" {
			return nil
		}
		// 检查是否为绝对路径
		if !filepath.IsAbs(path) {
			return fmt.Errorf("根路径必须是绝对路径")
		}
		// 创建目录
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		// 设置根路径
		opt.rootPath = path
		return nil
	}
}

// WithDownloadPath 设置下载路径
// 参数:
//   - path string: 要设置的下载路径
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置下载路径
func WithDownloadPath(path string) Option {
	return func(opt *Options) error {
		// 如果路径为空,则不进行设置
		if path == "" {
			return nil
		}
		// 检查是否为绝对路径
		if !filepath.IsAbs(path) {
			return fmt.Errorf("下载路径必须是绝对路径")
		}
		// 创建目录
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		// 设置下载路径
		opt.downloadPath = path
		return nil
	}
}

// WithDownloadMaximumSize 设置下载最大回复大小
// 参数:
//   - size int64: 要设置的最大回复大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大回复大小
func WithDownloadMaximumSize(size int64) Option {
	return func(opt *Options) error {
		// 设置下载最大回复大小
		opt.downloadMaximumSize = size
		return nil
	}
}

// WithMaxRetries 设置最大重试次数
// 参数:
//   - count int64: 要设置的最大重试次数
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大重试次数
func WithMaxRetries(count int64) Option {
	return func(opt *Options) error {
		// 设置最大重试次数
		opt.maxRetries = count
		return nil
	}
}

// WithRetryInterval 设置重试间隔
// 参数:
//   - interval time.Duration: 要设置的重试间隔时间
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置重试间隔
func WithRetryInterval(interval time.Duration) Option {
	return func(opt *Options) error {
		// 设置重试间隔
		opt.retryInterval = interval
		return nil
	}
}

// WithLocalStorage 设置是否开启本地存储
// 参数:
//   - enable bool: 是否启用本地存储
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置本地存储开关
func WithLocalStorage(enable bool) Option {
	return func(opt *Options) error {
		// 设置本地存储开关
		opt.localStorage = enable
		return nil
	}
}

// WithRoutingTableLow 设置路由表中连接的最小节点数量
// 参数:
//   - count int64: 要设置的最小节点数量
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最小节点数量
func WithRoutingTableLow(count int64) Option {
	return func(opt *Options) error {
		// 设置路由表最小节点数量
		opt.routingTableLow = count
		return nil
	}
}

// WithMaxXrefTable 设置Xref表中段的最大数量
// 参数:
//   - count int64: 要设置的最大段数量
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大段数量
func WithMaxXrefTable(count int64) Option {
	return func(opt *Options) error {
		// 设置Xref表最大段数量
		opt.maxXrefTable = count
		return nil
	}
}

// WithMaxUploadSize 设置最大上传大小
// 参数:
//   - size int64: 要设置的最大上传大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大上传大小
func WithMaxUploadSize(size int64) Option {
	return func(opt *Options) error {
		// 设置最大上传大小
		opt.maxUploadSize = size
		return nil
	}
}

// WithMinUploadSize 设置最小上传大小
// 参数:
//   - size int64: 要设置的最小上传大小(字节)
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最小上传大小
func WithMinUploadSize(size int64) Option {
	return func(opt *Options) error {
		// 设置最小上传大小
		opt.minUploadSize = size
		return nil
	}
}

// WithMaxConcurrentUploads 设置最大并发上传数量
// 参数:
//   - count int64: 要设置的最大并发上传数量
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大并发上传数量
func WithMaxConcurrentUploads(count int64) Option {
	return func(opt *Options) error {
		// 检查并发数量是否大于0
		if count <= 0 {
			return fmt.Errorf("最大并发上传数量必须大于0")
		}
		// 设置最大并发上传数量
		opt.maxConcurrentUploads = count
		return nil
	}
}

// WithMaxUploadRetries 设置上传失败时的最大重试次数
// 参数:
//   - count int: 要设置的最大重试次数
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大重试次数
func WithMaxUploadRetries(count int) Option {
	return func(opt *Options) error {
		// 检查重试次数是否为负数
		if count < 0 {
			return fmt.Errorf("最大重试次数不能为负数")
		}
		// 设置最大重试次数
		opt.maxUploadRetries = count
		return nil
	}
}

// WithMaxPeersPerCpl 设置每个CPL值允许的最大节点数
// 参数:
//   - count int: 要设置的最大节点数
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置每个CPL的最大节点数
func WithMaxPeersPerCpl(count int) Option {
	return func(opt *Options) error {
		// 检查节点数是否大于0
		if count <= 0 {
			return fmt.Errorf("每个CPL值允许的最大节点数必须大于0")
		}
		// 设置每个CPL的最大节点数
		opt.maxPeersPerCpl = count
		return nil
	}
}

// WithBucketSize 设置路由表桶的大小
// 参数:
//   - size int: 要设置的桶大小
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置路由表桶的大小
func WithBucketSize(size int) Option {
	return func(opt *Options) error {
		// 检查桶大小是否大于0
		if size <= 0 {
			return fmt.Errorf("路由表桶的大小必须大于0")
		}
		// 设置路由表桶的大小
		opt.bucketSize = size
		return nil
	}
}

// WithMinUploadServerNodes 设置上传所需最小服务端节点数
// 参数:
//   - count int: 要设置的最小节点数
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最小服务端节点数
func WithMinUploadServerNodes(count int) Option {
	return func(opt *Options) error {
		// 检查节点数是否大于0
		if count <= 0 {
			return fmt.Errorf("上传所需最小服务端节点数必须大于0")
		}
		// 设置最小服务端节点数
		opt.minUploadServerNodes = count
		return nil
	}
}

// WithMinDownloadServerNodes 设置下载所需最小服务端节点数
// 参数:
//   - count int: 要设置的最小节点数
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最小服务端节点数
func WithMinDownloadServerNodes(count int) Option {
	return func(opt *Options) error {
		// 检查节点数是否大于0
		if count <= 0 {
			return fmt.Errorf("下载所需最小服务端节点数必须大于0")
		}
		// 设置最小服务端节点数
		opt.minDownloadServerNodes = count
		return nil
	}
}

// WithNodePubSub 设置发布订阅系统实例
// 参数:
//   - nps *pubsub.NodePubSub: 要设置的发布订阅系统实例
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置发布订阅系统实例
func WithNodePubSub(nps *pubsub.NodePubSub) Option {
	return func(opt *Options) error {
		// 设置发布订阅系统实例
		opt.nps = nps
		return nil
	}
}

// WithMaxConcurrentDownloads 设置最大并发下载数量
// 参数:
//   - count int64: 要设置的最大并发下载数量
//
// 返回值:
//   - Option: 返回一个配置函数,用于设置最大并发下载数量
func WithMaxConcurrentDownloads(count int64) Option {
	return func(opt *Options) error {
		// 检查并发数量是否大于0
		if count <= 0 {
			return fmt.Errorf("最大并发下载数量必须大于0")
		}
		// 设置最大并发下载数量
		opt.maxConcurrentDownloads = count
		return nil
	}
}
