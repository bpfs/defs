package downloads

import (
	"fmt"

	"github.com/bpfs/defs/v2/pb"
)

// NotifyStatus 通知上传状态
// 参数:
//   - m: 上传管理器对象
//   - status: 要通知的状态信息
func (m *DownloadManager) NotifyStatus(status *pb.DownloadChan) {
	select {
	case m.statusChan <- status: // 通知成功
		logger.Debugf("已通知上传状态: %v", status)
	default: // 通道已满,尝试清空后重试
		select {
		case <-m.statusChan: // 清空通道
			m.statusChan <- status // 重新发送状态
			logger.Debugf("已通知上传状态(清空旧状态后): %v", status)
		default: // 无法清空通道
			logger.Warnf("无法通知上传状态,通道已满: %v", status)
		}
	}
}

// NotifyError 通知错误信息
// 参数:
//   - m: 上传管理器对象
//   - err: 要通知的错误信息
//   - args: 格式化参数
func (m *DownloadManager) NotifyError(err string, args ...interface{}) {
	errMsg := fmt.Errorf(err, args...)
	select {
	case m.errChan <- errMsg: // 通知成功
		logger.Debugf("已通知错误信息: %v", errMsg)
	default: // 通道已满,尝试清空后重试
		select {
		case <-m.errChan: // 清空通道
			m.errChan <- errMsg // 重新发送错误
			logger.Debugf("已通知错误信息(清空旧错误后): %v", errMsg)
		default: // 无法清空通道
			logger.Warnf("无法通知错误信息,通道已满: %v", errMsg)
		}
	}
}
