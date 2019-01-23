package veth

import (
	"encoding/json"
	"net"
	"path"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/e2e/helper"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ips"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
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
		nsPath, err := helper.NewNetNS(containerId)
		Expect(err).NotTo(HaveOccurred())
		result, err := helper.ExecCNIWithResult(cni, netConf, &invoke.Args{
			Command:     "ADD",
			ContainerID: containerId,
			NetNS:       path.Join(helper.NetNS_PATH, containerId),
		})
		Expect(err).NotTo(HaveOccurred())
		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("result: %s", string(data))
		Expect(string(data)).Should(Equal(`{"cniVersion":"0.2.0","ip4":{"ip":"172.16.92.2/32","gateway":"169.254.1.1","routes":[{"dst":"169.254.1.1/32"},{"dst":"0.0.0.0/0","gw":"169.254.1.1"}]},"dns":{}}`), "result: %s", string(data))
		_, err = helper.Ping(containerIP.String())
		Expect(err).NotTo(HaveOccurred())

		// check host iface topology, route, neigh, ip address is expected
		Expect(err).NotTo(HaveOccurred())
		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth"),
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
			Routes: []helper.Route{{Route: netlink.Route{Gw: net.ParseIP("169.254.1.1")}, LinkName: "eth0"}},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

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
