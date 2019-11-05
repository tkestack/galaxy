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
package k8s_vlan

import (
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/e2e/helper"
	"tkestack.io/galaxy/pkg/utils"
	"tkestack.io/galaxy/pkg/utils/ips"
)

var _ = Describe("galaxy-k8s-vlan vlan test", func() {
	cni := "galaxy-k8s-vlan"
	ifaceCidr := "192.168.0.66/26"
	containerCidr := "192.168.0.68/26"
	containerId := helper.NewContainerId()
	cidrIPNet, _ := ips.ParseCIDR(ifaceCidr)
	hostVeth1 := helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth").SetMaster(
		helper.NewLinkDevice(nil, "br2", "bridge"),
	)
	dummyVlan2 := helper.NewDummyVlan(cidrIPNet, 2)
	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupDummy()
		helper.CleanupIFace("br2")
		helper.CleanupIFace("br3")
		helper.CleanupIFace("dummy0.2")
		helper.CleanupIFace("dummy0.3")
	})
	It("vlan", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "vlan_name_prefix": "dummy0.",
    "bridge_name_prefix": "br"
}`)
		argsStr, err := helper.IPInfo(containerCidr, 2)
		Expect(err).NotTo(HaveOccurred())
		nsPath := helper.CmdAdd(containerId, ifaceCidr, argsStr, cni,
			`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`, netConf)
		//_, err = helper.Ping("192.168.0.68")
		//Expect(err).To(HaveOccurred()) // vlan is not reachable on host

		// check host iface topology, route, neigh, ip address is expected
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{hostVeth1, dummyVlan2},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		helper.CheckContainerTopology(nsPath, containerCidr, "192.168.0.65")

		// test DEL command
		helper.CmdDel(containerId, cni, netConf)
	})

	It("vlan second ips", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "vlan_name_prefix": "dummy0.",
    "bridge_name_prefix": "br"
}`)
		secondCidr := "192.168.1.3/24"
		argsStr, err := helper.IPInfos(containerCidr, 2, secondCidr, 3)
		Expect(err).NotTo(HaveOccurred())
		nsPath := helper.CmdAdd(containerId, ifaceCidr, argsStr, cni,
			`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"10.0.0.0/8"},{"dst":"172.16.0.0/12"},{"dst":"192.168.0.0/16"}]},"dns":{}}`, netConf)

		// check host iface topology, route, neigh, ip address is expected
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				hostVeth1,
				dummyVlan2,
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, "-2"), "veth").SetMaster(
					helper.NewLinkDevice(nil, "br3", "bridge"),
				),
				helper.NewDummyVlan(cidrIPNet, 3),
			},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		containerIPNet, err := ips.ParseCIDR(containerCidr)
		Expect(err).NotTo(HaveOccurred())
		secondIPNet, err := ips.ParseCIDR(secondCidr)
		Expect(err).NotTo(HaveOccurred())
		_, pANet, _ := net.ParseCIDR("10.0.0.0/8")
		_, pBNet, _ := net.ParseCIDR("172.16.0.0/12")
		_, pCNet, _ := net.ParseCIDR("192.168.0.0/16")
		err = (&helper.NetworkTopology{
			Netns: nsPath,
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(containerIPNet, "eth0", "veth"),
				helper.NewLinkDevice(secondIPNet, "eth1", "veth"),
			},
			Routes: []helper.Route{
				{Route: netlink.Route{Gw: net.ParseIP("192.168.1.1")}, LinkName: "eth1"},
				{Route: netlink.Route{Gw: net.ParseIP("192.168.0.65"), Dst: pANet}, LinkName: "eth0"},
				{Route: netlink.Route{Gw: net.ParseIP("192.168.0.65"), Dst: pBNet}, LinkName: "eth0"},
				{Route: netlink.Route{Gw: net.ParseIP("192.168.0.65"), Dst: pCNet}, LinkName: "eth0"}},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)
	})
})
