package peerdiversity

import (
	"fmt"
	"net"
	"sort"
	"sync"

	"github.com/dep2p/libp2p/core/peer"
	logging "github.com/dep2p/log"
	"github.com/libp2p/go-cidranger"
	asnutil "github.com/libp2p/go-libp2p-asn-util"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

var logger = logging.Logger("peerdiversity")

// asnStore 是一个接口，定义了获取 IPv6 地址的 ASN（自治系统号）的方法
type asnStore interface {
	// AsnForIPv6 根据给定的 IPv6 地址返回其对应的 ASN
	// 参数:
	//   - ip: IPv6 地址
	// 返回:
	//   - string: ASN 编号
	//   - error: 获取过程中的错误信息
	AsnForIPv6(ip net.IP) (string, error)
}

// PeerIPGroupKey 是一个唯一键，表示对等节点所属的 IP 组中的一个组。
// 一个对等节点每个地址都有一个 PeerIPGroupKey。因此，如果对等节点有多个地址，它可以属于多个组。
// 目前，给定一个对等节点地址，我们的分组机制如下：
//  1. 对于 IPv6 地址，我们按照 IP 地址的 ASN 进行分组。
//  2. 对于 IPv4 地址，所有属于同一个传统（Class A）/8 分配的地址
//     或者共享相同 /16 前缀的地址属于同一组。
type PeerIPGroupKey string

// legacyClassA 是一个字符串切片，包含了传统（Class A）/8 IPv4 地址块的列表
// 参考: https://en.wikipedia.org/wiki/List_of_assigned_/8_IPv4_address_blocks
var legacyClassA = []string{"12.0.0.0/8", "17.0.0.0/8", "19.0.0.0/8", "38.0.0.0/8", "48.0.0.0/8", "56.0.0.0/8", "73.0.0.0/8", "53.0.0.0/8"}

// PeerGroupInfo 表示对等节点的分组信息
type PeerGroupInfo struct {
	Id         peer.ID        // 对等节点的唯一标识符
	Cpl        int            // 共同前缀长度（Common Prefix Length）
	IPGroupKey PeerIPGroupKey // 对等节点所属的 IP 组键
}

// PeerIPGroupFilter 是由调用方实现的接口，用于实例化 `peerdiversity.Filter`。
// 此接口提供了被 `peerdiversity.Filter` 使用/调用的函数钩子。
type PeerIPGroupFilter interface {
	// Allow 测试具有给定分组信息的对等节点是否应该被 Filter 允许/拒绝
	// 参数:
	//   - PeerGroupInfo: 对等节点的分组信息
	// 返回:
	//   - bool: true 表示允许，false 表示拒绝
	Allow(PeerGroupInfo) (allow bool)

	// Increment 在将具有给定分组信息的对等节点添加到 Filter 状态时调用
	// 参数:
	//   - PeerGroupInfo: 对等节点的分组信息
	Increment(PeerGroupInfo)

	// Decrement 在将具有给定分组信息的对等节点从 Filter 中移除时调用
	// 参数:
	//   - PeerGroupInfo: 对等节点的分组信息
	Decrement(PeerGroupInfo)

	// PeerAddresses 用于获取给定对等节点的所有地址
	// 参数:
	//   - peer.ID: 对等节点的标识符
	// 返回:
	//   - []ma.Multiaddr: 对等节点的多地址列表
	PeerAddresses(peer.ID) []ma.Multiaddr
}

// Filter 是一个多样性过滤器，根据配置的白名单规则和传递给它的 PeerIPGroupFilter 接口的多样性策略来接受或拒绝对等节点
type Filter struct {
	mu            sync.Mutex                           // 用于同步的互斥锁
	pgm           PeerIPGroupFilter                    // 实现了 PeerIPGroupFilter 接口的对象，用于定义多样性策略
	peerGroups    map[peer.ID][]PeerGroupInfo          // 对等节点分组信息的映射，将对等节点的标识符映射到其对应的分组信息列表
	wlpeers       map[peer.ID]struct{}                 // 白名单中的对等节点的映射
	legacyCidrs   cidranger.Ranger                     // 传统 IPv4 Class A 网络的 CIDR 范围
	logKey        string                               // 日志键
	cplFnc        func(peer.ID) int                    // 获取对等节点共同前缀长度（CPL）的函数
	cplPeerGroups map[int]map[peer.ID][]PeerIPGroupKey // 根据共同前缀长度将对等节点分组的映射
	asnStore      asnStore                             // 用于获取 IPv6 地址的自治系统号（ASN）的接口实例
}

// NewFilter 创建一个用于对等节点多样性的过滤器
// 参数:
//   - pgm: 实现了 PeerIPGroupFilter 接口的对象
//   - logKey: 日志键
//   - cplFnc: 获取对等节点共同前缀长度的函数
//
// 返回:
//   - *Filter: 创建的过滤器实例
//   - error: 创建过程中的错误信息
func NewFilter(pgm PeerIPGroupFilter, logKey string, cplFnc func(peer.ID) int) (*Filter, error) {
	if pgm == nil {
		logger.Errorf("peergroup实现不能为空")
		return nil, fmt.Errorf("peergroup implementation can not be nil") // 如果 peergroup 实现为空，则返回错误
	}

	// 为传统 Class N 网络创建 Trie
	legacyCidrs := cidranger.NewPCTrieRanger()
	for _, cidr := range legacyClassA {
		_, nn, err := net.ParseCIDR(cidr)
		if err != nil {
			logger.Errorf("解析CIDR失败: %v", err)
			return nil, err
		}
		if err := legacyCidrs.Insert(cidranger.NewBasicRangerEntry(*nn)); err != nil {
			logger.Errorf("插入CIDR失败: %v", err)
			return nil, err
		}
	}

	return &Filter{
		pgm:           pgm,
		peerGroups:    make(map[peer.ID][]PeerGroupInfo),          // 创建对等节点分组信息的映射
		wlpeers:       make(map[peer.ID]struct{}),                 // 创建白名单中的对等节点的映射
		legacyCidrs:   legacyCidrs,                                // 传统 Class N 网络的 CIDR 范围
		logKey:        logKey,                                     // 日志键
		cplFnc:        cplFnc,                                     // 获取对等节点共同前缀长度的函数
		cplPeerGroups: make(map[int]map[peer.ID][]PeerIPGroupKey), // 根据共同前缀长度将对等节点分组的映射
		asnStore:      asnutil.Store,                              // ASN 存储接口实例
	}, nil
}

// Remove 从过滤器中移除指定的对等节点
// 参数:
//   - p: 要移除的对等节点的标识符
func (f *Filter) Remove(p peer.ID) {
	f.mu.Lock()         // 加锁，确保并发安全
	defer f.mu.Unlock() // 解锁

	cpl := f.cplFnc(p) // 获取对等节点的共同前缀长度（CPL）

	for _, info := range f.peerGroups[p] { // 遍历对等节点的信息
		f.pgm.Decrement(info) // 调用 pgm 的 Decrement 方法，减少 info 的计数
	}
	f.peerGroups[p] = nil           // 将对等节点的信息置为空
	delete(f.peerGroups, p)         // 从 peerGroups 中删除对等节点 p
	delete(f.cplPeerGroups[cpl], p) // 从 cplPeerGroups[cpl] 中删除对等节点 p

	if len(f.cplPeerGroups[cpl]) == 0 { // 如果 cplPeerGroups[cpl] 中没有其他对等节点
		delete(f.cplPeerGroups, cpl) // 从 cplPeerGroups 中删除 cpl
	}

	logger.Debugf("已移除对等节点: %v", p)
}

// TryAdd 尝试将对等节点添加到过滤器状态中
// 参数:
//   - p: 要添加的对等节点的标识符
//
// 返回:
//   - bool: true 表示添加成功，false 表示添加失败
func (f *Filter) TryAdd(p peer.ID) bool {
	f.mu.Lock()         // 加锁，确保并发安全
	defer f.mu.Unlock() // 解锁

	if _, ok := f.wlpeers[p]; ok { // 如果对等节点已经在白名单中
		logger.Debugf("对等节点已在白名单中: %v", p)
		return true // 直接返回 true
	}

	cpl := f.cplFnc(p) // 获取对等节点的共同前缀长度（CPL）

	// 不允许无法确定地址的对等节点
	addrs := f.pgm.PeerAddresses(p) // 获取对等节点的地址
	if len(addrs) == 0 {            // 如果地址数量为 0
		logger.Debugf("未找到对等节点的地址: %v", p)
		return false // 返回 false
	}

	peerGroups := make([]PeerGroupInfo, 0, len(addrs)) // 创建一个空的对等节点组切片，容量为地址数量
	for _, a := range addrs {                          // 遍历对等节点的地址
		ip, err := manet.ToIP(a) // 将地址转换为 IP
		if err != nil {          // 如果转换过程中发生错误
			logger.Errorf("从多地址解析IP失败: %v, 错误: %v", a.String(), err)
			return false // 返回 false
		}

		// 如果无法确定地址的分组，则拒绝该对等节点
		key, err := f.ipGroupKey(ip) // 获取 IP 的分组键
		if err != nil {              // 如果获取过程中发生错误
			logger.Errorf("查找分组键失败, IP: %v, 对等节点: %v, 错误: %v", ip.String(), p, err)
			return false // 返回 false
		}
		if len(key) == 0 { // 如果分组键为空
			logger.Errorf("分组键为空, IP: %v, 对等节点: %v", ip.String(), p)
			return false // 返回 false
		}
		group := PeerGroupInfo{Id: p, Cpl: cpl, IPGroupKey: key} // 创建对等节点组信息

		if !f.pgm.Allow(group) { // 如果 pgm 不允许该组信息
			logger.Debugf("对等节点组不被允许: %v", group)
			return false // 返回 false
		}

		peerGroups = append(peerGroups, group) // 将组信息添加到对等节点组切片中
	}

	if _, ok := f.cplPeerGroups[cpl]; !ok { // 如果 cplPeerGroups 中不存在 cpl 对应的条目
		f.cplPeerGroups[cpl] = make(map[peer.ID][]PeerIPGroupKey) // 创建一个空的对等节点组映射
	}

	for _, g := range peerGroups { // 遍历对等节点组信息
		f.pgm.Increment(g) // 调用 pgm 的 Increment 方法，增加该组信息的计数

		f.peerGroups[p] = append(f.peerGroups[p], g)                            // 将组信息添加到对等节点 p 的信息中
		f.cplPeerGroups[cpl][p] = append(f.cplPeerGroups[cpl][p], g.IPGroupKey) // 将组信息的 IPGroupKey 添加到 cplPeerGroups[cpl][p] 中
	}

	logger.Debugf("成功添加对等节点: %v", p)
	return true // 返回 true
}

// WhitelistPeers 将给定的对等节点添加到白名单中，这些对等节点将始终被允许
// 参数:
//   - peers: 要添加到白名单的对等节点列表
func (f *Filter) WhitelistPeers(peers ...peer.ID) {
	f.mu.Lock()         // 加锁，确保并发安全
	defer f.mu.Unlock() // 解锁

	for _, p := range peers { // 遍历给定的对等节点
		f.wlpeers[p] = struct{}{} // 将对等节点添加到白名单中
		logger.Debugf("对等节点已加入白名单: %v", p)
	}
}

// ipGroupKey 根据给定的 IP 返回对应的 PeerIPGroupKey
// 参数:
//   - ip: IP 地址
//
// 返回:
//   - PeerIPGroupKey: IP 组键
//   - error: 获取过程中的错误信息
func (f *Filter) ipGroupKey(ip net.IP) (PeerIPGroupKey, error) {
	switch bz := ip.To4(); bz {
	case nil:
		// TODO 清理 ASN 代码库
		// IPv6 地址 -> 获取 ASN
		s, err := f.asnStore.AsnForIPv6(ip)
		if err != nil {
			logger.Errorf("获取IPv6地址的ASN失败: %v, 错误: %v", ip.String(), err)
			return "", fmt.Errorf("获取 IPv6 地址 %s 的 ASN 失败：%w", ip.String(), err)
		}

		// 如果找不到 ASN，则使用 /32 前缀作为备选方案
		if len(s) == 0 {
			logger.Debugf("未知ASN, IP: %v", ip)
			s = fmt.Sprintf("未知 ASN：%s", net.CIDRMask(32, 128).String())
		}

		return PeerIPGroupKey(s), nil
	default:
		// 如果属于传统的 Class 8 网络，则返回 /8 前缀作为键
		rs, _ := f.legacyCidrs.ContainingNetworks(ip)
		if len(rs) != 0 {
			key := ip.Mask(net.IPv4Mask(255, 0, 0, 0)).String()
			return PeerIPGroupKey(key), nil
		}

		// 否则 -> 返回 /16 前缀作为键
		key := ip.Mask(net.IPv4Mask(255, 255, 0, 0)).String()
		return PeerIPGroupKey(key), nil
	}
}

// CplDiversityStats 包含了一个 Cpl 的对等节点多样性统计信息
type CplDiversityStats struct {
	Cpl   int                          // 表示最短路径长度
	Peers map[peer.ID][]PeerIPGroupKey // 将对等节点的 ID 与对等节点的 IP 组键列表关联起来的映射
}

// GetDiversityStats 返回每个 CPL 的多样性统计信息，并按 CPL 进行排序
// 返回:
//   - []CplDiversityStats: CPL 多样性统计信息的切片
func (f *Filter) GetDiversityStats() []CplDiversityStats {
	f.mu.Lock()
	defer f.mu.Unlock()

	stats := make([]CplDiversityStats, 0, len(f.cplPeerGroups))

	// 获取所有的 CPL，并进行排序
	var sortedCpls []int
	for cpl := range f.cplPeerGroups {
		sortedCpls = append(sortedCpls, cpl)
	}
	sort.Ints(sortedCpls)

	// 遍历排序后的 CPL
	for _, cpl := range sortedCpls {
		ps := make(map[peer.ID][]PeerIPGroupKey, len(f.cplPeerGroups[cpl]))
		cd := CplDiversityStats{cpl, ps}

		// 将对等节点组键映射添加到对应的 CPL 中
		for p, groups := range f.cplPeerGroups[cpl] {
			ps[p] = groups
		}
		stats = append(stats, cd)
	}

	logger.Debugf("获取多样性统计信息完成，共 %d 个CPL", len(stats))
	return stats
}
