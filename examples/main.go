// Package main 是DeFS系统的主包
package main

// 导入所需的包
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/kbucket"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/uploads"
	"github.com/bpfs/defs/utils/log"
	dht "github.com/dep2p/kaddht"
	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/libp2p/core/network"
	"github.com/dep2p/libp2p/core/peer"
	logging "github.com/dep2p/log"
	"github.com/multiformats/go-multiaddr"
	"github.com/pterm/pterm"
)

var logger = logging.Logger("examples")

// main 函数是程序的入口点
func main() {
	// 确保日志目录存在
	logDir := "../defsdata/logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("创建日志目录失败: %v\n", err)
		os.Exit(1)
	}

	// 设置日志输出到文件
	logFile, err := os.OpenFile(filepath.Join(logDir, "defs.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("创建日志文件失败: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// 设置defs日志输出
	log.SetLog(logFile.Name(), false)
	// 设置订阅日志
	//publog.SetLog(logFile.Name(), true)

	// 初始化DeFS核心
	logger.Info("开始初始化DeFS核心...")
	core, err := NewDefsCore()
	if err != nil {
		fmt.Printf("初始化DeFS核心失败: %v\n", err)
		os.Exit(1)
	}
	logger.Info("DeFS核心初始化成功")

	// 创建一个交互式提示符
	prompt := pterm.DefaultInteractiveTextInput.WithMultiLine(false)

	for {
		// 显示提示符并获取输入
		input, err := prompt.Show("请输入命令 > ")
		if err != nil {
			pterm.Error.Println("读取命令失败:", err)
			continue
		}

		// 处理命令
		args := strings.Fields(input)
		if len(args) == 0 {
			continue
		}

		// 处理退出命令
		if args[0] == "exit" || args[0] == "quit" {
			pterm.Info.Println("正在退出程序...")
			break
		}

		// 处理其他命令
		handleCommand(core, args)
	}
}

// handleCommand 处理用户输入的命令
// 参数:
//   - core: DeFS核心实例
//   - args: 用户输入的命令参数
func handleCommand(core *DefsCore, args []string) {
	if len(args) == 0 {
		return
	}

	fmt.Println() // 添加空行以提高可读性

	// 根据命令类型执行相应操作
	switch args[0] {
	case "help":
		showHelp()
	case "time":
		fmt.Println("当前时间:", time.Now().Format("2006-01-02 15:04:05"))
	case "upload":
		if len(args) < 2 {
			pterm.Error.Println("用法: upload <文件路径>")
			return
		}
		// 将除了第一个参数(命令名)外的所有参数合并为文件路径，并去除引号
		filePath := strings.Join(args[1:], " ")
		filePath = strings.Trim(filePath, `'"`) // 去除两端的引号
		if err := handleUpload(core, filePath); err != nil {
			pterm.Error.Printf("上传失败: %v\n", err)
		}
	case "download":
		if len(args) < 2 {
			pterm.Error.Println("用法: download <文件ID>")
			return
		}
		fileID := strings.Trim(args[1], `'"`) // 同样处理下载ID
		if err := handleDownload(core, fileID); err != nil {
			pterm.Error.Printf("下载失败: %v\n", err)
		}
	case "list":
		if err := handleListPeers(core); err != nil {
			pterm.Error.Printf("列出节点失败: %v\n", err)
		}
	case "addRoutingPeer":
		if len(args) < 2 {
			pterm.Error.Println("用法: addRoutingPeer <节点地址> <运行模式(1:客户端 2:服务端)>")
			return
		}
		// 将完整的参数传递给处理函数
		if err := handleAddRoutingPeer(core, args[1:]...); err != nil {
			pterm.Error.Printf("添加节点失败: %v\n", err)
		}
	case "addPubSubPeer":
		if len(args) < 2 {
			pterm.Error.Println("用法: addPubSubPeer <节点ID>")
			return
		}
		if err := handleAddPubSubPeer(core, args[1]); err != nil {
			pterm.Error.Printf("添加发布订阅节点失败: %v\n", err)
		}
	case "pauseUpload":
		if len(args) < 2 {
			pterm.Error.Println("用法: pauseUpload <taskID>")
			return
		}
		if err := handlePauseUpload(core, args[1]); err != nil {
			pterm.Error.Printf("暂停上传失败: %v\n", err)
		}
	case "resumeUpload":
		if len(args) < 2 {
			pterm.Error.Println("用法: resumeUpload <taskID>")
			return
		}
		if err := handleResumeUpload(core, args[1]); err != nil {
			pterm.Error.Printf("恢复上传失败: %v\n", err)
		}
	case "cancelUpload":
		if len(args) < 2 {
			pterm.Error.Println("用法: cancelUpload <taskID>")
			return
		}
		if err := handleCancelUpload(core, args[1]); err != nil {
			pterm.Error.Printf("取消上传失败: %v\n", err)
		}
	case "pauseDownload":
		if len(args) < 2 {
			pterm.Error.Println("用法: pauseDownload <taskID>")
			return
		}
		if err := handlePauseDownload(core, args[1]); err != nil {
			pterm.Error.Printf("暂停下载失败: %v\n", err)
		}
	case "resumeDownload":
		if len(args) < 2 {
			pterm.Error.Println("用法: resumeDownload <taskID>")
			return
		}
		if err := handleResumeDownload(core, args[1]); err != nil {
			pterm.Error.Printf("恢复下载失败: %v\n", err)
		}
	case "cancelDownload":
		if len(args) < 2 {
			pterm.Error.Println("用法: cancelDownload <taskID>")
			return
		}
		if err := handleCancelDownload(core, args[1]); err != nil {
			pterm.Error.Printf("取消下载失败: %v\n", err)
		}
	case "queryAssets":
		if err := handleQueryFileAssets(core); err != nil {
			pterm.Error.Printf("查询文件资产失败: %v\n", err)
		}
	case "listUploads":
		if err := handleListUploads(core); err != nil {
			pterm.Error.Printf("获取上传文件列表失败: %v\n", err)
		}
	case "triggerUpload":
		if len(args) < 2 {
			pterm.Error.Println("用法: triggerUpload <taskID>")
			return
		}
		if err := handleTriggerUpload(core, args[1]); err != nil {
			pterm.Error.Printf("触发上传失败: %v\n", err)
		}
	case "listDownloads":
		if err := handleListDownloads(core); err != nil {
			pterm.Error.Printf("获取下载文件列表失败: %v\n", err)
		}
	case "size":
		if len(args) > 1 {
			// 将完整的参数传递给处理函数
			if err := handleSize(core, args[1:]...); err != nil {
				pterm.Error.Printf("获取节点数量失败: %v\n", err)
			}
		} else {
			if err := handleSize(core); err != nil {
				pterm.Error.Printf("获取节点数量失败: %v\n", err)
			}
		}
	case "listPeers":
		if len(args) > 1 {
			// 将完整的参数传递给处理函数
			if err := handleListPeers(core, args[1:]...); err != nil {
				pterm.Error.Printf("列出节点失败: %v\n", err)
			}
		} else {
			if err := handleListPeers(core); err != nil {
				pterm.Error.Printf("列出节点失败: %v\n", err)
			}
		}
	case "printPeers":
		if len(args) > 1 {
			// 将完整的参数传递给处理函数
			if err := handlePrintPeers(core, args[1:]...); err != nil {
				pterm.Error.Printf("打印节点信息失败: %v\n", err)
			}
		} else {
			if err := handlePrintPeers(core); err != nil {
				pterm.Error.Printf("打印节点信息失败: %v\n", err)
			}
		}
	case "nearestPeers":
		if len(args) < 2 {
			pterm.Error.Println("用法: nearestPeers <目标节点ID> [mode] [count]")
			return
		}
		// 将完整的参数传递给处理函数
		if err := handleNearestPeers(core, args[1:]...); err != nil {
			pterm.Error.Printf("查找最近节点失败: %v\n", err)
		}
	case "id":
		if err := handleShowNodeID(core); err != nil {
			pterm.Error.Printf("获取节点ID失败: %v\n", err)
		}
	case "addr":
		if err := handleShowNodeAddr(core); err != nil {
			pterm.Error.Printf("获取节点地址失败: %v\n", err)
		}
	default:
		pterm.Warning.Printf("未知命令: %s\n", args[0])
		pterm.Info.Println("输入 'help' 获取可用命令列表")
	}
}

// showHelp 显示帮助信息
func showHelp() {
	logger.Info("显示帮助信息")

	// 创建标题
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).
		Println("DeFS 命令行界面帮助")

	// 基础命令
	pterm.DefaultSection.Println("基础命令")
	pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
		{Level: 0, Text: "help: 显示帮助信息"},
		{Level: 0, Text: "time: 显示当前时间"},
		{Level: 0, Text: "id: 显示当前节点ID"},
		{Level: 0, Text: "addr: 显示当前节点完整地址"},
	}).Render()

	// 节点管理
	pterm.DefaultSection.Println("节点管理")
	pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
		{Level: 0, Text: "list: 列出所有节点"},
		{Level: 0, Text: "addRoutingPeer <节点地址> <运行模式(1:客户端 2:服务端)>: 添加路由表节点"},
		{Level: 0, Text: "addPubSubPeer <节点ID>: 添加发布订阅节点"},
		{Level: 0, Text: "size [mode]: 显示节点数量 (mode: 1:客户端 2:服务端, 可选)"},
		{Level: 0, Text: "listPeers [mode]: 列出节点 (mode: 1:客户端 2:服务端, 可选)"},
		{Level: 0, Text: "printPeers [mode]: 打印节点详细信息 (mode: 1:客户端 2:服务端, 可选)"},
		{Level: 0, Text: "nearestPeers <目标节点ID> [mode] [count]: 查找最近的节点"},
	}).Render()

	// 上传管理
	pterm.DefaultSection.Println("上传管理")
	pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
		{Level: 0, Text: "upload <文件路径>: 上传文件"},
		{Level: 0, Text: "triggerUpload <taskID>: 触发指定任务的上传"},
		{Level: 0, Text: "pauseUpload <taskID>: 暂停上传任务"},
		{Level: 0, Text: "resumeUpload <taskID>: 恢复上传任务"},
		{Level: 0, Text: "cancelUpload <taskID>: 取消上传任务"},
		{Level: 0, Text: "queryAssets: 查询所有文件资产"},
		{Level: 0, Text: "listUploads: 显示所有上传任务"},
	}).Render()

	// 下载管理
	pterm.DefaultSection.Println("下载管理")
	pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
		{Level: 0, Text: "download <文件ID>: 下载文件"},
		{Level: 0, Text: "pauseDownload <taskID>: 暂停下载任务"},
		{Level: 0, Text: "resumeDownload <taskID>: 恢复下载任务"},
		{Level: 0, Text: "cancelDownload <taskID>: 取消下载任务"},
		{Level: 0, Text: "queryDownloads: 查询所有下载任务"},
		{Level: 0, Text: "listDownloads: 显示所有下载任务"},
	}).Render()

	// 退出
	pterm.DefaultSection.Println("退出")
	pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
		{Level: 0, Text: "exit/quit: 退出程序"},
	}).Render()
}

