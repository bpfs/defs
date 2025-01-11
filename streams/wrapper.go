package streams

import (
	"io"
	"net"
	"runtime/debug"
	"time"

	"github.com/dep2p/libp2p/core/network"
	pool "github.com/libp2p/go-buffer-pool"
	"github.com/libp2p/go-msgio"
	"github.com/sirupsen/logrus"
)

// 表示消息头的长度
const (
	messageHeaderLen = 17
	// MaxBlockSize     = 20000000 // 20M

)

var (
	messageHeader = headerSafe([]byte("/protobuf/msgio"))
	// 最大重试次数
	maxRetries = 3
	// 基础超时时间
	baseTimeout = 33 * time.Second
	// 最大消息大小
	MaxBlockSize = 1 << 25 // 32MB
)

// ReadStream 从流中读取消息，带指数退避重试
// 特别是考虑到避免资源的过度消耗和处理网络故障情况，引入指数退避重试策略，并设置合理的重试次数上限。
// 重要的改进点：
// 指数退避：每次重试的超时时间都会增加，减少在网络不稳定时的重试频率，减轻服务器压力。
// 有限的重试次数：通过maxRetries限制重试次数，防止在遇到持续的网络问题时无限重试。
// 清晰的错误处理：根据错误类型决定是否重试。例如，只有在遇到网络超时或其他指定的网络错误时才进行重试。
func ReadStream(stream network.Stream) ([]byte, error) {
	var (
		header [messageHeaderLen]byte
		err    error
	)

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 使用指数退避设置读取操作的超时时间
		timeout := baseTimeout * time.Duration(1<<attempt)
		stream.SetReadDeadline(time.Now().Add(timeout))

		// 尝试读取消息头部
		if _, err = io.ReadFull(stream, header[:]); err != nil {
			// 如果错误是网络超时或暂时性的，考虑重试
			if netErr, ok := err.(net.Error); ok && (netErr.Timeout() || netErr.Temporary()) {
				continue
			}
			// 对于非网络超时、非暂时性错误，直接退出
			break
		}

		// 如果头部读取成功，退出重试循环
		break
	}

	if err != nil {
		return nil, err
	}

	// 成功读取头部后，继续读取消息体，此时重置超时以避免中断正常读取
	stream.SetReadDeadline(time.Time{})
	reader := msgio.NewReaderSize(stream, MaxBlockSize)
	msg, err := reader.ReadMsg()
	if err != nil {
		return nil, err
	}
	defer reader.ReleaseMsg(msg)

	return msg, nil
}

// WriteStream 将消息写入流
// 采用类似于ReadStream的优化策略来增强其鲁棒性，尤其是在面对网络问题时。
// 重点将包括设置写操作的超时时间和对潜在的写操作失败进行适当的错误处理。
// 考虑到WriteStream的操作通常比读操作更简单（通常是一次性写入而非多次读取），通过设置整体的写超时来简化流程。
func WriteStream(msg []byte, stream network.Stream) error {
	// 设置写操作的超时时间
	if err := stream.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}

	// 先写入消息头
	_, err := stream.Write(messageHeader)
	if err != nil {
		return err
	}

	// 使用msgio包装流，以便于写入长度前缀和消息体
	writer := msgio.NewWriterWithPool(stream, pool.GlobalPool)
	if err = writer.WriteMsg(msg); err != nil {
		return err
	}

	// 写入操作成功后，清除写超时设置
	if err := stream.SetWriteDeadline(time.Time{}); err != nil {
		// 此处不返回错误，因为主要写入操作已经成功完成
	}

	return nil
}

// CloseStream 写入后关闭流，并等待EOF。
func CloseStream(stream network.Stream) error {
	if stream == nil {
		return nil
	}

	// 关闭写方向。如果流是全双工的，这会发送EOF给读方，而不会关闭整个流。
	if err := stream.CloseWrite(); err != nil {
		return err
	}

	// 关闭读方向。如果不需要读取EOF，也可以省略这一步。
	if err := stream.CloseRead(); err != nil {
		return err
	}

	go func() {
		// 设置超时，以防AwaitEOF卡住
		timer := time.NewTimer(EOFTimeout)
		defer timer.Stop()

		done := make(chan error, 1)
		go func() {
			// AwaitEOF等待给定流上的EOF，如果失败则返回错误。
			err := AwaitEOF(stream)
			done <- err
		}()

		select {
		case <-timer.C:
			// 超时后重置流，确保资源被释放
			if err := stream.Reset(); err != nil {
			}
		case err := <-done:
			if err != nil {
				// 有错误时记录，但通常不需要额外操作
			}
		}
	}()

	return nil
}

