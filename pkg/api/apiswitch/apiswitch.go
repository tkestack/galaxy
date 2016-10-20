package apiswitch

import (
	"net"

	"github.com/containernetworking/cni/pkg/types"
)

/*
	{"ip":"10.49.27.205/24","vlan":2,"gateway":"10.49.27.1"}
*/
type IPInfo struct {
	IP             types.IPNet `json:"ip"`
	Vlan           uint16      `json:"vlan"`
	Gateway        net.IP      `json:"gateway"`
	RoutableSubnet types.IPNet `json:"routable_subnet"`
}

type NetworkInfo map[string]map[string]string
