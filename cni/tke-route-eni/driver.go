// +build linux

package main

import (
	"fmt"
	"net"
	"syscall"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

const (
	// ip rules priority and leave 512 gap for future
	toPodRulePriority = 512
	// 1024 is reserved for (ip rule not to <vpc's subnet> table main)
	fromPodRulePriority = 1536

	mainRouteTable = 254

	ethernetMTU = 1500
)

type NetworkAPIs interface {
	SetupNS(hostVethName string, podVethName string, netns string, addr *net.IPNet, routeTable int) ([]*current.Interface, error)
	TeardownNS(podVethName string, netns string, routeTable int) error
}

type linuxNetwork struct {
}

func NewDriver() NetworkAPIs {
	return &linuxNetwork{}
}

func (network *linuxNetwork) SetupNS(
	hostVethName string, podVethName string, netns string,
	addr *net.IPNet, routeTable int) ([]*current.Interface, error) {

	if oldHostVeth, err := netlink.LinkByName(hostVethName); err == nil {
		if err = netlink.LinkDel(oldHostVeth); err != nil {
			return nil, fmt.Errorf("failed to delete old hostVeth %s: %v", hostVethName, err)
		}
	}

	contInf := &current.Interface{}
	hostInf := &current.Interface{}

	vethCtx := newVethPairCreateContext(hostVethName, podVethName, addr)
	if err := ns.WithNetNSPath(netns, func(hostNs ns.NetNS) error {
		contVeth, err := vethCtx.setupContainerNetwork(hostNs)
		if err != nil {
			return err
		}
		contInf.Name = vethCtx.podVethName
		contInf.Mac = contVeth.Attrs().HardwareAddr.String()
		contInf.Sandbox = netns
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to setup container network: %v", err)
	}

	hostVeth, err := netlink.LinkByName(hostVethName)
	if err != nil {
		return nil, fmt.Errorf("failed to find link %q: %v", hostVethName, err)
	}
	hostInf.Name = vethCtx.hostVethName
	hostInf.Mac = hostVeth.Attrs().HardwareAddr.String()

	// Explicitly set the veth to UP state, because netlink doesn't always do that on all the platforms with net.FlagUp.
	// veth won't get a link local address unless it's set to UP state.
	if err = netlink.LinkSetUp(hostVeth); err != nil {
		return nil, fmt.Errorf("failed to set link %q up: %v", hostVethName, err)
	}

	addrHostAddr := &net.IPNet{
		IP:   addr.IP,
		Mask: net.CIDRMask(32, 32)}

	// Add host route
	if err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: hostVeth.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       addrHostAddr}); err != nil {
		return nil, fmt.Errorf("failed to add host route: %v", err)
	}

	err = addToPodRule(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to add toPod rule: %v", err)
	}

	// add from-pod rule, only need it when it is not primary ENI
	if routeTable > 0 {
		err = addFromPodRule(addr, routeTable)

		if err != nil {
			return nil, fmt.Errorf("failed to add fromContainer rule: %v", err)
		}
	}
	return []*current.Interface{hostInf, contInf}, nil
}

