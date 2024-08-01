package uploads

import (
	"fmt"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/network"
	"github.com/bpfs/defs/util"

	"github.com/bpfs/dep2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// getTargetNodes 获取目标节点列表。
// p2p：P2P网络对象。
// segmentInfo：文件片段信息。
// excludedPeers：需要排除的节点列表。
// 返回目标节点列表和可能的错误。
// func getTargetNodes(p2p *dep2p.DeP2P, segmentInfo *FileSegmentInfo, receiver peer.ID, excludedPeers []string) ([]peer.ID, error) {
// 	// 在流操作之前获取互斥锁
// 	network.StreamMutex.Lock()

// 	peerDistance := PeerDistanceReq{
// 		SegmentID:     segmentInfo.SegmentID, // 文件片段的唯一标识
// 		Size:          segmentInfo.Size,      // 分片大小，单位为字节
// 		IsRsCodes:     segmentInfo.IsRsCodes, // 标记该分片是否使用了纠删码技术
// 		ExcludedPeers: excludedPeers,         // 需要过滤的节点ID列表
// 	}

// 	res, err := network.SendStream(p2p, StreamPeerDistanceProtocol, "", receiver, peerDistance)
// 	if err != nil || res == nil || res.Code != 200 || res.Data == nil {
// 		return nil, fmt.Errorf("")
// 	}

// 	peerInfo := new(PeerInfo)
// 	if err := util.DecodeFromBytes(res.Data, peerInfo); err != nil {
// 		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
// 		return nil, fmt.Errorf("")
// 	}

// 	// 过滤排除的节点
// 	var targetNodes []peer.ID
// 	for _, peer := range peerInfo.IDs {
// 		if !contains(excludedPeers, peer.String()) {
// 			targetNodes = append(targetNodes, peer)
// 		}
// 	}

// 	if len(targetNodes) == 0 {
// 		return nil, fmt.Errorf("没有找到合适的目标节点")
// 	}

// 	return targetNodes, nil
// }

// sendSliceToNode 向目标节点发送文件片段。
// p2p：P2P网络对象。
// segmentInfo：文件片段信息。
// node：目标节点。
// sliceByte：文件片段的字节数据。
// networkReceivedChan：网络响应通道。
// 返回可能的错误。
func sendSliceToNode(p2p *dep2p.DeP2P, segmentInfo *FileSegmentInfo, node peer.ID, sliceByte []byte, networkReceived chan *NetworkResponse) error {
	// 准备发送请求的数据
	sendingToNetworkReq := SendingToNetworkReq{
		FileID:        segmentInfo.FileID,
		SegmentID:     segmentInfo.SegmentID,
		TotalSegments: segmentInfo.TotalSegments,
		Index:         segmentInfo.Index,
		IsRsCodes:     segmentInfo.IsRsCodes,
		SliceByte:     sliceByte,
	}

	network.StreamMutex.Lock()
	// 发送文件片段到目标节点
	res, err := network.SendStream(p2p, StreamSendingToNetworkProtocol, "", node, sendingToNetworkReq)
	if err != nil {
		logrus.Errorf("[%s]向节点 %s 发送数据失败: %v", debug.WhereAmI(), node.String(), err)
		return err
	}

	if res == nil {
		return fmt.Errorf("向节点 %s 发送数据失败", node.String())
	}

	// 处理响应数据
	if res.Code == 200 && res.Data != nil {
		sendingToNetwork := new(SendingToNetworkRes)
		if err := util.DecodeFromBytes(res.Data, sendingToNetwork); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		}

		networkReceived <- &NetworkResponse{
			Index:          segmentInfo.Index,   // 分片索引
			ReceiverPeerID: sendingToNetwork.ID, // 存储该文件片段的节点ID
		}
		return nil
	}

	return fmt.Errorf("目标节点 %s 响应错误码：%d", node.String(), res.Code)
}
