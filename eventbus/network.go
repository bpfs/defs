package eventbus

import (
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

// NetworkBus 定义了网络总线的结构，包含客户端、服务器和共享的事件总线
type NetworkBus struct {
	Client    *Client            // 内嵌客户端
	Server    *Server            // 内嵌服务器
	sharedBus Bus                // 共享的事件总线
	address   string             // 网络总线的地址
	path      string             // 网络总线的路径
	service   *NetworkBusService // 网络总线的服务
}

// NetworkBusService 定义了网络总线服务的状态和同步机制
type NetworkBusService struct {
	wg      sync.WaitGroup // 同步等待组
	started bool           // 表示服务是否已启动
}

// NewNetworkBus 创建并初始化一个新的网络总线实例
func NewNetworkBus(address, path string) *NetworkBus {
	sharedBus := New() // 创建一个新的事件总线实例
	return &NetworkBus{
		Client:    NewClient(address, path, sharedBus), // 初始化客户端
		Server:    NewServer(address, path, sharedBus), // 初始化服务器
		sharedBus: sharedBus,                           // 设置共享的事件总线
		address:   address,                             // 设置地址
		path:      path,                                // 设置路径
		service:   &NetworkBusService{},                // 初始化网络总线服务
	}
}

// EventBus 返回当前网络总线关联的事件总线实例
func (nb *NetworkBus) EventBus() Bus {
	return nb.sharedBus
}

// Start 启动网络总线，包括其关联的 RPC 服务
func (nb *NetworkBus) Start() error {
	if nb.service.started {
		return fmt.Errorf("网络总线服务已经启动")
	}

	server := rpc.NewServer()                               // 创建一个新的RPC服务器
	server.RegisterName("ServerService", nb.Server.service) // 注册服务器服务
	server.RegisterName("ClientService", nb.Client.service) // 注册客户端服务
	server.HandleHTTP(nb.path, "/debug"+nb.path)            // 处理HTTP请求

	l, err := net.Listen("tcp", nb.address) // 在指定地址开始监听
	if err != nil {
		return fmt.Errorf("启动监听失败: %v", err)
	}

	nb.service.wg.Add(1)
	nb.service.started = true

	go func() {
		http.Serve(l, nil) // 启动HTTP服务器
		nb.service.wg.Done()
	}()

	return nil
}

// Stop 停止网络总线服务
func (nb *NetworkBus) Stop() {
	if nb.service.started {
		nb.service.started = false // 标记服务为未开始
		nb.service.wg.Wait()
	}
}