// handleUpload 处理文件上传
// 参数:
//   - core: DeFS核心实例
//   - filePath: 要上传的文件路径
//
// 返回:
//   - error: 错误信息
func handleUpload(core *DefsCore, filePath string) error {
	logger.Infof("开始上传文件: %s", filePath)
	// 创建上传任务
	file, err := core.fs.Upload().NewUpload(filePath, core.privateKey, true)
	if err != nil {
		logger.Errorf("创建上传任务失败: %v", err)
		return fmt.Errorf("创建上传任务失败: %v", err)
	}

	logger.Infof("文件上传成功，FileID: %s", file.FileId)
	return nil
}

// handleDownload 处理文件下载
// 参数:
//   - core: DeFS核心实例
//   - fileID: 要下载的文件ID
//
// 返回:
//   - error: 错误信息
func handleDownload(core *DefsCore, fileID string) error {
	taskID, err := core.fs.Download().NewDownload(core.privateKey, fileID)
	if err != nil {
		return fmt.Errorf("创建下载任务失败: %v", err)
	}

	// 动下载进度监控
	go monitorDownloadProgress(core.fs.DB().BadgerDB, taskID)
	logger.Infof("下载任务已创建，TaskID: %s", taskID)
	return nil
}

// handleAddRoutingPeer 处理添加新节点
// 参数:
//   - core: DeFS核心实例
//   - args: 要添加的节点地址和运行模式
//
// 返回:
//   - error: 错误信息
func handleAddRoutingPeer(core *DefsCore, args ...string) error {
	// 检查参数数量
	if len(args) < 2 {
		return fmt.Errorf("用法: addRoutingPeer <节点地址> <运行模式(1:客户端 2:服务端)>")
	}

	// 解析节点地址
	maddr, err := multiaddr.NewMultiaddr(args[0])
	if err != nil {
		logger.Errorf("解析节点地址失败: %v", err)
		return err
	}

	// 解运行模式
	mode, err := strconv.Atoi(args[1])
	if err != nil || (mode != 1 && mode != 2) {
		return fmt.Errorf("运行模式必须为 1(客户端) 或 2(服务端)")
	}

	// 转换为节点信息
	addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		logger.Errorf("转换节点地址失败: %v", err)
		return err
	}

	// 添加节点
	success, err := core.fs.AddRoutingPeer(*addrInfo, mode)
	if err != nil {
		logger.Errorf("添加节点失败: %v", err)
		return err
	}

	if success {
		logger.Infof("成功添加节点: %s, 运行模式: %d", addrInfo.ID, mode)
	} else {
		logger.Warnf("节点已存在或无法添加: %s", addrInfo.ID)
	}
	return nil
}

