package kbucket

// import (
// 	"context"
// 	"testing"
// 	"time"

// 	"github.com/bpfs/defs/v2/fscfg"

// 	dht "github.com/dep2p/kaddht"
// 	"github.com/dep2p/libp2p"
// 	"github.com/dep2p/libp2p/core/crypto"
// 	"github.com/dep2p/libp2p/core/host"
// 	"github.com/dep2p/libp2p/core/peer"
// 	"github.com/dep2p/libp2p/core/routing"
// 	"github.com/dep2p/libp2p/core/test"
// 	"github.com/dep2p/libp2p/p2p/protocol/circuitv2/relay"
// 	"github.com/dep2p/libp2p/p2p/security/noise"
// 	tls "github.com/dep2p/libp2p/p2p/security/tls"
// 	ma "github.com/multiformats/go-multiaddr"
// 	"github.com/stretchr/testify/require"
// )

// // StartDiscoveryRelays 启动中继节点发现服务,返回发现的中继节点信息。
// //
// // 参数:
// //   - ctx: 上下文对象,用于控制服务的生命周期
// //   - h: libp2p 主机实例,用于网络通信
// //   - ch: 用于返回发现的中继节点信息的通道
// //
// // 中继节点要求:
// // 1. 网络要求
// //   - 必须有公网 IP 地址
// //   - 必须开放必要端口(如 4001/tcp)
// //   - 必须能被其他节点稳定访问
// //
// // 2. 资源要求
// //   - 充足的带宽资源
// //   - 稳定的网络连接
// //   - 足够的处理能力
// //
// // 3. 协议要求
// //   - 实现 libp2p 中继协议(/libp2p/circuit/relay/v2)
// //   - 启用中继服务功能
// //
// // 4. 安全要求
// //   - 优先使用可信的已知节点
// //   - 建议使用白名单机制
// //   - 可通过信誉系统筛选
// //
// // 5. 地理分布
// //   - 建议选择不同地理位置的节点
// //   - 有助于提供更好的网络覆盖
// //
// // 中继访问机制:
// //  1. 连接流程
// //     Client ←→ Relay ←→ Target
// //     - Client: 发起连接的节点(通常在 NAT 后)
// //     - Relay: 中继节点(具有公网 IP)
// //     - Target: 目标节点(可能在 NAT 后)
// //
// // 2. 连接建立过程
// //   - Client 发现无法直接访问 Target
// //   - Client 通过 DHT 或配置找到 Relay 节点
// //   - Client 请求 Relay 建立到 Target 的中继连接
// //   - Relay 验证请求并建立连接通道
// //
// // 3. 访问控制
// //   - Relay 节点可设置访问策略
// //   - 可限制连接数量和带宽
// //   - 可实施白名单/黑名单
// //   - 可要求认证
// //
// // 4. 安全考虑
// //   - 所有通信需加密
// //   - Relay 节点可看到连接元数据
// //   - 无法访问加密的通信内容
// //
// // 5. 性能影响
// //   - 中继会增加延迟
// //   - 带宽受限于中继节点
// //   - 建议就近选择中继节点
// //
// // 注意事项:
// // - 中继节点需启用 EnableRelayService
// // - 客户端需启用 EnableAutoRelay
// // - 目标节点需允许被中继访问
// //
// // 实现流程:
// // 1. 启动一个 goroutine 处理中继节点发现
// // 2. 通过 DHT 查找网络中的中继节点
// // 3. 从预配置的静态中继节点列表获取可信节点
// // 4. 合并所有来源的中继节点并去重
// // 5. 通过通道返回发现的中继节点信息
// func StartDiscoveryRelays(ctx context.Context, h host.Host, ch chan peer.AddrInfo) {
// 	go func() {
// 		defer close(ch)

// 		// 1. 从 DHT 中查找中继节点
// 		kadDHT, err := dht.New(ctx, h)
// 		if err != nil {
// 			logger.Errorf("创建 DHT 失败: %v", err)
// 			return
// 		}
// 		defer kadDHT.Close()

// 		// 使用 GetClosestPeers 查找节点
// 		peerChan, err := kadDHT.GetClosestPeers(ctx, "relay-nodes")
// 		if err != nil {
// 			logger.Errorf("查找中继节点失败: %v", err)
// 			return
// 		}

