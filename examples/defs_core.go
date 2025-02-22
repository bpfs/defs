package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"math/big"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bpfs/defs/v2"
	"github.com/bpfs/defs/v2/fscfg"
	v2net "github.com/bpfs/defs/v2/net"
	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/discovery"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	routingdisc "github.com/dep2p/go-dep2p/p2p/discovery/routing"
	"github.com/dep2p/go-dep2p/p2p/discovery/util"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/dep2p/go-dep2p/p2p/host/resource-manager"
	"github.com/dep2p/go-dep2p/p2p/muxer/yamux"
	"github.com/dep2p/go-dep2p/p2p/net/connmgr"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/relay"
	"github.com/dep2p/go-dep2p/p2p/transport/tcp"
	dht "github.com/dep2p/kaddht"
	"github.com/dep2p/pubsub"
	"github.com/tyler-smith/go-bip32"
	"golang.org/x/crypto/pbkdf2"
)

const (
	RendezvousString = "[rendezvous] wesign.xyz CS-DeFS-1"

	// DefaultConnMgrHighWater 定义了连接管理器的高水位线
	// 当连接数超过此值时,将触发连接修剪
	//DefaultConnMgrHighWater = 200
	DefaultConnMgrHighWater = 96
	// DefaultConnMgrLowWater 定义了连接管理器的低水位线
	// 连接修剪时会保留此数量的连接
	//DefaultConnMgrLowWater = 100
	DefaultConnMgrLowWater = 32
	// DefaultConnMgrGracePeriod 定义了新建连接的宽限期
	// 在此期间内新连接不会被修剪
	DefaultConnMgrGracePeriod = time.Second * 20
)

// DefsCore 定义核心功能结构
type DefsCore struct {
	fs         *defs.DeFS
	privateKey *ecdsa.PrivateKey
	ctx        context.Context
}

// NewDefsCore 创建新的 DefsCore 实例
func NewDefsCore() (*DefsCore, error) {
	logger.Info("开始创建 DefsCore 实例...")
	ctx := context.Background()

	// 获取 MAC 地址
	logger.Info("正在获取 MAC 地址...")
	macAddr, err := GetPrimaryMACAddress()
	if err != nil {
		logger.Errorf("获取MAC地址失败: %v", err)
		return nil, fmt.Errorf("获取MAC地址失败")
	}
	logger.Infof("获取到MAC地址: %s", macAddr)

	// 生成密钥对
	logger.Info("正在生成密钥对...")
	privateKey, _, err := GenerateECDSAKeyPair([]byte(macAddr), nil, 2048, 64, true)
	if err != nil {
		logger.Errorf("生成密钥对失败: %v", err)
		return nil, fmt.Errorf("生成密钥对失败")
	}
	logger.Info("密钥对生成成功")

	// 生成libp2p私钥
	logger.Info("正在生成libp2p私钥...")
	privKey, _, err := crypto.ECDSAKeyPairFromKey(privateKey)
	if err != nil {
		logger.Errorf("生成libp2p私钥失败: %v", err)
		return nil, fmt.Errorf("生成libp2p私钥失败")
	}
	logger.Info("libp2p私钥生成成功")

	// 获取空闲端口
	logger.Info("正在获取空闲端口...")
	port, err := getFreePort()
	if err != nil {
		logger.Errorf("获取空闲端口失败: %v", err)
		return nil, fmt.Errorf("获取空闲端口失败")
	}
	logger.Infof("获取到空闲端口: %s", port)

	// 创建 libp2p host
	logger.Info("正在创建 libp2p host...")
	// 构建主机配置选项
	option, err := buildHostOptions(privKey, port)
	if err != nil {
		logger.Errorf("设置P2P选项时失败: %v", err)
		return nil, err
	}

	h, err := dep2p.New(option...)
	if err != nil {
		logger.Errorf("创建 libp2p host 失败: %v", err)
		return nil, fmt.Errorf("创建 libp2p host 失败")
	}
	logger.Infof("libp2p host 创建成功，节点ID: %s", h.ID().String())
	logger.Infof("libp2p host 创建成功，节点地址: %s", h.Addrs())
	// 创建并启动 DHT
	logger.Info("正在创建并启动 DHT...")
	kadDHT, err := createDHT(ctx, h, dht.ModeClient)
	if err != nil {
		logger.Errorf("创建 DHT 失败: %v", err)
		return nil, fmt.Errorf("创建 DHT 失败")
	}
	logger.Info("DHT 创建并启动成功")

	// 创建节点发现服务
	logger.Info("正在创建节点发现服务...")
	disc := routingdisc.NewRoutingDiscovery(kadDHT)
	logger.Info("节点发现服务创建成功")

	// 配置DeFS选项
	logger.Info("正在配置DeFS选项...")
	opts := []fscfg.Option{
		fscfg.WithBucketSize(200),   // 设置K桶大小
		fscfg.WithMaxPeersPerCpl(5), // 设置每个K桶的最大对等节点数
		fscfg.WithPubSubOption(pubsub.WithSetFollowupTime(1 * time.Second)), // 设置发布订阅的跟随时间
		fscfg.WithPubSubOption(pubsub.WithSetGossipFactor(0.3)),             // 设置发布订阅的八卦因子
		fscfg.WithPubSubOption(pubsub.WithSetMaxMessageSize(2 << 20)),       // 设置发布订阅的最大消息大小
		fscfg.WithPubSubOption(pubsub.WithSetMaxMessageSize(2 << 20)),       // 设置发布订阅的最大消息大小
		fscfg.WithPubSubOption(pubsub.WithNodeDiscovery(disc)),              // 设置发布订阅的节点发现
	}

	// 初始化DeFS实例
	logger.Info("正在初始化DeFS实例...")
	fs, err := defs.Open(h, opts...)
	if err != nil {
		logger.Errorf("初始化DeFS失败: %v", err)
		return nil, fmt.Errorf("初始化DeFS失败")
	}
	logger.Info("DeFS实例初始化成功")

	// 启动节点发现
	logger.Info("正在启动节点发现...")
	if err := startDiscovery(ctx, disc, h); err != nil {
		logger.Errorf("启动节点发现失败: %v", err)
		return nil, fmt.Errorf("启动节点发现失败")
	}
	logger.Info("节点发现启动成功")

	logger.Info("DefsCore 实例创建完成")
	return &DefsCore{
		fs:         fs,
		privateKey: privateKey,
		ctx:        ctx,
	}, nil
}

