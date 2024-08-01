package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	libp2ppubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-pubsub/timecache"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	"github.com/bpfs/defs"

	"github.com/bpfs/defs/opts"

	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
	"github.com/tyler-smith/go-bip32"
	"golang.org/x/crypto/pbkdf2"
)

func main() {
	ctx := context.Background()
	//pskmd5 := md5.New()
	//psk := []byte(hex.EncodeToString(pskmd5.Sum(nil))) // TODO: 可能报错

	psk := []byte(nil) // TODO: 可能报错

	macAddressEnv, err := GetPrimaryMACAddress()
	if err != nil {
		logrus.Errorf("GetPrimaryMACAddress 报错:\t%v", err)
		return
	}
	privateKey, _, err := GenerateECDSAKeyPair([]byte(macAddressEnv), nil, 2048, 64, true)
	if err != nil {
		logrus.Errorf("GenerateECDSAKeyPair 报错:\t%v", err)
		return
	}

	// privateKey1, _, err := GenerateECDSAKeyPair([]byte("1111"), nil, 2048, 64, true)
	// if err != nil {
	// 	logrus.Errorf("GenerateECDSAKeyPair 报错:\t%v", err)
	// 	return
	// }

	// 生成私钥
	privKey, _, err := crypto.ECDSAKeyPairFromKey(privateKey)
	if err != nil {
		logrus.Errorf("ECDSAKeyPairFromKey 报错:\t%v", err)
		return
	}

	// 获取空闲端口号
	port, err := getFreePort()
	if err != nil {
		logrus.Errorf("getFreePort 报错:\t%v", err)
		return
	}

	p2p, err := dep2p.NewDeP2P(ctx,
		dep2p.WithLibp2pOpts(buildHostOptions(psk, privKey, port)), // 设置libp2p选项
		dep2p.WithDhtOpts(buildDHTOptions(2)),                      // 设置dht选项
		dep2p.WithRendezvousString(RendezvousString),               // 配置
	)
	if err != nil {
		logrus.Errorf("NewBpfs 报错:\t%v", err)
		return
	}

	pubsub, err := NewBpfsPubSub(ctx, p2p)
	if err != nil {
		logrus.Errorf("NewBpfsPubSub 报错:\t%v", err)
		return
	}

	// 设置一个推荐选项列表以获得良好的性能
	opt := opts.DefaultOptions()
	// 设置是否启动本地存储选项
	opt.BuildLocalStorage(true)
	//opt.BuildLocalStorage(false)
	opt.BuildRoutingTableLow(1)
	//opt.BuildDefaultFileKey("11111")
	opt.BuildStorageMode(opts.RS_Proportion)
	// opt.BuildRootPath("/Users/qinglong/go/src/chaincodes/BPFS/BPFSFILE/")
	opt.BuildDownloadMaximumSize(int64(MBMultiplier(3)))
	// _, err = defs.Open(opt, p2p, pubsub)
	fs, err := defs.Open(opt, p2p, pubsub)
	if err != nil {
		logrus.Errorf("Open 报错:\t%v", err)
		return
	}
	logrus.Debugf("FS 启动 %v", fs)

	// 【测试须知】
	// 至少需要启动 2 个节点，分别扮演[文件存储节点]和[业务请求节点]
	// 文件存储节点，直接在当前路径运行: go run main.go 即可
	// 业务请求节点，需要分别注销 第120行、第141行、第143行、第154行，它们分别是【上传】和【下载】的测试操作
	// 你可以同时测试上传+下载，也可以单独测试上传后，结束再测试下载
	// 如果你将上传和下载分开测试，注意需要将 第行 的"file.FileID"修改为文件ID，如果测试的是"BPFS 白皮书.pdf"，复制下面的那行即可；如果测试的是自己的文件，可在上传的打印日志里复制

	/**

	time.Sleep(60 * time.Second) // 延时20秒

	path := "BPFS 白皮书.pdf"
	// fcd550eff8fa3e77c897e430177ea1b098480eb527022bf159bff95fa44a2846

	logrus.Println(time.Now())
	file, err := fs.Upload().NewUpload(fs.Opt(), fs.Afero(), fs.P2P(), fs.Pubsub(), path, privateKey)
	if err != nil {
		logrus.Errorf("Upload 报错:\t%v", err)
		return
	}
	logrus.Print("\n\n===== 测试方法 =====\n")
	logrus.Println(time.Now())
	logrus.Print("\n\n返回值:\n")
	logrus.Printf("FileID:\t%v\n", file.FileID)
	logrus.Printf("Size:\t%v\n", file.Size)
	logrus.Printf("UploadTime:\t%v\n", file.UploadTime)
	logrus.Print("===== 测试方法 =====\n\n")

	*/

	/**

	time.Sleep(60 * time.Second) // 延时60秒

	download, err := fs.Download().NewDownload(fs.Opt(), fs.Afero(), fs.P2P(), fs.Pubsub(), file.FileID, privateKey)
	if err != nil {
		logrus.Errorf("downloads 报错:\t%v", err)
		return
	}
	logrus.Printf("下载任务开启\t%s\n\n", download.TaskID)

	*/

	// logrus.Printf("开始更新名字任务\t%s\n\n", file.FileID)

	// // newName := "测试BPFS白皮书.pdf"
	// // if err := edits.EditName(fs.P2P(), fs.Pubsub(), file.FileID, privateKey, newName); err != nil {
	// // 	logrus.Errorf("Download 报错:\t%v", err)
	// // 	return
	// // }

	// logrus.Printf("开始修改共享任务\t%s\n\n", file.FileID)

	// if err := edits.EditShared(fs.P2P(), fs.Pubsub(), file.FileID, true, privateKey); err != nil {
	// 	logrus.Errorf("Download 报错:\t%v", err)
	// 	return
	// }

	// time.Sleep(10 * time.Second) // 延时10秒
	// logrus.Printf("开始搜索下载任务\t%s\n\n", file.FileID)

	// // 使用私钥和文件校验和生成秘密
	// secret, err := uploads.GenerateSecretFromPrivateKeyAndChecksum(privateKey, []byte(file.FileID))
	// if err != nil {
	// 	return
	// }

	// searchDownload, err := downloads.NewSearchDownload(fs.Ctx(), fs.Opt(), fs.P2P(), fs.Pubsub(), fs.Download(), file.FileID, secret, privateKey1)
	// if err != nil {
	// 	logrus.Errorf("downloads 报错:\t%v", err)
	// }
	// logrus.Printf("搜索下载任务完成\t%s\n\n", searchDownload.TaskID)

	// // logrus.Printf("开始新增分享任务\t%s\n\n", file.FileID)
	// // address := "12gpXQVcCL2qhTNQgyLVdCFG2Qs2px98nV"
	// // userPubHash, err := wallet.GetPubKeyHash(address)
	// // if err != nil {
	// // 	return
	// // }
	// // currentTime := time.Now()
	// // logrus.Println("当前时间:", currentTime)

	// // // 在当前时间加一天
	// // tomorrow := currentTime.AddDate(0, 0, 1)
	// // if err := edits.AddShared(fs.P2P(), fs.Pubsub(), file.FileID, userPubHash, tomorrow); err != nil {
	// // 	logrus.Errorf("Download 报错:\t%v", err)
	// // 	return
	// // }
	// // time.Sleep(10 * time.Second) // 延时10秒 防止还没有修改完

	// // logrus.Printf("开始再次下载任务\t%s\n\n", file.FileID)

	// // address2 := "1JYy9LLVAAQ5edAuu9QP7LjmFa632J1PZR"
	// // userPubHash2, err := wallet.GetPubKeyHash(address2)
	// // if err != nil {

	// // 	return
	// // }

	// // if err := downloads.NewDownload(fs.Ctx(), fs.Opt(), fs.P2P(), fs.Pubsub(), fs.Download(), file.FileID, userPubHash2); err != nil {
	// // 	logrus.Errorf("Download 报错:\t%v", err)
	// // 	return
	// // }
	select {}
}