// handleAddPubSubPeer 处理添加新的发布订阅节点
// 参数:
//   - core: DeFS核心实例,包含文件系统和网络功能
//   - peerID: 要添加的节点ID字符串
//
// 返回:
//   - error: 如果添加失败则返回错误信息,成功则返回nil
func handleAddPubSubPeer(core *DefsCore, peerID string) error {
	// 将字符串格式的节点ID解析为peer.ID类型
	pid, err := peer.Decode(peerID)
	if err != nil {
		logger.Errorf("解析节点ID失败: %v", err)
		return err
	}

	// 调用DeFS的AddPubSubPeer方法
	if err := core.fs.AddPubSubPeer(pid); err != nil {
		logger.Errorf("添加发布订阅节点失败: %v", err)
		return err
	}

	logger.Infof("成功添加发布订阅节点: %s", pid)
	return nil
}

// monitorDownloadProgress 监控下载进度
// 参数:
//   - db: BadgerDB实例
//   - taskId: 下载任务ID
func monitorDownloadProgress(db *badgerhold.Store, taskId string) {
	// 创建下载段存储
	store := database.NewDownloadSegmentStore(db)
	// 创建定时器,每5秒检查一次进度
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// 创建超时上下文,30分钟超时
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	logger.Infof("\n开始监控下载进度 - TaskId: %s", taskId)

	// 循环检查下载进度
	for {
		select {
		case <-ctx.Done():
			// 超时退出
			logger.Infof("\n下载监控已结束 - TaskId: %s", taskId)
			return
		case <-ticker.C:
			// 获取下载进度
			_, completedIndices, dataSegmentCount, err := store.TaskByFileID(taskId)
			if err != nil {
				logger.Errorf("获取下载进度失败: TaskId=%s, Error=%v", taskId, err)
				continue
			}

			// 计算并显示完成率
			completionRate := float64(len(completedIndices)) / float64(dataSegmentCount) * 100
			logger.Infof("\r下载进度: %.2f%% (%d/%d) ", completionRate, len(completedIndices), dataSegmentCount)

			// 检查是否下载完成
			if len(completedIndices) == dataSegmentCount {
				logger.Infof("\n文件下载完成 - TaskId: %s", taskId)
				return
			}
		}
	}
}

