// Package net 提供了DeFS网络相关的测试实现
package net

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/dep2p/go-dep2p/p2p/security/noise"
	tls "github.com/dep2p/go-dep2p/p2p/security/tls"
	"github.com/stretchr/testify/require"
)

// testEnv 定义测试环境结构体
type testEnv struct {
	host host.Host  // libp2p主机实例
	t    *testing.T // 测试实例
}

// newTestEnv 创建一个新的测试环境
// 参数:
//   - t: 测试实例
//
// 返回值:
//   - *testEnv: 测试环境实例
func newTestEnv(t *testing.T) *testEnv {
	// 配置libp2p选项
	opts := []dep2p.Option{
		dep2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), // 监听本地随机端口
		dep2p.Security(tls.ID, tls.New),                 // 启用TLS安全传输
		dep2p.Security(noise.ID, noise.New),             // 启用Noise安全传输
		dep2p.DisableRelay(),                            // 禁用中继功能
	}

	// 创建新的libp2p主机
	h, err := dep2p.New(opts...)
	require.NoError(t, err)

	// 注册清理函数
	t.Cleanup(func() { h.Close() })

	return &testEnv{
		host: h,
		t:    t,
	}
}

// TestHandshake 测试握手功能
// 参数:
//   - t: 测试实例
func TestHandshake(t *testing.T) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建两个测试环境
	env1 := newTestEnv(t)
	env2 := newTestEnv(t)

	// 测试基本握手功能
	t.Run("基本握手功能", func(t *testing.T) {
		// 为两个环境注册握手协议
		RegisterHandshakeProtocol(env1.host)
		RegisterHandshakeProtocol(env2.host)

		// 创建测试节点
		testPeers := make([]peer.AddrInfo, 3)
		for i := 0; i < 3; i++ {
			env := newTestEnv(t)
			testPeers[i] = peer.AddrInfo{
				ID:    env.host.ID(),
				Addrs: env.host.Addrs(),
			}
			// 将测试节点添加到env1的节点存储中
			env1.host.Peerstore().AddAddrs(env.host.ID(), env.host.Addrs(), time.Hour)
		}

		// 准备env2的节点信息
		host2Info := peer.AddrInfo{
			ID:    env2.host.ID(),
			Addrs: env2.host.Addrs(),
		}

		// 执行握手操作
		peers, err := Handshake(ctx, env1.host, host2Info)
		require.NoError(t, err)
		require.NotEmpty(t, peers)

		// 验证握手结果
		addrs := env2.host.Peerstore().Addrs(env1.host.ID())
		require.NotEmpty(t, addrs)
	})

	// 测试节点数量限制
	t.Run("节点数量限制", func(t *testing.T) {
		// 添加超过限制数量的测试节点
		for i := 0; i < maxPeersToShare*2; i++ {
			env := newTestEnv(t)
			env1.host.Peerstore().AddAddrs(env.host.ID(), env.host.Addrs(), time.Hour)
		}

		// 准备env2的节点信息
		host2Info := peer.AddrInfo{
			ID:    env2.host.ID(),
			Addrs: env2.host.Addrs(),
		}

		// 执行握手并验证返回节点数量不超过限制
		peers, err := Handshake(ctx, env1.host, host2Info)
		require.NoError(t, err)
		require.LessOrEqual(t, len(peers), maxPeersToShare)
	})

	// 测试错误处理
	t.Run("错误处理", func(t *testing.T) {
		// 创建一个不存在的节点ID
		testPeerID, err := peer.Decode("12D3KooWGFLL95aAMtaKXQFN2pkhZ9RdBF6d2V5YWzxK6yqiGs6X")
		require.NoError(t, err)

		// 创建一个不可达的地址
		addr, err := multiaddr.NewMultiaddr("/ip4/192.0.2.1/tcp/1234")
		require.NoError(t, err)

		// 构造无效的节点信息
		invalidInfo := peer.AddrInfo{
			ID:    testPeerID,
			Addrs: []multiaddr.Multiaddr{addr},
		}

		// 尝试与不可达节点握手
		_, err = Handshake(ctx, env1.host, invalidInfo)
		require.Error(t, err)
		require.Contains(t, err.Error(), "连接失败")
	})

	// 测试无效节点ID
	t.Run("无效节点ID", func(t *testing.T) {
		// 构造带有空节点ID的信息
		invalidInfo := peer.AddrInfo{
			ID:    "",
			Addrs: nil,
		}

		// 尝试与无效节点握手
		_, err := Handshake(ctx, env1.host, invalidInfo)
		require.Error(t, err)
	})

	// 测试超时处理
	t.Run("超时处理", func(t *testing.T) {
		// 创建短超时上下文
		shortCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// 创建并关闭测试节点
		env := newTestEnv(t)
		env.host.Close()

		// 构造已关闭节点的信息
		info := peer.AddrInfo{
			ID:    env.host.ID(),
			Addrs: env.host.Addrs(),
		}

		// 尝试与关闭的节点握手
		_, err := Handshake(shortCtx, env1.host, info)
		require.Error(t, err)
	})
}

// TestHandshakeProtocolRegistration 测试握手协议注册功能
// 参数:
//   - t: 测试实例
func TestHandshakeProtocolRegistration(t *testing.T) {
	env := newTestEnv(t)

	// 测试协议注册
	t.Run("协议注册", func(t *testing.T) {
		RegisterHandshakeProtocol(env.host)
		protocols := env.host.Mux().Protocols()
		require.Contains(t, protocols, protocol.ID(HandshakeProtocol))
	})

	// 测试重复注册
	t.Run("重复注册", func(t *testing.T) {
		// 多次注册同一协议
		RegisterHandshakeProtocol(env.host)
		RegisterHandshakeProtocol(env.host)

		// 验证协议只被注册一次
		protocols := env.host.Mux().Protocols()
		count := 0
		for _, p := range protocols {
			if p == protocol.ID(HandshakeProtocol) {
				count++
			}
		}
		require.Equal(t, 1, count)
	})
}

// TestConcurrentHandshakes 测试并发握手功能
// 参数:
//   - t: 测试实例
func TestConcurrentHandshakes(t *testing.T) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建主测试环境
	env1 := newTestEnv(t)
	RegisterHandshakeProtocol(env1.host)

	// 定义并发数量
	const numHosts = 5
	done := make(chan struct{}, numHosts)

	// 并发执行多个握手操作
	for i := 0; i < numHosts; i++ {
		go func() {
			env := newTestEnv(t)
			RegisterHandshakeProtocol(env.host)

			// 准备节点信息
			info := peer.AddrInfo{
				ID:    env.host.ID(),
				Addrs: env.host.Addrs(),
			}

			// 执行握手
			_, err := Handshake(ctx, env1.host, info)
			require.NoError(t, err)
			done <- struct{}{}
		}()
	}

	// 等待所有握手完成
	for i := 0; i < numHosts; i++ {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("握手超时")
		}
	}
}