// NewBpfsPubSub 新的 BPFS 主题
func NewBpfsPubSub(ctx context.Context, p2p *dep2p.DeP2P) (*pubsub.DeP2PPubSub, error) {
	// 初始化
	pb, err := pubsub.NewPubsub(ctx, p2p.Host()) // 初始化 PubSub
	if err != nil {
		return nil, err
	}
	// 返回新创建的 LibP2pPubSub 实例
	if err := pb.Start(buildPubSub()...); err != nil {
		return nil, err
	}

	return pb, nil
}

const (
	// DefaultConnMgrHighWater 是连接管理器'high water'标记的默认值
	DefaultConnMgrHighWater = 96
	// DefaultConnMgrLowWater 是连接管理器'low water'标记的默认值
	DefaultConnMgrLowWater = 32
	// DefaultConnMgrGracePeriod 是连接管理器宽限期的默认值
	DefaultConnMgrGracePeriod = time.Second * 20

	// RendezvousString = "rendezvous:wesign.xyz.cs41"
	RendezvousString = "[rendezvous] wesign.xyz CS-DeFS-0"
)

// 设置libp2p选项
func buildHostOptions(psk pnet.PSK, sk crypto.PrivKey, portNumber string) []config.Option {
	// IPFS配置
	grace := DefaultConnMgrGracePeriod
	low := int(DefaultConnMgrLowWater)
	high := int(DefaultConnMgrHighWater)

	// NewConnManager 使用提供的参数创建一个新的 BasicConnMgr：lo 和 hi 是管理将维护的连接数量的水印。
	// 当对等点计数超过'high water'时，许多对等点将被修剪（并且它们的连接终止），直到保留'low water'对等点。
	cm, err := connmgr.NewConnManager(low, high, connmgr.WithGracePeriod(grace))
	if err != nil {
		logrus.Errorf("初始化cm节点%v", err)
	}

	// NewPeerstore 创建内存中线程安全的对等点集合。
	// 调用者有责任调用RemovePeer以确保peerstore的内存消耗不会无限制地增长。
	libp2pPeerstore, err := pstoremem.NewPeerstore()
	if err != nil {
		logrus.Errorf("初始化存储节点%v", err)
	}

	// rcmgrObs.MustRegisterWith(prometheus.DefaultRegisterer) 注册 rcmgrObs 对象到 Prometheus 的默认注册器中，
	// 以便将其暴露为 Prometheus 指标。
	//rcmgrObs.MustRegisterWith(prometheus.DefaultRegisterer)

	// str 是一个 StatsTraceReporter 对象，用于收集和报告资源管理器的统计信息和追踪数据。
	// rcmgrObs.NewStatsTraceReporter() 用于创建 StatsTraceReporter 对象。
	// 如果创建过程中发生错误，则将错误记录下来并终止程序的执行。
	str, err := rcmgr.NewStatsTraceReporter()
	if err != nil {
		logrus.Errorf("初始化rcmgrObs%v", err)
	}

	// rmgr 是一个资源管理器对象，用于管理和分配资源。
	// rcmgr.NewResourceManager() 用于创建资源管理器对象，参数包括资源限制器和追踪报告器等选项。
	// 如果创建过程中发生错误，则将错误记录下来并终止程序的执行。
	rmgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.DefaultLimits.AutoScale()), rcmgr.WithTraceReporter(str))
	if err != nil {
		logrus.Fatal(err)
	}
	// 默认中继配置选项
	def := relay.DefaultResources()
	options := []libp2p.Option{
		// Peerstore 配置 libp2p 以使用给定的对等存储。
		libp2p.Peerstore(libp2pPeerstore),
		// Ping 会配置libp2p 来支持ping 服务； 默认启用。
		libp2p.Ping(false),
		libp2p.Identity(sk),
		// DefaultSecurity 是默认的安全选项。
		// 当您想要扩展而不是替换受支持的传输安全协议时非常有用。
		libp2p.DefaultSecurity,
		// ConnectionManager 将 libp2p 配置为使用给定的连接管理器。
		libp2p.ConnectionManager(cm),
		// ResourceManager 将 libp2p 配置为使用给定的 ResourceManager。
		// 当使用 ResourceManager 接口的 p2p/host/resource-manager 实现时，建议通过调用 SetDefaultServiceLimits 设置 libp2p 协议的限制。
		libp2p.ResourceManager(rmgr),
		// 对于大文件传输，选择合适的传输协议很重要。您可以尝试使用 QUIC（基于 UDP 的传输协议），因为它具有低延迟、高并发和连接迁移等优点。
		// libp2p.Transport(quic.NewTransport),
		// TODO: 需要根据新包优化
		// 使用 QUIC（基于 UDP 的传输协议）并设置连接超时
		// libp2p.Transport(func(u *tptu.Upgrader) *quic.Transport {
		//  t := quic.NewTransport(u)
		//  t.SetHandshakeTimeout(5 * time.Minute) // 设置为 5 分钟，您可以根据需求调整这个值
		//  return t
		// }),
		// Muxer 配置 libp2p 以使用给定的流多路复用器。 name 是协议名称。
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport), // 添加 yamux 传输协议
		// 尝试使用 uPNP 为 NATed 主机打开端口。
		libp2p.NATPortMap(),

		// EnableRelay 配置 libp2p 以启用中继传输。
		libp2p.EnableRelay(),
		// 实验性 EnableHolePunching 通过启用 NATT 的对等点来启动和响应打孔尝试以创建与其他对等点的直接/NAT 遍历连接来启用 NAT 遍历。
		libp2p.EnableHolePunching(),
		// EnableNATService 将 libp2p 配置为向对等点提供服务以确定其可达性状态。
		libp2p.EnableNATService(),
		// 启用中继服务
		libp2p.EnableRelayService(
			relay.WithResources(
				relay.Resources{
					Limit: &relay.RelayLimit{
						Data:     def.Limit.Data,     // 128K，设置每个中继数据的限制为128K
						Duration: def.Limit.Duration, // 设置每个中继的持续时间为2分钟
					},
					MaxCircuits:            def.MaxCircuits,            // 设置最大的中继电路数量为16个
					BufferSize:             def.BufferSize,             // 缓冲区大小由2048（2kb）调整为20480（20kb）
					ReservationTTL:         def.ReservationTTL,         // 设置中继预留的生存时间为1小时
					MaxReservations:        def.MaxReservations,        // 设置最大中继预留数量为128个
					MaxReservationsPerIP:   def.MaxReservationsPerIP,   // 设置每个IP地址最大的中继预留数量为8个
					MaxReservationsPerPeer: def.MaxReservationsPerPeer, // 设置每个对等节点最大的中继预留数量为4个
					MaxReservationsPerASN:  def.MaxReservationsPerASN,  // 设置每个ASN（自治系统号）最大的中继预留数量为32个
				},
			),
		),
	}
	// 如果是中继节点指定host主机端口
	if portNumber != "" {
		// ListenAddrStrings 配置 libp2p 来监听给定的（未解析的）地址。
		options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", portNumber)))
		// Transport 将 libp2p 配置为使用给定的传输（或传输构造函数）。
		options = append(options, libp2p.Transport(tcp.NewTCPTransport, tcp.WithMetrics()))
		// ForceReachabilityPublic 覆盖了AutoNAT子系统中的自动可达性检测，迫使本地节点相信它是可以从外部到达的。
		options = append(options, libp2p.ForceReachabilityPublic())
	} else {
		// ForceReachabilityPrivate 覆盖 AutoNAT 子系统中的自动可达性检测，强制本地节点相信它在 NAT 后面并且无法从外部访问。
		options = append(options, libp2p.ForceReachabilityPrivate())
	}

	// 私有网络
	if psk != nil {
		options = append(options, []libp2p.Option{
			// PrivateNetwork 将 libp2p 配置为使用给定的专用网络保护器。
			libp2p.PrivateNetwork(psk),
		}...)
	}

	return options
}