// createDHT 创建并启动 DHT
func createDHT(ctx context.Context, h host.Host, mode dht.ModeOpt) (*dht.IpfsDHT, error) {
	logger.Info("开始创建 DHT...")
	var options []dht.Option
	options = append(options, dht.Mode(mode))

	kdht, err := dht.New(ctx, h, options...)
	if err != nil {
		logger.Errorf("创建 DHT 实例失败: %v", err)
		return nil, fmt.Errorf("创建 DHT 实例失败")
	}

	logger.Info("正在启动 DHT bootstrap...")
	if err = kdht.Bootstrap(ctx); err != nil {
		logger.Errorf("DHT bootstrap 失败: %v", err)
		return nil, fmt.Errorf("DHT bootstrap 失败")
	}
	logger.Info("DHT 创建并启动成功")

	return kdht, nil
}

// startDiscovery 启动节点发现
func startDiscovery(ctx context.Context, disc discovery.Discovery, h host.Host) error {
	logger.Info("开始启动节点发现...")

	// 连接至引导节点
	// if err := connectToBootstrapPeers(ctx, h, nil); err != nil {
	// 	logger.Errorf("连接至引导节点失败%v", err)
	// 	return err
	// }

	// 广播节点信息
	logger.Info("正在广播节点信息...")
	util.Advertise(ctx, disc, RendezvousString)

	logger.Info("节点信息广播成功")

	// 查找其他节点
	logger.Info("启动节点查找协程...")
	go func() {
		for {
			logger.Debug("开始新一轮节点查找...")
			peers, err := disc.FindPeers(ctx, RendezvousString)
			if err != nil {
				logger.Errorf("查找节点失败: %v", err)
				time.Sleep(time.Second * 5)
				continue
			}

			for peer := range peers {
				if peer.ID == h.ID() {
					continue
				}
				if h.Network().Connectedness(peer.ID) != network.Connected {
					logger.Debugf("尝试连接节点: %s", peer.ID)
					_, err = h.Network().DialPeer(ctx, peer.ID)
					if err != nil {
						logger.Debugf("连接节点失败: %v", err)
					} else {
						logger.Debugf("成功连接到节点: %s", peer.ID)
					}
				}
			}
			time.Sleep(time.Second * 5)
		}
	}()
	logger.Info("节点发现启动成功")

	return nil
}

