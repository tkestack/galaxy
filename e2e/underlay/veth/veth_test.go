package veth

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"tkestack.io/galaxy/e2e/helper"
	"tkestack.io/galaxy/pkg/utils"
)

var _ = Describe("galaxy-underlay-veth vlan test", func() {
	cni := "galaxy-underlay-veth"
	ifaceCidr := "192.168.0.66/26"
	vlanCidr := "192.168.2.68/26"
	containerCidr := "192.168.2.69/26"
	containerId := helper.NewContainerId()

	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupIFace("dummy0.2")
		helper.CleanupDummy()
	})
	It("vlan", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-underlay-veth",
    "device": "dummy0"
}`)
		Expect(helper.SetupDummyDev("dummy0", ifaceCidr)).NotTo(HaveOccurred())
		Expect(helper.SetupVlanDev("dummy0.2", "dummy0", vlanCidr, 2)).NotTo(HaveOccurred())
		argsStr, err := helper.IPInfo(containerCidr, 2)
		Expect(err).NotTo(HaveOccurred())
		nsPath := helper.CmdAdd(containerId, "", argsStr, cni,
			`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.2.69/26","gateway":"192.168.2.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`, netConf)
		_, err = helper.Ping("192.168.2.69")
		Expect(err).NotTo(HaveOccurred())

		err = (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth"),
			},
		}).Verify()
		Expect(err).NotTo(HaveOccurred())

		// check container iface topology, route, neigh, ip address is expected
		helper.CheckContainerTopology(nsPath, containerCidr, "192.168.2.65")

		// test DEL command
		helper.CmdDel(containerId, cni, netConf)
	})
})
