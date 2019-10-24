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
	result, err := ExecCNIWithResult(cniName, netConfStdin, invokeArgs)
	Expect(err).NotTo(HaveOccurred())
	data, err := json.Marshal(result)
	Expect(err).NotTo(HaveOccurred())
	glog.V(4).Infof("result: %s", string(data))
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
