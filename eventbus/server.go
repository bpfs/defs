package eventbus

import (
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

// SubscribeType 定义了客户端订阅的方式
type SubscribeType int

const (
	// Subscribe - 普通订阅，即接收所有事件
	Subscribe SubscribeType = iota
	// SubscribeOnce - 订阅一次，即事件触发后取消订阅
	SubscribeOnce
)

// 常量，定义服务器订阅服务的方法名
const RegisterService = "ServerService.Register"

// SubscribeArg 定义了远程处理器的订阅参数结构
type SubscribeArg struct {
	ClientAddr    string        // 客户端地址
	ClientPath    string        // 客户端路径
	ServiceMethod string        // 服务方法名
	SubscribeType SubscribeType // 订阅类型
	Topic         string        // 主题名称
}

// Server 定义了能够被远程处理器订阅的服务端结构
type Server struct {
	eventBus    Bus                        // 本地事件总线
	address     string                     // 服务器地址
	path        string                     // 服务器路径
	subscribers map[string][]*SubscribeArg // 记录所有的订阅者
	service     *ServerService             // 服务器服务
}

// NewServer 创建并初始化一个新的Server实例
func NewServer(address, path string, eventBus Bus) *Server {
	return &Server{
		eventBus:    eventBus,
		address:     address,
		path:        path,
		subscribers: make(map[string][]*SubscribeArg),
		service:     &ServerService{server: &Server{}, wg: &sync.WaitGroup{}, started: false},
	}
}

// EventBus 返回服务端关联的事件总线
func (server *Server) EventBus() Bus {
	return server.eventBus
}

// rpcCallback 创建一个RPC回调函数来处理远程订阅的事件
func (server *Server) rpcCallback(subscribeArg *SubscribeArg) func(args ...interface{}) {
	return func(args ...interface{}) {
		client, err := rpc.DialHTTPPath("tcp", subscribeArg.ClientAddr, subscribeArg.ClientPath)
		if err != nil {
			fmt.Printf("连接错误: %v\n", err)
			return
		}
		defer client.Close()

		clientArg := &ClientArg{
			Topic: subscribeArg.Topic,
			Args:  args,
		}

		var reply bool
		err = client.Call(subscribeArg.ServiceMethod, clientArg, &reply)
		if err != nil {
			fmt.Printf("远程调用失败: %v\n", err)
		}
	}
}

// HasClientSubscribed 检查客户端是否已经订阅了指定的主题
func (server *Server) HasClientSubscribed(arg *SubscribeArg) bool {
	for _, subscriber := range server.subscribers[arg.Topic] {
		if *subscriber == *arg {
			return true
		}
	}
	return false
}

// Start 启动服务器来监听远程客户端的订阅请求
func (server *Server) Start() error {
	if server.service.started {
		return fmt.Errorf("服务器总线已经启动")
	}

	rpcServer := rpc.NewServer()
	rpcServer.Register(server.service)
	rpcServer.HandleHTTP(server.path, "/debug"+server.path)

	l, err := net.Listen("tcp", server.address)
	if err != nil {
		return fmt.Errorf("启动监听失败: %v", err)
	}

	server.service.started = true
	server.service.wg.Add(1)
	go func() {
		http.Serve(l, nil)
		server.service.wg.Done()
	}()

	return nil
}

// Stop 停止服务并释放所有资源
func (server *Server) Stop() {
	if server.service.started {
		server.service.started = false
		server.service.wg.Wait()
	}
}

// ServerService 定义了服务器服务结构，处理客户端的远程订阅
type ServerService struct {
	server  *Server
	wg      *sync.WaitGroup
	started bool
}

// Register 允许客户端订阅指定的主题
func (service *ServerService) Register(arg *SubscribeArg, success *bool) error {
	if service.server.HasClientSubscribed(arg) {
		*success = false
		return nil
	}

	callback := service.server.rpcCallback(arg)
	switch arg.SubscribeType {
	case Subscribe:
		service.server.eventBus.Subscribe(arg.Topic, callback)
	case SubscribeOnce:
		service.server.eventBus.SubscribeOnce(arg.Topic, callback)
	}

	service.server.subscribers[arg.Topic] = append(service.server.subscribers[arg.Topic], arg)
	*success = true
	return nil
}
