package bandwidth

import (
	"context"
	"encoding/json"
	"io/ioutil"

	"github.com/libp2p/go-libp2p/core/host"
	metrics "github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const bandwidthProtocolID = protocol.ID("/bandwidth/1.0.0")

type BandwidthMessage struct {
	TotalIn  int64
	TotalOut int64
}

func bandwidthRequestHandler(s network.Stream, counter *metrics.BandwidthCounter) {
	defer s.Close()

	stats := counter.GetBandwidthTotals()
	message := BandwidthMessage{
		TotalIn:  stats.TotalIn,
		TotalOut: stats.TotalOut,
	}

	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	s.Write(data)
}

// RegisterBandwidthProtocol 注册带宽协议处理程序。
func RegisterBandwidthProtocol(h host.Host, counter *metrics.BandwidthCounter) {
	h.SetStreamHandler(bandwidthProtocolID, func(s network.Stream) {
		bandwidthRequestHandler(s, counter)
	})
}

// RequestBandwidthInfo 从目标主机请求带宽信息。
func RequestBandwidthInfo(h host.Host, targetPeer peer.ID) (*BandwidthMessage, error) {
	s, err := h.NewStream(context.Background(), targetPeer, bandwidthProtocolID)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	data, err := ioutil.ReadAll(s)
	if err != nil {
		return nil, err
	}

	var message BandwidthMessage
	err = json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}

	return &message, nil
}
