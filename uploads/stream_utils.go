package uploads

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bpfs/defs/v2/pb"
)

// StreamUtils 包含流操作的公共方法
type StreamUtils struct {
	reader *bufio.Reader
	writer *bufio.Writer
	conn   net.Conn
}

// NewStreamUtils 创建一个新的StreamUtils实例
// 参数:
//   - conn: 网络连接
//
// 返回值:
//   - *StreamUtils: 新的StreamUtils实例
func NewStreamUtils(conn net.Conn) *StreamUtils {
	return &StreamUtils{
		reader: bufio.NewReaderSize(conn, MaxBlockSize),
		writer: bufio.NewWriterSize(conn, MaxBlockSize),
		conn:   conn,
	}
}

// WriteSegmentData 写入分片数据
// 参数:
//   - payload: 分片数据
//
// 返回值:
//   - error: 如果写入失败，返回相应的错误信息
func (s *StreamUtils) WriteSegmentData(payload *pb.FileSegmentStorage) error {
	// 序列化数据
	data, err := payload.Marshal()
	if err != nil {
		logger.Errorf("序列化数据失败: %v", err)
		return err
	}

	// 写入长度前缀
	lenBuf := make([]byte, 4)
	lenBuf[0] = byte(len(data) >> 24)
	lenBuf[1] = byte(len(data) >> 16)
	lenBuf[2] = byte(len(data) >> 8)
	lenBuf[3] = byte(len(data))

	// 设置写入超时
	s.conn.SetDeadline(time.Now().Add(ConnTimeout))

	// 写入长度前缀
	if _, err := s.writer.Write(lenBuf); err != nil {
		logger.Errorf("写入长度前缀失败: %v", err)
		return err
	}

	// 写入数据
	if _, err := s.writer.Write(data); err != nil {
		logger.Errorf("写入数据失败: %v", err)
		return err
	}

	// 刷新缓冲区
	if err := s.writer.Flush(); err != nil {
		logger.Errorf("刷新缓冲区失败: %v", err)
		return err
	}

	return nil
}

// ReadResponse 读取响应
// 参数:
//   - s: StreamUtils实例
//
// 返回值:
//   - error: 如果读取失败，返回相应的错误信息
func (s *StreamUtils) ReadResponse() error {
	// 设置读取超时
	s.conn.SetDeadline(time.Now().Add(ConnTimeout))

	// 读取响应
	response, err := s.reader.ReadString('\n')
	if err != nil {
		logger.Errorf("读取响应失败: %v", err)
		return err
	}

	// 验证响应
	if !strings.Contains(response, "success") {
		logger.Errorf("响应验证失败: %s", response)
		return fmt.Errorf("响应验证失败: %s", response)
	}

	return nil
}
