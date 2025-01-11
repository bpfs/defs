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

	"github.com/bpfs/defs"
	"github.com/bpfs/defs/fscfg"
	dht "github.com/dep2p/kaddht"
	"github.com/dep2p/libp2p"
	"github.com/dep2p/libp2p/config"
	"github.com/dep2p/libp2p/core/crypto"
	"github.com/dep2p/libp2p/core/discovery"
	"github.com/dep2p/libp2p/core/host"
	"github.com/dep2p/libp2p/core/network"
	"github.com/dep2p/libp2p/core/peer"
	routingdisc "github.com/dep2p/libp2p/p2p/discovery/routing"
	"github.com/dep2p/libp2p/p2p/discovery/util"
	"github.com/dep2p/libp2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/dep2p/libp2p/p2p/host/resource-manager"
	"github.com/dep2p/libp2p/p2p/muxer/yamux"
	"github.com/dep2p/libp2p/p2p/net/connmgr"
	"github.com/dep2p/libp2p/p2p/transport/tcp"
	"github.com/dep2p/pubsub"
	"github.com/tyler-smith/go-bip32"
	"golang.org/x/crypto/pbkdf2"
)

const (
	RendezvousString = "[rendezvous] wesign.xyz CS-DeFS-1"
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
	h, err := libp2p.New(buildHostOptions(privKey, port)...)
	if err != nil {
		logger.Errorf("创建 libp2p host 失败: %v", err)
		return nil, fmt.Errorf("创建 libp2p host 失败")
	}
	logger.Infof("libp2p host 创建成功，节点ID: %s", h.ID().String())

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
		fscfg.WithBucketSize(200),
		fscfg.WithMaxPeersPerCpl(5),
		fscfg.WithPubSubOption(pubsub.WithSetFollowupTime(1 * time.Second)),
		fscfg.WithPubSubOption(pubsub.WithSetGossipFactor(0.3)),
		fscfg.WithPubSubOption(pubsub.WithSetMaxMessageSize(2 << 20)),
		fscfg.WithPubSubOption(pubsub.WithNodeDiscovery(disc)),
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
	if err := connectToBootstrapPeers(ctx, h, nil); err != nil {
		return err
	}

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

// buildHostOptions 构建 libp2p host 选项
func buildHostOptions(priv crypto.PrivKey, port string) []config.Option {
	logger.Info("开始构建 libp2p host 选项...")
	var opts []config.Option

	// 基本选项
	logger.Debug("添加基本选项...")
	opts = append(opts,
		// 设置节点身份,使用提供的私钥
		libp2p.Identity(priv),
		// 配置监听地址,监听所有网卡的指定端口
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)),
		// 使用默认的多路复用器配置
		libp2p.DefaultMuxers,
		// 使用默认的安全传输配置
		libp2p.DefaultSecurity,
	)

	// 资源管理器 - 用于限制和管理节点资源使用
	logger.Debug("配置资源管理器...")
	// 自动根据系统资源调整限制
	limiter := rcmgr.DefaultLimits.AutoScale()
	rm, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limiter))
	if err != nil {
		logger.Errorf("创建资源管理器失败: %s", err)
		panic("创建资源管理器失败")
	}
	opts = append(opts, libp2p.ResourceManager(rm))

	// 连接管理器 - 控制与其他节点的连接数量
	logger.Debug("配置连接管理器...")
	cm, err := connmgr.NewConnManager(
		300,                                  // 最小连接数(LowWater),当连接数低于此值时会主动寻找新连接
		1000,                                 // 最大连接数(HighWater),当连接数超过此值时会断开不活跃连接
		connmgr.WithGracePeriod(time.Minute), // 设置宽限期,新建立的连接在此期间内不会被连接管理器断开,即使超过最大连接数
		connmgr.WithEmergencyTrim(true),      // 启用紧急裁剪,当系统内存不足时立即触发连接裁剪,不考虑宽限期
	)
	if err != nil {
		logger.Errorf("创建连接管理器失败: %s", err)
		panic("创建连接管理器失败")
	}
	opts = append(opts, libp2p.ConnectionManager(cm))

	// Peerstore - 用于存储和管理对等节点信息
	logger.Debug("配置 Peerstore...")
	libp2pPeerstore, err := pstoremem.NewPeerstore()
	if err != nil {
		logger.Errorf("初始化存储节点失败: %v", err)
		panic("初始化存储节点失败")
	}

	// 其他网络传输和功能选项
	logger.Debug("添加其他选项...")
	opts = append(opts,
		// 配置TCP传输层
		libp2p.Transport(tcp.NewTCPTransport),
		// 配置YAMUX多路复用协议
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		// 设置节点存储实例
		libp2p.Peerstore(libp2pPeerstore),
		// 启用中继功能,允许通过中继节点建立连接
		libp2p.EnableRelay(),
		// 启用NAT穿透功能
		libp2p.EnableHolePunching(),
		// 启用NAT端口映射,改善网络连通性
		libp2p.NATPortMap(),
	)

	// 中继配置 - 设置自动中继发现
	logger.Debug("配置中继选项...")
	relaysCh := make(chan peer.AddrInfo)
	opts = append(opts,
		// 启用自动中继发现,当需要时可以获取中继节点信息
		libp2p.EnableAutoRelayWithPeerSource(func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
			return relaysCh
		}),
	)

	logger.Info("libp2p host 选项构建完成")
	return opts
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
