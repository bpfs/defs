package defs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// 存储模式
type StorageMode int

const (
	FileMode      StorageMode = iota // 文件模式
	SliceMode                        // 切片模式，将文件分割成有限个切片
	RS_Size                          // 纠删码(大小)模式
	RS_Proportion                    // 纠删码(比例)模式
)

// Options 是用于创建文件存储对象的参数
type Options struct {
	storageMode     StorageMode   // 存储模式
	defaultBufSize  int64         // 常用缓冲区的大小(在 Go 标准库中，常常使用的缓冲区大小是 4096 或 8192 字节)
	maxBufferSize   int64         // 最大缓冲区的大小
	maxSliceSize    int64         // 最大切片的大小
	minSliceSize    int64         // 最小切片的大小
	dataShards      int64         // 数据片段的数量
	parityShards    int64         // 奇偶校验片段的数量
	shardSize       int64         // 文件片段的大小
	parityRatio     float64       // 奇偶校验片段占比(根据文件大小计算并向上取整)
	downloadPath    string        // 下载路径
	maxRetries      int64         // 最大重试次数
	retryInterval   time.Duration // 重试间隔
	localStorage    bool          // 是否开启本地存储，上传成功后保留本地文件片段
	routingTableLow int64         // 路由表中连接的最小节点数量
}

// DefaultOptions 设置一个推荐选项列表以获得良好的性能。
func DefaultOptions() *Options {
	return &Options{
		storageMode:     RS_Proportion,            // 纠删码(比例)模式
		defaultBufSize:  1 << 12,                  // 4KB
		maxBufferSize:   1 << 30,                  // 1GB
		maxSliceSize:    1 << 25,                  // 32M
		minSliceSize:    1 << 10,                  // 1KB
		shardSize:       1 << 19,                  // 512KB
		parityRatio:     0.3,                      // 30%
		downloadPath:    GetDefaultDownloadPath(), // 默认下载路径
		maxRetries:      5,                        // 最大重试次数
		retryInterval:   50 * time.Second,         // 重试间隔为50秒
		localStorage:    true,                     // 默认开启本地存储
		routingTableLow: 2,                        // 路由表中连接的最小连接2个节点
	}
}

// BuildShardsOptions 设置奇偶分片大小选项
func (opt *Options) BuildShardsOptions(dataShards, parityShards int64) error {
	if dataShards == 0 {
		return fmt.Errorf("数据片段的数量不可为 %d", dataShards)
	}
	if parityShards == 0 {
		return fmt.Errorf("奇偶校验片段的数量不可为 %d", parityShards)
	}
	// 确保奇偶校验片段不超过数据片段的一半，以防止过多的冗余
	if parityShards > (dataShards / 2) {
		return fmt.Errorf("奇偶校验片段的数量 %d 过大", parityShards)
	}

	// 不可大于50%
	opt.storageMode = RS_Size       // 大小模式
	opt.dataShards = dataShards     // 数据片段的数量
	opt.parityShards = parityShards // 奇偶校验片段的数量

	return nil
}

// GetShardsOptions 获取奇偶分片大小选项
func (opt *Options) GetShardsOptions() (int64, int64, bool) {
	if opt.storageMode == RS_Size {
		return opt.dataShards, opt.parityShards, true
	}
	return 0, 0, false
}

// BuildSizeAndRatioOptions 设置奇偶分片比例选项
// shardSize 以字节为单位
func (opt *Options) BuildSizeAndRatioOptions(shardSize int64, parityRatio float64) error {
	if shardSize == 0 {
		return fmt.Errorf("文件片段的大小不可为 %d", shardSize)
	}
	if shardSize > opt.maxSliceSize {
		return fmt.Errorf("文件片段的大小 %d 不可大于 %d", shardSize, opt.maxSliceSize)
	}
	if shardSize < opt.minSliceSize {
		return fmt.Errorf("文件片段的大小 %d 不可小于 %d", shardSize, opt.minSliceSize)
	}
	if parityRatio == 0 {
		return fmt.Errorf("奇偶校验片段占比不可为 %f", parityRatio)
	}
	// 确保奇偶校验片段不超过数据片段的一半，以防止过多的冗余
	if parityRatio > 0.5 {
		return fmt.Errorf("奇偶校验片段占比 %f 过大", parityRatio)
	}

	opt.storageMode = RS_Size     // 比例模式
	opt.shardSize = shardSize     // 文件片段的大小
	opt.parityRatio = parityRatio // 奇偶校验片段占比

	return nil
}

// GetSizeAndRatioOptions 获取奇偶分片比例选项
func (opt *Options) GetSizeAndRatioOptions() (int64, float64, bool) {
	if opt.storageMode == RS_Size {
		return opt.shardSize, opt.parityRatio, true
	}
	return 0, 0, false
}

// BuildMaxSliceSize 设置最大切片的大小选项
func (opt *Options) BuildMaxSliceSize(maxSliceSize int64) error {
	if maxSliceSize > 1<<25 { // 32M
		return fmt.Errorf("最大切片的大小 %d 不可大于 %d", maxSliceSize, 1<<25)
	}

	opt.maxSliceSize = maxSliceSize

	return nil
}

// BuildMinSliceSize 设置最小切片的大小
func (opt *Options) BuildMinSliceSize(minSliceSize int64) error {
	if minSliceSize < 1<<10 { // 1KB
		return fmt.Errorf("最大切片的大小 %d 不可小于 %d", minSliceSize, 1<<10)
	}

	opt.minSliceSize = minSliceSize

	return nil
}

// BuildLocalStorage 设置是否启动本地存储选项
func (opt *Options) BuildLocalStorage(isEnable bool) {
	opt.localStorage = isEnable
}

// BuildRoutingTableLow 设置路由表中连接的最小节点数量
func (opt *Options) BuildRoutingTableLow(low int64) {
	if low > 0 {
		opt.routingTableLow = low
	}
}

// BuildRootPath 设置文件根路径
func (opt *Options) BuildRootPath(path string) {
	// 检查路径是否为空
	if path == "" {
		return
	}

	// 检查路径是否是一个绝对路径
	if !filepath.IsAbs(path) {
		// 可以返回错误或记录日志
		return
	}

	// 检查路径是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 如果路径不存在，尝试创建它
		if err := os.MkdirAll(path, 0755); err != nil {
			return
		}
	}

	// 设置根路径
	RootPath = path
}

// BuildDownloadPath 设置下载路径
func (opt *Options) BuildDownloadPath(path string) {
	// 检查路径是否为空
	if path == "" {
		return
	}

	// 检查路径是否是一个绝对路径
	if !filepath.IsAbs(path) {
		// 可以返回错误或记录日志
		return
	}

	// 检查路径是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 如果路径不存在，尝试创建它
		if err := os.MkdirAll(path, 0755); err != nil {
			return
		}
	}

	// 设置下载路径
	opt.downloadPath = path
}
