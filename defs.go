package defs

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/downloads"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/uploads"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// FS 是一个封装了DeFS去中心化(动态)存储的结构体
type FS struct {
	ctx          context.Context              // 全局上下文
	opt          *opts.Options                // 文件存储选项配置
	afe          afero.Afero                  // 文件系统接口
	p2p          *dep2p.DeP2P                 // 网络主机
	pub          *pubsub.DeP2PPubSub          // 网络订阅
	upload       *uploads.UploadManager       // 管理上传会话
	uploadChan   chan *uploads.UploadChan     // 上传对外通道
	download     *downloads.DownloadManager   // 管理下载任务
	downloadChan chan *downloads.DownloadChan // 下载对外通道
}

// Open 返回一个新的文件存储对象
func Open(opt *opts.Options, p2p *dep2p.DeP2P, pub *pubsub.DeP2PPubSub) (*FS, error) {
	if err := checkAndSetOptions(); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	afe, err := paths.InitDirectories(opt.GetRootPath())
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	ctx := context.Background()
	fs := &FS{
		ctx: ctx,
		opt: opt,
		afe: afe,
		p2p: p2p,
		pub: pub,
	}

	// fx 配置项
	opts := []fx.Option{
		globalInit(fs),
		fx.Provide(
			uploads.NewUploadManager,     // 管理所有上传会话
			downloads.NewDownloadManager, // 管理所有下载会话
			// 管理所有片段会话
		),
		fx.Invoke(
			uploads.RegisterUploadStreamProtocol,     // 注册上传流
			downloads.RegisterPubsubProtocol,         // 注册下载订阅
			downloads.RegisterDownloadStreamProtocol, // 注册下载流
		),
	}
	opts = append(opts, fx.Populate(
		&fs.ctx,
		&fs.opt,
		&fs.afe,
		&fs.p2p,
		&fs.pub,
		&fs.upload,
		&fs.uploadChan,
		&fs.download,
		&fs.downloadChan,
	))
	app := fx.New(opts...)

	// 启动所有长时间运行的 goroutine，例如网络服务器或消息队列消费者。
	return fs, app.Start(fs.ctx)
}

// checkAndSetOptions 检查并设置选项
func checkAndSetOptions() error {
	return nil
}

// globalInit 全局初始化
func globalInit(fs *FS) fx.Option {
	return fx.Provide(
		// 获取上下文
		func(lc fx.Lifecycle) context.Context {
			lc.Append(fx.Hook{
				OnStop: func(_ context.Context) error {
					return nil
				},
			})
			return fs.ctx
		},
		func() *opts.Options {
			return fs.opt
		},
		func() afero.Afero {
			return fs.afe
		},
		func() *dep2p.DeP2P {
			return fs.p2p
		},
		func() *pubsub.DeP2PPubSub {
			return fs.pub
		},
		func() chan *uploads.UploadChan {
			return make(chan *uploads.UploadChan)
		},
		func() chan *downloads.DownloadChan {
			return make(chan *downloads.DownloadChan)
		},
	)
}

// Ctx 获取全局上下文
func (fs *FS) Ctx() context.Context {
	return fs.ctx
}

// Opt 获取文件存储选项配置
func (fs *FS) Opt() *opts.Options {
	return fs.opt
}

// Afero 获取文件系统接口
func (fs *FS) Afero() afero.Afero {
	return fs.afe
}

// P2P 获取DeP2P网络主机
func (fs *FS) P2P() *dep2p.DeP2P {
	return fs.p2p
}

// Pubsub 获取DeP2P网络订阅
func (fs *FS) Pubsub() *pubsub.DeP2PPubSub {
	return fs.pub
}

// Registry 获取事件总线
// func (fs *FS) Registry() *eventbus.EventRegistry {
// 	return fs.registry
// }

// UploadChan 上传对外通道
func (fs *FS) UploadChan() chan *uploads.UploadChan {
	return fs.uploadChan
}

// DownloadChan 下载对外通道
func (fs *FS) DownloadChan() chan *downloads.DownloadChan {
	return fs.downloadChan
}

// Upload 管理所有上传会话
func (fs *FS) Upload() *uploads.UploadManager {
	return fs.upload
}

// Download 管理所有下载会话
func (fs *FS) Download() *downloads.DownloadManager {
	return fs.download
}

// Cache 获取缓存实例
// func (fs *FS) Cache() *ristretto.Cache {
// 	return fs.cache
// }

///////////

// WriteReader 将读取器的内容写入指定路径
// 参数：
//   - path: string 文件路径
//   - r: io.Reader 读取器
//
// 返回值：
//   - error: 错误信息
func (fs FS) WriteReader(path string, r io.Reader) (err error) {
	return afero.WriteReader(fs.afe, path, r)
}

