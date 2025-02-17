// Package net 提供了网络相关的功能实现
package net

/*
本文件 (rcmgr_defaults.go) 主要实现了 libp2p 网络中资源管理器的默认配置。主要功能和组件包括：

1. infiniteResourceLimits 变量:
   定义了无限制的资源限制，用于某些不需要限制的场景。

2. createDefaultLimitConfig 函数:
   这是文件的核心函数，用于创建默认的资源限制配置。主要功能包括：
   a) 计算默认的最大内存限制（总系统内存的一半）。
   b) 解析配置中的最大内存和文件描述符限制。
   c) 设置系统级别的资源限制，包括内存、文件描述符、连接数等。
   d) 设置瞬时连接的资源限制（系统限制的 25%）。
   e) 设置允许列表中资源的无限制。
   f) 设置服务、协议、连接和流的默认限制。
   g) 设置对等节点的默认资源限制。
   h) 根据连接管理器的配置调整系统入站连接限制。
   i) 生成启动日志消息，包含计算得出的资源限制信息。

   参数:
   - cfg: config.SwarmConfig 类型，包含 Swarm 配置信息。

   返回值:
   - limitConfig: rcmgr.ConcreteLimitConfig 类型，具体的限制配置。
   - logMessageForStartup: string 类型，启动时的日志消息。
   - err: error 类型，可能的错误。

这个函数在 libp2p 网络中扮演着关键角色，它为资源管理器提供了默认的配置。
通过细致的资源限制设置，它帮助网络在各种环境下保持稳定和高效运行。
特别是在处理大规模网络或资源受限的环境时，这种配置非常重要。

注意事项：
1. 配置会根据系统的总内存自动调整，以适应不同的硬件环境。
2. 文件中包含了详细的注释，解释了每个限制的设置原因和计算方法。
3. 配置考虑了系统级别、瞬时连接、对等节点等多个层面的资源管理。
4. 启动日志消息提供了配置的概览，有助于运维和调试。

这个文件的设计体现了 libp2p 在资源管理上的灵活性和可配置性，
允许网络在不同场景下都能得到良好的性能表现。
*/

import (

	// libp2p 核心库
	"time"

	"github.com/bpfs/defs/v2/net/fd"

	"github.com/dep2p/go-dep2p"
	rcmgr "github.com/dep2p/go-dep2p/p2p/host/resource-manager" // libp2p 资源管理器
	"github.com/dustin/go-humanize"                             // 用于人性化显示字节大小
	"github.com/pbnjay/memory"                                  // 用于获取系统内存信息
)