// connectToBootstrapPeers 连接至自配引导节点
// 参数:
//   - ctx: context.Context 上下文
//   - host: host.Host libp2p主机实例
//   - bootstrapPeers: []string 引导节点地址列表
//
// 返回值:
//   - error: 如果生错误返回error,否则返回nil
func connectToBootstrapPeers(ctx context.Context, host host.Host, bootstrapPeers []string) error {
	var defaultMultiaddrs []multiaddr.Multiaddr

	// 解析引导节点的 Multiaddr
	for _, peer := range bootstrapPeers {
		maddr, err := multiaddr.NewMultiaddr(peer)
		if err == nil {
			defaultMultiaddrs = append(defaultMultiaddrs, maddr)
		}
	}

	// 获取默认的引导节点
	defaultBootstrapPeers := dht.DefaultBootstrapPeers
	defaultBootstrapPeers = append(defaultBootstrapPeers, defaultMultiaddrs...)

	var wg sync.WaitGroup
	successfulConnection := false

	// 并发连接所有引导节点
	for _, peerAddr := range defaultBootstrapPeers {
		// 将 Multiaddr 转换为 AddrInfo
		peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			logger.Errorf("地址 %s 转换失败: %v", peerAddr.String(), err)
			continue
		}

		wg.Add(1)
		go func(peerInfo peer.AddrInfo) {
			defer wg.Done()
			// 尝试连接引导节点
			if err := host.Connect(ctx, peerInfo); err != nil {
				logger.Debugf("连接引导节点警告: %v", err)
			} else {
				logger.Infof("连接引导节点成功: %s", peerInfo.ID)
				successfulConnection = true
			}
		}(*peerInfo)
	}

	wg.Wait() // 等待所有连接试完成

	if !successfulConnection {
		return fmt.Errorf("未能连接至引导节点")
	}

	return nil
}

