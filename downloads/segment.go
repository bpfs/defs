package downloads

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// FileSegment 描述一个文件分片的详细信息及其下载状态
type FileSegment struct {
	Index     int                   // 分片索引，表示该片段在文件中的顺序
	SegmentID string                // 文件片段的唯一标识
	Checksum  []byte                // 分片的校验和，用于校验分片数据的完整性和一致性
	IsRsCodes bool                  // 是否是纠删码片段
	Nodes     sync.Map              // 使用并发安全的 sync.Map 存储节点信息，键是节点ID (peer.ID)，值是节点是否可用 (bool)
	Status    SegmentDownloadStatus // 下载状态
}

// AddNode 向 Nodes 中添加节点
// 参数：
//   - nodeID: peer.ID 节点的ID
//   - active: bool 节点是否可用
func (fs *FileSegment) AddNode(nodeID peer.ID, active bool) {
	if nodeID != "" {
		fs.Nodes.Store(nodeID, active)
	}
}

// GetNodes 返回 Nodes 中所有节点的副本
// 返回值：
//   - map[peer.ID]bool: 所有节点的副本，键是节点ID，值是节点是否可用
func (fs *FileSegment) GetNodes() map[peer.ID]bool {
	nodes := make(map[peer.ID]bool)
	fs.Nodes.Range(func(key, value interface{}) bool {
		nodeID := key.(peer.ID)
		active := value.(bool)
		nodes[nodeID] = active
		return true
	})
	return nodes
}

// DeleteNode 从 Nodes 中删除节点
// 参数：
//   - nodeID: peer.ID 节点的ID
func (fs *FileSegment) DeleteNode(nodeID peer.ID) {
	if nodeID != "" {
		fs.Nodes.Delete(nodeID)
	}
}

// NodeExists 检查节点是否存在
// 参数：
//   - nodeID: peer.ID 节点的ID
//
// 返回值：
//   - bool: 如果节点存在，返回 true；否则返回 false
func (fs *FileSegment) NodeExists(nodeID peer.ID) bool {
	_, exists := fs.Nodes.Load(nodeID)
	return exists
}

// GetNodeActive 获取节点的可用状态
// 参数：
//   - nodeID: peer.ID 节点的ID
//
// 返回值：
//   - bool: 节点是否可用
//   - bool: 如果节点存在，返回 true；否则返回 false
func (fs *FileSegment) GetNodeActive(nodeID peer.ID) (bool, bool) {
	value, exists := fs.Nodes.Load(nodeID)
	if !exists {
		return false, false
	}
	return value.(bool), true
}

// UpdateNodes 更新文件片段的节点信息
// 参数：
//   - peers: []peer.ID 文件片段所在的节点ID。
//
// 默认情况下，所有提供的节点ID都将标记为可用。如果节点已存在但标记为不可用，会更新其状态为可用。
func (fs *FileSegment) UpdateNodes(peers []peer.ID) {
	if len(peers) != 0 {
		for _, peerID := range peers {
			// 检查节点是否已存在
			if value, exists := fs.Nodes.Load(peerID); exists {
				// 如果节点存在且不可用，更新为可用
				if !value.(bool) {
					fs.Nodes.Store(peerID, true)
				}
			} else {
				// 如果节点不存在，添加新的节点，标记为可用
				fs.Nodes.Store(peerID, true)
			}
		}
	}
}

// SetStatus 设置文件片段的下载状态
// 参数：
//   - status: SegmentDownloadStatus 文件片段的下载状态
func (fs *FileSegment) SetStatus(status SegmentDownloadStatus) {
	fs.Status = status
}

// IsStatus 检查文件片段的状态
// 参数：
//   - status: SegmentDownloadStatus 文件片段的下载状态
//
// 返回值：
//   - bool: 如果文件片段的状态为指定状态，返回 true；否则返回 false
func (fs *FileSegment) IsStatus(status SegmentDownloadStatus) bool {
	return fs.Status == status
}

// IsCompleted 检查文件片段是否已经下载完成
// 返回值：
//   - bool: 文件片段是否下载完成
func (fs *FileSegment) IsCompleted() bool {
	return fs.Status == SegmentStatusCompleted
}

// IsNodesEmpty 检查节点信息是否为空
// 返回值：
//   - bool: 节点信息是否为空
func (fs *FileSegment) IsNodesEmpty() bool {
	isEmpty := true
	fs.Nodes.Range(func(key, value interface{}) bool {
		isEmpty = false
		return false
	})
	return isEmpty
}

// HasActiveNodes 检查是否存在可用节点
// 返回值：
//   - bool: 是否存在可用节点
func (fs *FileSegment) HasActiveNodes() bool {
	hasActiveNodes := false
	fs.Nodes.Range(func(key, value interface{}) bool {
		if value.(bool) {
			hasActiveNodes = true
			return false
		}
		return true
	})
	return hasActiveNodes
}

// IsNodeActive 检查指定节点是否可用
// 参数：
//   - nodeID: peer.ID 节点的ID
//
// 返回值：
//   - bool: 节点是否可用
func (fs *FileSegment) IsNodeActive(nodeID peer.ID) bool {
	active, exists := fs.GetNodeActive(nodeID)
	return exists && active
}

// SetNodeInactive 将指定节点设置为不可用
// 参数：
//   - nodeID: peer.ID 节点的ID
func (fs *FileSegment) SetNodeInactive(nodeID peer.ID) {
	if nodeID != "" {
		fs.Nodes.Store(nodeID, false)
	}
}

// GetSegmentID 返回文件分片的唯一标识
//
// 返回值：
//   - string: 文件分片的唯一标识
func (fs *FileSegment) GetSegmentID() string {
	return fs.SegmentID
}

// HasNodes 检查节点信息是否存在
// 返回值：
//   - bool: 是否存在节点信息
func (fs *FileSegment) HasNodes() bool {
	hasNodes := false
	fs.Nodes.Range(func(key, value interface{}) bool {
		hasNodes = true
		return false
	})
	return hasNodes
}

// GetSegmentID 获取文件片段的唯一标识
// 参数：
//   - index: int 文件片段的索引
//
// 返回值：
//   - string 文件片段的唯一标识
func (df *DownloadFile) GetSegmentID(index int) string {
	if segment, ok := df.GetSegment(index); ok {
		return segment.GetSegmentID()
	}
	return ""
}

// IsRsCodes 检查文件片段是否是纠删码片段
// 参数：
//   - index: int 文件片段的索引
//
// 返回值：
//   - bool 是否是纠删码片段
func (df *DownloadFile) IsRsCodes(index int) bool {
	if segment, ok := df.GetSegment(index); ok {
		return segment.IsRsCodes
	}
	return false
}
