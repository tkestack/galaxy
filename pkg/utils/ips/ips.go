package ips

import "net"

func ParseIPv4Mask(mask string) net.IPMask {
	ip := net.ParseIP(mask)
	if ip == nil {
		return nil
	}
	return net.IPv4Mask(ip[12], ip[13], ip[14], ip[15])
}