// parseCommandLine 解析命令行输入，正确处理带引号和空格的参数
// 参数:
//   - cmd: 要解析的命令行字符串
//
// 返回值:
//   - []string: 解析后参数列表
// func parseCommandLine(cmd string) []string {
// 	// 存储解析后的参数
// 	var args []string
// 	// 用于构建当前参数
// 	var currentArg strings.Builder
// 	// 标记是否在引号内
// 	inQuotes := false
// 	// 当前使用的引号字符
// 	var quoteChar rune

// 	// 历命令行字符串的每个字符
// 	for _, char := range cmd {
// 		switch char {
// 		case '"', '\'':
// 			if inQuotes && char == quoteChar {
// 				// 如果在引号内且遇到相同的引号字符，结束引号
// 				inQuotes = false
// 				if currentArg.Len() > 0 {
// 					args = append(args, currentArg.String())
// 					currentArg.Reset()
// 				}
// 			} else if !inQuotes {
// 				// 如果不在引号内，开始新的引号
// 				inQuotes = true
// 				quoteChar = char
// 			} else {
// 				// 如果在其他���型的引号内，当作普通字符处理
// 				currentArg.WriteRune(char)
// 			}
// 		case ' ':
// 			if inQuotes {
// 				// 如果在引号内，空格作为参数的一部分
// 				currentArg.WriteRune(char)
// 			} else if currentArg.Len() > 0 {
// 				// 如果不在引号内且当前参数不为空，结束当前参数
// 				args = append(args, currentArg.String())
// 				currentArg.Reset()
// 			}
// 		default:
// 			// 其他字符直接添加到当前参数
// 			currentArg.WriteRune(char)
// 		}
// 	}

// 	// 处理最后一个参数
// 	if currentArg.Len() > 0 {
// 		args = append(args, currentArg.String())
// 	}

// 	// 去除参数两端的空格和引号
// 	for i, arg := range args {
// 		args[i] = strings.Trim(arg, "\" '\t")
// 	}

// 	return args
// }

// handlePauseUpload 处理暂停上传任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要暂停的上传任务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handlePauseUpload(core *DefsCore, taskID string) error {
	// 调用文件系统的强制暂停上传方法
	if err := core.fs.Upload().PauseUpload(taskID); err != nil {
		logger.Errorf("暂停上传失败: %v", err)
		return err
	}
	fmt.Printf("成功暂停上传任务: %s\n", taskID)
	return nil
}

// handleResumeUpload 处理恢复上传任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要恢复的上传任务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleResumeUpload(core *DefsCore, taskID string) error {
	// 调用文件系统的强制恢复上传方法
	if err := core.fs.Upload().ResumeUpload(taskID); err != nil {
		logger.Errorf("恢复上传失败: %v", err)
		return err
	}
	fmt.Printf("成功恢复上传任务: %s\n", taskID)
	return nil
}

// handleCancelUpload 处理取消上传任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要取消的上传任务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleCancelUpload(core *DefsCore, taskID string) error {
	// 调用文件系统的取消上传方法
	if err := core.fs.Upload().CancelUpload(taskID); err != nil {
		logger.Errorf("取消上传失败: %v", err)
		return err
	}
	fmt.Printf("成功取消上传任务: %s\n", taskID)
	return nil
}

// handlePauseDownload 处理暂停下载任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要暂停的下载任务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handlePauseDownload(core *DefsCore, taskID string) error {
	// 调用文件系统的暂停下载方法
	if err := core.fs.Download().PauseDownload(taskID); err != nil {
		logger.Errorf("暂停下载失败: %v", err)
		return err
	}
	fmt.Printf("成功暂停下载任务: %s\n", taskID)
	return nil
}

// handleResumeDownload 处理恢复下载任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要恢复的下载任务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleResumeDownload(core *DefsCore, taskID string) error {
	// 调用文件系统的恢复下载方法
	if err := core.fs.Download().ResumeDownload(taskID); err != nil {
		logger.Errorf("恢复下载失败: %v", err)
		return err
	}
	fmt.Printf("成功恢复下载任务: %s\n", taskID)
	return nil
}

