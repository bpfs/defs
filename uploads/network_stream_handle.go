package uploads

// handleSendingToNetwork 处理发送任务到网络的请求
// 参数:
//   - req: *streams.RequestMessage 类型，包含请求的详细信息
//   - res: *streams.ResponseMessage 类型，用于设置响应内容
//
// 返回值:
//   - int32: 状态码，表示处理结果
//   - string: 状态消息，对处理结果的描述
// func (sp *StreamProtocol) handleSendingToNetwork(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
// 	// 解析请求载荷
// 	payload := new(pb.FileSegmentStorage)
// 	if err := payload.Unmarshal(req.Payload); err != nil {
// 		logger.Errorf("解码请求载荷失败: %v", err)
// 		return 6603, err.Error()
// 	}

// 	// 打印分片ID信息
// 	logger.Infof("=====> SegmentId: %v", payload.SegmentId)

// 	// 检查参数有效性
// 	if sp.opt == nil || sp.fs == nil || payload.SegmentContent == nil {
// 		logger.Error("文件写入的参数无效: 选项配置、文件系统或分片内容为空")
// 		return 500, "文件写入的参数无效"
// 	}

// 	// 构建文件片段存储map并将其存储为文件
// 	if err := buildAndStoreFileSegment(payload, sp.host.ID().String()); err != nil {
// 		logger.Errorf("存储接收内容失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 创建 FileSegmentStorageStore 实例
// 	store := database.NewFileSegmentStorageSqlStore(sp.db.SqliteDB)
// 	payloadSql, err := database.ToFileSegmentStorageSql(payload)
// 	if err != nil {
// 		logger.Errorf("将 FileSegmentStorage 转换为 FileSegmentStorageSql失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 将文件片段存储记录保存到数据库
// 	if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
// 		logger.Errorf("保存文件片段存储记录失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 清空分片内容防止通道内容过大，在转发时重新查询
// 	payload.SegmentContent = nil

// 	// 将payload发送到转发通道
// 	//sp.upload.TriggerForward(payload)

// 	// 清空数据和请求载荷以释放内存
// 	payload = nil
// 	req.Payload = nil

// 	// 强制进行垃圾回收
// 	runtime.GC()

// 	return 200, "成功"
// }

// handleForwardToNetwork 处理转发任务到网络的请求
// 参数:
//   - req: *streams.RequestMessage 类型，包含请求的详细信息
//   - res: *streams.ResponseMessage 类型，用于设置响应内容
//
// 返回值:
//   - int32: 状态码，表示处理结果
//   - string: 状态消息，对处理结果的描述
// func (sp *StreamProtocol) handleForwardToNetwork(req *streams.RequestMessage, res *streams.ResponseMessage) (int32, string) {
// 	// 解析请求载荷
// 	payload := new(pb.FileSegmentStorage)
// 	if err := payload.Unmarshal(req.Payload); err != nil {
// 		logger.Errorf("解码请求载荷失败: %v", err)
// 		return 6603, err.Error()
// 	}

// 	logger.Infof("转发=====> SegmentId: %v,内容%d", payload.SegmentId, len(payload.SegmentContent))

// 	// 检查参数有效性
// 	if sp.opt == nil || sp.fs == nil || payload.SegmentContent == nil {
// 		logger.Error("文件写入的参数无效: 选项配置、文件系统或分片内容为空")
// 		return 500, "文件写入的参数无效"
// 	}

// 	// 构建文件片段存储map并将其存储为文件
// 	if err := buildAndStoreFileSegment(payload, sp.host.ID().String()); err != nil {
// 		logger.Errorf("存储接收内容失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 创建 FileSegmentStorageStore 实例
// 	store := database.NewFileSegmentStorageSqlStore(sp.db.SqliteDB)
// 	payloadSql, err := database.ToFileSegmentStorageSql(payload)
// 	if err != nil {
// 		logger.Errorf("将 FileSegmentStorage 转换为 FileSegmentStorageSql失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 将文件片段存储记录保存到数据库
// 	if err := store.CreateFileSegmentStorage(payloadSql); err != nil {
// 		logger.Errorf("保存文件片段存储记录失败: %v", err)
// 		return 500, err.Error()
// 	}

// 	// 清空数据和请求载荷以释放内存
// 	payload = nil
// 	req.Payload = nil

// 	// 强制进行垃圾回收
// 	runtime.GC()

// 	return 200, "成功"
// }