// 		// 2. 从配置的静态中继节点列表中获取
// 		staticRelays := []peer.AddrInfo{
// 			// 预配置的可信中继节点
// 			{
// 				ID: "QmRelay1...",
// 				Addrs: []ma.Multiaddr{
// 					ma.StringCast("/ip4/x.x.x.x/tcp/4001"),
// 				},
// 			},
// 		}

// 		// 3. 合并所有来源的中继节点
// 		seenPeers := make(map[peer.ID]struct{})
// 		count := 0

// 		// 发送静态配置的中继节点
// 		for _, relay := range staticRelays {
// 			if _, seen := seenPeers[relay.ID]; !seen {
// 				select {
// 				case ch <- relay:
// 					seenPeers[relay.ID] = struct{}{}
// 					count++
// 				case <-ctx.Done():
// 					return
// 				}
// 			}
// 		}

// 		// 发送从 DHT 发现的中继节点
// 		for _, p := range peerChan {
// 			if _, seen := seenPeers[p]; !seen {
// 				select {
// 				case ch <- peer.AddrInfo{ID: p, Addrs: kadDHT.Host().Peerstore().Addrs(p)}:
// 					seenPeers[p] = struct{}{}
// 					count++
// 				case <-ctx.Done():
// 					return
// 				}
// 			}
// 		}
// 	}()
// }

// // NewTestHost 创建一个用于测试的 libp2p 主机实例
// func NewTestHost(t *testing.T) host.Host {
// 	ctx := context.Background()
// 	// 基础配置
// 	opts := []libp2p.Option{
// 		// 启用 TLS 加密传输安全
// 		// - 使用 TLS 1.3 协议
// 		// - 提供身份验证和数据加密
// 		// - 防止中间人攻击
// 		libp2p.Security(tls.ID, tls.New),

// 		// 启用 Noise 加密协议
// 		// - 提供轻量级加密
// 		// - 适用于 P2P 网络
// 		// - 补充 TLS 的安全性
// 		libp2p.Security(noise.ID, noise.New),

// 		// 启用 NAT 穿透服务
// 		// - 自动处理 NAT 端口映射
// 		// - 提高节点可发现性
// 		// - 支持 UPnP 和 NAT-PMP 协议
// 		libp2p.EnableNATService(),

// 		// 启用打洞功能
// 		// - 允许 NAT 后的节点直接通信
// 		// - 通过 STUN 服务发现外部地址
// 		// - 减少对中继节点的依赖
// 		libp2p.EnableHolePunching(),

// 		// 配置监听地址
// 		// - 监听本地回环地址
// 		// - 端口 0 表示随机选择可用端口
// 		// - 同时支持 IPv4 和 IPv6
// 		libp2p.ListenAddrStrings(
// 			"/ip4/127.0.0.1/tcp/0", // IPv4 本地回环地址
// 			"/ip6/::1/tcp/0",       // IPv6 本地回环地址
// 		),
// 	}

// 	// 作为中继节点
// 	opts = append(opts,
// 		// 启用中继服务功能
// 		libp2p.EnableRelayService(
// 			// 设置中继服务的资源限制
// 			relay.WithResources(
// 				// 使用默认的资源配置
// 				// DefaultResources() 返回包含默认限制值的 Resources 对象
// 				// 包括最大连接数、带宽限制等
// 				relay.DefaultResources(),
// 			),
// 			relay.WithLimit(&relay.RelayLimit{
// 				Duration: time.Hour,
// 				Data:     1 << 20, // 1MB
// 			}),
// 		),
// 		// 启用 DHT 服务，用于节点发现
// 		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
// 			return dht.New(ctx, h, dht.Mode(dht.ModeServer))
// 		}),
// 	)

// 	// 创建通道用于接收中继节点
// 	relaysCh := make(chan peer.AddrInfo)
// 	// 作为目标节点
// 	opts = append(opts,
// 		// 启用自动中继功能,并设置 peer 源为 getPeerSource(h)
// 		libp2p.EnableAutoRelayWithPeerSource(func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
// 			return relaysCh // 返回通道
// 		}),
// 	)

