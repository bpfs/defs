package downloads

import (
	"fmt"
	"time"

	"github.com/bpfs/defs/v2/pb"
)

// ForceSegmentIndex 强制触发片段索引请求
// 请求文件片段的索引信息，如果通道已满则先清空再写入
func (t *DownloadTask) ForceSegmentIndex() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chSegmentIndex <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chSegmentIndex:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chSegmentIndex <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceSegmentProcess 强制触发片段处理
// 将文件片段整合并写入队列，如果通道已满则先清空再写入
func (t *DownloadTask) ForceSegmentProcess() error {
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
func (t *DownloadTask) ForceNodeDispatch() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chNodeDispatch <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chNodeDispatch:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chNodeDispatch <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceNetworkTransfer 强制触发网络传输
// 向目标节点传输文件片段，支持重试机制
func (t *DownloadTask) ForceNetworkTransfer(item *NetworkTransferItem) error {
	for {
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case t.chNetworkTransfer <- item:
			logger.Debugf("成功将片段分配加入传输队列: segments=%d", len(item.Segments))
			return nil
		default:
			logger.Debugf("网络传输通道已满，等待重试: segments=%d", len(item.Segments))
			select {
			case <-time.After(100 * time.Millisecond):
				continue
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			}
		}
	}
}

// ForceSegmentVerify 强制触发片段验证
// 验证已传输片段的完整性，如果通道已满则先清空再写入
func (t *DownloadTask) ForceSegmentVerify() error {
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

// ForceSegmentMerge 强制触发片段合并
// 合并已下载的文件片段，如果通道已满则先清空再写入
func (t *DownloadTask) ForceSegmentMerge() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("任务已取消")
	case t.chSegmentMerge <- struct{}{}:
		return nil
	default:
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("任务已取消")
		case <-t.chSegmentMerge:
			select {
			case <-t.ctx.Done():
				return fmt.Errorf("任务已取消")
			case t.chSegmentMerge <- struct{}{}:
				return nil
			}
		}
	}
}

// ForceFileFinalize 强制触发文件完成处理
// 处理文件下载完成后的操作，如果通道已满则先清空再写入
func (t *DownloadTask) ForceFileFinalize() error {
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
// 暂停当前下载任务，如果通道已满则先清空再写入
func (t *DownloadTask) ForcePause() error {
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
// 取消当前下载任务，如果通道已满则先清空再写入
func (t *DownloadTask) ForceCancel() error {
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
// 删除当前下载任务及相关资源，如果通道已满则先清空再写入
func (t *DownloadTask) ForceDelete() error {
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
func (t *DownloadTask) NotifySegmentStatus(status *pb.DownloadChan) {
	select {
	case t.chSegmentStatus <- status:
		return
	case <-time.After(100 * time.Millisecond):
		logger.Warnf("片段状态通知超时: taskID=%s", t.taskId)
	}
}

// NotifyTaskError 通知任务错误
// 向外部通知任务执行过程中的错误，超时后记录警告日志
func (t *DownloadTask) NotifyTaskError(err error) {
	select {
	case t.chError <- err:
		return
	case <-time.After(100 * time.Millisecond):
		logger.Warnf("任务错误通知超时: taskID=%s, err=%v", t.taskId, err)
	}
}

// 通用异步错误处理封装
func (t *DownloadTask) safeHandle(fn func() error) {
	go func() {
		if err := fn(); err != nil {
			select {
			case t.chError <- err:
			case <-t.ctx.Done():
			}
		}
	}()
}
