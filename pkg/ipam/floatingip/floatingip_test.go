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
	"testing"

	"tkestack.io/galaxy/pkg/utils/nets"
)

var (
	ipNet = &net.IPNet{IP: net.IPv4(10, 173, 14, 0), Mask: net.IPv4Mask(255, 255, 255, 0)}
	ip    = net.IPv4(10, 173, 14, 1)
)

// TestMarshalFloatingIPPoolConf test FloatingIPPoolConf Marshal function.
func TestMarshalFloatingIPPoolConf(t *testing.T) {
	subnet := nets.NetsIPNet(ipNet)
	conf := FloatingIPPoolConf{
		RoutableSubnet: subnet,
		IPs: []string{
			"10.173.14.205",
			"10.173.14.206~10.173.14.208",
		},
		Subnet:  subnet,
		Gateway: ip,
		Vlan:    2,
	}
	if _, err := json.Marshal(conf); err != nil {
		t.Fatal(err)
	}
}

// TestMarshalFloatingIPPool test FloatingIPPool Marshal function.
func TestMarshalFloatingIPPool(t *testing.T) {
	ipr := nets.ParseIPRange("10.173.14.206~10.173.14.208")
	fip := FloatingIPPool{
		RoutableSubnet: ipNet,
		SparseSubnet: nets.SparseSubnet{
			IPRanges: []nets.IPRange{nets.IPtoIPRange(net.ParseIP("10.173.14.205")), *ipr},
			Gateway:  net.ParseIP("10.173.14.1"),
			Mask:     net.CIDRMask(24, 8*net.IPv4len),
			Vlan:     2,
		},
	}
	if _, err := json.Marshal(&fip); err != nil {
		t.Fatal(err)
	}
}

// #lizard forgives
// TestUnmarshalFloatingIPPool test FloatingIPPool unmarshal function.
func TestUnmarshalFloatingIPPool(t *testing.T) {
	var (
		confStr  = `{"routableSubnet":"10.173.14.1/24","ips":["10.173.14.203","10.173.14.206~10.173.14.208"],"subnet":"10.173.14.0/24","gateway":"10.173.14.1","vlan":2}`
		wrongStr = `{"routableSubnet":"10.173.14.0/24","ips":["10.173.14.205","10.173.14.206~10.173.14.208"],"subnet":"10.173.14.0/24","gateway":"10.173.14.1","vlan":2}`
		fip      FloatingIPPool
	)
	if err := json.Unmarshal([]byte(confStr), &fip); err != nil {
		t.Fatal(err)
	}
	if fip.RoutableSubnet.String() != ipNet.String() {
		t.Fatal()
	}
	if fip.IPNet().String() != ipNet.String() {
		t.Fatal()
	}
	if !fip.Gateway.Equal(ip) {
		t.Fatal()
	}
	if fip.Vlan != 2 {
		t.Fatal()
	}
	if len(fip.IPRanges) != 2 {
		t.Fatal()
	}
	if fip.IPRanges[0].First.String() != "10.173.14.203" {
		t.Fatal()
	}
	if fip.IPRanges[0].Last.String() != "10.173.14.203" {
		t.Fatal()
	}
	if fip.IPRanges[1].First.String() != "10.173.14.206" {
		t.Fatal()
	}
	if fip.IPRanges[1].Last.String() != "10.173.14.208" {
		t.Fatal()
	}
	if err := json.Unmarshal([]byte(confStr), &fip); err == nil {
		t.Fatal(wrongStr)
	}
}

// #lizard forgives
// TestInsertRemoveIP test FloatingIPPool's InsertIP and RemoveIP functions.
func TestInsertRemoveIP(t *testing.T) {
	fip := &FloatingIPPool{
		SparseSubnet: nets.SparseSubnet{
			Gateway: net.ParseIP("10.166.141.65"),
			Mask:    net.CIDRMask(26, 32),
		},
	}
	fip.InsertIP(net.ParseIP("10.166.141.115"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.123"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.122"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.117"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.125"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123 10.166.141.125]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.116"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115~10.166.141.117 10.166.141.122~10.166.141.123 10.166.141.125]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.116"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123 10.166.141.125]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.125"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.117"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.122"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.123"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.115"))
	if len(fip.IPRanges) != 0 {
		t.Fatal(fip.IPRanges)
	}
}