// 	// 作为外部节点
// 	opts = append(opts,
// 		libp2p.EnableRelay(), // 启用中继支持
// 	)

// 	// 创建一个基本的 libp2p 主机，用于测试
// 	h, err := libp2p.New(opts...)
// 	require.NoError(t, err)

// 	// 启动发现中继节点
// 	StartDiscoveryRelays(ctx, h, relaysCh)

// 	t.Cleanup(func() {
// 		h.Close()
// 	})

// 	return h
// }

// // NewTestHostWithID 创建一个具有指定 ID 的测试主机
// func NewTestHostWithID(t *testing.T, id peer.ID) host.Host {
// 	// 创建一个带有特定 ID 的主机
// 	h, err := libp2p.New(
// 		libp2p.Identity(generateIdentityForPeerID(t)),
// 		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
// 	)

// 	require.NoError(t, err)
// 	require.Equal(t, id, h.ID())

// 	t.Cleanup(func() {
// 		h.Close()
// 	})

// 	return h
// }

// // NewTestHostWithContext 创建一个带有上下文的测试主机
// func NewTestHostWithContext(t *testing.T, ctx context.Context) host.Host {
// 	h, err := libp2p.New(
// 		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
// 	)

// 	require.NoError(t, err)

// 	t.Cleanup(func() {
// 		h.Close()
// 	})

// 	return h
// }

// // generateIdentityForPeerID 生成与指定 peer.ID 匹配的身份密钥
// func generateIdentityForPeerID(t *testing.T) crypto.PrivKey {
// 	priv, _, err := crypto.GenerateKeyPair(
// 		crypto.Ed25519,
// 		256,
// 	)
// 	require.NoError(t, err)
// 	return priv
// }

// // TestCreateRoutingTable 测试创建带有多样性过器的路由表
// func TestCreateRoutingTable(t *testing.T) {
// 	t.Parallel()

// 	// 使用新的测试主机创建方法
// 	h := NewTestHost(t)

// 	// 创建配置选项
// 	opt := fscfg.DefaultOptions()
// 	err := opt.ApplyOptions(
// 		fscfg.WithBucketSize(10),
// 		fscfg.WithMaxPeersPerCpl(3),
// 	)
// 	require.NoError(t, err)

// 	// 创建路由表
// 	rt, err := CreateRoutingTable(h, opt, NoOpThreshold)
// 	require.NoError(t, err)
// 	require.NotNil(t, rt)

// 	// 验证路由表初始状态
// 	require.Equal(t, 10, rt.bucketsize)
// 	require.Equal(t, ConvertPeerID(h.ID()), rt.local)
// }

// // TestPeerDiversityFilter 测试节点多样性过滤器的功能
// func TestPeerDiversityFilter(t *testing.T) {
// 	t.Parallel()

// 	// 使用新的测试主机创建方法
// 	h := NewTestHost(t)

// 	// 创建配置选项，设置每个 CPL 最多允许 1 个节点
// 	opt := fscfg.DefaultOptions()
// 	err := opt.ApplyOptions(
// 		fscfg.WithBucketSize(20),
// 		fscfg.WithMaxPeersPerCpl(1), // 修改为 1，这样第二个节点就应该被拒绝
// 	)
// 	require.NoError(t, err)

// 	// 创建路由表
// 	rt, err := CreateRoutingTable(h, opt, NoOpThreshold)
// 	require.NoError(t, err)

// 	// 生成测试节点
// 	peers := make([]peer.ID, 3)
// 	for i := range peers {
// 		peers[i] = test.RandPeerIDFatal(t)
// 		// 添加一个测试地址
// 		h.Peerstore().AddAddrs(peers[i], []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/1234")}, time.Hour)
// 	}

// 	// 测试添加节点
// 	for i, p := range peers {
// 		success, err := rt.TryAddPeer(p, 0, true, false)
// 		if i < 1 { // 只有第一个节点应该可以添加成功
// 			require.NoError(t, err)
// 			require.True(t, success, "第 %d 个节点应该添加成功", i)
// 		} else { // 后面的节点应该被过滤器拒绝
// 			require.False(t, success, "第 %d 个节点应该被拒绝", i)
// 		}
// 	}

