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
package main

import (
	"flag"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/network/vlan"
	"tkestack.io/galaxy/pkg/utils"
)

var (
	flagDevice  = flag.String("device", "", "The device which has the ip address, eg. eth1 or eth1.12 (A vlan device)")
	flagNetns   = flag.String("netns", "", "The netns path for the container")
	flagIP      = flag.String("ip", "", "The ip in cidr format for the container")
	flagVlan    = flag.Uint("vlan", 0, "The vlan id of the ip")
	flagGateway = flag.String("gateway", "", "The gateway for the ip")
)

/*
 ./setupvlan -logtostderr -device bond1
 ip netns add ctn2; ./setupvlan -logtostderr -device bond1 -netns=/var/run/netns/ctn2 -ip=10.2.1.111/24 -gateway=10.2.1.1
*/
func main() {
	flag.Parse()
	d := &vlan.VlanDriver{}
	if *flagDevice == "" {
		glog.Fatal("device unset")
	}
	d.NetConf = &vlan.NetConf{Device: *flagDevice}
	if err := d.Init(); err != nil {
		glog.Fatalf("Error setting up bridge %v", err)
	}
	glog.Infof("setuped bridge docker")
	if *flagNetns == "" {
		return
	}
	ip, ipNet, err := net.ParseCIDR(*flagIP)
	if err != nil {
		glog.Fatalf("invalid cidr %s", *flagIP)
	}
	ipNet.IP = ip
	gateway := net.ParseIP(*flagGateway)
	if gateway == nil {
		glog.Fatalf("invalid gateway %s", *flagGateway)
	}
	if *flagVlan != 0 {
		bridgeName, err := d.CreateBridgeAndVlanDevice(uint16(*flagVlan))
		if err != nil {
			glog.Fatalf("Error creating vlan device %v", err)
		}
		if err := utils.VethConnectsHostWithContainer(&t020.Result{
			IP4: &t020.IPConfig{
				IP:      *ipNet,
				Gateway: gateway,
				Routes: []types.Route{{
					Dst: net.IPNet{
						IP:   net.IPv4(0, 0, 0, 0),
						Mask: net.IPv4Mask(0, 0, 0, 0),
					},
				}},
			},
		}, &skel.CmdArgs{Netns: *flagNetns, IfName: "eth0"}, bridgeName, ""); err != nil {
			glog.Fatalf("Error creating veth %v", err)
		}
	}

	glog.Infof("privisioned container %s", *flagNetns)
}