// buildHostOptions 构建libp2p主机的配置选项
//
// 参数:
//   - psk: 私有网络的预共享密钥
//   - sk: 节点的私钥
//   - portNumber: 监听端口号
//
// 返回:
//   - []config.Option: libp2p配置选项列表
//   - error: 如果发生错误则返回
func buildHostOptions(sk crypto.PrivKey, port string) ([]config.Option, error) {
	// 设置连接管理器参数
	cm, err := connmgr.NewConnManager(
		DefaultConnMgrLowWater,                             // 最小连接数
		DefaultConnMgrHighWater,                            // 最大连接数
		connmgr.WithGracePeriod(DefaultConnMgrGracePeriod), // 宽限期
		connmgr.WithEmergencyTrim(true),
	)
	if err != nil {
		logger.Errorf("创建连接管理器失败: %v", err)
		return nil, err
	}

	// 创建对等点存储
	// 用于存储和管理已知的对等节点信息
	libp2pPeerstore, err := pstoremem.NewPeerstore()
	if err != nil {
		logger.Errorf("创建对等点存储失败: %v", err)
		return nil, err
	}

	// 使用默认的中继资源配置
	def := relay.DefaultResources()

	// 创建默认限制配置
	limitConfig, err := v2net.CreateDefaultLimitConfig()
	if err != nil {
		logger.Errorf("创建资源管理器限��配置失败: %v", err)
		return nil, err
	}

	// 创建资源管理器
	rm, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limitConfig))
	if err != nil {
		logger.Errorf("创建资源管理器失败: %v", err)
		return nil, err
	}

	// 构建基本配置选项
	options := []dep2p.Option{
		dep2p.ResourceManager(rm),
		dep2p.Peerstore(libp2pPeerstore), // 设置对等点存储
		dep2p.Ping(false),                // 禁用ping服务
		dep2p.Identity(sk),               // 设置节点身份(私钥)
		dep2p.DefaultSecurity,            // 使用默认安全选项
		dep2p.ConnectionManager(cm),      // 设置连接管理器

		dep2p.Muxer(yamux.ID, yamux.DefaultTransport), // 设置yamux多路复用器
		dep2p.NATPortMap(),                            // 启用NAT端口映射
		dep2p.EnableRelay(),                           // 启用中继功能
		dep2p.EnableHolePunching(),                    // 启用NAT穿透
		dep2p.EnableNATService(),                      // 启用NAT服务
		// 配置中继服务
		dep2p.EnableRelayService(
			relay.WithResources(
				relay.Resources{
					Limit: &relay.RelayLimit{
						Data:     def.Limit.Data,     // 设置中继数据限制
						Duration: def.Limit.Duration, // 设置中继持续时间
					},
					MaxCircuits:            def.MaxCircuits,            // 最大中继电路数
					BufferSize:             def.BufferSize,             // 缓冲区大小
					ReservationTTL:         def.ReservationTTL,         // 预留时间
					MaxReservations:        def.MaxReservations,        // 最大预留数
					MaxReservationsPerIP:   def.MaxReservationsPerIP,   // 每IP最大预留数
					MaxReservationsPerPeer: def.MaxReservationsPerPeer, // 每节点最大预留数
					MaxReservationsPerASN:  def.MaxReservationsPerASN,  // 每ASN最大预留数
				},
			),
		),
	}

	// 如果指定了端口，配置多个监听地址
	if port != "" {
		// 配置全面的监听地址列表,包括IPv4/IPv6和不同传输协议
		addresses := []string{
			// TCP 监听地址 - 支持IPv4和IPv6
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port), // IPv4 所有网络接口

		}

		// 将监听地址和传输配置添加到选项中
		options = append(options,
			// 设置所有监听地址
			dep2p.ListenAddrStrings(addresses...),

			// TCP传输配置 - 包含多个优化选项
			dep2p.Transport(tcp.NewTCPTransport,
				tcp.WithMetrics(),                         // 启用TCP指标收集
				tcp.DisableReuseport(),                    // 禁用SO_REUSEPORT
				tcp.WithConnectionTimeout(42*time.Second), // 设置连接超时时间
				tcp.WithMetrics(),                         // 再次确保指标收集
			),

			// 将节点可达性设置为公网可访问
			dep2p.ForceReachabilityPublic(),
		)
	} else {
		// 如果未指定端口,将节点设置为私网节点
		options = append(options,
			dep2p.ForceReachabilityPrivate(),
		)
	}

	return options, nil
}

