package util

import "sync"

// waitForStatusChange 是一个通用的等待状态变化的函数。它接受一个条件变量和一个检查状态的函数。
// 它会阻塞调用它的goroutine直到checkStatus函数返回true。
func WaitForStatusChange(cond *sync.Cond, checkStatus func() bool) {
	cond.L.Lock()         // 加锁以保证条件检查和等待的原子性
	defer cond.L.Unlock() // 方法结束时解锁

	for !checkStatus() { // 检查状态，如果不符合期望，则等待
		cond.Wait() // 如果当前状态不是目标状态，等待状态变化的信号
	}
	// 当checkStatus返回true时，循环结束，继续执行后续操作
}
