package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/pkg/utils"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

type VethConf struct {
	types.NetConf
	RouteSrc string `json:"routeSrc"`
	Mtu      int    `json:"mtu"`
}

func loadConf(bytes []byte) (*VethConf, error) {
	n := &VethConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load vethconf: %v", err)
	}
	return n, nil
}

func addHostRoute(containerIP *net.IPNet, vethHostName string, src string) error {
	vethHost, err := netlink.LinkByName(vethHostName)
	if err != nil {
		return err
	}
	s := net.ParseIP(src)
	if err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: vethHost.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       containerIP,
		Gw:        nil,
		Src:       s,
	}); err != nil {
		if s != nil {
			// compatible change for old kernel which does not support src option
			if err1 := netlink.RouteAdd(&netlink.Route{
				LinkIndex: vethHost.Attrs().Index,
				Scope:     netlink.SCOPE_LINK,
				Dst:       containerIP,
				Gw:        nil,
			}); err1 != nil {
				return fmt.Errorf("failed to add route '%v dev %v for old linux kernel': %v. With src option err: %v",
				containerIP, vethHostName, err1, err)
			} else {
				return nil
			}
		}
		return fmt.Errorf("failed to add route '%v dev %v src %v': %v", containerIP, vethHostName, s.String(), err)
	}
	return nil
}

// #lizard forgives
func connectsHostWithContainer(result *t020.Result, args *skel.CmdArgs, conf *VethConf) error {
	mask32 := net.IPv4Mask(255, 255, 255, 255)
	linkLocalAddress := net.IPv4(169, 254, 1, 1)
	defaultDst := net.IPNet{IP: net.IPv4(0, 0, 0, 0), Mask: net.IPv4Mask(0, 0, 0, 0)}
	//configure the following two routes
	//ip netns exec $ctn ip route add 169.254.1.1 dev veth_sbx scope link
	//ip netns exec $ctn ip route add default via 169.254.1.1 dev veth_sbx scope global

	//only for outprint of the plugin.
	result.IP4 = &t020.IPConfig{
		IP: net.IPNet{
			IP:   result.IP4.IP.IP,
			Mask: mask32,
		},
		Gateway: linkLocalAddress,
		Routes: []types.Route{
			{Dst: net.IPNet{IP: linkLocalAddress, Mask: mask32}},
			{Dst: defaultDst, GW: linkLocalAddress},
		},
	}
	host, sbox, err := utils.CreateVeth(args.ContainerID, conf.Mtu, "")
	if err != nil {
		return err
	}
	// nolint: errcheck
	defer func() {
		if err != nil {
			if host != nil {
				netlink.LinkDel(host)
			}
			if sbox != nil {
				netlink.LinkDel(sbox)
			}
		}
	}()

	// Up the host interface after finishing all netlink configuration
	if err = netlink.LinkSetUp(host); err != nil {
		return fmt.Errorf("could not set link up for host interface %q: %v", host.Attrs().Name, err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close() // nolint: errcheck
	// move sbox veth device to ns
	if err = netlink.LinkSetNsFd(sbox, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move sbox device %q to netns: %v", sbox.Attrs().Name, err)
	}
	return netns.Do(func(_ ns.NetNS) error {
		if err := netlink.LinkSetName(sbox, args.IfName); err != nil {
			return fmt.Errorf("failed to rename sbox device %q to %q: %v", sbox.Attrs().Name, args.IfName, err)
		}
		// Add IP and routes to sbox, including default route
		if err := configureIface(args.IfName, &result.IP4.IP, []netlink.Route{
			{Dst: &net.IPNet{IP: linkLocalAddress, Mask: mask32}, Scope: netlink.SCOPE_LINK},
			{Dst: &defaultDst, Gw: linkLocalAddress, Scope: netlink.SCOPE_UNIVERSE},
		}); err != nil {
			return err
		}

		if err := netlink.NeighAdd(&netlink.Neigh{
			IP:           linkLocalAddress,
			LinkIndex:    sbox.Attrs().Index,
			HardwareAddr: host.Attrs().HardwareAddr,
			State:        netlink.NUD_PERMANENT,
		}); err != nil {
			return fmt.Errorf("failed to add neigh entry ip %s, link index %d, mac %s: %v", linkLocalAddress.String(),
			sbox.Attrs().Index, host.Attrs().HardwareAddr.String(), err)
		}
		return nil
	})
}

func configureIface(ifName string, ip *net.IPNet, routes []netlink.Route) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup %q: %v", ifName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set %q UP: %v", ifName, err)
	}

	addr := &netlink.Addr{IPNet: ip, Label: ""}
	if err = netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add IP addr to %q: %v", ifName, err)
	}

	for _, r := range routes {
		r.LinkIndex = link.Attrs().Index
		if err = netlink.RouteAdd(&r); err != nil {
			// we skip over duplicate routes as we assume the first one wins
			if !os.IsExist(err) {
				return fmt.Errorf("failed to add route '%v via %v dev %v scope %v': %v", r.Dst, r.Gw, ifName,
				r.Scope, err)
			}
		}
	}

	return nil
}

// Usage with flannel plugin
// {"galaxy-flannel":{"delegate":{"type":"galaxy-veth"},"subnetFile":"/run/flannel/subnet.env"}}
func cmdAdd(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	// run the IPAM plugin and get back the config to apply
	generalResult, err := ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}
	result020, err := generalResult.GetAsVersion(t020.ImplementedSpecVersion)
	if err != nil {
		return err
	}
	result, ok := result020.(*t020.Result)
	if !ok {
		return fmt.Errorf("failed to convert result")
	}
	if result.IP4 == nil {
		return fmt.Errorf("IPAM plugin returned missing IPv4 config")
	}
	if err := connectsHostWithContainer(result, args, conf); err != nil {
		return err
	}
	if err := addHostRoute(&result.IP4.IP, utils.HostVethName(args.ContainerID, ""), conf.RouteSrc); err != nil {
		return err
	}
	result.DNS = conf.DNS
	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}

	if err := utils.DeleteVeth(args.Netns, args.IfName); err != nil {
		return err
	}

	if err := utils.DeleteHostVeth(args.ContainerID); err != nil {
		return err
	}

	if err := ipam.ExecDel(conf.IPAM.Type, args.StdinData); err != nil {
		return err
	}
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}
