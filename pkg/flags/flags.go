package flags

import (
	"flag"
	"net"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/network"
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
			addrs, err := netlink.AddrList(nic, netlink.FAMILY_V4)
			if err != nil {
				continue
			}
			filteredAddr := network.FilterLoopbackAddr(addrs)
			if len(filteredAddr) == 1 {
				nodeIP = filteredAddr[0].IPNet.String()
				return
			} else if len(filteredAddr) > 1 {
				glog.Warningf("multiple address %v found on device %s, ignore this device", filteredAddr, dev)
				continue
			}
		}
		glog.Fatalf("cann't find valid node ip from ifaces %s", nodeIP)
	})
	return nodeIP
}
