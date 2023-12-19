package eventbus

import (
	"fmt"
	"reflect"
	"sync"
)

// BusSubscriber 定义了与订阅相关的总线行为
type BusSubscriber interface {
	Subscribe(topic string, fn interface{}) error                          // 订阅主题
	SubscribeAsync(topic string, fn interface{}, transactional bool) error // 异步订阅主题
	SubscribeOnce(topic string, fn interface{}) error                      // 订阅一次主题
	SubscribeOnceAsync(topic string, fn interface{}) error                 // 异步订阅一次主题
	Unsubscribe(topic string, handler interface{}) error                   // 取消订阅
}

// BusPublisher 定义了与发布相关的总线行为
type BusPublisher interface {
	Publish(topic string, args ...interface{}) // 发布主题
}

// BusController 定义了总线控制行为（检查处理程序的存在，同步）
type BusController interface {
	HasCallback(topic string) bool // 检查是否有回调
	WaitAsync()                    // 等待异步回调完成
}

// Bus 整合全局（订阅，发布，控制）总线行为
type Bus interface {
	BusController
	BusSubscriber
	BusPublisher
}

// EventBus 提供了一个实现了 Bus 接口的基本事件总线
type EventBus struct {
	handlers map[string][]*eventHandler // 存储各主题的处理程序
	lock     sync.Mutex                 // 保护处理程序的互斥锁
	wg       sync.WaitGroup             // 用于等待所有异步事件处理完成的等待组
}

// eventHandler 存储事件处理程序的元信息
type eventHandler struct {
	callBack      reflect.Value // 事件处理函数
	once          sync.Once     // 确保处理程序只执行一次
	transactional bool          // 是否使用事务
}

// New 创建一个新的 EventBus 实例
func New() Bus {
	return &EventBus{
		handlers: make(map[string][]*eventHandler),
	}
}

// doSubscribe 是一个通用的订阅函数，被其他订阅函数调用
func (bus *EventBus) doSubscribe(topic string, fn interface{}, once, transactional bool) error {
	if topic == "" {
		return fmt.Errorf("主题不能为空")
	}

	if fn == nil {
		return fmt.Errorf("处理程序函数不能为空")
	}

	bus.lock.Lock()
	defer bus.lock.Unlock()

	handler := &eventHandler{
		callBack:      reflect.ValueOf(fn),
		transactional: transactional,
	}

	if once {
		handler.once.Do(func() {
			bus.removeHandler(topic, handler)
		})
	}

	bus.handlers[topic] = append(bus.handlers[topic], handler)
	return nil
}

// Subscribe 订阅指定主题的事件
func (bus *EventBus) Subscribe(topic string, fn interface{}) error {
	return bus.doSubscribe(topic, fn, false, false)
}

// SubscribeAsync 异步订阅指定主题的事件
func (bus *EventBus) SubscribeAsync(topic string, fn interface{}, transactional bool) error {
	return bus.doSubscribe(topic, fn, false, transactional)
}

// SubscribeOnce 订阅一次指定主题的事件
func (bus *EventBus) SubscribeOnce(topic string, fn interface{}) error {
	return bus.doSubscribe(topic, fn, true, false)
}

// SubscribeOnceAsync 异步订阅一次指定主题的事件
func (bus *EventBus) SubscribeOnceAsync(topic string, fn interface{}) error {
	return bus.doSubscribe(topic, fn, true, false)
}

// HasCallback 检查指定主题是否有回调处理程序
func (bus *EventBus) HasCallback(topic string) bool {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	_, ok := bus.handlers[topic]
	return ok
}

// Unsubscribe 取消订阅指定主题的事件
func (bus *EventBus) Unsubscribe(topic string, handler interface{}) error {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	if _, ok := bus.handlers[topic]; !ok {
		return fmt.Errorf("没有找到主题: %s", topic)
	}

	bus.removeHandler(topic, &eventHandler{callBack: reflect.ValueOf(handler)})
	return nil
}

// Publish 发布事件到指定主题
func (bus *EventBus) Publish(topic string, args ...interface{}) {
	bus.lock.Lock()
	handlers, ok := bus.handlers[topic]
	bus.lock.Unlock()

	if !ok {
		return
	}

	for _, handler := range handlers {
		if handler.transactional {
			bus.wg.Add(1)
			go func(handler *eventHandler) {
				defer bus.wg.Done()
				handler.callBack.Call(bus.createArgs(handler, args...))
			}(handler)
		} else {
			handler.callBack.Call(bus.createArgs(handler, args...))
		}
	}
}

// createArgs 创建处理程序函数的参数
func (bus *EventBus) createArgs(handler *eventHandler, args ...interface{}) []reflect.Value {
	var arguments []reflect.Value
	for _, arg := range args {
		arguments = append(arguments, reflect.ValueOf(arg))
	}
	return arguments
}

// removeHandler 从指定主题中移除处理程序
func (bus *EventBus) removeHandler(topic string, handler *eventHandler) {
	handlers, ok := bus.handlers[topic]
	if !ok {
		return
	}

	for i, h := range handlers {
		if h == handler {
			bus.handlers[topic] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

// WaitAsync 等待所有异步处理程序执行完成
func (bus *EventBus) WaitAsync() {
	bus.wg.Wait()
}
