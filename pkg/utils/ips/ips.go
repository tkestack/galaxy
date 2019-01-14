package ips

import "net"

func ParseIPv4Mask(mask string) net.IPMask {
	ip := net.ParseIP(mask)
	if ip == nil {
		return nil
	}
	return net.IPv4Mask(ip[12], ip[13], ip[14], ip[15])
}

// ParseCIDR returns cidr notation IP address
// This func differs with net.ParseCIDR which returns the masked cidr
func ParseCIDR(cidr string) (*net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ipNet.IP = ip
	return ipNet, nil
}
