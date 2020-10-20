/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package nets

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

const IPRangeSeparator string = "~"

// IPNet add marshal and unmarshal func for net.IPNet
type IPNet net.IPNet

func (ipNet *IPNet) MarshalJSON() ([]byte, error) {
	return json.Marshal(ipNet.ToIPNet().String())
}

func (ipNet *IPNet) UnmarshalJSON(data []byte) (err error) {
	if len(data) < 3 {
		return fmt.Errorf("invalid data %s", string(data))
	}
	ip, netIPNet, err := net.ParseCIDR(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	//netIPNet is masked, we should preserve lower bytes
	netIPNet.IP = ip
	*ipNet = IPNet(*netIPNet)
	return
}

func (ipNet *IPNet) ToIPNet() *net.IPNet {
	i := net.IPNet(*ipNet)
	return &i
}

func (ipNet *IPNet) String() string {
	return ipNet.ToIPNet().String()
}

func bytesEqual(x, y []byte) bool {
	if len(x) != len(y) {
		return false
	}
	for i, b := range x {
		if y[i] != b {
			return false
		}
	}
	return true
}

//FIXME add test
func (ipNet *IPNet) Equal(netIPNet *net.IPNet) bool {
	var b1, b2 int
	if ipNet == nil {
		b1 = 1
	}
	if netIPNet == nil {
		b2 = 1
	}
	if (b1 ^ b2) == 1 {
		return false
	}
	origin := ipNet.ToIPNet()
	if !origin.IP.Equal(netIPNet.IP) {
		return false
	}
	if !bytesEqual(origin.Mask, netIPNet.Mask) {
		return false
	}
	return true
}

func NetsIPNet(ipNet *net.IPNet) *IPNet {
	i := IPNet(*ipNet)
	return &i
}

// IPRange represents a continuous ips, first and last included
type IPRange struct {
	First, Last net.IP
}

func (ipr IPRange) Size() uint32 {
	if len(ipr.First) == 0 || len(ipr.Last) == 0 {
		return 0
	}
	return IPToInt(ipr.Last) - IPToInt(ipr.First) + 1
}

func (ipr IPRange) Contains(ip net.IP) bool {
	ipInt := IPToInt(ip)
	return ipInt >= IPToInt(ipr.First) && ipInt <= IPToInt(ipr.Last)
}

func (ipr IPRange) String() string {
	if ipr.First.Equal(ipr.Last) {
		return ipr.First.String()
	}
	return fmt.Sprintf("%s%s%s", ipr.First.String(), IPRangeSeparator, ipr.Last.String())
}

func IPtoIPRange(ip net.IP) IPRange {
	return IPRange{ip, ip}
}

// ParseIPRange parses ip range from the format 192.168.0.1~192.168.2.3
// returns nil if it's an invalid format
func ParseIPRange(ipr string) *IPRange {
	if strings.Contains(ipr, IPRangeSeparator) {
		strs := strings.SplitN(ipr, IPRangeSeparator, 2)
		first := net.ParseIP(strs[0])
		if first == nil {
			return nil
		}
		last := net.ParseIP(strs[1])
		if last == nil {
			return nil
		}
		if IPToInt(first) > IPToInt(last) {
			return nil
		}
		return &IPRange{first, last}
	} else {
		ip := net.ParseIP(ipr)
		if len(ip) == 0 {
			return nil
		}
		return &IPRange{ip, ip}
	}
}

func (ipr IPRange) MarshalJSON() ([]byte, error) {
	return json.Marshal(ipr.String())
}

func (ipr *IPRange) UnmarshalJSON(data []byte) error {
	if len(data) < 3 {
		return fmt.Errorf("bad IPRange format %s", string(data))
	}
	r := ParseIPRange(string(data[1 : len(data)-1]))
	if r == nil {
		return fmt.Errorf("bad IPRange format %s", string(data))
	}
	*ipr = *r
	return nil
}

// SparseSubnet represents a sparse subnet
type SparseSubnet struct {
	IPRanges []IPRange  `json:"ranges"`
	Gateway  net.IP     `json:"gateway"`
	Mask     net.IPMask `json:"mask"`
	Vlan     uint16     `json:"vlan"`
}

func (subnet SparseSubnet) IPNet() *net.IPNet {
	return &net.IPNet{
		IP:   subnet.Gateway.Mask(subnet.Mask),
		Mask: subnet.Mask,
	}
}

func (subnet SparseSubnet) String() string {
	return fmt.Sprintf("{%s %d}", subnet.IPNet().String(), subnet.Vlan)
}

func (subnet SparseSubnet) Size() uint32 {
	var size uint32
	for _, ipr := range subnet.IPRanges {
		size += ipr.Size()
	}
	return size
}

// IPv4ToInt convert ip to uint32
// returns 0 if it's an invalid ip
func IPToInt(ip net.IP) uint32 {
	if len(ip) == net.IPv6len {
		return binary.BigEndian.Uint32(ip[12:16])
	} else if len(ip) == net.IPv4len {
		return binary.BigEndian.Uint32(ip)
	}
	return 0
}

// IntToIP convert uint32 to ip
func IntToIP(i uint32) net.IP {
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, i)
	return ip
}

func LastIPV4(ipNet *net.IPNet) net.IP {
	s := 0
	if len(ipNet.IP) == net.IPv6len {
		s = 12
	}
	p := make(net.IP, net.IPv4len)
	for i := range []byte(ipNet.IP[s:]) {
		p[i] = []byte(ipNet.IP)[i+s] | ([]byte(ipNet.Mask)[i] ^ 255)
	}
	return p
}

func FirstAndLastIP(ipNet *net.IPNet) (uint32, uint32) {
	return IPToInt(ipNet.IP.Mask(ipNet.Mask)), IPToInt(LastIPV4(ipNet))
}
