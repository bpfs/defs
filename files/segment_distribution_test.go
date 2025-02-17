package files

import (
	"testing"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/stretchr/testify/assert"
)

// TestNewSegmentDistribution 测试创建新的分片分配管理器
func TestNewSegmentDistribution(t *testing.T) {
	// 创建新实例
	sd := NewSegmentDistribution()

	// 验证实例是否正确初始化
	assert.NotNil(t, sd, "分片分配管理器不应为nil")
	assert.NotNil(t, sd.list, "内部列表不应为nil")
	length := sd.GetLength()
	assert.Equal(t, 0, length, "新创建的管理器长度应为0")
}

// TestAddDistribution 测试添加分片分配映射
func TestAddDistribution(t *testing.T) {
	sd := NewSegmentDistribution()

	// 准备测试数据
	peerID1 := peer.ID("peer1")
	peerInfo1 := peer.AddrInfo{
		ID: peerID1,
		// Addrs 可以为空，因为测试不需要实际的网络地址
	}
	segments1 := []string{"segment1", "segment2", "segment3"}

	// 添加分配
	sd.AddDistribution(peerInfo1, segments1)

	// 验证长度
	length := sd.GetLength()
	assert.Equal(t, 1, length, "添加一个分配后长度应为1")

	// 获取并验证内容
	nextDist, ok := sd.GetNextDistribution()
	assert.True(t, ok, "应成功获取分配")
	assert.Equal(t, peerInfo1, nextDist.PeerInfo, "获取的节点信息应与添加的相同")
	assert.Equal(t, segments1, nextDist.Segments, "获取的分片列表应与添加的相同")
}

// TestGetNextDistribution 测试获取下一个分配
func TestGetNextDistribution(t *testing.T) {
	sd := NewSegmentDistribution()

	// 准备测试数据
	peerID1 := peer.ID("peer1")
	peerInfo1 := peer.AddrInfo{ID: peerID1}
	segments1 := []string{"segment1", "segment2"}

	peerID2 := peer.ID("peer2")
	peerInfo2 := peer.AddrInfo{ID: peerID2}
	segments2 := []string{"segment3", "segment4"}

	// 添加两个分配
	sd.AddDistribution(peerInfo1, segments1)
	sd.AddDistribution(peerInfo2, segments2)

	// 验证初始长度
	assert.Equal(t, 2, sd.GetLength(), "添加两个分配后长度应为2")

	// 获取第一个分配
	nextDist1, ok1 := sd.GetNextDistribution()
	assert.True(t, ok1, "应成功获取第一个分配")
	assert.Equal(t, peerInfo1, nextDist1.PeerInfo, "第一个分配的节点信息应匹配")
	assert.Equal(t, segments1, nextDist1.Segments, "第一个分配的分片列表应匹配")
	assert.Equal(t, 1, sd.GetLength(), "获取一个后长度应为1")

	// 获取第二个分配
	nextDist2, ok2 := sd.GetNextDistribution()
	assert.True(t, ok2, "应成功获取第二个分配")
	assert.Equal(t, peerInfo2, nextDist2.PeerInfo, "第二个分配的节点信息应匹配")
	assert.Equal(t, segments2, nextDist2.Segments, "第二个分配的分片列表应匹配")
	assert.Equal(t, 0, sd.GetLength(), "获取两个后长度应为0")

	// 尝试从空列表获取
	nextDist3, ok3 := sd.GetNextDistribution()
	assert.False(t, ok3, "从空列表获取应返回false")
	assert.Nil(t, nextDist3, "从空列表获取应返回nil")
}

// TestClear 测试清空分片分配列表
func TestClear(t *testing.T) {
	sd := NewSegmentDistribution()

	// 准备测试数据
	peerID1 := peer.ID("peer1")
	peerInfo1 := peer.AddrInfo{ID: peerID1}
	segments1 := []string{"segment1", "segment2"}

	// 添加测试数据
	sd.AddDistribution(peerInfo1, segments1)

	// 验证添加成功
	assert.Equal(t, 1, sd.GetLength(), "添加后长度应为1")

	// 清空列表
	sd.Clear()

	// 验证清空成功
	assert.Equal(t, 0, sd.GetLength(), "清空后长度应为0")

	// 验证可以继续添加
	sd.AddDistribution(peerInfo1, segments1)
	assert.Equal(t, 1, sd.GetLength(), "清空后重新添加的长度应为1")
}

// TestConcurrentAccess 测试并发访问安全性
func TestConcurrentAccess(t *testing.T) {
	sd := NewSegmentDistribution()

	// 并发添加
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			peerID := peer.ID(string(rune(id)))
			peerInfo := peer.AddrInfo{ID: peerID}
			segments := []string{string(rune(id))}
			sd.AddDistribution(peerInfo, segments)
			done <- true
		}(i)
	}

	// 等待所有添加完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证长度
	assert.Equal(t, 10, sd.GetLength(), "并发添加后长度应为10")

	// 并发获取
	results := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, ok := sd.GetNextDistribution()
			results <- ok
		}()
	}

	// 统计成功获取的数量
	successCount := 0
	for i := 0; i < 10; i++ {
		if <-results {
			successCount++
		}
	}

	assert.Equal(t, 10, successCount, "应该成功获取10个分配")
	assert.Equal(t, 0, sd.GetLength(), "获取完后长度应为0")
}
