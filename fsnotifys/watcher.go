// backend/fsnotify/watcher.go

package fsnotify

import (
	"fmt"

	"github.com/bpfs/defs/paths"
	"github.com/sirupsen/logrus"
)

// 启动观察器
func StartWatcher(name string) error {
	if err := paths.DirExistsAndMkdirAll(name); err != nil {
		logrus.Errorf("检查路径失败: %v", err)
		return err
	}

	// Create new watcher.
	watcher, err := NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// 添加路径
	if err := watcher.Add(name); err != nil {
		logrus.Errorf("添加路径失败: %v", err)
		return err
	}

	// 启动监听文件对象事件协程
	fmt.Println("开始监听文件变化")
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if ok {
					// Has 报告此事件是否具有给定的操作。
					if event.Has(Create) {
						//	logrus.Println("创建文件:", event.Name)
						// 调用回调函数处理文件创建事件
						//callback(event.Name) // 调用回调函数处理逻辑
						// 发送时间
						// 但是发送到发送
						// input.UploadEvent.Publish("file:Upload", event.Name)
					}
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					logrus.Errorf("监听文件失败: %v", err)
					return
				}
			}
		}
	}()

	return nil
}

// 新观察者
// func NewWatcher(lc fx.Lifecycle) (*fsnotify.Watcher, error) {
// 	// NewWatcher 与底层操作系统建立一个新的观察者并开始等待事件。
// 	watcher, err := fsnotify.NewWatcher()
// 	if err != nil {
// 		return nil, err
// 	}

// 	lc.Append(fx.Hook{
// 		OnStop: func(ctx context.Context) error {
// 			return watcher.Close()
// 		},
// 	})

// 	return watcher, nil
// }