// HandlerWithClose 用关闭流和从恐慌中恢复来包装处理程序
func HandlerWithClose(f network.StreamHandler) network.StreamHandler {
	return func(stream network.Stream) {
		defer func() {
			// 从panic中恢复并记录错误信息
			if r := recover(); r != nil {
				// 打印堆栈信息以便调试
				logrus.Errorf("Panic stack trace: %s", string(debug.Stack()))
				// 尝试重置stream以确保资源被释放
				if err := stream.Reset(); err != nil {
				}
			}
		}()

		// 调用原始的stream处理函数
		f(stream)

		// 尝试优雅地关闭stream
		if err := CloseStream(stream); err != nil {
		}
	}
}

// HandlerWithWrite 通过写入、关闭流和从恐慌中恢复来包装处理程序
func HandlerWithWrite(f func(request *RequestMessage) error) network.StreamHandler {
	return func(stream network.Stream) {
		var req RequestMessage
		if err := f(&req); err != nil {
			return
		}

		// 序列化请求
		requestByte, err := req.Marshal()
		if err != nil {
			return
		}

		// WriteStream 将消息写入流。
		if err := WriteStream(requestByte, stream); err != nil {
			return
		}
	}
}

// HandlerWithRead 用读取、关闭流和从恐慌中恢复来包装处理程序
func HandlerWithRead(f func(request *RequestMessage)) network.StreamHandler {
	return func(stream network.Stream) {

		var req RequestMessage

		// ReadStream 从流中读取消息。
		requestByte, err := ReadStream(stream)
		if err != nil {
			return
		}
		if err := req.Unmarshal(requestByte); err != nil {
			return
		}

		f(&req)
	}
}

// HandlerWithRW 用于读取、写入、关闭流以及从恐慌中恢复，来包装处理程序。
// 处理程序 f 现在接收 RequestMessage 和 ResponseMessage，允许直接在函数内部定义成功或错误的响应。
func HandlerWithRW(f func(request *RequestMessage, response *ResponseMessage) (int32, string)) network.StreamHandler {
	return func(stream network.Stream) {
		var req RequestMessage
		var res ResponseMessage

		// 从流中读取请求消息
		requestByte, err := ReadStream(stream)
		if err != nil || len(requestByte) == 0 {
			logrus.Errorf("读取请求失败: %v", err)
			SendErrorResponse(stream, 500, "读取请求失败")
			return
		}

		// 解析请求消息
		if err := req.Unmarshal(requestByte); err != nil {
			logrus.Errorf("请求解析错误: %v", err)
			SendErrorResponse(stream, 400, "请求解析错误")
			return
		}

		// 调用处理函数处理请求，并根据返回的错误码和消息设置响应
		code, msg := f(&req, &res)

		// 设置响应码和消息
		res.Code = code
		res.Msg = msg

		// 将处理后的响应消息编码并写入流中
		responseByte, err := res.Marshal()
		if err != nil {
			logrus.Errorf("响应编码失败: %v", err)
			SendErrorResponse(stream, 500, "响应编码失败")
			return
		}

		if err := WriteStream(responseByte, stream); err != nil {
			logrus.Errorf("写入响应消息失败: %v", err)
			// 这里不再返回，因为已经是发送响应的步骤
		}
	}
}

// SendErrorResponse 是一个辅助函数，用于向流中发送错误响应。
func SendErrorResponse(stream network.Stream, code int32, msg string) {
	res := ResponseMessage{
		Code:    code,
		Message: &Message{Sender: stream.Conn().LocalPeer().String()},
		Msg:     msg,
	}
	responseByte, _ := res.Marshal()
	_ = WriteStream(responseByte, stream) // 这里忽略错误处理，因为已在错误路径中
}
