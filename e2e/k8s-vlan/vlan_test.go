package k8s_vlan

import (
	"encoding/json"
	"net"
	"path"

	"git.code.oa.com/tkestack/galaxy/e2e/helper"
	"git.code.oa.com/tkestack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/tkestack/galaxy/pkg/utils"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/ips"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
)

var _ = Describe("galaxy-k8s-vlan vlan test", func() {
	cni := "galaxy-k8s-vlan"
	ifaceCidr := "192.168.0.66/26"
	containerCidr := "192.168.0.68/26"
	containerId := helper.NewContainerId()
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
		nsPath, err := helper.NewNetNS(containerId)
		Expect(err).NotTo(HaveOccurred())
		err = helper.SetupDummyDev("dummy0", ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		argsStr, err := helper.IPInfo(containerCidr, 2)
		Expect(err).NotTo(HaveOccurred())
		result, err := helper.ExecCNIWithResult(cni, netConf, &invoke.Args{
			Command:       "ADD",
			ContainerID:   containerId,
			NetNS:         path.Join(helper.NetNS_PATH, containerId),
			PluginArgsStr: cniutil.BuildCNIArgs(map[string]string{constant.IPInfosKey: argsStr}),
		})
		Expect(err).NotTo(HaveOccurred())
		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("result: %s", string(data))
		Expect(string(data)).Should(Equal(`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`), "result: %s", string(data))
		//_, err = helper.Ping("192.168.0.68")
		//Expect(err).To(HaveOccurred()) // vlan is not reachable on host

		// check host iface topology, route, neigh, ip address is expected
		cidrIPNet, err := ips.ParseCIDR(ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth").SetMaster(
					helper.NewLinkDevice(nil, "br2", "bridge"),
				),
				helper.NewLinkDevice(nil, "dummy0.2", "vlan").SetMaster(
					helper.NewLinkDevice(nil, "br2", "bridge"),
				).SetParent(
					helper.NewLinkDevice(cidrIPNet, "dummy0", "dummy"),
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

		// test DEL command
		err = helper.ExecCNI(cni, netConf, &invoke.Args{
			Command:     "DEL",
			ContainerID: containerId,
			NetNS:       path.Join(helper.NetNS_PATH, containerId),
		})
		Expect(err).Should(BeNil(), "%v", err)
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
		nsPath, err := helper.NewNetNS(containerId)
		Expect(err).NotTo(HaveOccurred())
		err = helper.SetupDummyDev("dummy0", ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		argsStr, err := helper.IPInfos(containerCidr, 2, secondCidr, 3)
		Expect(err).NotTo(HaveOccurred())
		result, err := helper.ExecCNIWithResult(cni, netConf, &invoke.Args{
			Command:       "ADD",
			ContainerID:   containerId,
			NetNS:         path.Join(helper.NetNS_PATH, containerId),
			PluginArgsStr: cniutil.BuildCNIArgs(map[string]string{constant.IPInfosKey: argsStr}),
		})
		Expect(err).NotTo(HaveOccurred())
		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("result: %s", string(data))
		Expect(string(data)).Should(Equal(`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"10.0.0.0/8"},{"dst":"172.16.0.0/12"},{"dst":"192.168.0.0/16"}]},"dns":{}}`), "result: %s", string(data))
		// check host iface topology, route, neigh, ip address is expected
		cidrIPNet, err := ips.ParseCIDR(ifaceCidr)
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth").SetMaster(
					helper.NewLinkDevice(nil, "br2", "bridge"),
				),
				helper.NewLinkDevice(nil, "dummy0.2", "vlan").SetMaster(
					helper.NewLinkDevice(nil, "br2", "bridge"),
				).SetParent(
					helper.NewLinkDevice(cidrIPNet, "dummy0", "dummy"),
				),

				helper.NewLinkDevice(nil, utils.HostVethName(containerId, "-2"), "veth").SetMaster(
					helper.NewLinkDevice(nil, "br3", "bridge"),
				),
				helper.NewLinkDevice(nil, "dummy0.3", "vlan").SetMaster(
					helper.NewLinkDevice(nil, "br3", "bridge"),
				).SetParent(
					helper.NewLinkDevice(cidrIPNet, "dummy0", "dummy"),
				),
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
