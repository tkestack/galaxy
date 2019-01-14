package k8s_vlan_test

import (
	"encoding/json"
	"net"
	"path"

	"git.code.oa.com/gaiastack/galaxy/e2e/helper"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ips"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("galaxy-k8s-vlan bridge and pure test", func() {
	cni := "galaxy-k8s-vlan"
	ifaceCidr := "192.168.0.66/26"
	containerCidr := "192.168.0.68/26"
	containerId := helper.NewContainerId()
	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupDummy()
		helper.CleanupIFace("brtest")
	})
	It("bridge", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "default_bridge_name": "brtest"
}`)
		nsPath, err := helper.NewNetNS(containerId)
		Expect(err).NotTo(HaveOccurred())
		err = helper.SetupDummyDev("dummy0", ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		argsStr, err := helper.IPInfo(containerCidr, 0)
		Expect(err).NotTo(HaveOccurred())
		result, err := helper.ExecCNIWithResult(cni, netConf, &invoke.Args{
			Command:       "ADD",
			ContainerID:   containerId,
			NetNS:         path.Join(helper.NetNS_PATH, containerId),
			PluginArgsStr: cniutil.BuildCNIArgs(map[string]string{cniutil.IPInfoInArgs: argsStr}),
		})
		Expect(err).NotTo(HaveOccurred())
		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("result: %s", string(data))
		Expect(string(data)).Should(Equal(`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`), "result: %s", string(data))
		_, err = helper.Ping("192.168.0.68")
		Expect(err).NotTo(HaveOccurred())

		// check host iface topology, route, neigh, ip address is expected
		cidrIPNet, err := ips.ParseCIDR(ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth").SetMaster(
					helper.NewLinkDevice(cidrIPNet, "brtest", "bridge"),
				),
				helper.NewLinkDevice(nil, "dummy0", "dummy").SetMaster(
					helper.NewLinkDevice(cidrIPNet, "brtest", "bridge"),
				),
			},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		containerIPNet, err := ips.ParseCIDR(containerCidr)
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			Netns: nsPath,
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(containerIPNet, "eth0", "veth"),
			},
			Routes: []helper.Route{{Route: netlink.Route{Gw: net.ParseIP("192.168.0.65")}, LinkName: "eth0"}},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)
	})

	It("pure switch", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "switch": "pure"
}`)
		nsPath, err := helper.NewNetNS(containerId)
		Expect(err).NotTo(HaveOccurred())
		err = helper.SetupDummyDev("dummy0", ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		argsStr, err := helper.IPInfo(containerCidr, 0)
		Expect(err).NotTo(HaveOccurred())
		result, err := helper.ExecCNIWithResult(cni, netConf, &invoke.Args{
			Command:       "ADD",
			ContainerID:   containerId,
			NetNS:         path.Join(helper.NetNS_PATH, containerId),
			PluginArgsStr: cniutil.BuildCNIArgs(map[string]string{cniutil.IPInfoInArgs: argsStr}),
		})
		Expect(err).NotTo(HaveOccurred())
		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("result: %s", string(data))
		Expect(string(data)).Should(Equal(`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`), "result: %s", string(data))
		_, err = helper.Ping("192.168.0.68")
		Expect(err).NotTo(HaveOccurred())

		// check host iface topology, route, neigh, ip address is expected
		cidrIPNet, err := ips.ParseCIDR(ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth"),
				helper.NewLinkDevice(cidrIPNet, "dummy0", "dummy"),
			},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		containerIPNet, err := ips.ParseCIDR(containerCidr)
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			Netns: nsPath,
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(containerIPNet, "eth0", "veth"),
			},
			Routes: []helper.Route{{Route: netlink.Route{Gw: net.ParseIP("192.168.0.65")}, LinkName: "eth0"}},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)
	})
})
