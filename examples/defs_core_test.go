package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/bpfs/defs/utils/logger"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestMassiveConnections 测试大量节点同时连接的情况
func TestMassiveConnections(t *testing.T) {
	// 创建目标节点（被连接的节点）
	//targetHost, err := createTestHost(t, 4001)
	targetAddr, err := parseAddr("/ip4/119.96.25.188/tcp/4001/p2p/12D3KooWQZfF5DCUVQxhr9ye5sKZxHj5YLBPjWBEwhEaSnpYysSQ")
	require.NoError(t, err)
	//defer targetHost.Close()

	fmt.Printf("\n目标节点创建成功 - ID: %s\n", targetAddr.ID.String())
	fmt.Printf("目标节点地址: %v\n\n", targetAddr.Addrs)

	// 测试参数
	const (
		totalNodes     = 1000 // 总节点数
		batchSize      = 50   // 每批并发连接数
		connectTimeout = 10 * time.Second
	)

	// 修改：添加连接状态监控
	connectedPeers := make(map[peer.ID]struct{})
	var connectedPeersMutex sync.Mutex

	// targetHost.Network().Notify(&network.NotifyBundle{
	// 	ConnectedF: func(n network.Network, conn network.Conn) {
	// 		connectedPeersMutex.Lock()
	// 		connectedPeers[conn.RemotePeer()] = struct{}{}
	// 		connectedPeersMutex.Unlock()
	// 	},
	// 	DisconnectedF: func(n network.Network, conn network.Conn) {
	// 		connectedPeersMutex.Lock()
	// 		delete(connectedPeers, conn.RemotePeer())
	// 		connectedPeersMutex.Unlock()
	// 	},
	// })

	// 修改：增强结果统计
	type enhancedResult struct {
		connectionResult
		peerID peer.ID
	}

	// 使用 WaitGroup 来等待所有 goroutine 完成
	var wg sync.WaitGroup
	results := make(chan enhancedResult, totalNodes)

	// 创建目标节点的 AddrInfo
	targetInfo := peer.AddrInfo{
		ID:    targetAddr.ID,
		Addrs: targetAddr.Addrs,
	}

	fmt.Printf("开始测试 %d 个节点的连接...\n", totalNodes)
	fmt.Printf("每批并发数: %d\n\n", batchSize)

	// 按批次创建节点并尝试连接
	for batch := 0; batch < totalNodes; batch += batchSize {
		batchEnd := batch + batchSize
		if batchEnd > totalNodes {
			batchEnd = totalNodes
		}

		fmt.Printf("正在处理第 %d 到 %d 个节点...\n", batch+1, batchEnd)

		for i := batch; i < batchEnd; i++ {
			wg.Add(1)
			go func(nodeIndex int) {
				defer wg.Done()

				// 为每个测试节点创建上下文
				ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
				defer cancel()

				// 创建测试节点
				sourceHost, err := createTestHost(t, 0) // 使用随机端口
				if err != nil {
					results <- enhancedResult{
						connectionResult: connectionResult{
							nodeIndex: nodeIndex,
							success:   false,
							error:     fmt.Errorf("创建节点失败: %v", err),
						},
					}
					return
				}
				defer sourceHost.Close()

				// 尝试连接到目标节点
				startTime := time.Now()
				err = sourceHost.Connect(ctx, targetInfo)
				duration := time.Since(startTime)

				results <- enhancedResult{
					connectionResult: connectionResult{
						nodeIndex: nodeIndex,
						success:   err == nil,
						error:     err,
						duration:  duration,
					},
					peerID: sourceHost.ID(),
				}
			}(i)
		}

		// 每批次完成后等待一小段时间
		time.Sleep(100 * time.Millisecond)
	}

	// 等待所有 goroutine 完成后再关闭通道
	go func() {
		wg.Wait()
		close(results)
	}()

	// 修改：增强统计信息
	type connectionStats struct {
		attempts    int
		successes   int
		failures    int
		activeConns int
		errorTypes  map[string]int
		durations   []time.Duration
	}

	stats := connectionStats{
		errorTypes: make(map[string]int),
		durations:  make([]time.Duration, 0, totalNodes),
	}

	// 处理结果
	fmt.Printf("\n详细连接测试结果:\n")
	fmt.Printf("----------------------------------------\n")
	for result := range results {
		stats.attempts++

		if result.success {
			stats.successes++
			stats.durations = append(stats.durations, result.duration)
		} else {
			stats.failures++
			errorType := categorizeError(result.error)
			stats.errorTypes[errorType]++
			fmt.Printf("节点 %d (%s) 连接失败: %v\n",
				result.nodeIndex,
				result.peerID.String()[:12],
				result.error)
		}
	}

	// 获取最终的活动连接数
	connectedPeersMutex.Lock()
	stats.activeConns = len(connectedPeers)
	connectedPeersMutex.Unlock()

	// 打印增强的统计信息
	fmt.Printf("\n综合统计信息:\n")
	fmt.Printf("----------------------------------------\n")
	fmt.Printf("总尝试连接数: %d\n", stats.attempts)
	fmt.Printf("连接成功数: %d (%.2f%%)\n",
		stats.successes,
		float64(stats.successes)/float64(stats.attempts)*100)
	fmt.Printf("连接失败数: %d (%.2f%%)\n",
		stats.failures,
		float64(stats.failures)/float64(stats.attempts)*100)
	fmt.Printf("当前活动连接数: %d\n", stats.activeConns)

	if len(stats.durations) > 0 {
		sort.Slice(stats.durations, func(i, j int) bool {
			return stats.durations[i] < stats.durations[j]
		})

		fmt.Printf("\n连接时间统计:\n")
		fmt.Printf("最短连接时间: %v\n", stats.durations[0])
		fmt.Printf("最长连接时间: %v\n", stats.durations[len(stats.durations)-1])
		fmt.Printf("中位数连接时间: %v\n",
			stats.durations[len(stats.durations)/2])

		var total time.Duration
		for _, d := range stats.durations {
			total += d
		}
		fmt.Printf("平均连接时间: %v\n", total/time.Duration(len(stats.durations)))
	}

	if len(stats.errorTypes) > 0 {
		fmt.Printf("\n错误类型统计:\n")
		for errType, count := range stats.errorTypes {
			fmt.Printf("%s: %d 次 (%.2f%%)\n",
				errType,
				count,
				float64(count)/float64(stats.failures)*100)
		}
	}
}