// 设置dht选项
func buildDHTOptions(mode int) []dep2p.Option {
	baseOpts := []dep2p.Option{}
	switch mode {
	case 0:
		baseOpts = append(baseOpts, dep2p.Mode(dep2p.ModeAuto))
	case 1:

		baseOpts = append(baseOpts, dep2p.Mode(dep2p.ModeClient)) // 客户端
	case 2:

		baseOpts = append(baseOpts, dep2p.Mode(dep2p.ModeServer)) // 服务器
	case 3:
		baseOpts = append(baseOpts, dep2p.Mode(dep2p.ModeAutoServer))
	}

	return baseOpts
}

func buildPubSub() []libp2ppubsub.Option {
	var pubsubOptions []libp2ppubsub.Option

	ttl, err := time.ParseDuration("10s")
	if err != nil {
		panic(err)
	}

	pubsubOptions = append(
		pubsubOptions,
		// 设置 pubsub 有线消息的全局最大消息大小。 默认值为 1MiB (DefaultMaxMessageSize)
		libp2ppubsub.WithMaxMessageSize(pubsub.DefaultLibp2pPubSubMaxMessageSize),
		// WithSeenMessagesTTL 配置何时可以忘记以前看到的消息 ID
		libp2ppubsub.WithSeenMessagesTTL(ttl),
		// Stategy_LastSeen 使上次被 Add 或 Has 触及的条目过期。
		libp2ppubsub.WithSeenMessagesStrategy(timecache.Strategy_LastSeen),
	)
	return pubsubOptions
}

