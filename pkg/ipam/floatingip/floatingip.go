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
package floatingip

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/utils/nets"
)

// FloatingIP defines a floating ip
type FloatingIP struct {
	Key       string
	IP        net.IP
	UpdatedAt time.Time
	Labels    map[string]string
	Policy    uint16
	NodeName  string
	PodUid    string
	pool      *FloatingIPPool
}

func (f FloatingIP) String() string {
	return fmt.Sprintf("FloatingIP{ip:%s key:%s policy:%d nodeName:%s podUid:%s}",
		f.IP.String(), f.Key, f.Policy, f.NodeName, f.PodUid)
}

// New creates a new FloatingIP
func New(pool *FloatingIPPool, ip net.IP, key string, attr *Attr, updateAt time.Time) *FloatingIP {
	fip := &FloatingIP{IP: ip, pool: pool}
	fip.Assign(key, attr, updateAt)
	return fip
}

// Assign updates key, attr, updatedAt of FloatingIP
func (f *FloatingIP) Assign(key string, attr *Attr, updateAt time.Time) *FloatingIP {
	f.Key = key
	f.Policy = uint16(attr.Policy)
	f.UpdatedAt = updateAt
	f.NodeName = attr.NodeName
	f.PodUid = attr.Uid
	return f
}

// CloneWith creates a new FloatingIP and updates key, attr, updatedAt
func (f *FloatingIP) CloneWith(key string, attr *Attr, updateAt time.Time) *FloatingIP {
	fip := &FloatingIP{
		IP:   f.IP,
		pool: f.pool,
	}
	return fip.Assign(key, attr, updateAt)
}

// FloatingIPPool is FloatingIPPool structure.
type FloatingIPPool struct {
	NodeSubnets []*net.IPNet // the node subnets
	nets.SparseSubnet
	sync.RWMutex
	nodeSubnets sets.String // the node subnets, string set format
	index       int         // the index of []FloatingIPPool
}

// FloatingIPPoolConf is FloatingIP config structure.
type FloatingIPPoolConf struct {
	NodeSubnets []*nets.IPNet `json:"nodeSubnets"` // the node subnets
	// Deprecated, use NodeSubnets instead
	RoutableSubnet *nets.IPNet `json:"routableSubnet,omitempty"` // the node subnet
	IPs            []string    `json:"ips"`
	Subnet         *nets.IPNet `json:"subnet"` // the vip subnet
	Gateway        net.IP      `json:"gateway"`
	Vlan           uint16      `json:"vlan,omitempty"`
}

// MarshalJSON can marshal FloatingIPPoolConf to byte slice.
func (fip *FloatingIPPool) MarshalJSON() ([]byte, error) {
	conf := FloatingIPPoolConf{}
	for i := range fip.NodeSubnets {
		conf.NodeSubnets = append(conf.NodeSubnets, nets.NetsIPNet(fip.NodeSubnets[i]))
	}
	conf.Subnet = nets.NetsIPNet(fip.IPNet())
	conf.Gateway = fip.Gateway
	conf.Vlan = fip.Vlan
	conf.IPs = make([]string, 0)
	for _, ipr := range fip.IPRanges {
		conf.IPs = append(conf.IPs, ipr.String())
	}
	return json.Marshal(conf)
}

// UnmarshalJSON can unmarshal byte slice to FloatingIPPoolConf
func (fip *FloatingIPPool) UnmarshalJSON(data []byte) error {
	var conf FloatingIPPoolConf
	if err := json.Unmarshal(data, &conf); err != nil {
		return err
	}
	if conf.RoutableSubnet == nil && len(conf.NodeSubnets) == 0 {
		return fmt.Errorf("node subnet is empty")
	}
	fip.NodeSubnets = []*net.IPNet{}
	if conf.RoutableSubnet != nil {
		ipNet := conf.RoutableSubnet.ToIPNet()
		fip.NodeSubnets = append(fip.NodeSubnets, &net.IPNet{IP: ipNet.IP.Mask(ipNet.Mask), Mask: ipNet.Mask})
	} else {
		m := map[string]string{}
		for i := range conf.NodeSubnets {
			ipNet := conf.NodeSubnets[i].ToIPNet()
			ipNet.IP = ipNet.IP.Mask(ipNet.Mask)
			if _, ok := m[ipNet.String()]; !ok {
				fip.NodeSubnets = append(fip.NodeSubnets, ipNet)
				m[ipNet.String()] = ""
			}
		}
	}
	if conf.Gateway != nil {
		fip.Gateway = conf.Gateway
	} else {
		return fmt.Errorf("gateway is empty")
	}
	if conf.Subnet != nil {
		fip.Mask = conf.Subnet.Mask
	} else {
		return fmt.Errorf("subnet is empty")
	}
	fip.Vlan = conf.Vlan
	fip.IPRanges = []nets.IPRange{}
	for _, str := range conf.IPs {
		ipr := nets.ParseIPRange(str)
		if ipr != nil {
			fip.IPRanges = append(fip.IPRanges, *ipr)
		} else {
			return fmt.Errorf("invalid ip range %s", str)
		}
	}
	return fipCheck(fip)
}

func fipCheck(fip *FloatingIPPool) error {
	net := net.IPNet{IP: fip.Gateway, Mask: fip.Mask}
	for i := range fip.IPRanges {
		if !net.Contains(fip.IPRanges[i].First) || !net.Contains(fip.IPRanges[i].Last) {
			return fmt.Errorf("ip range %s not in subnet %s", fip.IPRanges[i].String(), net.String())
		}
		if i != 0 {
			if nets.IPToInt(fip.IPRanges[i].First) <= nets.IPToInt(fip.IPRanges[i-1].Last)+1 {
				return fmt.Errorf("ip range %s and %s can be merge to one or has wrong order",
					fip.IPRanges[i-1].String(), fip.IPRanges[i].String())
			}
		}
	}
	return nil
}

