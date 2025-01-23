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

### 1. 启动存储节点

启动一个存储节点，监听 9000 端口：

```bash
go run main.go defs_core.go -mode server -port 9000
```

### 2. 启动客户端

#### 2.1 单命令模式

```bash
# 上传文件（支持以下任意格式的文件路径）
go run main.go defs_core.go -mode client -op upload -file /path/to/file
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

```bash
# 下载文件
go run main.go defs_core.go -mode client -op download -id <file-id>

# 添加对等节点
go run main.go defs_core.go -mode client -op addpeer -peer <peer-multiaddr> [-pmode <mode>] [-query] [-replace]

# 列出K桶节点
go run main.go defs_core.go -mode client -op listpeers
```

示例：

```bash
# 添加服务器节点
go run main.go defs_core.go -mode client -op addpeer \
  -peer /ip4/127.0.0.1/tcp/9000/p2p/QmHash \
  -pmode 2

# 添加可查询的客户端节点
go run main.go defs_core.go -mode client -op addpeer \
  -peer /ip4/192.168.1.100/tcp/9001/p2p/QmHash \
  -pmode 1 -query
```

#### 2.2 交互式模式

启动交互式客户端：

```bash
go run main.go defs_core.go -mode client -i
```

在交互式模式中可用的命令：

```
基础命令:
  - help                    # 显示帮助信息
  - time                    # 显示当前时间

节点管理:
  - list                    # 列出所有节点
  - addRoutingPeer <节点地址> <mode>  # 添加路由表节点
  - addPubSubPeer <节点ID>  # 添加发布订阅节点
  - size [mode]            # 显示节点数量
  - listPeers [mode]       # 列出节点
  - printPeers [mode]      # 打印节点详细信息

上传管理:
  - upload <文件路径>        # 上传文件
  - triggerUpload <taskID> # 触发指定任务的上传
  - pauseUpload <taskID>   # 暂停上传任务
  - resumeUpload <taskID>  # 恢复上传任务
  - cancelUpload <taskID>  # 取消上传任务
  - queryAssets           # 查询所有文件资产
  - listUploads          # 显示所有上传任务

下载管理:
  - download <文件ID>       # 下载文件
  - pauseDownload <taskID> # 暂停下载任务
  - resumeDownload <taskID># 恢复下载任务
  - cancelDownload <taskID># 取消下载任务
  - queryDownloads        # 查询所有下载任务
  - listDownloads        # 显示所有下载任务

退出:
  - exit/quit             # 退出程序
```

参数说明：
- `mode`: 节点运行模式 (可选参数)
  - `0`: 客户端模式
  - `1`: 服务端模式
  如果不指定 mode，则显示/操作所有节点
- `taskID`: 任务ID，上传或下载任务的唯一标识符
- `节点地址`: 完整的节点多地址，包含 IP、端口和节点ID
- `节点ID`: 节点的唯一标识符

命令示例：
```bash
# 节点管理
DeFS> addRoutingPeer /ip4/127.0.0.1/tcp/9000/p2p/QmHash 1  # 添加服务端节点
DeFS> addPubSubPeer QmPeerID                               # 添加发布订阅节点
DeFS> size 1                                               # 显示服务端节点数量
DeFS> listPeers 0                                         # 列出客户端节点
DeFS> printPeers                                          # 打印所有节点详情

# 上传管理
DeFS> upload /path/to/file.txt                            # 上传文件
DeFS> pauseUpload task123                                # 暂停上传任务
DeFS> resumeUpload task123                               # 恢复上传任务
DeFS> listUploads                                        # 显示所有上传任务

# 下载管理
DeFS> download QmFileHash                                # 下载指定文件
DeFS> pauseDownload task456                             # 暂停下载任务
DeFS> resumeDownload task456                            # 恢复下载任务
DeFS> listDownloads                                     # 显示所有下载任务
```

## 参数说明

- `-mode`: 运行模式 (server/client)
- `-i`: 启用交互式模式
- `-port`: 服务端口号 (仅服务器模式需要)
- `-op`: 操作类型 (upload/download/addpeer/listpeers)
- `-file`: 文件路径(上传用)
- `-id`: 文件ID(下载用)
- `-peer`: 对等节点的多地址
- `-pmode`: 节点模式 (0:Auto, 1:Client, 2:Server)
- `-query`: 是否为查询节点
- `-replace`: 是否允许替换现有节点

## 注意事项

1. 所有命令都需要在 `examples` 目录下执行
2. 运行时必须同时指定 `main.go` 和 `defs_core.go`
3. 至少需要启动一个存储节点（server模式）
4. 文件ID在上传完成后会在日志中显示，下载时需要使用该ID
5. 建议先启动存储节点等待一段时间后再执行上传/下载操作
6. 添加节点时需要指定完整的多地址，包括节点ID
7. 交互式模式支持连续执行多个命令，更适合进行多个操作
8. 单命令模式适合脚本用或单次操作

## 日志说明

- 程序运行日志保存在 `defs.log` 文件中
- 客户端操作日志保存在 `defs-client.log` 文件中

## 示例工作流程

1. 启动存储节点：

```bash
cd examples
go run main.go defs_core.go -mode server -port 9000
```

2. 启动交互式客户端：

```bash
cd examples  # 如果不在 examples 目录
go run main.go defs_core.go -mode client -i
```

3. 添加存储节点：

```bash
DeFS> addpeer /ip4/127.0.0.1/tcp/9000/p2p/QmHash
```

4. 上传文件：

```bash
DeFS> upload /path/to/myfile.txt
```

5. 下载文件：

```bash
DeFS> download QmFileHash
```

6. 查看节点列表：

```bash
DeFS> list
```

## 常见问题

1. 如果遇到 "找不到包" 的错误，请确保已正确安装所有依赖：

```bash
go mod tidy
```

2. 如果遇到端口被占用的错误，可以尝试使用其他端口号：

```bash
go run main.go defs_core.go -mode server -port 9001
```

3. 如果日志输出干扰了交互式界面，可以查看 `defs.log` 文件获取详细日志信息

这个 README.md 文件提供了：
   1. 详细的使用说明
   2. 命令行参数解释
   3. 交互式模式的使用方法
   4. 注意事项和最佳实践
   5. 具体的示例工作流程

用户可以根据这个文档快速了解如何使用 DeFS 示例程序。
