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
package vlan

import (
	"encoding/json"
	"net"
	"os/exec"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/pkg/network/netns"
	"tkestack.io/galaxy/pkg/utils/ips"
)

func TestUnmarshalVlanNetConf(t *testing.T) {
	var nc NetConf
	if err := json.Unmarshal([]byte("{}"), &nc); err != nil {
		t.Fatal(err)
	}
	if nc.DisableDefaultBridge != nil {
		t.Fatal(*nc.DisableDefaultBridge)
	}
	if err := json.Unmarshal([]byte(`{"disable_default_bridge": true}`), &nc); err != nil {
		t.Fatal(err)
	}
	if nc.DisableDefaultBridge == nil || *nc.DisableDefaultBridge != true {
		t.Fatalf("%v", nc.DisableDefaultBridge)
	}
}

// #lizard forgives
func TestInit(t *testing.T) {
	vlanDriver := &VlanDriver{
		NetConf: &NetConf{
			Device:            "du0",
			DefaultBridgeName: "docker",
		},
	}
	ipNet, _ := ips.ParseCIDR("192.168.0.2/24")
	ipNet10, _ := ips.ParseCIDR("10.0.0.0/24")
	netns.NsInvoke(func() {
		dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "du0"}}
		if err := netlink.LinkAdd(dummy); err != nil {
			t.Fatal(err)
		}
		if err := netlink.LinkSetUp(dummy); err != nil {
			t.Fatal(err)
		}
		if err := netlink.AddrAdd(dummy, &netlink.Addr{IPNet: ipNet}); err != nil {
			t.Fatal(err)
		}
		if err := netlink.RouteAdd(&netlink.Route{Dst: ipNet10, LinkIndex: dummy.Attrs().Index}); err != nil {
			t.Fatal(err)
		}
		if err := netlink.RouteAdd(&netlink.Route{Gw: net.ParseIP("192.168.0.1"), LinkIndex: dummy.Attrs().Index}); err != nil {
			t.Fatal(err)
		}
		routeStr, err := iproute()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range []string{
			"default via 192.168.0.1 dev du0",
			"10.0.0.0/24 dev du0",
			"192.168.0.0/24 dev du0 proto kernel scope link src 192.168.0.2",
		} {
			if !strings.Contains(routeStr, r) {
				t.Fatal(routeStr)
			}
		}
		if err := vlanDriver.Init(); err != nil {
			t.Fatal(err)
		}
		routeStr, err = iproute()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range []string{
			"default via 192.168.0.1 dev docker",
			"10.0.0.0/24 dev docker",
			"192.168.0.0/24 dev docker proto kernel scope link src 192.168.0.2",
		} {
			if !strings.Contains(routeStr, r) {
				t.Fatal(routeStr)
			}
		}
	})
}

func iproute() (string, error) {
	data, err := exec.Command("ip", "route").CombinedOutput()
	if err != nil {
		return "", err
	}
	routeStr := string(data)
	// old kernel outputs:  `192.168.0.0/24 dev du0  proto kernel  scope link  src 192.168.0.2`
	// while newest outputs:`192.168.0.0/24 dev du0 proto kernel scope link src 192.168.0.2`
	// replace two space with a single one
	routeStr = strings.Replace(routeStr, "  ", " ", -1)
	return routeStr, nil
}
