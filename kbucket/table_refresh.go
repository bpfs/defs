package kbucket

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/bpfs/defs/utils/logger"
	"github.com/libp2p/go-libp2p/core/peer"

	mh "github.com/multiformats/go-multihash"
)

// maxCplForRefresh 是我们支持的刷新的最大公共前缀长度（Cpl）。
// 这个限制存在是因为目前我们只能生成 'maxCplForRefresh' 位的前缀。
const maxCplForRefresh uint = 15

// GetTrackedCplsForRefresh 返回我们正在追踪的用于刷新的 Cpl 时间戳列表。
//
// 返回值:
//   - []time.Time: 每个 Cpl 对应的最后刷新时间的切片。调用者可以自由修改返回的切片，因为它是一个防御性副本。
func (rt *RoutingTable) GetTrackedCplsForRefresh() []time.Time {
	// 获取最大公共前缀长度，不超过 maxCplForRefresh
	maxCommonPrefix := rt.maxCommonPrefix()
	if maxCommonPrefix > maxCplForRefresh {
		maxCommonPrefix = maxCplForRefresh
	}

	// 获取读锁以确保并发安全
	rt.cplRefreshLk.RLock()
	defer rt.cplRefreshLk.RUnlock()

	// 创建一个切片用于存储时间戳，长度为 maxCommonPrefix+1
	cpls := make([]time.Time, maxCommonPrefix+1)
	for i := uint(0); i <= maxCommonPrefix; i++ {
		// 如果该 Cpl 还没有刷新过，则使用零值时间
		cpls[i] = rt.cplRefreshedAt[i]
	}
	logger.Debugf("获取追踪的Cpls刷新时间列表，长度: %d", len(cpls))
	return cpls
}

// randUint16 生成一个随机的 uint16 值。
//
// 返回值:
//   - uint16: 生成的随机数
//   - error: 如果生成随机数失败则返回错误
func randUint16() (uint16, error) {
	// 创建一个 2 字节的缓冲区用于存储随机数
	var prefixBytes [2]byte
	// 从加密安全的随机源读取随机字节
	_, err := rand.Read(prefixBytes[:])
	if err != nil {
		logger.Errorf("生成随机uint16失败: %v", err)
		return 0, err
	}
	// 将字节转换为 uint16 并返回
	return binary.BigEndian.Uint16(prefixBytes[:]), err
}

// GenRandPeerID 为给定的公共前缀长度（Cpl）生成一个随机的对等节点 ID。
//
// 参数:
//   - targetCpl: 目标公共前缀长度
//
// 返回值:
//   - peer.ID: 生成的随机对等节点 ID
//   - error: 如果生成失败则返回错误
func (rt *RoutingTable) GenRandPeerID(targetCpl uint) (peer.ID, error) {
	// 检查目标 Cpl 是否超过最大限制
	if targetCpl > maxCplForRefresh {
		logger.Errorf("目标Cpl %d 超过最大限制 %d", targetCpl, maxCplForRefresh)
		return "", fmt.Errorf("无法为大于 %d 的 Cpl 生成 peer ID", maxCplForRefresh)
	}

	// 将本地前缀转换为 uint16
	localPrefix := binary.BigEndian.Uint16(rt.local)

	// 计算切换后的本地前缀，通过在目标位置翻转位来实现
	toggledLocalPrefix := localPrefix ^ (uint16(0x8000) >> targetCpl)
	// 生成随机前缀
	randPrefix, err := randUint16()
	if err != nil {
		logger.Errorf("生成随机前缀失败: %v", err)
		return "", err
	}

	// 创建掩码以保留前 targetCpl+1 位
	mask := (^uint16(0)) << (16 - (targetCpl + 1))
	// 组合切换后的本地前缀和随机位
	targetPrefix := (toggledLocalPrefix & mask) | (randPrefix & ^mask)

	// 将前缀转换为对等节点 ID
	key := keyPrefixMap[targetPrefix]
	id := [32 + 2]byte{mh.SHA2_256, 32}
	binary.BigEndian.PutUint32(id[2:], key)
	logger.Debugf("生成随机PeerID，目标Cpl: %d", targetCpl)
	return peer.ID(id[:]), nil
}

// GenRandomKey 根据提供的公共前缀长度（Cpl）生成一个匹配的随机键。
//
// 参数:
//   - targetCpl: 目标公共前缀长度
//
// 返回值:
//   - ID: 生成的随机键
//   - error: 如果生成失败则返回错误
func (rt *RoutingTable) GenRandomKey(targetCpl uint) (ID, error) {
	// 检查目标 Cpl 是否有效
	if int(targetCpl+1) >= len(rt.local)*8 {
		logger.Errorf("目标Cpl %d 超过键长度", targetCpl)
		return nil, fmt.Errorf("无法为大于键长度的 Cpl 生成键")
	}

	// 计算需要复制的完整字节数
	partialOffset := targetCpl / 8

	// 创建输出缓冲区并复制本地键的前 partialOffset 个字节
	output := make([]byte, len(rt.local))
	copy(output, rt.local[:partialOffset])
	// 用随机数填充剩余字节
	_, err := rand.Read(output[partialOffset:])
	if err != nil {
		logger.Errorf("生成随机字节失败: %v", err)
		return nil, err
	}

	// 计算剩余需要处理的位数
	remainingBits := 8 - targetCpl%8
	orig := rt.local[partialOffset]

	// 创建用于位操作的掩码
	origMask := ^uint8(0) << remainingBits
	randMask := ^origMask >> 1
	flippedBitOffset := remainingBits - 1
	flippedBitMask := uint8(1) << flippedBitOffset

	// 组合原始位、翻转位和随机位
	output[partialOffset] = orig&origMask | (orig & flippedBitMask) ^ flippedBitMask | output[partialOffset]&randMask

	logger.Debugf("生成随机键，目标Cpl: %d", targetCpl)
	return ID(output), nil
}

// ResetCplRefreshedAtForID 重置给定 ID 的公共前缀长度（Cpl）的刷新时间。
//
// 参数:
//   - id: 要重置的 ID
//   - newTime: 新的刷新时间
func (rt *RoutingTable) ResetCplRefreshedAtForID(id ID, newTime time.Time) {
	// 计算给定 ID 与本地键的公共前缀长度
	cpl := CommonPrefixLen(id, rt.local)
	// 如果 Cpl 超过最大可刷新限制，则直接返回
	if uint(cpl) > maxCplForRefresh {
		logger.Warnf("Cpl %d 超过最大可刷新限制 %d，跳过重置", cpl, maxCplForRefresh)
		return
	}

	// 获取写锁以确保并发安全
	rt.cplRefreshLk.Lock()
	defer rt.cplRefreshLk.Unlock()

	// 更新指定 Cpl 的刷新时间
	rt.cplRefreshedAt[uint(cpl)] = newTime
	logger.Debugf("重置ID的Cpl %d 刷新时间为 %v", cpl, newTime)
}
