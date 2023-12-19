package mempool

// const DefaultPingInterval = 5 * time.Second

// type NodeDetails struct {
// 	dataIDs          []string  // 数据ID
// 	relatedNodes     []string  // 关联节点
// 	initialWriteTime time.Time // 首次写入时间
// 	lastUpdateTime   time.Time // 上次更新时间
// 	lastDisturbance  time.Time // 上次震荡时间

// 	// 计算节点的可靠性，可靠性较低则降低目前存储的权重
// 	pingSuccess        int64 // Ping 成功的次数
// 	pingFail           int64 // Ping 失败的次数
// 	networkDisturbance int64 // 网络震荡的次数。Ping 失败，但目标节点在线
// }

// type NodePool struct {
// 	ctx        context.Context                         // 用于管理ping服务的生命周期
// 	mutex      sync.RWMutex                            // 用于并发访问目标数组的读写互斥锁
// 	host       host.Host                               // ping 服务关联的 libp2p 主机
// 	pingPeriod time.Duration                           // ping 请求之间的时间间隔
// 	pinger     *ping.PingService                       // 用于发送和接收 ping 请求和响应的 libp2p PingService
// 	targets    []peer.AddrInfo                         // 一组 peer.AddrInfo 结构，表示要 ping 的一组对等点
// 	onPing     func(target peer.ID, rtt time.Duration) // 处理对端成功 ping 的结果的函数
// 	onOffline  func(target peer.ID)                    // 处理对端 ping 失败结果的函数
// 	offline    map[peer.ID]bool                        // 下线集合

// 	nodeDetailsMap map[peer.ID]NodeDetails // 节点映射内存池

// 	creationTime   time.Time // 首次创建池时间
// 	lastUpdateTime time.Time // 上次更新池时间

// 	// 占比较小则可以提供存储的权重
// 	totalCapacity    int64 // 容量的总额
// 	occupiedCapacity int64 // 已占用的容量

// }

// // PingOption 表示配置 NodePool 的 ping 选项
// type PingOption func(*NodePool)

// // WithPingPeriod 设置 NodePool 的 ping 周期
// func WithPingPeriod(pingPeriod time.Duration) PingOption {
// 	return func(p *NodePool) {
// 		p.pingPeriod = pingPeriod
// 	}
// }

// // WithPingTarget 为 NodePool 设置目标点
// func WithPingTarget(targets []peer.AddrInfo) PingOption {
// 	return func(p *NodePool) {
// 		p.targets = targets
// 	}
// }

// // WithPingHandler 为 NodePool 设置 ping 和离线事件处理程序
// func WithPingHandler(onPing func(target peer.ID, rtt time.Duration), onOffline func(target peer.ID)) PingOption {
// 	return func(p *NodePool) {
// 		p.onPing = onPing
// 		p.onOffline = onOffline
// 	}
// }

// // NewNodePool 创建一个新的 NodePool
// func NewNodePool(ctx context.Context, host host.Host, options ...PingOption) *NodePool {
// 	np := &NodePool{
// 		ctx:            ctx,
// 		pingPeriod:     DefaultPingInterval,
// 		nodeDetailsMap: make(map[peer.ID]NodeDetails),
// 		creationTime:   time.Now(),
// 	}

// 	for _, option := range options {
// 		option(np)
// 	}

// 	if np.onPing == nil {
// 		np.onPing = func(target peer.ID, rtt time.Duration) {

// 			logrus.Printf("PING: %s - RTT: %s\n", target.String(), rtt.String())
// 		}
// 	}

// 	if np.onOffline == nil {
// 		np.onOffline = func(target peer.ID) {
// 			logrus.Printf("离线 1: %s\n", target.String())
// 		}
// 	}

// 	np.pinger = ping.NewPingService(np.host)

// 	// 在主机的 Mux 上设置协议处理程序
// 	host.SetStreamHandler(ping.ID, np.pinger.PingHandler)

// 	go np.start()

// 	return np
// }

// // AddTarget 添加目标点
// // func (np *NodePool) AddTarget(target peer.AddrInfo, dataId string) {
// // 	np.mutex.Lock()
// // 	defer np.mutex.Unlock()

// // 	var counter bool = false
// // 	// 处理Ping目标对象
// // 	for _, addrInfo := range np.targets {
// // 		// 节点已经存在ping中
// // 		if addrInfo.ID == target.ID {
// // 			counter = true
// // 			break
// // 		}
// // 	}

// // 	if !counter {
// // 		np.targets = append(np.targets, target)
// // 	}

// // 	counter = false // 计数器归零

// // 	// 处理节点与文件的映射
// // 	nodeDetails, ok := np.nodeDetailsMap[target.ID]
// // 	// 节点不存在于内存池中
// // 	if !ok {
// // 		np.nodeDetailsMap[target.ID] = NodeDetails{
// // 			dataIDs:            []string{dataId}, // 数据ID
// // 			relatedNodes:       []string{},       // 关联节点
// // 			initialWriteTime:   time.Now(),       // 首次写入时间
// // 			lastDisturbance:    time.Time{},      // 上次震荡时间
// // 			pingSuccess:        0,                // Ping 成功的次数
// // 			pingFail:           0,                // Ping 失败的次数
// // 			networkDisturbance: 0,                // 网络震荡的次数。Ping 失败，但目标节点在线
// // 		}
// // 	} else {
// // 		// 判断map中的数组是否有重复值
// // 		for _, v := range nodeDetails.dataIDs {
// // 			// 已经有值，直接退出
// // 			if v == dataId {
// // 				counter = true
// // 				break
// // 			}
// // 		}