const (

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

// DefaultResourceMgrMinInboundConns 定义了作为良好网络公民的最小入站连接数
// 这是一个经验值，通常足以保证网络的正常运行
// const DefaultResourceMgrMinInboundConns = 800
// 3. 增加最小入站连接数
const DefaultResourceMgrMinInboundConns = 800 // 从800增加到1600

// infiniteResourceLimits 定义了无限制的资源限制配置
// 用于某些不需要限制的特殊场景
var infiniteResourceLimits = rcmgr.InfiniteLimits.ToPartialLimitConfig().System

// DefaultConnMgrType 定义了连接管理器的默认类型
// 当前设置为 "basic" 基础类型
const DefaultConnMgrType = "basic"

// createDefaultLimitConfig 创建并返回一个默认的资源限制配置
//
// 此函数根据系统资源情况自动计算和设置各种限制，包括内存、文件描述符、
// 连接数和流量等。它遵循 docs/libp2p-resource-management.md 中的文档规范。
//
// 返回值:
//   - rcmgr.ConcreteLimitConfig: 包含所有具体限制配置的结构体
//   - error: 如果在创建过程中发生错误则返回错误信息
func CreateDefaultLimitConfig() (limitConfig rcmgr.ConcreteLimitConfig, err error) {
	// 计算默认最大内存值（系统总内存的一半）
	maxMemoryDefaultString := humanize.Bytes(uint64(memory.TotalMemory()) / 2)
	// 1. 增加系统内存使用比例（从1/2增加到2/3）
	//maxMemoryDefaultString := humanize.Bytes(uint64(memory.TotalMemory()) * 2 / 3)

	// 使用默认的最大内存值
	maxMemoryString := maxMemoryDefaultString

	// 将内存字符串解析为字节数
	maxMemory, err := humanize.ParseBytes(maxMemoryString)
	if err != nil {
		logger.Errorf("解析最大内存值失败: %v", err)
		return rcmgr.ConcreteLimitConfig{}, err
	}

	// 将最大内存转换为 MB 单位，便于后续计算
	maxMemoryMB := maxMemory / (1024 * 1024)

	// 获取系统最大文件描述符数的一半作为限制
	maxFD := int(int64(fd.GetNumFDs()) / 2)

	// 设置每 MB 内存允许 1 个入站连接
	// systemConnsInbound := int(1 * maxMemoryMB)
	// 2. 增加每MB内存允许的连接数（从1增加到2）
	// systemConnsInbound := int(2 * maxMemoryMB)

	// 至少从 2023-01-25 起，可以通过 libp2p 资源管理器/会计师打开一个
	// 使用 libp2p 资源管理器/会计师打开一个不占用任何内存的连接（参见 ）。
	// 的连接（见 https://github.com/libp2p/go-libp2p/issues/2010#issuecomment-1404280736）。
	// 因此，我们目前无法依靠内存限制来完全保护我们。
	// 在 https://github.com/libp2p/go-libp2p/issues/2010 解决之前、
	// 我们现在采取的代理方法是限制每 MB 只能有一个入站连接。
	// 注意：这比 go-libp2p 的默认自动缩放限制要宽松得多。
	// 每 1GB 64 个连接
	// （参见 https://github.com/libp2p/go-libp2p/blob/master/p2p/host/resource-manager/limit_defaults.go#L357 ）。
	systemConnsInbound := int(1 * maxMemoryMB)

	// 创建部分限制配置
	partialLimits := rcmgr.PartialLimitConfig{
		// System: 系统级别的资源限制
		System: rcmgr.ResourceLimits{
			Memory: rcmgr.LimitVal64(maxMemory), // 系统可使用的最大内存量
			FD:     rcmgr.LimitVal(maxFD),       // 系统可使用的最大文件描述符数量

			// 连接数限制
			Conns:         rcmgr.Unlimited,                    // 系统总连接数，设为无限制
			ConnsInbound:  rcmgr.LimitVal(systemConnsInbound), // 入站连接数限制，基于内存计算
			ConnsOutbound: rcmgr.Unlimited,                    // 出站连接数，设为无限制

			// 流数限制
			Streams:         rcmgr.Unlimited, // 系统总流数，设为无限制
			StreamsOutbound: rcmgr.Unlimited, // 出站流数，设为无限制
			StreamsInbound:  rcmgr.Unlimited, // 入站流数，设为无限制
		},

		// Transient: 瞬时资源限制，通常设置为系统限制的 25%
		Transient: rcmgr.ResourceLimits{
			Memory: rcmgr.LimitVal64(maxMemory / 4), // 瞬时最大内存使用量
			FD:     rcmgr.LimitVal(maxFD / 4),       // 瞬时最大文件描述符数
			// Memory: rcmgr.Unlimited64, // 瞬时最大内存使用量
			// FD:     rcmgr.Unlimited,   // 瞬时最大文件描述符数

			Conns:        rcmgr.Unlimited,                        // 瞬时总连接数，无限制
			ConnsInbound: rcmgr.LimitVal(systemConnsInbound / 4), // 瞬时入站连接数限制
			//ConnsInbound:  rcmgr.Unlimited, // 瞬时入站连接数限制
			ConnsOutbound: rcmgr.Unlimited, // 瞬时出站连接数，无限制

			Streams:         rcmgr.Unlimited, // 瞬时总流数，无限制
			StreamsInbound:  rcmgr.Unlimited, // 瞬时入站流数，无限制
			StreamsOutbound: rcmgr.Unlimited, // 瞬时出站流数，无限制
		},

		// 白名单资源限制配置
		AllowlistedSystem:    infiniteResourceLimits, // 白名单系统资源无限制
		AllowlistedTransient: infiniteResourceLimits, // 白名单瞬时资源无限制

		// 服务级别限制
		ServiceDefault:     infiniteResourceLimits, // 默认服务资源限制
		ServicePeerDefault: infiniteResourceLimits, // 默认服务对等点资源限制

		// 协议级别限制
		ProtocolDefault:     infiniteResourceLimits, // 默认协议资源限制
		ProtocolPeerDefault: infiniteResourceLimits, // 默认协议对等点资源限制

		// 连接和流限制
		Conn:   infiniteResourceLimits, // 单个连接的资源限制
		Stream: infiniteResourceLimits, // 单个流的资源限制

		// 对等点默认限制
		PeerDefault: rcmgr.ResourceLimits{
			Memory: rcmgr.Unlimited64, // 对等点内存使用无限制
			FD:     rcmgr.Unlimited,   // 对等点文件描述符无限制

			Conns:         rcmgr.Unlimited,    // 对等点总连接数无限制
			ConnsInbound:  rcmgr.DefaultLimit, // 对等点入站连接使用默认限制
			ConnsOutbound: rcmgr.Unlimited,    // 对等点出站连接无限制

			Streams:         rcmgr.Unlimited,    // 对等点总流数无限制
			StreamsInbound:  rcmgr.DefaultLimit, // 对等点入站流使用默认限制
			StreamsOutbound: rcmgr.Unlimited,    // 对等点出站流无限制
		},
	}

	// 获取默认的缩放限制配置
	scalingLimitConfig := rcmgr.DefaultLimits
	dep2p.SetDefaultServiceLimits(&scalingLimitConfig)

	// 根据系统资源构建部分限制配置
	partialLimits = partialLimits.Build(scalingLimitConfig.Scale(int64(maxMemory), maxFD)).ToPartialLimitConfig()

	// 根据连接管理器类型调整系统入站连接限制
	if partialLimits.System.ConnsInbound > rcmgr.DefaultLimit && DefaultConnMgrType != "none" {
		maxInboundConns := int64(partialLimits.System.ConnsInbound)

		// 确保最大入站连接数不小于连接管理器高水位线的两倍
		if connmgrHighWaterTimesTwo := DefaultConnMgrHighWater * 2; maxInboundConns < int64(connmgrHighWaterTimesTwo) {
			maxInboundConns = int64(connmgrHighWaterTimesTwo)
		}

		// 确保最大入站连接数不小于最小入站连接数
		if maxInboundConns < DefaultResourceMgrMinInboundConns {
			maxInboundConns = DefaultResourceMgrMinInboundConns
		}

		// 按比例调整系统入站流限制
		if partialLimits.System.StreamsInbound > rcmgr.DefaultLimit {
			partialLimits.System.StreamsInbound = rcmgr.LimitVal(maxInboundConns * int64(partialLimits.System.StreamsInbound) / int64(partialLimits.System.ConnsInbound))
		}
		partialLimits.System.ConnsInbound = rcmgr.LimitVal(maxInboundConns)
	}

	// 构建并返回最终的具体限制配置
	return partialLimits.Build(rcmgr.ConcreteLimitConfig{}), nil
}
