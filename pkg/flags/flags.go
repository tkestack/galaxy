package flags

import (
	"flag"
	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
	"net"
	"strings"
	"sync"
)

var (
	flagIface *string = flag.String("iface", "eth1,docker,bond1", "interface to use (CIDR or name) for inter-host communication")
	nodeIP    string
	once      sync.Once
)

func GetNodeIP() string {
	once.Do(func() {
		nodeIP = strings.TrimSpace(*flagIface)
		if nodeIP == "" {
			glog.Fatal("iface unconfigured")
		}
		_, _, err := net.ParseCIDR(nodeIP)
		if err == nil {
			return
		}
		devices := strings.Split(nodeIP, ",")
		if len(devices) == 0 {
			glog.Fatalf("invalid iface configuration %s", nodeIP)
		}
		for _, dev := range devices {
			nic, err := netlink.LinkByName(dev)
			if err != nil {
				continue
			}
			addr, err := netlink.AddrList(nic, netlink.FAMILY_V4)
			if err != nil {
				continue
			}
			if len(addr) == 1 {
				if addr[0].IPNet != nil && addr[0].IP != nil {
					if addr[0].IP.IsLoopback() {
						glog.Infof("ignore loopback address %s on device %s", addr[0].IPNet.String(), dev)
						continue
					}
				}
				nodeIP = addr[0].IPNet.String()
				return
			}
		}
		glog.Fatalf("cann't find valid node ip from ifaces %s", nodeIP)
	})
	return nodeIP
}