// String can transform FloatingIP to string.
func (fip *FloatingIPPool) String() string {
	data, err := fip.MarshalJSON()
	if err != nil {
		return "<nil>"
	}
	return string(data)
}

// Contains judge whether FloatingIP struct contains a given ip.
func (fip *FloatingIPPool) Contains(ip net.IP) bool {
	for _, ipr := range fip.IPRanges {
		if ipr.Contains(ip) {
			return true
		}
	}
	return false
}

// #lizard forgives
// InsertIP can insert a given ip to FloatingIP struct.
func (fip *FloatingIPPool) InsertIP(ip net.IP) bool {
	if !fip.SparseSubnet.IPNet().Contains(ip) {
		return false
	}
	if len(fip.IPRanges) == 0 {
		fip.IPRanges = append(fip.IPRanges, nets.IPtoIPRange(ip))
		return true
	}
	for i := range fip.IPRanges {
		if fip.IPRanges[i].Contains(ip) {
			return false
		}
		ret := Minus(fip.IPRanges[i].First, ip)
		if ret > 1 {
			// ip first-last
			if i == 0 {
				fip.IPRanges = append([]nets.IPRange{nets.IPtoIPRange(ip)}, fip.IPRanges...)
			} else {
				fip.IPRanges = append(fip.IPRanges[:i], append([]nets.IPRange{nets.IPtoIPRange(ip)}, fip.IPRanges[i:]...)...)
			}
			return true
		} else if ret == 1 {
			// ip-last
			fip.IPRanges[i].First = ip
			fip.tryMerge(i - 1)
			return true
		}
		if Minus(fip.IPRanges[i].Last, ip) == -1 {
			// first-ip
			fip.IPRanges[i].Last = ip
			fip.tryMerge(i)
			return true
		}
	}
	//first-last first-last ... ip
	fip.IPRanges = append(fip.IPRanges, nets.IPtoIPRange(ip))
	return true
}

func (fip *FloatingIPPool) tryMerge(i int) {
	if i < 0 || i+1 == len(fip.IPRanges) {
		return
	}
	if Minus(fip.IPRanges[i+1].First, fip.IPRanges[i].Last) == 1 {
		fip.IPRanges[i].Last = fip.IPRanges[i+1].Last
		if i+2 < len(fip.IPRanges) {
			fip.IPRanges = append(fip.IPRanges[0:i+1], fip.IPRanges[i+2:]...)
		} else {
			fip.IPRanges = fip.IPRanges[0 : i+1]
		}
	}
}

// RemoveIP can remove a given ip from FloatingIP struct.
func (fip *FloatingIPPool) RemoveIP(ip net.IP) bool {
	if !fip.IPNet().Contains(ip) {
		return false
	}
	if len(fip.IPRanges) == 0 {
		return false
	}

	for i := range fip.IPRanges {
		ipRange := fip.IPRanges[i]
		if ipRange.Contains(ip) {
			ipn := nets.IPToInt(ip)
			switch {
			case ipRange.First.Equal(ipRange.Last):
				fip.IPRanges = append(fip.IPRanges[:i], fip.IPRanges[i+1:]...)
			case ipRange.First.Equal(ip):
				ipRange.First = nets.IntToIP(nets.IPToInt(ipRange.First) + 1)
				fip.IPRanges[i] = ipRange
			case ipRange.Last.Equal(ip):
				ipRange.Last = nets.IntToIP(nets.IPToInt(ipRange.Last) - 1)
				fip.IPRanges[i] = ipRange
			default:
				fip.IPRanges = append(fip.IPRanges[:i+1], append([]nets.IPRange{ipRange}, fip.IPRanges[i+1:]...)...)
				fip.IPRanges[i].Last = nets.IntToIP(ipn - 1)
				fip.IPRanges[i+1].First = nets.IntToIP(ipn + 1)
			}
			return true
		}
	}
	return false
}

// Minus compute how many ips between two given ip.
func Minus(a, b net.IP) int64 {
	return int64(nets.IPToInt(a)) - int64(nets.IPToInt(b))
}

type FloatingIPSlice []*FloatingIPPool

// Len returns number of FloatingIPSlice.
func (s FloatingIPSlice) Len() int {
	return len(s)
}

// Swap can swap two ip in FloatingIPSlice.
func (s FloatingIPSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less compares two given ip.
func (s FloatingIPSlice) Less(i, j int) bool {
	return nets.IPToInt(s[i].Gateway) < nets.IPToInt(s[j].Gateway)
}

// Attr stores attrs about this pod
type Attr struct {
	// NodeName is needed to send unassign request to cloud provider on resync
	NodeName string
	// uid is used to differentiate a deleting pod and a newly created pod with the same name such as statefulsets
	// or tapp pod
	Uid string
	// Release policy
	Policy constant.ReleasePolicy `json:"-"`
}

func (a Attr) String() string {
	return fmt.Sprintf("Attr{policy:%d nodeName:%s uid:%s}", a.Policy, a.NodeName, a.Uid)
}

// unmarshalAttr unmarshal attributes and assign PodUid and NodeName
// Make sure invoke this func in a copied FloatingIP
func (f *FloatingIP) unmarshalAttr(attrStr string) error {
	if attrStr == "" {
		return nil
	}
	var attr Attr
	if err := json.Unmarshal([]byte(attrStr), &attr); err != nil {
		return fmt.Errorf("unmarshal attr %s for %s %s: %v", attrStr, f.Key, f.IP.String(), err)
	} else {
		f.NodeName = attr.NodeName
		f.PodUid = attr.Uid
	}
	return nil
}
