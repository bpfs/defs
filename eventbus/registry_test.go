package eventbus

import (
	"fmt"
	"testing"
)

func TestEventRegistry(t *testing.T) {
	// 创建事件注册器
	registry := NewEventRegistry()

	// 注册示例A的事件总线
	registry.RegisterEvent("example:A", New())

	// 注册示例B的事件总线
	registry.RegisterEvent("example:B", New())

	// 获取事件总线
	Example_A_Bus := registry.GetEventBus("example:A")
	if Example_A_Bus == nil {
		t.Error("无法获取示例A的事件总线")
	} else {
		// 使用事件总线...
		fmt.Println("已注册 示例A 的事件总线")
	}

	// 注册
	Example_A_Bus.Subscribe("example:A", func(
		a string, // "example:A" 需要的请求参数
	) error {
		fmt.Printf("打印example:A事件参数:\t%s\n\n",a)
		return nil
	})


	Example_A_Bus.Publish("example:A", "参数a")

	///////////////////////////////////////////

	Example_B_Bus := registry.GetEventBus("example:B")
	if Example_B_Bus == nil {
		t.Error("无法获取示例B的事件总线。")
	} else {
		// 使用事件总线...
		fmt.Println("已注册 示例B 的事件总线")
	}

	// 注册
	Example_B_Bus.Subscribe("example:B", func(
		b string, // "example:B" 需要的请求参数
	) error {
		fmt.Printf("打印example:B事件参数:\t%s\n\n",b)
		return nil
	})

	Example_B_Bus.Publish("example:B", "参数b")
}