// // 		if !counter {
// // 			nodeDetails.dataIDs = append(nodeDetails.dataIDs, dataId)
// // 			nodeDetails.lastUpdateTime = time.Now()

// // 			np.nodeDetailsMap[target.ID] = nodeDetails
// // 		}

// // 		np.targets = append(np.targets, target)
// // 	}

// // 	// 从下线节点中删除
// // 	delete(np.offline, target.ID)
// // }

// // addTargetToPingList 将目标添加到Ping列表中
// func (np *NodePool) addTargetToPingList(target peer.AddrInfo) {
// 	for _, addrInfo := range np.targets {
// 		if addrInfo.ID == target.ID {
// 			return
// 		}
// 	}
// 	np.targets = append(np.targets, target)
// }

// // addDataIDToNodeDetails 将数据ID添加到节点详细信息中
// func (np *NodePool) addDataIDToNodeDetails(targetID peer.ID, dataId string) {
// 	nodeDetails, exists := np.nodeDetailsMap[targetID]
// 	if !exists {
// 		np.nodeDetailsMap[targetID] = NodeDetails{
// 			dataIDs:          []string{dataId},
// 			relatedNodes:     []string{},
// 			initialWriteTime: time.Now(),
// 		}
// 		return
// 	}

// 	for _, existingDataID := range nodeDetails.dataIDs {
// 		if existingDataID == dataId {
// 			return
// 		}
// 	}

// 	nodeDetails.dataIDs = append(nodeDetails.dataIDs, dataId)
// 	nodeDetails.lastUpdateTime = time.Now()
// 	np.nodeDetailsMap[targetID] = nodeDetails
// }

// // AddTarget 添加目标点
// func (np *NodePool) AddTarget(target peer.AddrInfo, dataId string) {
// 	np.mutex.Lock()
// 	defer np.mutex.Unlock()

// 	// 将目标添加到Ping列表中
// 	np.addTargetToPingList(target)
// 	// 将数据ID添加到节点详细信息中
// 	np.addDataIDToNodeDetails(target.ID, dataId)

// 	// 从下线节点中删除
// 	delete(np.offline, target.ID)
// }

// // RemoveTarget 删除目标点
// func (np *NodePool) RemoveTarget(target peer.ID) bool {
// 	np.mutex.Lock()
// 	defer np.mutex.Unlock()

// 	for i, t := range np.targets {
// 		if t.ID == target {
// 			np.targets = append(np.targets[:i], np.targets[i+1:]...)
// 			return true
// 		}
// 	}
// 	return false
// }

// // start 启动 PingService
// func (np *NodePool) start() {
// 	// NewTicker 返回一个新的 Ticker，其中包含一个通道，该通道将在每次报价后在通道上发送当前时间。
// 	// 滴答周期由持续时间参数指定。
// 	// 自动收报机将调整时间间隔或减少滴答声以弥补较慢的接收器。
// 	// 持续时间 d 必须大于零； 否则，NewTicker 会恐慌。
// 	// 停止自动收报机以释放相关资源。
// 	ticker := time.NewTicker(np.pingPeriod)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-np.ctx.Done():
// 			return
// 		case <-ticker.C:
// 			// 获取当前 pinger 中存储的所有目标地址
// 			targets := np.getTargets()

// 			//go func() {
// 			// ping 指定的目标并调用 ping 和离线事件处理程序。
// 			np.pingTargets(targets)
// 			//}()
// 		}
// 	}
// }

// // 获取当前 pinger 中存储的所有目标地址
// func (np *NodePool) getTargets() []peer.AddrInfo {
// 	np.mutex.RLock()
// 	defer np.mutex.RUnlock()

// 	targets := make([]peer.AddrInfo, len(np.targets))
// 	// copy 内置函数将源切片中的元素复制到目标切片中。
// 	copy(targets, np.targets)

// 	return targets
// }

// // pingTargets ping 指定的目标并调用 ping 和离线事件处理程序。
// func (np *NodePool) pingTargets(targets []peer.AddrInfo) {
// 	for _, target := range targets {
// 		results := np.pinger.Ping(np.ctx, target.ID)
// 		select {
// 		case res := <-results:
// 			//logrus.Println("ping通", target.ID.String(), "在", res.RTT)
// 			/**
// 			stream scope not attached to a protocol
// 			未附加到协议的流范围

// 			stream reset
// 			流重置
// 			*/

// 			// 无法 ping 通
// 			if res.Error != nil {
// 				// logrus.Errorf("节点:%s pin错误: %v", target.ID.String(), res.Error)
// 				// 且报错不属于“流重置”
// 				if res.Error.Error() != "stream reset" && res.Error.Error() != "stream scope not attached to a protocol" {
// 					np.onOffline(target.ID)
// 					// 删除目标点
// 					// p.RemoveTarget(target.ID)
// 				}
// 			} else {
// 				logrus.Printf("节点:%s OK", target.ID.String())
// 				np.onPing(target.ID, DefaultPingInterval)
// 			}
// 		case <-np.ctx.Done():
// 			// 当上下文结束时，退出循环
// 			return
// 		}
// 	}
// }
