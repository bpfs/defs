package bandwidth

import (
	"context"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func setupTestHosts(t *testing.T) (host.Host, host.Host) {
	h1, err := libp2p.New()
	assert.NoError(t, err)

	h2, err := libp2p.New()
	assert.NoError(t, err)

	return h1, h2
}

func TestBandwidthReportingAndRequest(t *testing.T) {
	h1, h2 := setupTestHosts(t)

	counter := CreateBandwidthCounter()
	time.Sleep(500 * time.Millisecond)

	RegisterBandwidthProtocol(h1, counter)

	// Connect the hosts
	err := h1.Connect(context.Background(), peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: h2.Addrs(),
	})
	assert.NoError(t, err)

	// Request bandwidth info
	info, err := RequestBandwidthInfo(h1, h2.ID())
	assert.NoError(t, err)
	assert.NotNil(t, info)
}

// func main() {
// 	// 创建一个LibP2P节点
// 	node, err := libp2p.New()
// 	if err != nil {
// 		fmt.Println(err)
// 		return
// 	}

// 	// 建立与给定对等点的连接
// 	conn, err := node.Network().DialPeer(context.Background(), "/ip4/127.0.0.1/tcp/12345")
// 	if err != nil {
// 		fmt.Println(err)
// 		return
// 	}

// 	// 获取连接的统计信息
// 	stats := conn.Stat()

// 	a := stats.NumStreams
// 	b := stats.Stats.Direction

// 	metrics.BandwidthCounter
// 	// https://github.com/libp2p/go-libp2p-core/blob/654214c1b3401c0363ef464ce9db7e1b04a7ef3f/metrics/reporter.go

// 	// 获取带宽
// 	fmt.Println(stats.Bandwidth)

// 	// 创建一个Bandwidth接口
// 	bw := bandwidth.New(node)

// 	// 开始测量带宽
// 	bw.Start(context.Background(), "/ip4/127.0.0.1/tcp/12345")

// 	// 获取带宽
// 	fmt.Println(bw.Bandwidth())

// 	// 停止测量带宽
// 	bw.Stop()

// }
