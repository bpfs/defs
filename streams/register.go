package streams

import (
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/protocol"
)

const (
	// DefaultSteamProtocol 默认 dep2p 流协议
	DefaultSteamProtocol = "/dep2p/stream/1.0.0"
)

// RegisterStreamHandler 注册流处理程序
// 参数:
//   - h: 主机，用于管理网络连接
//   - p: 协议ID，用于标识流协议
//   - handler: 流处理程序，用于处理网络流
func RegisterStreamHandler(h host.Host, p protocol.ID, handler network.StreamHandler) {
	if handler == nil {
		return
	}
	if p == "" {
		p = DefaultSteamProtocol
	}
	f := func(s network.Stream) {
		if h.ConnManager() != nil {
			h.ConnManager().Protect(s.Conn().RemotePeer(), string(p))
			defer h.ConnManager().Unprotect(s.Conn().RemotePeer(), string(p))
		}
		handler(s)
	}

	// SetStreamHandler 在主机的 Mux 上设置协议处理程序。
	h.SetStreamHandler(p, HandlerWithClose(f))
}
