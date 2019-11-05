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
package helper

import (
	"encoding/json"
	"net"
	"path"

	"github.com/containernetworking/cni/pkg/invoke"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/cniutil"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/utils/ips"
)

// CmdAdd calls cni binary to do cmdAdd for container
func CmdAdd(containerId, ifaceCidr, argsStr, cniName, expectResult string, netConfStdin []byte) string {
	nsPath, err := NewNetNS(containerId)
	Expect(err).NotTo(HaveOccurred())
	if ifaceCidr != "" {
		err = SetupDummyDev("dummy0", ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
	}
	invokeArgs := &invoke.Args{
		Command:     "ADD",
		ContainerID: containerId,
		NetNS:       path.Join(NetNS_PATH, containerId),
	}
	if argsStr != "" {
		invokeArgs.PluginArgsStr = cniutil.BuildCNIArgs(map[string]string{constant.IPInfosKey: argsStr})
	}
	// execute cmd add
	result, err := ExecCNIWithResult(cniName, netConfStdin, invokeArgs)
	Expect(err).NotTo(HaveOccurred())
	data, err := json.Marshal(result)
	Expect(err).NotTo(HaveOccurred())
	glog.V(4).Infof("result: %s", string(data))
	// check result is expected
	Expect(string(data)).Should(Equal(expectResult), "result: %s", string(data))
	return nsPath
}

// CmdDel calls cni binary to do cmdDel for container
func CmdDel(containerId, cniName string, netConfStdin []byte) {
	err := ExecCNI(cniName, netConfStdin, &invoke.Args{
		Command:     "DEL",
		ContainerID: containerId,
		NetNS:       path.Join(NetNS_PATH, containerId),
	})
	Expect(err).Should(BeNil(), "%v", err)
}

// CheckContainerTopology checks network topology is expected in container
func CheckContainerTopology(nsPath, containerCidr, gwIP string) {
	containerIPNet, err := ips.ParseCIDR(containerCidr)
	Expect(err).NotTo(HaveOccurred())
	err = (&NetworkTopology{
		Netns: nsPath,
		LeaveDevices: []*LinkDevice{
			NewLinkDevice(containerIPNet, "eth0", "veth"),
		},
		Routes: []Route{{Route: netlink.Route{Gw: net.ParseIP(gwIP)}, LinkName: "eth0"}},
	}).Verify()
	Expect(err).Should(BeNil(), "%v", err)
}
