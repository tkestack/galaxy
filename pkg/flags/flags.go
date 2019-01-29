package flags

import (
	"flag"
	"net"
	"strings"
	"sync"

	"git.code.oa.com/gaiastack/galaxy/pkg/network"
	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
)

var (
	flagIface *string = flag.String("iface", "eth1,docker,bond1", "interface to use (IP or CIDR or name) for inter-host communication")
	nodeIP    string
	once      sync.Once
)

func GetNodeIP() string {
	once.Do(func() {
		nodeIP = strings.TrimSpace(*flagIface)
		if nodeIP == "" {
			glog.Fatal("iface unconfigured")
		}
		// try parse as a cidr format
		_, _, err := net.ParseCIDR(nodeIP)
		if err == nil {
			return
		}
		// mayebe just an IP without mask
		ip := net.ParseIP(nodeIP)
		if ip != nil {
			links, err := netlink.LinkList()
			if err != nil {
				glog.Fatal(err)
			}
			for _, link := range links {
				addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
				if err != nil {
					glog.Error(err)
					continue
				}
				for _, addr := range addrs {
					if addr.IP.String() == ip.String() {
						nodeIP = addr.IPNet.String()
						return
					}
				}
			}
			glog.Fatalf("couldn't found any device which carries IP %s", nodeIP)
		}
		// or a list of device names
		devices := strings.Split(nodeIP, ",")
		if len(devices) == 0 {
			glog.Fatalf("invalid iface configuration %s", nodeIP)
		}
		for _, dev := range devices {
			nic, err := netlink.LinkByName(dev)
			if err != nil {
				glog.Error(err)
				continue
			}
			addrs, err := netlink.AddrList(nic, netlink.FAMILY_V4)
			if err != nil {
				glog.Error(err)
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
