package k8s_vlan_test

import (
	"encoding/json"
	"path"

	"git.code.oa.com/gaiastack/galaxy/e2e/helper"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bridge", func() {
	cni := "galaxy-k8s-vlan"
	cidr := "192.168.0.68/26"
	containerId := helper.NewContainerId()
	netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "default_bridge_name": "brtest"
}`)
	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupDummy()
		helper.CleanupIFace("brtest")
	})
	It("Add a container", func() {
		err := helper.NewNetNS(containerId)
		Expect(err).NotTo(HaveOccurred())
		err = helper.SetupDummyDev("dummy0", cidr)
		Expect(err).NotTo(HaveOccurred())
		argsStr, err := helper.IPInfo(cidr, 0)
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
		Expect(string(data)).Should(Equal(`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`))
		//TODO check iface topology, route, neigh, ip address is expected
		_, err = helper.Ping("192.168.0.68")
		Expect(err).NotTo(HaveOccurred())
	})

})