// connectionResult 存储单个连接测试的结果
type connectionResult struct {
	nodeIndex int
	success   bool
	error     error
	duration  time.Duration
}

// createTestHost 创建测试用的 libp2p host
func createTestHost(t *testing.T, port int) (host.Host, error) {
	var opts []libp2p.Option

	// 配置监听地址
	if port > 0 {
		opts = append(opts, libp2p.ListenAddrStrings(
			// TCP 监听地址 - 支持IPv4和IPv6
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),   // IPv4 所有网络接口
			fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port), // IPv4 本地回环地址
			fmt.Sprintf("/ip6/::/tcp/%d", port),        // IPv6 所有网络接口
			fmt.Sprintf("/ip6/::1/tcp/%d", port),       // IPv6 本地回环地址

			// QUIC-v1 监听地址 - 基于UDP的传输协议
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port),   // IPv4 QUIC 所有接口
			fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", port), // IPv4 QUIC 本地回环
			fmt.Sprintf("/ip6/::/udp/%d/quic-v1", port),        // IPv6 QUIC 所有接口
			fmt.Sprintf("/ip6/::1/udp/%d/quic-v1", port),       // IPv6 QUIC 本地回环

			// WebTransport 监听地址 - 基于QUIC的Web传输协议
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1/webtransport", port), // IPv4 WebTransport
			fmt.Sprintf("/ip6/::/udp/%d/quic-v1/webtransport", port),      // IPv6 WebTransport
		))
	}

	// 配置连接管理器
	cm, err := connmgr.NewConnManager(
		1000,                                 // 最小连接数
		4000,                                 // 最大连接数
		connmgr.WithGracePeriod(time.Minute), // 宽限期
		connmgr.WithEmergencyTrim(true),
	)
	require.NoError(t, err)
	opts = append(opts, libp2p.ConnectionManager(cm))

	// 创建 host
	return libp2p.New(opts...)
}

// categorizeError 对错误进行分类
func categorizeError(err error) string {
	if err == nil {
		return "成功"
	}

	errStr := err.Error()
	switch {
	case contains(errStr, "context deadline exceeded"):
		return "连接超时"
	case contains(errStr, "connection refused"):
		return "连接被拒绝"
	case contains(errStr, "no route to host"):
		return "无法路由到主机"
	case contains(errStr, "connection reset"):
		return "连接被重置"
	case contains(errStr, "dial backoff"):
		return "拨号退避"
	case contains(errStr, "max dial attempts exceeded"):
		return "超过最大拨号尝试次数"
	case contains(errStr, "resource limit exceeded"):
		return "资源限制超出"
	default:
		return "其他错误"
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr
}

func parseAddr(peerAddr string) (*peer.AddrInfo, error) {
	maddr, err := multiaddr.NewMultiaddr(peerAddr)
	if err != nil {
		logger.Errorf("解析存储节点地址失败: %v", err)
		return nil, err
	}
	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		logger.Errorf("从地址 %s 提取节点信息失败: %v", peerInfo.String(), err)
		return peerInfo, err
	}
	return peerInfo, err
}
