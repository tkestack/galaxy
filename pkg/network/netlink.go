package network

import "github.com/vishvananda/netlink"

func IsNeighResolving(state int) bool {
	return (state & (netlink.NUD_INCOMPLETE | netlink.NUD_STALE | netlink.NUD_DELAY | netlink.NUD_PROBE)) != 0
}

func FilterLoopbackAddr(addrs []netlink.Addr) []netlink.Addr {
	filteredAddr := []netlink.Addr{}
	for _, addr := range addrs {
		if addr.IPNet != nil && addr.IP != nil && !addr.IP.IsLoopback() {
			filteredAddr = append(filteredAddr, addr)
		}
	}
	return filteredAddr
}
