package files

import (
	"testing"

	"github.com/dep2p/libp2p/core/peer"
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
	distribution := map[peer.ID][]string{
		"peer1": {"segment1", "segment2", "segment3"},
		"peer2": {"segment4", "segment5", "segment6"},
	}

	// 添加分配
	sd.AddDistribution(distribution)

	// 验证长度
	length := sd.GetLength()
	assert.Equal(t, 1, length, "添加一个分配后长度应为1")

	// 获取并验证内容
	nextDist, ok := sd.GetNextDistribution()
	assert.True(t, ok, "应成功获取分配")
	assert.Equal(t, distribution, nextDist, "获取的分配应与添加的相同")
}

// TestGetNextDistribution 测试获取下一个分配
func TestGetNextDistribution(t *testing.T) {
	sd := NewSegmentDistribution()

	// 准备测试数据
	distribution1 := map[peer.ID][]string{
		"peer1": {"segment1", "segment2"},
	}
	distribution2 := map[peer.ID][]string{
		"peer2": {"segment3", "segment4"},
	}

	// 添加两个分配
	sd.AddDistribution(distribution1)
	sd.AddDistribution(distribution2)

	// 验证初始长度
	assert.Equal(t, 2, sd.GetLength(), "添加两个分配后长度应为2")

	// 获取第一个分配
	nextDist1, ok1 := sd.GetNextDistribution()
	assert.True(t, ok1, "应成功获取第一个分配")
	assert.Equal(t, distribution1, nextDist1, "第一个分配应匹配")
	assert.Equal(t, 1, sd.GetLength(), "获取一个后长度应为1")

	// 获取第二个分配
	nextDist2, ok2 := sd.GetNextDistribution()
	assert.True(t, ok2, "应成功获取第二个分配")
	assert.Equal(t, distribution2, nextDist2, "第二个分配应匹配")
	assert.Equal(t, 0, sd.GetLength(), "获取两个后长度应为0")

	// 尝试从空列表获取
	nextDist3, ok3 := sd.GetNextDistribution()
	assert.False(t, ok3, "从空列表获取应返回false")
	assert.Nil(t, nextDist3, "从空列表获取应返回nil")
}

// TestClear 测试清空分片分配列表
func TestClear(t *testing.T) {
	sd := NewSegmentDistribution()

	// 添加测试数据
	distribution := map[peer.ID][]string{
		"peer1": {"segment1", "segment2"},
		"peer2": {"segment3", "segment4"},
	}
	sd.AddDistribution(distribution)

	// 验证添加成功
	assert.Equal(t, 1, sd.GetLength(), "添加后长度应为1")

	// 清空列表
	sd.Clear()

	// 验证清空成功
	assert.Equal(t, 0, sd.GetLength(), "清空后长度应为0")

	// 验证可以继续添加
	sd.AddDistribution(distribution)
	assert.Equal(t, 1, sd.GetLength(), "清空后重新添加的长度应为1")
}

// TestConcurrentAccess 测试并发访问安全性
func TestConcurrentAccess(t *testing.T) {
	sd := NewSegmentDistribution()

	// 并发添加
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			distribution := map[peer.ID][]string{
				peer.ID(string(rune(id))): {string(rune(id))},
			}
			sd.AddDistribution(distribution)
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