// 	// 验证路由表状态
// 	require.Equal(t, 1, rt.Size(), "路由表中应该只有 1 个节点")
// }

// // TestPeerDiversityFilterRemoval 测试节点移除时的计数器更新
// func TestPeerDiversityFilterRemoval(t *testing.T) {
// 	t.Parallel()

// 	// 使用新的测试主机创建方法
// 	h := NewTestHost(t)

// 	// 创建配置选项
// 	opt := fscfg.DefaultOptions()
// 	err := opt.ApplyOptions(
// 		fscfg.WithBucketSize(20),
// 		fscfg.WithMaxPeersPerCpl(2),
// 	)
// 	require.NoError(t, err)

// 	// 创建路由表
// 	rt, err := CreateRoutingTable(h, opt, NoOpThreshold)
// 	require.NoError(t, err)

// 	// 生成并添加测试节点
// 	p1 := test.RandPeerIDFatal(t)
// 	p2 := test.RandPeerIDFatal(t)
// 	h.Peerstore().AddAddrs(p1, []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/1234")}, time.Hour)
// 	h.Peerstore().AddAddrs(p2, []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/1235")}, time.Hour)

// 	// 添加两个节点
// 	success, err := rt.TryAddPeer(p1, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, success)

// 	success, err = rt.TryAddPeer(p2, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, success)

// 	// 移除一个节点
// 	rt.RemovePeer(p1)

// 	// 验证可以添加新节点
// 	p3 := test.RandPeerIDFatal(t)
// 	h.Peerstore().AddAddrs(p3, []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/1236")}, time.Hour)
// 	success, err = rt.TryAddPeer(p3, 0, true, false)
// 	require.NoError(t, err)
// 	require.True(t, success)
// }

// // TestPeerDiversityFilterWithDifferentCPLs 测试不同 CPL 值的节点添加
// func TestPeerDiversityFilterWithDifferentCPLs(t *testing.T) {
// 	t.Parallel()

// 	// 使用新的测试主机创建方法
// 	h := NewTestHost(t)

// 	// 创建配置选项
// 	opt := fscfg.DefaultOptions()
// 	err := opt.ApplyOptions(
// 		fscfg.WithBucketSize(20),
// 		fscfg.WithMaxPeersPerCpl(2),
// 	)
// 	require.NoError(t, err)

// 	// 创建路由表
// 	rt, err := CreateRoutingTable(h, opt, NoOpThreshold)
// 	require.NoError(t, err)

// 	// 生成不同 CPL 值的测试节点
// 	peers := generatePeersWithDifferentCPLs(t, h.ID(), 3, 2)

// 	// 测试添加节点
// 	for cpl, peerList := range peers {
// 		for i, p := range peerList {
// 			h.Peerstore().AddAddrs(p, []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/1234")}, time.Hour)
// 			success, err := rt.TryAddPeer(p, 0, true, false)
// 			if i < 2 { // 每个 CPL 值只允许添加 2 个节点
// 				require.NoError(t, err)
// 				require.True(t, success, "应该可以添加 CPL %d 的第 %d 个节点", cpl, i)
// 			} else {
// 				require.False(t, success, "不应该可以添加 CPL %d 的第 %d 个节点", cpl, i)
// 			}
// 		}
// 	}
// }

// // generatePeersWithDifferentCPLs 生成具有不同 CPL 值的测试节点
// func generatePeersWithDifferentCPLs(t *testing.T, localID peer.ID, cplCount, peersPerCpl int) map[int][]peer.ID {
// 	result := make(map[int][]peer.ID)
// 	localDhtID := ConvertPeerID(localID)

// 	for cpl := 0; cpl < cplCount; cpl++ {
// 		result[cpl] = make([]peer.ID, peersPerCpl)
// 		for i := 0; i < peersPerCpl; i++ {
// 			// 生成随机节点，直到找到具有正确 CPL 值的节点
// 			for {
// 				p := test.RandPeerIDFatal(t)
// 				if CommonPrefixLen(localDhtID, ConvertPeerID(p)) == cpl {
// 					result[cpl][i] = p
// 					break
// 				}
// 			}
// 		}
// 	}
// 	return result
// }