// handleCancelDownload 处理取消下载任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要取消的下载任务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleCancelDownload(core *DefsCore, taskID string) error {
	// 调用文件系统的取消下载方法
	if err := core.fs.Download().CancelDownload(taskID); err != nil {
		logger.Errorf("取消下载失败: %v", err)
		return err
	}
	fmt.Printf("成功取消下载任务: %s\n", taskID)
	return nil
}

// handleQueryFileAssets 处理查询文件资产的请求
// 参数:
//   - core: DeFS核心实例
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleQueryFileAssets(core *DefsCore) error {
	logger.Info("开始查询文件资产")
	// 查询文件资产列表
	assets, totalCount, currentPage, pageSize, err := uploads.QueryFileAssets(
		core.fs.DB().BadgerDB, // 数据库实例
		nil,                   // pubkeyHash，如果不需要可以传 nil
		0,                     // start
		10,                    // pageSize
		"",                    // query
	)
	if err != nil {
		logger.Errorf("查询文件资产失败: %v", err)
		return err
	}

	// 打印查询结果
	fmt.Println("\n文件资产列表:")
	fmt.Printf("总记录数: %d, 当前页: %d, 每页记录数: %d\n", totalCount, currentPage, pageSize)
	for i, asset := range assets {
		fmt.Printf("\n资产 #%d:\n", i+1)
		fmt.Printf("文件ID: %s\n", asset.FileId)
		fmt.Printf("文件名: %s\n", asset.Name)
		fmt.Printf("文件大小: %d 字节\n", asset.Size())
		fmt.Printf("文件类型: %s\n", asset.ContentType)
		fmt.Printf("扩展名: %s\n", asset.Extension)
		fmt.Printf("标签: %s\n", asset.Labels)
		fmt.Printf("类型: %s\n", getFileType(asset.Type))
		fmt.Printf("是否共享: %v\n", asset.IsShared)
		if asset.IsShared {
			fmt.Printf("共金额: %.2f\n", asset.ShareAmount)
		}
		fmt.Printf("上传时间: %s\n", time.Unix(asset.UploadTime, 0).Format("2006-01-02 15:04:05"))
		fmt.Printf("修改时间: %s\n", time.Unix(asset.ModTime, 0).Format("2006-01-02 15:04:05"))
		fmt.Printf("------------------------\n")
	}

	logger.Infof("成功查询到 %d 个文件资产", len(assets))
	return nil
}

// getFileType 根据类型值返回文件类型描述
// 参数:
//   - fileType: 文件类型的数值表示
//
// 返回值:
//   - string: 文件类型的文字描述
func getFileType(fileType int64) string {
	switch fileType {
	case 0:
		return "文件"
	case 1:
		return "文件夹"
	default:
		return "未知类型"
	}
}

// handleListUploads 处理列出所有上传任务的请求
// 参数:
//   - core: DeFS核心实例
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleListUploads(core *DefsCore) error {
	logger.Info("开始获取上传文件列表")
	// 获取所有上传文件的摘要信息
	summaries, err := core.fs.Upload().GetAllUploadFilesSummaries()
	if err != nil {
		logger.Errorf("获取上传文件列表失败: %v", err)
		return err
	}

	// 打印上传文件列表
	fmt.Println("\n上传文件列表:")
	for i, summary := range summaries {
		fmt.Printf("\n文件 #%d:\n", i+1)
		fmt.Printf("任务ID: %s\n", summary.TaskId)
		fmt.Printf("文件名: %s\n", summary.Name)
		fmt.Printf("大小: %d 字节\n", summary.TotalSize)
		fmt.Printf("状态: %s\n", summary.UploadStatus)
		fmt.Printf("上传进度: %.2f%%\n", float64(summary.Progress))
		fmt.Printf("------------------------\n")
	}

	logger.Infof("成功获取到 %d 个上传文件", len(summaries))
	return nil
}

// handleTriggerUpload 处理触发上传任务的请求
// 参数:
//   - core: DeFS核心实例
//   - taskID: 要触发的上传���务ID
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleTriggerUpload(core *DefsCore, taskID string) error {
	logger.Infof("开始触发上传任务: %s", taskID)
	// 调用文件系统的触发上传方法
	if err := core.fs.Upload().TriggerUpload(taskID); err != nil {
		logger.Errorf("触发上传失败: %v", err)
		return err
	}

	fmt.Printf("成功触发上传任务: %s\n", taskID)
	return nil
}

