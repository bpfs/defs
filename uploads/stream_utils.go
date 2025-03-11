package uploads

import (
	"fmt"
	"net"
	"time"

	"github.com/bpfs/defs/v2/pb"
	defsproto "github.com/bpfs/defs/v2/utils/protocol"
)

// ProtocolHandler 替代原来的StreamUtils，使用defsproto.Handler处理网络通信
type ProtocolHandler struct {
	handler *defsproto.Handler
}

// NewProtocolHandler 创建一个新的协议处理器
// 参数:
//   - conn: 网络连接
//
// 返回值:
//   - *ProtocolHandler: 新的协议处理器实例
func NewProtocolHandler(conn net.Conn) *ProtocolHandler {
	handler := defsproto.NewHandler(conn, &defsproto.Options{
		MaxRetries:     3,
		RetryDelay:     time.Second,
		HeartBeat:      30 * time.Second,
		DialTimeout:    ConnTimeout,
		WriteTimeout:   ConnTimeout * 2,
		ReadTimeout:    ConnTimeout * 2,
		ProcessTimeout: ConnTimeout * 2,
		MaxConns:       100,
		Rate:           50 * 1024 * 1024, // 50MB/s
		Window:         20 * 1024 * 1024, // 20MB window
		Threshold:      10 * 1024 * 1024, // 10MB threshold
		QueueSize:      1000,
		QueuePolicy:    defsproto.PolicyBlock,
	})

	// 启动心跳
	handler.StartHeartbeat()

	return &ProtocolHandler{
		handler: handler,
	}
}

// WriteSegmentData 写入分片数据
// 参数:
//   - payload: 分片数据
//
// 返回值:
//   - error: 如果写入失败，返回相应的错误信息
func (p *ProtocolHandler) WriteSegmentData(payload *pb.FileSegmentStorage) error {
	dataMsg := &SegmentDataMessage{
		Payload: payload,
	}
	return p.handler.SendMessage(dataMsg)
}

// ReadResponse 读取响应
// 参数:
//   - s: StreamUtils实例
//
// 返回值:
//   - error: 如果读取失败，返回相应的错误信息
func (p *ProtocolHandler) ReadResponse() error {
	respMsg := &SegmentResponseMessage{}
	if err := p.handler.ReceiveMessage(respMsg); err != nil {
		logger.Errorf("接收响应失败: %v", err)
		return err
	}

	// 检查响应
	if !respMsg.Success {
		logger.Errorf("接收到错误响应: %s", respMsg.Error)
		return fmt.Errorf("接收到错误响应: %s", respMsg.Error)
	}

	return nil
}

// Close 关闭处理器
func (p *ProtocolHandler) Close() error {
	if p.handler != nil {
		p.handler.StopHeartbeat()
		return p.handler.Close()
	}
	return nil
}
