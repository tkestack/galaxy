package types

import (
	"net"

	"github.com/vishvananda/netlink"
)

type Route struct {
	Dst   net.IPNet
	GW    net.IP
	Scope netlink.Scope
}

// AddRoute adds a route to a device.
func AddRoute(ipn *net.IPNet, gw net.IP, dev netlink.Link, scope netlink.Scope) error {
	return netlink.RouteAdd(&netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     scope,
		Dst:       ipn,
		Gw:        gw,
	})
}
