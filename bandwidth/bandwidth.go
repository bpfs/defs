package bandwidth

import (
	"context"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	metrics "github.com/libp2p/go-libp2p/core/metrics"
)

// CreateBandwidthCounter 创建并返回一个新的带宽计数器。
func CreateBandwidthCounter() *metrics.BandwidthCounter {
	return metrics.NewBandwidthCounter()
}

// SetupBandwidthReporting 配置节点以报告带宽使用情况。
func SetupBandwidthReporting(ctx context.Context, counter *metrics.BandwidthCounter) (host.Host, error) {
	return libp2p.New(libp2p.BandwidthReporter(counter))
}
