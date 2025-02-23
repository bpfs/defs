# DeFS 示例程序

## 准备工作

1. 进入示例目录：

```bash
cd examples
```

2. 确保目录中包含以下文件：
   - `main.go`：主程序文件
   - `defs_core.go`：DeFS 核心功能实现

## 使用说明

### 1. 启动节点

启动一个 DeFS 节点：

```bash
go run main.go defs_core.go -port 9000 [-bootstrap] [-datadir <path>] [-logdir <path>]
```

参数说明：

- `-port`: 节点监听端口
- `-bootstrap`: 是否作为引导节点启动
- `-datadir`: 数据存储目录路径(默认: ../defsdata)
- `-logdir`: 日志存储目录路径(默认: ../defsdata/logs)

### 2. 使用交互式命令行

启动后会进入交互式命令行界面，支持以下命令：

#### 2.1 上传文件

```bash
# 上传文件
DeFS> upload /path/to/file

# 管理上传任务
DeFS> pauseUpload <taskID>    # 暂停上传
DeFS> resumeUpload <taskID>   # 恢复上传
DeFS> cancelUpload <taskID>   # 取消上传
DeFS> listUploads            # 查看所有上传任务
```

文件路径支持以下格式：

```bash
# 1. 带单引号的路径（适用于从Finder/资源管理器拖拽的文件）
upload '/Users/username/Downloads/带 空格的文件.pdf'

# 2. 带双引号的路径
upload "/Users/username/Downloads/带 空格的文件.pdf"

# 3. 转义空格的路径
upload /Users/username/Downloads/带\ 空格的文件.pdf
```

> 注意：当文件路径包含空格时，必须使用以上任一方式处理路径，否则命令将无法正确识别文件位置。

#### 2.2 下载文件

```bash
# 下载文件
DeFS> download QmFileHash

# 管理下载任务
DeFS> pauseDownload <taskID>  # 暂停下载
DeFS> resumeDownload <taskID> # 恢复下载
DeFS> cancelDownload <taskID> # 取消下载
DeFS> listDownloads          # 查看所有下载任务
```

#### 2.3 添加节点

```bash
# 添加路由节点
DeFS> addRoutingPeer /ip4/127.0.0.1/tcp/9000/p2p/QmHash

# 添加发布订阅节点
DeFS> addPubSubPeer QmHash
```

#### 2.4 查看节点信息

```bash
# 显示当前节点ID
DeFS> showid

# 显示当前节点地址
DeFS> showaddr

# 查看已连接的节点列表
DeFS> list
```

#### 2.5 查询命令

```bash
# 查询文件资产
DeFS> queryAssets [page] [size]

# 查询上传任务
DeFS> queryUploads [page] [size]

# 查询下载任务
DeFS> queryDownloads [page] [size]
```

## 常见问题

0. 首次使用时需要先启动一个引导节点：
```bash
go run main.go defs_core.go -port 9000 -bootstrap
```
然后其他节点才能通过这个引导节点加入网络。

1. 如果遇到 "找不到包" 的错误，请确保已正确安装所有依赖：

```bash
go mod tidy
```

2. 如果遇到端口被占用的错误，可以尝试使用其他端口号：

```bash
go run main.go defs_core.go -port 9001
```

3. 如果日志输出干扰了交互式界面，可以查看日志文件获取详细信息

4. 程序数据和日志存储在以下位置：
   - 数据目录: ../defsdata/
   - 日志目录: ../defsdata/logs/
   - 下载目录: ../defsdata/downloads/

这个 README.md 文件提供了：
   1. 详细的使用说明
   2. 命令行参数解释
   3. 交互式模式的使用方法
   4. 注意事项和最佳实践
   5. 具体的示例工作流程

用户可以根据这个文档快速了解如何使用 DeFS 示例程序。
