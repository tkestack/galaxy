package network

import (
	"fmt"
	"io/ioutil"
	"net"
)

func DisableIPv6(ifName string) error {
	path := fmt.Sprintf("/proc/sys/net/ipv6/conf/%s/disable_ipv6", ifName)
	return ioutil.WriteFile(path, []byte{'1', '\n'}, 0644)
}

func ParseCIDR(cidr string) (*net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ipNet.IP = ip
	return ipNet, nil
}
