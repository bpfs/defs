package uploads

// sendForwardRequest 发送转发请求到网络中的其他节点
// 参数:
//   - ctx: context.Context 类型，用于控制请求的上下文
//   - h: host.Host 类型，libp2p主机实例
//   - routingTable: *kbucket.RoutingTable 类型，用于查找目标节点
//   - payload: *pb.FileSegmentStorage 类型，要转发的文件片段存储对象
//
// 返回值:
//   - error: 如果转发过程中发生错误，返回相应的错误信息
// func sendForwardRequest(ctx context.Context, h host.Host, routingTable *kbucket.RoutingTable, payload *pb.FileSegmentStorage) error {
// 	maxRetries := 3 // 最大重试次数
// 	for retry := 0; retry < maxRetries; retry++ {
// 		// 检查节点数量是否足够
// 		if routingTable.Size() < 1 {
// 			logger.Warnf("转发时所需节点不足: %d", routingTable.Size())
// 			time.Sleep(1 * time.Second)
// 			continue
// 		}

// 		// 获取最近的节点
// 		receiverPeers := routingTable.NearestPeers(kbucket.ConvertKey(payload.SegmentId), 5, 2)
// 		if len(receiverPeers) == 0 {
// 			logger.Warn("没有找到合适的节点，重试中...")
// 			continue
// 		}

// 		// 随机选择一个节点
// 		node := receiverPeers[rand.Intn(len(receiverPeers))]
// 		network.StreamMutex.Lock()

// 		// 序列化存储对象
// 		data, err := payload.Marshal()
// 		if err != nil {
// 			logger.Errorf("序列化payload失败: %v", err)
// 			return err
// 		}

// 		// 发送文件片段到目标节点
// 		res, err := network.SendStream(ctx, h, StreamForwardToNetworkProtocol, "", node, data)
// 		if err != nil || res == nil {
// 			logger.Warnf("向节点 %s 发送数据失败: %v，重试中...", node.String(), err)
// 			continue
// 		}
// 		if res.Code != 200 {
// 			logger.Warnf("向节点 %s 发送数据失败，响应码: %d，重试中...", node.String(), res.Code)
// 			continue
// 		}

// 		logger.Infof("=====> 成功转发 SegmentId: %v", payload.SegmentId)
// 		return nil
// 	}

// 	return fmt.Errorf("转发文件片段失败，已达到最大重试次数")
// }
