package flags

import (
	"testing"

	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/network"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/netns"
)

func TestNodeIP(t *testing.T) {
	flagIface = &string{"lo"}
	netns.NsInvoke(func() {
		lo, err := netlink.LinkByName("lo")
		if err != nil {
			t.Fatal(err)
		}
		if err := netlink.LinkSetUp(lo); err != nil {
			t.Fatal(err)
		}
		ipNet, err := network.ParseCIDR("10.0.0.1/24")
		if err != nil {
			t.Fatal(err)
		}
		netlink.AddrAdd(lo, &netlink.Addr{IPNet: ipNet})
		ipNet, err = network.ParseCIDR("127.0.0.1/32")
		if err != nil {
			t.Fatal(err)
		}
		netlink.AddrAdd(lo, &netlink.Addr{IPNet: ipNet})
		if "10.0.0.1/24" != GetNodeIP() {
			t.Fatal(GetNodeIP())
		}
	})
}
