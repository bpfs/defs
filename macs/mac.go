// MAC 地址

package macs

import (
	"fmt"
	"net"
	"strings"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// GetPrimaryMACAddress 返回电脑上的主要MAC地址。
// 此函数首先获取所有网络接口，然后根据一系列规则确定最适合的MAC地址。
func GetPrimaryMACAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		logrus.Errorf("[%s] 获取网络接口列表失败: %v", debug.WhereAmI(), err)
		return "", err
	}

	var bestIface struct {
		mac    string
		weight int
		index  int // 保持接口索引以处理相同权重的情况
	}

	for i, iface := range interfaces {
		// 跳过没有MAC地址或者是回环接口的设备
		if iface.HardwareAddr == nil || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// 排除虚拟网络接口
		if strings.Contains(iface.Name, "vmnet") || strings.Contains(iface.Name, "vboxnet") || strings.Contains(iface.Name, "docker") || strings.Contains(iface.Name, "lo") {
			continue
		}

		info := struct {
			mac    string
			weight int
			index  int // 同样添加index字段
		}{
			mac:    iface.HardwareAddr.String(),
			weight: 0,
			index:  i, // 存储当前接口的索引
		}

		// 为有线接口增加权重
		if strings.Contains(iface.Name, "eth") {
			info.weight += 20
		}

		// 为状态为"up"的接口增加权重
		if iface.Flags&net.FlagUp != 0 {
			info.weight += 10
		}

		addrs, err := iface.Addrs()
		if err != nil {
			logrus.Errorf("[%s] 获取接口地址失败: %v", debug.WhereAmI(), err)
			continue
		}

		// 为有IPv4地址的接口增加权重
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				info.weight += 10
				break
			}
		}

		// 选择权重最高且索引最小的接口
		if info.weight > bestIface.weight || (info.weight == bestIface.weight && i < bestIface.index) {
			bestIface = info // 现在可以直接赋值，因为info和bestIface结构相同
		}
	}

	if bestIface.mac == "" {
		errMsg := "未找到有效的MAC地址"
		logrus.Errorf("[%s] %s", debug.WhereAmI(), errMsg)
		return "", fmt.Errorf(errMsg)
	}

	return bestIface.mac, nil
}