func (network *linuxNetwork) TeardownNS(podVethName string, netns string, routeTable int) error {
	//	var addrList []netlink.Addr
	//	var hostVethIdx int
	//
	//	ns.WithNetNSPath(netns, func(netNS ns.NetNS) error {
	//		link, err := netlink.LinkByName(podVethName)
	//		if err != nil {
	//			return fmt.Errorf("failed to get ns %s link %s: %v", netns, podVethName, err)
	//		}
	//		veth, ok := link.(*netlink.Veth)
	//		if ok {
	//			hostVethIdx, err = netlink.VethPeerIndex(veth)
	//			if err != nil {
	//				return fmt.Errorf("failed to get ns %s link %s veth peer index: %v", netns, podVethName, err)
	//			}
	//		} else {
	//			return fmt.Errorf("failed to figure out interface: ns %s link %s not veth peer", netns, podVethName)
	//		}
	//
	//		addrList, err = netlink.AddrList(link, netlink.FAMILY_V4)
	//		if err != nil && err != syscall.ENOENT {
	//			return fmt.Errorf("failed to list ns %s link %s addr: %v", netns, podVethName, err)
	//		}
	//		if len(addrList) == 0 {
	//			return fmt.Errorf("failed to list ns %s link %s addr: %v", netns, podVethName, err)
	//		}
	//		return nil
	//	})
	//
	//	var errs []error
	//	for _, addr2 := range addrList {
	//		addr := addr2.IPNet
	//		// remove to-pod rule
	//		podRule := netlink.NewRule()
	//		podRule.Dst = addr
	//		podRule.Priority = toPodRulePriority
	//
	//		err := netlink.RuleDel(podRule)
	//		if err != nil && !containsNoSuchRule(err) {
	//			errs = append(errs, fmt.Errorf("failed to delete toPod rule %+v: %v", addr, err))
	//		}
	//
	//		// delete from-pod rule, only need it when it is not primary ENI
	//		if routeTable > 0 {
	//			podRule := netlink.NewRule()
	//			podRule.Src = addr
	//			podRule.Table = routeTable
	//			podRule.Priority = fromPodRulePriority
	//
	//			err = netlink.RuleDel(podRule)
	//			if err != nil && !containsNoSuchRule(err) {
	//				errs = append(errs, fmt.Errorf("failed to delete fromPod rule %+v: %v", addr, err))
	//			}
	//		}
	//
	//		// Del host route
	//		if err = netlink.RouteDel(&netlink.Route{
	//			LinkIndex: hostVethIdx,
	//			Scope:     netlink.SCOPE_LINK,
	//			Dst:       addr}); err != nil {
	//			errs = append(errs, fmt.Errorf("failed to delete host route %+v: %v", addr, err))
	//		}
	//	}
	//	if len(errs) != 0 {
	//		errStr := "Failed to teardown ns"
	//		for _, err := range errs {
	//			errStr = errStr + " : " + err.Error()
	//		}
	//		return fmt.Errorf(errStr)
	//	}
	return nil
}

type vethPairCreateContext struct {
	hostVethName string
	podVethName  string
	addr         *net.IPNet
}

func newVethPairCreateContext(hostVethName, podVethName string, addr *net.IPNet) *vethPairCreateContext {
	return &vethPairCreateContext{
		hostVethName: hostVethName,
		podVethName:  podVethName,
		addr:         addr,
	}
}

func (ctx *vethPairCreateContext) setupContainerNetwork(hostNs ns.NetNS) (netlink.Link, error) {
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:  ctx.podVethName,
			Flags: net.FlagUp,
			MTU:   ethernetMTU,
		},
		PeerName: ctx.hostVethName,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return nil, err
	}

	hostVeth, err := netlink.LinkByName(ctx.hostVethName)
	if err != nil {
		return nil, err
	}

	contVeth, err := netlink.LinkByName(ctx.podVethName)
	if err != nil {
		return nil, err
	}

	if err = netlink.LinkSetUp(hostVeth); err != nil {
		return nil, fmt.Errorf("failed to set link %q up: %v", ctx.hostVethName, err)
	}

	podVeth, err := netlink.LinkByName(ctx.podVethName)
	if err != nil {
		return nil, fmt.Errorf("failed to find link %q: %v", ctx.podVethName, err)
	}

	if err = netlink.LinkSetUp(podVeth); err != nil {
		return nil, fmt.Errorf("failed to seti link %q up: %v", ctx.podVethName, err)
	}

	if err = netlink.AddrAdd(podVeth, &netlink.Addr{IPNet: ctx.addr}); err != nil {
		return nil, fmt.Errorf("failed to add IP addr to %q: %v", ctx.podVethName, err)
	}

	// Add a connected route to a dummy next hop (169.254.1.1)
	// # ip route show
	// default via 169.254.1.1 dev eth0  src 10.0.32.140
	// 169.254.1.1 dev eth0  scope link
	gw := net.IPv4(169, 254, 1, 1)
	gwNet := &net.IPNet{IP: gw, Mask: net.CIDRMask(32, 32)}

	if err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: podVeth.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       gwNet,
	}); err != nil {
		return nil, fmt.Errorf("failed to add direct route: %v", err)
	}

	defaultRoute := netlink.Route{
		LinkIndex: podVeth.Attrs().Index,
		Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
		Scope:     netlink.SCOPE_UNIVERSE,
		Gw:        gw,
		Src:       ctx.addr.IP,
	}

	if err = netlink.RouteAdd(&defaultRoute); err != nil {
		return nil, fmt.Errorf("failed to add default gateway: %v", err)
	}

	// add static ARP entry for default gateway
	// we are using routed mode on the host and container need this static ARP entry to resolve its default gateway.
	neigh := &netlink.Neigh{
		LinkIndex:    podVeth.Attrs().Index,
		State:        netlink.NUD_PERMANENT,
		IP:           gwNet.IP,
		HardwareAddr: hostVeth.Attrs().HardwareAddr,
	}

	if err = netlink.NeighAdd(neigh); err != nil {
		return nil, fmt.Errorf("failed to add static ARP: %v", err)
	}

	// Now that the everything has been successfully set up in the container, move the "host" end of the
	// veth into the host namespace.
	if err = netlink.LinkSetNsFd(hostVeth, int(hostNs.Fd())); err != nil {
		return nil, fmt.Errorf("failed to move veth to host netns: %v", err)
	}

	return contVeth, nil
}