// SafeWriteReader 将读取器的内容安全地写入指定路径
// 参数：
//   - path: string 文件路径
//   - r: io.Reader 读取器
//
// 返回值：
//   - error: 错误信息
func (fs FS) SafeWriteReader(path string, r io.Reader) (err error) {
	return afero.SafeWriteReader(fs.afe, path, r)
}

// GetTempDir 获取临时目录路径
// 参数：
//   - subPath: string 子路径
//
// 返回值：
//   - string: 临时目录路径
func (fs FS) GetTempDir(subPath string) string {
	return afero.GetTempDir(fs.afe, subPath)
}

// FileContainsBytes 检查文件是否包含指定的字节切片
// 参数：
//   - filename: string 文件名
//   - subslice: []byte 字节切片
//
// 返回值：
//   - bool: 是否包含
//   - error: 错误信息
func (fs FS) FileContainsBytes(filename string, subslice []byte) (bool, error) {
	return afero.FileContainsBytes(fs.afe, filename, subslice)
}

// FileContainsAnyBytes 检查文件是否包含任意一个指定的字节切片
// 参数：
//   - filename: string 文件名
//   - subslices: [][]byte 字节切片数组
//
// 返回值：
//   - bool: 是否包含
//   - error: 错误信息
func (fs FS) FileContainsAnyBytes(filename string, subslices [][]byte) (bool, error) {
	return afero.FileContainsAnyBytes(fs.afe, filename, subslices)
}

// DirExists 检查路径是否存在并且是一个目录。
// 参数：
//   - path: string 路径
//
// 返回值：
//   - bool: 是否存在并且是目录
//   - error: 错误信息
func (fs FS) DirExists(path string) (bool, error) {
	return afero.DirExists(fs.afe, path)
}

// IsDir 检查给定路径是否是目录。
// 参数：
//   - path: string 路径
//
// 返回值：
//   - bool: 是否是目录
//   - error: 错误信息
func (fs FS) IsDir(path string) (bool, error) {
	return afero.IsDir(fs.afe, path)
}

// IsEmpty 检查给定文件或目录是否为空。
// 参数：
//   - path: string 路径
//
// 返回值：
//   - bool: 是否为空
//   - error: 错误信息
func (fs FS) IsEmpty(path string) (bool, error) {
	return afero.IsEmpty(fs.afe, path)
}

// Exists 检查文件或目录是否存在。
// 参数：
//   - path: string 路径
//
// 返回值：
//   - bool: 是否存在
//   - error: 错误信息
func (fs FS) Exists(path string) (bool, error) {
	return afero.Exists(fs.afe, path)
}

// ReadDir 读取指定目录的内容并返回排序后的目录条目列表
// 参数：
//   - dirname: string 目录名
//
// 返回值：
//   - []os.FileInfo: 目录条目列表
//   - error: 错误信息
func (fs FS) ReadDir(dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(fs.afe, dirname) // 调用 ReadDir 函数
}

// ReadFile 读取指定文件的内容并返回
// 参数：
//   - filename: string 文件名
//
// 返回值：
//   - []byte: 文件内容
//   - error: 错误信息
func (fs FS) ReadFile(filename string) ([]byte, error) {
	return afero.ReadFile(fs.afe, filename) // 调用 ReadFile 函数
}

// WriteFile 将数据写入指定名称的文件
// 参数：
//   - filename: string 文件名
//   - data: []byte 数据
//   - perm: os.FileMode 文件权限
//
// 返回值：
//   - error: 错误信息
func (fs FS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(fs.afe, filename, data, perm) // 调用 WriteFile 函数
}

// TempFile 在指定目录中创建一个新的临时文件
// 参数：
//   - dir: string 目录路径
//   - pattern: string 文件名模式
//
// 返回值：
//   - File: 临时文件对象
//   - error: 错误信息
func (fs FS) TempFile(dir, pattern string) (f afero.File, err error) {
	return afero.TempFile(fs.afe, dir, pattern) // 调用 TempFile 函数
}

// TempDir 在指定目录中创建一个新的临时目录
// 参数：
//   - dir: string 目录路径
//   - prefix: string 目录名前缀
//
// 返回值：
//   - string: 目录路径
//   - error: 错误信息
func (fs FS) TempDir(dir, prefix string) (name string, err error) {
	return afero.TempDir(fs.afe, dir, prefix) // 调用 TempDir 函数
}

// Walk 遍历根目录为 root 的文件树，调用 walkFn 函数处理树中的每个文件或目录，包括根目录。
// 参数：
//   - root: string 根目录
//   - walkFn: filepath.WalkFunc 处理函数
//
// 返回值：
//   - error: 错误信息
func (fs FS) Walk(root string, walkFn filepath.WalkFunc) error {
	return afero.Walk(fs.afe, root, walkFn) // 调用 Walk 函数
}