func GenerateECDSAKeyPair(password []byte, salt []byte, iterations, keyLength int, useSHA512 bool) (*ecdh.PrivateKey, []byte, error) {
	curve := elliptic.P256() // 根据需要选择合适的曲线

	// 选择合适的哈希函数
	var hashFunc func() hash.Hash
	if useSHA512 {
		hashFunc = sha512.New
	} else {
		hashFunc = sha256.New
	}

	combined := append([]byte("BPFS"), salt...)

	// 使用 PBKDF2 生成强密钥
	key := pbkdf2.Key(password, combined, iterations, keyLength, hashFunc)

	// 生成主钱包
	masterKey, _ := bip32.NewMasterKey(key) //?????? 如果不使用启动host会报错

	// 生成私钥
	privateKey := &ecdh.PrivateKey{
		PublicKey: ecdh.PublicKey{
			Curve: curve,
		},
		D: new(big.Int).SetBytes(masterKey.Key),
	}

	// 计算公钥
	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(masterKey.Key)

	// 生成公钥
	pubKey := append(privateKey.PublicKey.X.Bytes(), privateKey.PublicKey.Y.Bytes()...)

	return privateKey, pubKey, nil
}

type ifaceInfo struct {
	mac    string
	weight int
}

// GetPrimaryMACAddress 返回电脑上的主要MAC地址。
func GetPrimaryMACAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var bestIface ifaceInfo

	for _, iface := range interfaces {
		if iface.HardwareAddr == nil || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		info := ifaceInfo{mac: iface.HardwareAddr.String()}

		// 为非虚拟接口增加权重
		if !strings.Contains(iface.Name, "vmnet") && !strings.Contains(iface.Name, "vboxnet") {
			info.weight += 10
		}

		// 为状态为"up"的接口增加权重
		if iface.Flags&net.FlagUp != 0 {
			info.weight += 10
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// 为有IPv4地址的接口增加权重
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				info.weight += 10
				break
			}
		}

		// 选择权重最高的接口
		if info.weight > bestIface.weight {
			bestIface = info
		}
	}

	if bestIface.mac == "" {
		return "", fmt.Errorf("no MAC address found")
	}

	return bestIface.mac, nil
}

// 获取空闲端口号
func getFreePort() (string, error) {
	// 监听一个未指定端口的TCP地址
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	defer listener.Close()

	// 获取监听的地址
	address := listener.Addr().(*net.TCPAddr)
	port := strconv.Itoa(address.Port)

	return port, nil
}

// MBMultiplier 返回给定大小的1MB（兆字节）的倍数。
// 参数 size 表示要计算的1MB的倍数。
func MBMultiplier(size int) int {
	mbInBytes := 1 * 1024 * 1024 // 1MB等于 1 * 1024 * 1024 字节
	return size * mbInBytes
}