func addToPodRule(addr *net.IPNet) error {
	podRule := netlink.NewRule()
	podRule.Dst = addr
	podRule.Table = mainRouteTable
	podRule.Priority = toPodRulePriority

	err := netlink.RuleDel(podRule)
	if err != nil && !containsNoSuchRule(err) {
		return fmt.Errorf("failed to delete old pod rule %+v: %v", addr, err)
	}

	err = netlink.RuleAdd(podRule)
	if err != nil {
		return fmt.Errorf("failed to add pod rule %+v: %v", addr, err)
	}

	return nil
}

func addFromPodRule(addr *net.IPNet, table int) error {
	podRule := netlink.NewRule()
	podRule.Src = addr
	podRule.Table = table
	podRule.Priority = fromPodRulePriority

	err := netlink.RuleDel(podRule)
	if err != nil && !containsNoSuchRule(err) {
		return fmt.Errorf("failed to delete old pod rule %+v: %v", addr, err)
	}

	err = netlink.RuleAdd(podRule)
	if err != nil {
		return fmt.Errorf("failed to add pod rule %+v: %v", addr, err)
	}

	return nil
}

func containsNoSuchRule(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return errno == syscall.ENOENT
	}
	return false
}

func cleanHostRule(saddr string, routeTable int) error {
	ip := net.ParseIP(saddr)
	addr := &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}

	// remove to-pod rule
	podRule := netlink.NewRule()
	podRule.Dst = addr
	podRule.Priority = toPodRulePriority

	err := netlink.RuleDel(podRule)
	if err != nil && !containsNoSuchRule(err) {
		return fmt.Errorf("failed to delete to pod rule %+v: %v", addr, err)
	}

	// delete from-pod rule, only need it when it is not primary ENI
	if routeTable > 0 {
		podRule := netlink.NewRule()
		podRule.Src = addr
		podRule.Table = routeTable
		podRule.Priority = fromPodRulePriority

		err = netlink.RuleDel(podRule)
		if err != nil && !containsNoSuchRule(err) {
			return fmt.Errorf("failed to delete from pod rule %+v: %v", addr, err)
		}
	}

	// Del host route
	if err = netlink.RouteDel(&netlink.Route{
		Scope: netlink.SCOPE_LINK,
		Dst:   addr}); err != nil && !containerNoSuchRoute(err) {
		return fmt.Errorf("failed to delete host route %+v: %v", addr, err)
	}
	return nil
}

func containerNoSuchRoute(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return errno == syscall.ESRCH
	}
	return false
}