// GenerateECDSAKeyPair 生成ECDSA密钥对
func GenerateECDSAKeyPair(password, salt []byte, iterations, keyLen int, compressed bool) (*ecdsa.PrivateKey, []byte, error) {
	logger.Info("开始生成ECDSA密钥对...")

	if salt == nil {
		salt = []byte("bpfs-salt")
		logger.Debug("使用默认盐值")
	}

	logger.Debug("生成密钥...")
	key := pbkdf2.Key(password, salt, iterations, keyLen, sha512.New)
	hasher := sha256.New()
	hasher.Write(key)
	seed := hasher.Sum(nil)

	logger.Debug("创建主密钥...")
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		logger.Errorf("创建主密钥失败: %v", err)
		return nil, nil, fmt.Errorf("创建主密钥失败")
	}

	logger.Debug("生成ECDSA密钥...")
	curve := elliptic.P256()
	privateKey := new(ecdsa.PrivateKey)
	privateKey.PublicKey.Curve = curve
	privateKey.D = new(big.Int).SetBytes(masterKey.Key)

	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(masterKey.Key)

	var pubKey []byte
	if compressed {
		logger.Debug("使用压缩格式的公钥")
		pubKey = append([]byte{0x02 + byte(privateKey.PublicKey.Y.Bit(0))},
			privateKey.PublicKey.X.Bytes()...)
	} else {
		logger.Debug("使用未压缩格式的公钥")
		pubKey = append(privateKey.PublicKey.X.Bytes(),
			privateKey.PublicKey.Y.Bytes()...)
	}

	logger.Info("ECDSA密钥对生成成功")
	return privateKey, pubKey, nil
}

// GetPrimaryMACAddress 获取主要MAC地址
func GetPrimaryMACAddress() (string, error) {
	logger.Info("开始获取主要MAC地址...")
	interfaces, err := net.Interfaces()
	if err != nil {
		logger.Errorf("获取网络接口列表失败: %v", err)
		return "", fmt.Errorf("获取网络接口列表失败")
	}

	var bestIface ifaceInfo
	for _, iface := range interfaces {
		if iface.HardwareAddr == nil || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		info := ifaceInfo{mac: iface.HardwareAddr.String()}
		logger.Debugf("检查接口 %s (MAC: %s)", iface.Name, info.mac)

		if !strings.Contains(iface.Name, "vmnet") && !strings.Contains(iface.Name, "vboxnet") {
			info.weight += 10
			logger.Debug("非虚拟接口 +10分")
		}

		if iface.Flags&net.FlagUp != 0 {
			info.weight += 10
			logger.Debug("接口已启用 +10分")
		}

		addrs, err := iface.Addrs()
		if err != nil {
			logger.Debugf("获取接口地址失败: %v", err)
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				info.weight += 10
				logger.Debug("有效IPv4地址 +10分")
				break
			}
		}

		logger.Debugf("接口 %s 总分: %d", iface.Name, info.weight)
		if info.weight > bestIface.weight {
			bestIface = info
			logger.Debugf("更新最佳接口为: %s", iface.Name)
		}
	}

	if bestIface.mac == "" {
		logger.Error("未找到有效的MAC地址")
		return "", fmt.Errorf("未找到有效的MAC地址")
	}

	logger.Infof("选择的MAC地址: %s", bestIface.mac)
	return bestIface.mac, nil
}

// getFreePort 获取空闲端口号
func getFreePort() (string, error) {
	logger.Info("开始获取空闲端口...")
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		logger.Errorf("创建临时监听器失败: %v", err)
		return "", fmt.Errorf("创建临时监听器失败")
	}
	defer listener.Close()

	address := listener.Addr().(*net.TCPAddr)
	port := strconv.Itoa(address.Port)

	goos := runtime.GOOS
	if goos == "linux" {
		port = "4001"
	}

	logger.Infof("获取到空闲端口: %s", port)
	return port, nil
}

type ifaceInfo struct {
	mac    string
	weight int
}
