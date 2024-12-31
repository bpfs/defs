package kbucket

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/test"
	"github.com/stretchr/testify/require"
)

// TestCloser 测试 Closer() 方法的行为
//
// 参数:
//   - t: 测试上下文对象
//
// 测试场景:
// 1. 当 Pa 比 Pb 更接近目标 X 时，验证 Closer(Pa, Pb, X) 返回 true
// 2. 当 Pb 比 Pa 更接近目标 X 时，验证 Closer(Pa, Pb, X) 返回 false
func TestCloser(t *testing.T) {
	// 生成两个随机的对等节点 ID 作为测试数据
	Pa := test.RandPeerIDFatal(t)
	Pb := test.RandPeerIDFatal(t)
	var X string

	// 测试场景 1: 找到一个目标 X，使得 Pa 比 Pb 更接近 X
	// 即满足 d(Pa, X) < d(Pb, X) 的条件
	for {
		// 生成随机目标 ID
		X = string(test.RandPeerIDFatal(t))
		// 计算并比较 Pa 和 Pb 到 X 的距离
		if xor(ConvertPeerID(Pa), ConvertKey(X)).less(xor(ConvertPeerID(Pb), ConvertKey(X))) {
			break
		}
	}

	// 验证当 Pa 更接近 X 时，Closer(Pa, Pb, X) 返回 true
	require.True(t, Closer(Pa, Pb, X))

	// 测试场景 2: 找到一个目标 X，使得 Pb 比 Pa 更接近 X
	// 即满足 d(Pa, X) > d(Pb, X) 的条件
	for {
		// 生成随机目标 ID
		X = string(test.RandPeerIDFatal(t))
		// 计算并比较 Pb 和 Pa 到 X 的距离
		if xor(ConvertPeerID(Pb), ConvertKey(X)).less(xor(ConvertPeerID(Pa), ConvertKey(X))) {
			break
		}
	}

	// 验证当 Pb 更接近 X 时，Closer(Pa, Pb, X) 返回 false
	require.False(t, Closer(Pa, Pb, X))
}
