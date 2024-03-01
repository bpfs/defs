package eventbus

import (
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

const (
	// PublishService - 客户端服务方法
	PublishService = "ClientService.PushEvent"
)

// ClientArg - 包含客户端要在本地发布的事件的对象
type ClientArg struct {
	Args  []interface{} // 参数
	Topic string        // 主题
}

// Client - 能够订阅远程事件总线的对象
type Client struct {
	eventBus Bus            // 事件总线
	address  string         // 地址
	path     string         // 路径
	service  *ClientService // 服务
}

// NewClient - 使用地址、路径和事件总线创建一个新的客户端
func NewClient(address, path string, eventBus Bus) *Client {
	client := &Client{
		eventBus: eventBus,
		address:  address,
		path:     path,
		service:  &ClientService{client: nil, wg: &sync.WaitGroup{}, started: false}, // 初始化客户端服务
	}
	client.service.client = client // 设置客户端服务的客户端引用
	return client
}

// EventBus - 返回底层的事件总线
func (client *Client) EventBus() Bus {
	return client.eventBus
}

func (client *Client) doSubscribe(topic string, fn interface{}, serverAddr, serverPath string, subscribeType SubscribeType) error {
	// 连接到远程RPC服务器
	rpcClient, err := rpc.DialHTTPPath("tcp", serverAddr, serverPath)
	if err != nil {
		return fmt.Errorf("连接错误: %v", err) // 返回错误，而不是仅打印
	}
	defer rpcClient.Close() // 延迟关闭 RPC 客户端

	args := &SubscribeArg{client.address, client.path, PublishService, subscribeType, topic}
	reply := new(bool)

	// 远程调用注册服务
	err = rpcClient.Call(RegisterService, args, reply)
	if err != nil {
		return fmt.Errorf("注册错误: %v", err) // 返回错误，而不是仅打印
	}

	if *reply {
		// 在本地事件总线上订阅主题
		client.eventBus.Subscribe(topic, fn)
	}
	return nil // 返回 nil 表示无错误
}

// Subscribe - 在远程事件总线上订阅主题，并处理任何可能的错误
func (client *Client) Subscribe(topic string, fn interface{}, serverAddr, serverPath string) error {
	return client.doSubscribe(topic, fn, serverAddr, serverPath, Subscribe)
}

// SubscribeOnce - 一次性订阅远程事件总线上的主题，并处理任何可能的错误
func (client *Client) SubscribeOnce(topic string, fn interface{}, serverAddr, serverPath string) error {
	return client.doSubscribe(topic, fn, serverAddr, serverPath, SubscribeOnce)
}

// Start - 启动客户端服务以侦听远程事件
func (client *Client) Start() error {
	service := client.service
	if service.started {
		return fmt.Errorf("客户端服务已启动") // 返回错误，服务已经启动
	}

	// 创建新的RPC服务器
	server := rpc.NewServer()
	server.Register(service)
	server.HandleHTTP(client.path, "/debug"+client.path)

	// 启动TCP监听
	l, err := net.Listen("tcp", client.address)
	if err != nil {
		return fmt.Errorf("监听错误: %v", err) // 返回错误，监听失败
	}

	service.wg.Add(1)
	service.started = true

	// 启动HTTP服务
	go http.Serve(l, nil)
	return nil // 返回 nil 表示无错误
}

// Stop - 发出停止服务的信号
func (client *Client) Stop() {
	service := client.service
	if service.started {
		service.wg.Done()
		service.started = false
	}
}

// ClientService - 侦听远程事件总线中发布的事件的服务对象
type ClientService struct {
	client  *Client         // 客户端
	wg      *sync.WaitGroup // 同步等待组
	started bool            // 是否已启动
}

// PushEvent - 侦听远程事件的导出服务
func (service *ClientService) PushEvent(arg *ClientArg, reply *bool) error {
	// 在本地事件总线上发布事件
	service.client.eventBus.Publish(arg.Topic, arg.Args...)
	*reply = true
	return nil
}