// getDownloadStatusText 将下载状态枚举值转换为可读文本
// 参数:
//   - status: 下载状态的枚举值
//
// 返回值:
//   - string: 下载状态的文字描述
func getDownloadStatusText(status pb.DownloadStatus) string {
	switch status {
	case pb.DownloadStatus_DOWNLOAD_STATUS_UNSPECIFIED:
		return "未指定"
	case pb.DownloadStatus_DOWNLOAD_STATUS_FETCHING_INFO:
		return "获取文件信息中"
	case pb.DownloadStatus_DOWNLOAD_STATUS_PENDING:
		return "待下载"
	case pb.DownloadStatus_DOWNLOAD_STATUS_DOWNLOADING:
		return "下载中"
	case pb.DownloadStatus_DOWNLOAD_STATUS_PAUSED:
		return "已暂停"
	case pb.DownloadStatus_DOWNLOAD_STATUS_COMPLETED:
		return "已完成"
	case pb.DownloadStatus_DOWNLOAD_STATUS_FAILED:
		return "下载失败"
	case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELLED:
		return "已取消"
	default:
		return "未知状态"
	}
}

// handleListDownloads 处理列出所有下载任务的请求
// 参数:
//   - core: DeFS核心实例
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleListDownloads(core *DefsCore) error {
	logger.Info("开始获取下载文件列表")
	// 查询下载记录
	records, totalCount, currentPage, pageSize, err := core.fs.Download().QueryDownload(
		0,  // start
		10, // pageSize
	)
	if err != nil {
		logger.Errorf("获取下载文件列表失败: %v", err)
		return err
	}

	// 打印下载文件列表
	fmt.Println("\n下载文件列表:")
	fmt.Printf("总记录数: %d, 当前页: %d, 每页记录数: %d\n", totalCount, currentPage, pageSize)
	for i, record := range records {
		fmt.Printf("\n文件 #%d:\n", i+1)
		fmt.Printf("任务ID: %s\n", record.TaskId)
		fmt.Printf("文件ID: %s\n", record.FileId)
		fmt.Printf("文件名: %s\n", record.FileMeta.Name)
		fmt.Printf("文件大小: %d 字节\n", record.FileMeta.Size())
		fmt.Printf("状态: %s\n", getDownloadStatusText(record.Status))
		if record.StartedAt > 0 {
			fmt.Printf("开始时间: %s\n", time.Unix(record.StartedAt, 0).Format("2006-01-02 15:04:05"))
		}
		if record.FinishedAt > 0 {
			fmt.Printf("完成时间: %s\n", time.Unix(record.FinishedAt, 0).Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("------------------------\n")
	}

	logger.Infof("成功获取到 %d 个下载文件", len(records))
	return nil
}

// handleSize 处理获取节点数量的请求
// 参数:
//   - core: DeFS核心实例
//   - args: 可选的运行模式过滤器
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleSize(core *DefsCore, args ...string) error {
	if len(args) > 0 {
		// 解析运行模式
		mode, err := strconv.Atoi(args[0])
		if err != nil || (mode != 1 && mode != 2) {
			return fmt.Errorf("运行模式必须为 1(客户端) 或 2(服务端)")
		}
		size := core.fs.RoutingTable().Size(mode)
		fmt.Printf("节点数量 (mode=%d): %d\n", mode, size)
	} else {
		size := core.fs.RoutingTable().Size()
		fmt.Printf("总节点数量: %d\n", size)
	}
	return nil
}

// handleListPeers 处理列出所有节点的请求
// 参数:
//   - core: DeFS核心实例
//   - args: 可选的运行模式过滤器
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleListPeers(core *DefsCore, args ...string) error {
	var peers []peer.ID
	if len(args) > 0 {
		// 解析运行模式
		mode, err := strconv.Atoi(args[0])
		if err != nil || (mode != 1 && mode != 2) {
			return fmt.Errorf("运行模式必须为 1(客户端) 或 2(服务端)")
		}
		peers = core.fs.RoutingTable().ListPeers(mode)
		logger.Infof("获取到 %d 个节点 (mode=%d)", len(peers), mode)
	} else {
		peers = core.fs.RoutingTable().ListPeers()
		logger.Infof("获取到 %d 个节点", len(peers))
	}

	// 遍历并显示每个节点的信息
	for i, p := range peers {
		addrs := core.fs.Host().Peerstore().Addrs(p)
		connected := core.fs.Host().Network().Connectedness(p) == network.Connected
		protocols, _ := core.fs.Host().Peerstore().GetProtocols(p)

		fmt.Printf("\n节点 #%d:\n"+
			"ID: %s\n"+
			"地址: %v\n"+
			"连接状态: %v\n"+
			"支持协议: %v\n"+
			"------------------------\n",
			i+1, p, addrs, connected, protocols)
	}
	return nil
}

// handlePrintPeers 处理打印节点详细信息的请求
// 参数:
//   - core: DeFS核心实例
//   - args: 可选的运行模式过滤器
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handlePrintPeers(core *DefsCore, args ...string) error {
	if len(args) > 0 {
		// 解析运行模式
		mode, err := strconv.Atoi(args[0])
		if err != nil || (mode != 1 && mode != 2) {
			return fmt.Errorf("运行模式必须为 1(客户端) 或 2(服务端)")
		}
		core.fs.RoutingTable().Print(mode)
	} else {
		core.fs.RoutingTable().Print()
	}
	return nil
}

// handleNearestPeers 处理查找最近节点的请求
// 参数:
//   - core: DeFS核心实例
//   - args: 要查找的目标节点ID和运行模式
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleNearestPeers(core *DefsCore, args ...string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: nearestPeers <目标节点ID> [mode] [count]")
	}

	// 解析目标节点ID
	targetID, err := peer.Decode(args[0])
	if err != nil {
		return fmt.Errorf("解析目标节点ID失败: %v", err)
	}

	// 默认返回10个节点
	count := 10
	var mode int

	// 解析可选参数
	if len(args) > 1 {
		// 解析运行模式
		mode, err = strconv.Atoi(args[1])
		if err != nil || (mode != 1 && mode != 2) {
			return fmt.Errorf("运行模式必须为 1(客户端) 或 2(服务端)")
		}
	}

	// 解析返回数量
	if len(args) > 2 {
		count, err = strconv.Atoi(args[2])
		if err != nil || count <= 0 {
			return fmt.Errorf("返回数量必须为正整数")
		}
	}

	// 查找最近的节点
	var peers []peer.ID
	if mode == 1 || mode == 2 {
		peers = core.fs.RoutingTable().NearestPeers(kbucket.ConvertKey(targetID.String()), count, mode)
		logger.Infof("找到 %d 个最近的节点 (mode=%d)", len(peers), mode)
	} else {
		peers = core.fs.RoutingTable().NearestPeers(kbucket.ConvertKey(targetID.String()), count)
		logger.Infof("找到 %d 个最近的节点", len(peers))
	}

	// 显示节点信息
	fmt.Printf("\n最近的节点列表 (目标节点: %s):\n", targetID)
	for i, p := range peers {
		addrs := core.fs.Host().Peerstore().Addrs(p)
		connected := core.fs.Host().Network().Connectedness(p) == network.Connected
		protocols, _ := core.fs.Host().Peerstore().GetProtocols(p)

		fmt.Printf("\n节点 #%d:\n"+
			"ID: %s\n"+
			"地址: %v\n"+
			"连接状态: %v\n"+
			"支持协议: %v\n"+
			"------------------------\n",
			i+1, p, addrs, connected, protocols)
	}

	return nil
}

// handleShowNodeID 处理显示当前节点ID的请求
// 参数:
//   - core: DeFS核心实例
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleShowNodeID(core *DefsCore) error {
	// 获取本地节点ID
	nodeID := core.fs.Host().ID()
	fmt.Printf("当前节点ID: %s\n", nodeID.String())
	fmt.Printf("(可用于 addPubSubPeer 命令)\n")
	return nil
}

// handleShowNodeAddr 处理显示当前节点完整地址的请求
// 参数:
//   - core: DeFS核心实例
//
// 返回值:
//   - error: 操作过程中的错误，如果成功则为nil
func handleShowNodeAddr(core *DefsCore) error {
	// 获取本地节点的所有地址
	addrs := core.fs.Host().Addrs()
	nodeID := core.fs.Host().ID()

	fmt.Println("当前节点地址:")
	for _, addr := range addrs {
		// 将地址和节点ID组合成完整的多地址
		fullAddr := addr.String() + "/p2p/" + nodeID.String()
		fmt.Printf("%s\n", fullAddr)
		fmt.Printf("(可用于 addRoutingPeer 命令)\n")
	}
	return nil
}
