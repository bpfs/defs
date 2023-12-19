package mempool

// // PingService 表示定期 ping 指定对等点的 ping 服务.
// type PingService struct {
// 	Ctx         context.Context                         // 用于管理ping服务的生命周期
// 	Host        host.Host                               // ping 服务关联的 libp2p 主机
// 	PingPeriod  time.Duration                           // ping 请求之间的时间间隔
// 	Targets     []peer.AddrInfo                         // 一组 peer.AddrInfo 结构，表示要 ping 的一组对等点
// 	OnPing      func(target peer.ID, rtt time.Duration) // 处理对端成功 ping 的结果的函数
// 	OnOffline   func(target peer.ID)                    // 处理对端 ping 失败结果的函数
// 	Offline     map[peer.ID]bool                        // 下线集合
// 	Pinger      *ping.PingService                       // 用于发送和接收 ping 请求和响应的 libp2p PingService
// 	Mutex       sync.RWMutex                            // 用于并发访问目标数组的读写互斥锁
// 	PendingPool map[peer.ID][]string                    // (节点——文件)的映射内存池(索引是peerID、数组是切片名称)
// 	QueuedPool  map[string]struct {                     // (文件——节点)的映射内存池(索引是切片名称、数组是peerID)
// 		State  bool    // Ping状态
// 		Target peer.ID // 目标节点
// 	}
// 	HandleChan chan []string // 处理通道
// }

// // NewPingService 创建一个新的 PingService
// func NewPingService(ctx context.Context, host host.Host, options ...PingOption) *PingService {
// 	p := &PingService{
// 		Ctx:         ctx,
// 		Host:        host,
// 		PingPeriod:  DefaultPingInterval,
// 		PendingPool: make(map[peer.ID][]string),
// 		Offline:     make(map[peer.ID]bool),
// 		QueuedPool: make(map[string]struct {
// 			State  bool    // Ping状态
// 			Target peer.ID // 目标节点
// 		}),
// 		HandleChan: make(chan []string, 2048), // 初始化chan通道
// 	}

// 	// for _, option := range options {
// 	// option(p)
// 	// }

// 	if p.OnPing == nil {
// 		p.OnPing = func(target peer.ID, rtt time.Duration) {

// 			logrus.Printf("PING: %s - RTT: %s\n", target.String(), rtt.String())
// 		}
// 	}

// 	if p.OnOffline == nil {
// 		p.OnOffline = func(target peer.ID) {
// 			logrus.Printf("离线 1: %s\n", target.String())
// 		}
// 	}

// 	p.Pinger = ping.NewPingService(p.Host)
// 	// 在主机的 Mux 上设置协议处理程序
// 	host.SetStreamHandler(ping.ID, p.Pinger.PingHandler)

// 	go p.start()

// 	return p
// }

// // AddTarget 添加目标点
// func (p *PingService) AddTarget(target peer.AddrInfo, sliceName string) {
// 	p.Mutex.Lock()
// 	defer p.Mutex.Unlock()

// 	var counter bool = false
// 	// 处理Ping目标对象
// 	for _, t := range p.Targets {
// 		// 节点已经存在ping中
// 		if t.ID == target.ID {
// 			counter = true
// 			break
// 		}
// 	}
// 	if !counter {
// 		p.Targets = append(p.Targets, target)
// 	}

// 	counter = false // 计数器归零

// 	// 处理节点与文件的映射
// 	_, ok := p.PendingPool[target.ID]
// 	if !ok {
// 		p.PendingPool[target.ID] = []string{sliceName}
// 	} else {
// 		// 判断map中的数组是否有重复值
// 		for _, v := range p.PendingPool[target.ID] {
// 			// 已经有值，直接退出
// 			if v == sliceName {
// 				counter = true
// 				break
// 			}
// 		}

// 		if !counter {
// 			p.PendingPool[target.ID] = append(p.PendingPool[target.ID], sliceName)
// 		}

// 		p.Targets = append(p.Targets, target)
// 	}

// 	// 从下线节点中删除
// 	delete(p.Offline, target.ID)
// 	// 处理文件与节点的映射
// 	p.QueuedPool[sliceName] = struct {
// 		State  bool    // Ping状态
// 		Target peer.ID // 目标节点
// 	}{
// 		State:  true,
// 		Target: target.ID,
// 	}
// }

// // RemoveTarget 删除目标点
// func (p *PingService) RemoveTarget(target peer.ID) bool {
// 	p.Mutex.Lock()
// 	defer p.Mutex.Unlock()

// 	for i, t := range p.Targets {
// 		if t.ID == target {
// 			p.Targets = append(p.Targets[:i], p.Targets[i+1:]...)
// 			return true
// 		}
// 	}
// 	return false
// }

// // start 启动 PingService
// func (p *PingService) start() {
// 	// NewTicker 返回一个新的 Ticker，其中包含一个通道，该通道将在每次报价后在通道上发送当前时间。
// 	// 滴答周期由持续时间参数指定。
// 	// 自动收报机将调整时间间隔或减少滴答声以弥补较慢的接收器。
// 	// 持续时间 d 必须大于零； 否则，NewTicker 会恐慌。
// 	// 停止自动收报机以释放相关资源。
// 	ticker := time.NewTicker(p.PingPeriod)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-p.Ctx.Done():
// 			return
// 		case <-ticker.C:
// 			// 获取当前 pinger 中存储的所有目标地址
// 			targets := p.getTargets()

// 			//go func() {
// 			// ping 指定的目标并调用 ping 和离线事件处理程序。
// 			p.pingTargets(targets)
// 			//}()
// 		}
// 	}
// }

// // 获取当前 pinger 中存储的所有目标地址
// func (p *PingService) getTargets() []peer.AddrInfo {
// 	p.Mutex.RLock()
// 	defer p.Mutex.RUnlock()

// 	targets := make([]peer.AddrInfo, len(p.Targets))
// 	// copy 内置函数将源切片中的元素复制到目标切片中。
// 	copy(targets, p.Targets)

// 	return targets
// }

// // pingTargets ping 指定的目标并调用 ping 和离线事件处理程序。
// func (p *PingService) pingTargets(targets []peer.AddrInfo) {
// 	for _, target := range targets {
// 		results := p.Pinger.Ping(p.Ctx, target.ID)
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
// 					p.OnOffline(target.ID)
// 					// 删除目标点
// 					// p.RemoveTarget(target.ID)
// 				}
// 			} else {
// 				logrus.Printf("节点:%s OK", target.ID.String())
// 				p.OnPing(target.ID, DefaultPingInterval)
// 			}
// 		case <-p.Ctx.Done():
// 			// 当上下文结束时，退出循环
// 			return
// 		}
// 	}
// }
