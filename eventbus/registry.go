package eventbus

import (
	"sync"
)

/**
动态地处理多种事件类型
	1. 定义一个新的事件类型注册器，用于注册和获取事件总线实例。
	2. 使用一个映射（map）来管理这些事件总线实例。
*/

// EventRegistry 用于注册和获取事件总线实例
type EventRegistry struct {
	eventBuses map[string]Bus
	mu         sync.Mutex
}

// NewEventRegistry 创建一个新的事件注册器
func NewEventRegistry() *EventRegistry {
	return &EventRegistry{
		eventBuses: make(map[string]Bus),
	}
}

// RegisterEvent 注册一个新的事件总线
func (er *EventRegistry) RegisterEvent(eventType string, bus Bus) {
	er.mu.Lock()
	defer er.mu.Unlock()
	er.eventBuses[eventType] = bus
}

// GetEventBus 获取一个事件总线，如果不存在则返回nil
func (er *EventRegistry) GetEventBus(eventType string) Bus {
	er.mu.Lock()
	defer er.mu.Unlock()
	return er.eventBuses[eventType]
}
