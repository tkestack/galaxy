package flags

import (
	"sync"
	"testing"

	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/network"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/netns"
)

func TestNodeIP(t *testing.T) {
	// test input value is a device
	*flagIface = "lo"
	once = sync.Once{}
	var nodeIP string
	f := func(ipNets []string) func() {
		return func() {
			lo, err := netlink.LinkByName("lo")
			if err != nil {
				t.Fatal(err)
			}
			if err := netlink.LinkSetUp(lo); err != nil {
				t.Fatal(err)
			}
			for _, ipNet := range ipNets {
				ipNet, err := network.ParseCIDR(ipNet)
				if err != nil {
					t.Fatal(err)
				}
				netlink.AddrAdd(lo, &netlink.Addr{IPNet: ipNet})
			}
			nodeIP = GetNodeIP()
		}
	}
	netns.NsInvoke(f([]string{"10.0.0.1/24", "127.0.0.1/32"}))
	if "10.0.0.1/24" != nodeIP {
		t.Fatal(nodeIP)
	}
	// test input value is an IP
	*flagIface = "172.16.0.1"
	once = sync.Once{}
	netns.NsInvoke(f([]string{"172.16.0.1/24"}))
	if "172.16.0.1/24" != nodeIP {
		t.Fatal(nodeIP)
	}
	// test input value is a cidr
	*flagIface = "10.16.0.3/23"
	once = sync.Once{}
	netns.NsInvoke(f([]string{"10.16.0.3/23"}))
	if "10.16.0.3/23" != nodeIP {
		t.Fatal(nodeIP)
	}
}
