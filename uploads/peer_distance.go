// peer_distance.go
// 处理有关节点间距离的逻辑，包括计算距离和寻找最近的节点。

package uploads

import (
	"errors"
	"math/big"
	"sort"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Distance 计算两个ID之间的XOR距离。
func Distance(id1, id2 peer.ID) (*big.Int, error) {
	b1, err := id1.MarshalBinary()
	if err != nil {
		return nil, err
	}
	b2, err := id2.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if len(b1) != len(b2) {
		return nil, errors.New("ID长度不一致")
	}

	dist := new(big.Int)
	for i := range b1 {
		byteDist := big.NewInt(int64(b1[i] ^ b2[i]))
		byteDist.Lsh(byteDist, uint(8*(len(b1)-i-1))) // 左移以保留原始位序
		dist.Or(dist, byteDist)
	}

	return dist, nil
}

// PeerDistanceInfo 包含单个peer的距离信息
type PeerDistanceInfo struct {
	Peer peer.ID
	Dist *big.Int
}

// PeerInfo 表示一组网络节点的基本信息，包括节点ID数组、与给定ID的距离，以及最近的节点是否为本地节点的标志。
type PeerInfo struct {
	IDs     []peer.ID // 节点的ID数组
	IsLocal bool      // 是否有本地节点为最近节点的标志
}

// FindNearestPeer 根据给定的目标ID和一组节点ID，找到并返回按距离排序的节点信息数组。
// func FindNearestPeer(targetID []byte, peers []peer.ID) (*PeerInfo, error) {
func FindNearestPeer(targetID peer.ID, peers []peer.ID) (*PeerInfo, error) {
	if len(targetID) == 0 || len(peers) == 0 {
		return nil, errors.New("无效的输入参数")
	}

	var distances []PeerDistanceInfo
	for _, peerID := range peers {
		dist, err := Distance(targetID, peerID)
		if err != nil {
			return nil, err
		}
		distances = append(distances, PeerDistanceInfo{Peer: peerID, Dist: dist})
	}

	// 按距离排序
	sort.Slice(distances, func(i, j int) bool {
		return distances[i].Dist.Cmp(distances[j].Dist) < 0
	})

	var sortedIDs []peer.ID
	for _, di := range distances {
		sortedIDs = append(sortedIDs, di.Peer)
	}

	// 假设本地节点不在排序后的节点列表中
	return &PeerInfo{IDs: sortedIDs, IsLocal: false}, nil
}
