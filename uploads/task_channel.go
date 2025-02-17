package uploads

import (
	"fmt"
	"time"

	"github.com/bpfs/defs/v2/pb"
)

// ForceSegmentProcess 强制触发片段处理
// 将文件片段整合并写入队列，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForceSegmentProcess() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chSegmentProcess <- struct{}{}:
		return nil
	default:
		// 通道已满，尝试清空并重新写入
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chSegmentProcess:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chSegmentProcess <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceNodeDispatch 强制触发节点分发
// 以节点为单位从队列中读取文件片段进行分发，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
// func (t *UploadTask) ForceNodeDispatch() error {
// 	select {
// 	case <-t.ctx.Done():
// 		return fmt.Errorf("任务已取消")
// 	case t.chNodeDispatch <- struct{}{}:
// 		return nil
// 	default:
// 		select {
// 		case <-t.ctx.Done():
// 			return fmt.Errorf("任务已取消")
// 		case <-t.chNodeDispatch:
// 			select {
// 			case <-t.ctx.Done():
// 				return fmt.Errorf("任务已取消")
// 			case t.chNodeDispatch <- struct{}{}:
// 				return nil
// 			}
// 		}
// 	}
// }

// ForceNetworkTransfer 强制触发网络传输
// 向目标节点传输文件片段，支持重试机制
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
// func (t *UploadTask) ForceNetworkTransfer(peerSegments map[peer.ID][]string) error {
// 	for {
// 		select {
// 		case <-t.ctx.Done():
// 			return fmt.Errorf("任务已取消")
// 		case t.chNetworkTransfer <- peerSegments:
// 			logger.Debugf("成功将片段分配加入传输队列: segments=%d", len(peerSegments))
// 			return nil
// 		default:
// 			logger.Debugf("网络传输通道已满，等待重试: segments=%d", len(peerSegments))
// 			select {
// 			case <-time.After(100 * time.Millisecond):
// 				continue
// 			case <-t.ctx.Done():
// 				return fmt.Errorf("任务已取消")
// 			}
// 		}
// 	}
// }

// ForceSendClosest 强制触发发送最近的节点
// 发送最近的节点，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForceSendClosest(segmentId string) error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chSendClosest <- segmentId:

		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chSendClosest:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chSendClosest <- segmentId:
				return nil
			}
		}
	}

}

// ForceSegmentVerify 强制触发片段验证
// 验证已传输片段的完整性，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForceSegmentVerify() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chSegmentVerify <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chSegmentVerify:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chSegmentVerify <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceFileFinalize 强制触发文件完成处理
// 处理文件上传完成后的操作，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForceFileFinalize() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chFileFinalize <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chFileFinalize:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chFileFinalize <- struct{}{}:
				return nil
			}
		}
	}
}

// ForcePause 强制触发任务暂停
// 暂停当前上传任务，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForcePause() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chPause <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chPause:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chPause <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceCancel 强制触发任务取消
// 取消当前上传任务，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForceCancel() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chCancel <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chCancel:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chCancel <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceDelete 强制触发任务删除
// 删除当前上传任务及相关资源，如果通道已满则先清空再写入
// 返回值:
//   - error: 如果处理过程中发生错误，返回相应的错误信息
func (t *UploadTask) ForceDelete() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chDelete <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chDelete:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chDelete <- struct{}{}:
				return nil
			}
		}
	}
}

// NotifySegmentStatus 通知片段状态更新
// 向外部通知文件片段的处理状态，超时后记录警告日志
// 参数:
//   - status: 文件片段处理状态
func (t *UploadTask) NotifySegmentStatus(status *pb.UploadChan) {
	select {
	case t.chSegmentStatus <- status:
		return
	case <-time.After(100 * time.Millisecond):
		logger.Warnf("片段状态通知超时: taskID=%s", t.taskId)
	}
}

// NotifyTaskError 通知任务错误
// 向外部通知任务执行过程中的错误，超时后记录警告日志
// 参数:
//   - err: 错误信息
func (t *UploadTask) NotifyTaskError(err error) {
	select {
	case t.chError <- err:
		return
	case <-time.After(100 * time.Millisecond):
		logger.Warnf("任务错误通知超时: taskID=%s, err=%v", t.taskId, err)
	}
}
