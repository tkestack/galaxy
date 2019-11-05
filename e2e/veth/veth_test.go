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
package veth

import (
	"net"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/e2e/helper"
	"tkestack.io/galaxy/pkg/utils"
)

var _ = Describe("galaxy-veth veth test", func() {
	cni := "galaxy-veth"
	containerCidr := "172.16.92.2/32"
	containerIP := net.ParseIP("172.16.92.2")
	containerId := helper.NewContainerId()
	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupCNIData("myveth")
	})
	It("veth", func() {
		netConf := []byte(`{
    "name": "myveth",
    "type": "galaxy-veth",
    "ipam": {"routes":[{"dst":"172.16.0.0/13"}],"subnet":"172.16.92.0/24","type":"host-local"}
}`)
		nsPath := helper.CmdAdd(containerId, "", "", cni,
			`{"cniVersion":"0.2.0","ip4":{"ip":"172.16.92.2/32","gateway":"169.254.1.1","routes":[{"dst":"169.254.1.1/32"},{"dst":"0.0.0.0/0","gw":"169.254.1.1"}]},"dns":{}}`, netConf)
		_, err := helper.Ping(containerIP.String())
		Expect(err).NotTo(HaveOccurred())

		// check host iface topology, route, neigh, ip address is expected
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth"),
			},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		helper.CheckContainerTopology(nsPath, containerCidr, "169.254.1.1")

		// specify an empty netns path on deleting to check if it can delete host veth
		err = helper.ExecCNI(cni, netConf, &invoke.Args{
			Command:     "DEL",
			ContainerID: containerId,
			NetNS:       "",
		})
		Expect(err).Should(BeNil(), "%v", err)
		_, err = netlink.LinkByName(utils.HostVethName(containerId, ""))
		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "Link not found")).Should(BeTrue(), "%v", err)
	})
})
