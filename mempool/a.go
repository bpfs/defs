package mempool

// type StartPingServiceInput struct {
// 	fx.In
// 	Ctx    context.Context
// 	Host   host.Host                // 主机
// 	Table  *kbucket.RoutingTable    // 路由表
// 	Topics map[string]*pubsub.Topic // 订阅主题
// }

// type StartPingServiceOutput struct {
// 	fx.Out
// 	PingService *PingService // ping 服务
// }

// // 启动Ping服务
// func StartPingService(lc fx.Lifecycle, input StartPingServiceInput) (out StartPingServiceOutput, err error) {
// 	pingService := NewPingService(input.Ctx, input.Host,
// 		// 为 PingService 设置目标点
// 		WithPingTarget([]peer.AddrInfo{}),
// 		// 设置 PingService 的 ping 周期
// 		WithPingPeriod(5*time.Second),
// 		// 为 PingService 设置 ping 和离线事件处理程序
// 		// ping.WithPingHandler(
// 		// 	func(target peer.ID, rtt time.Duration) {
// 		// 		logrus.Printf("PING: %s - RTT: %s\n", target.String(), rtt.String())
// 		// 	},
// 		// 	func(target peer.ID) {
// 		// 		// 节点离线事件
// 		// 		// 从K桶找节点发送出去
// 		// 		// 添加ping
// 		// 		logrus.Printf("离线 2: %s\n", target.String())

// 		// 	},
// 		// ),
// 	)
// 	// 当 ping 其他节点失败时，打印错误信息

// 	pingService.OnOffline = func(target peer.ID) {
// 		logrus.Printf("%s 已离线\n", target.String())

// 		// 证明下线节点没有在集合中存在,如果存在证明已经处理过了，已经把离线数据发给通道了，不做处理
// 		if ok := pingService.Offline[target]; !ok {

// 			pingService.Offline[target] = true

// 			// RemovePeer 应该在调用者确定对等点对查询没有用时调用。
// 			// 例如：对端可能已经停止支持 DHT 协议。
// 			// 它将对等体从路由表中逐出。
// 			input.Table.RemovePeer(target)

// 			// 去查询下线节点下所有的切片数据通知到通道需要重新找节点
// 			sliceNames, ok := pingService.PendingPool[target]
// 			if ok {

// 				// 循环处理节点对应的所有切片
// 				for _, v := range sliceNames {
// 					pingService.QueuedPool[v] = struct {
// 						State  bool    // Ping状态
// 						Target peer.ID // 目标节点
// 					}{
// 						State:  false,
// 						Target: target,
// 					}
// 				}
// 				// 发送至排队通道
// 				pingService.HandleChan <- sliceNames

// 			}
// 		}
// 	}
// 	lc.Append(fx.Hook{
// 		OnStart: func(ctx context.Context) error {

// 			return nil
// 		},
// 		OnStop: func(ctx context.Context) error {
// 			return nil
// 		},
// 	})

// 	out.PingService = pingService // ping 服务
// 	return out, nil
// }
