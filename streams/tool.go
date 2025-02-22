package streams

import (
	"bytes"
	fmt "fmt"
	"io"
	"runtime"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
)

// panicTrace 跟踪恐慌堆栈信息。
func panicTrace(kb int) []byte {
	s := []byte("/src/runtime/panic.go")
	e := []byte("\ngoroutine ")
	line := []byte("\n")
	stack := make([]byte, kb<<10) //4KB
	length := runtime.Stack(stack, true)
	start := bytes.Index(stack, s)
	stack = stack[start:length]
	start = bytes.Index(stack, line) + 1
	stack = stack[start:]
	end := bytes.LastIndex(stack, line)
	if end != -1 {
		stack = stack[:end]
	}
	end = bytes.Index(stack, e)
	if end != -1 {
		stack = stack[:end]
	}
	stack = bytes.TrimRight(stack, "\n")
	return stack
}

// EOFTimeout 是等待成功观察流上的 EOF 的最长时间。 默认为 60 秒。
var EOFTimeout = time.Second * 60

// ErrExpectedEOF 当我们在期望 EOF 的情况下读取数据时返回。
var ErrExpectedEOF = fmt.Errorf("期望 EOF 时读取数据")

// AwaitEOF 等待给定流上的 EOF，如果失败则返回错误。
// 它最多等待 EOFTimeout（默认为 1 分钟），然后重置流。
func AwaitEOF(s network.Stream) error {
	// 所以我们不会永远等待
	// 设置流的超时，防止无限等待
	_ = s.SetDeadline(time.Now().Add(EOFTimeout)) // 设定截止日期

	// 必须观察到 EOF。 否则，可能会导致流泄漏。
	// 在理论上，我们应该在 SendMessage 返回之前执行此操作，
	// 因为在我们看到 EOF 之前，消息还没有真正发送，
	// 但我们实际上并不知道对方正在使用什么协议。
	n, err := s.Read([]byte{0})
	// 如果流中还有数据（n>0）或者没有发生错误（err==nil），那么将流重置，并返回一个预期EOF的错误。
	if n > 0 || err == nil {
		// 关闭流的两端。 用它来告诉远端挂断电话并离开。
		_ = s.Reset()
		// ErrExpectedEOF 当我们在期望 EOF 的情况下读取数据时返回。
		return ErrExpectedEOF
	}
	// EOF 是 Read 在没有更多输入可用时返回的错误。
	// 如果错误不是EOF，那么重置流，并返回这个错误。
	if err != io.EOF {
		// 关闭流的两端。 用它来告诉远端挂断电话并离开。
		_ = s.Reset()
		return err
	}
	// 如果没有错误，那么关闭流，并返回nil
	return s.Close()
}

func headerSafe(path []byte) []byte {
	l := len(path) + 1 // + \n
	buf := make([]byte, l+1)
	buf[0] = byte(l)
	copy(buf[1:], path)
	buf[l] = '\n'
	return buf
}
