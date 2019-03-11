package cni_request_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"

	"git.code.oa.com/gaiastack/galaxy/e2e"
	"git.code.oa.com/gaiastack/galaxy/e2e/helper"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/galaxy"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func NewFakeRequest(method string) (string, error) {
	containerId := helper.NewContainerId()
	nsPath, err := helper.NewNetNS(containerId)
	if err != nil {
		return "", err
	}
	root := helper.ProjectDir()
	pluginDir := path.Join(root, "bin")
	rawReq := `{
    "env": {
        "CNI_COMMAND": "%s",
        "CNI_CONTAINERID": "%s",
        "CNI_NETNS": "%s",
        "CNI_IFNAME": "eth0",
        "CNI_PATH": "%s",
        "CNI_ARGS": "IgnoreUnknown=true;K8S_POD_NAMESPACE=mynamespace;K8S_POD_NAME=mypod-0;K8S_POD_INFRA_CONTAINER_ID=%s"
    },
    "config":"eyJjYXBhYmlsaXRpZXMiOnsicG9ydE1hcHBpbmdzIjp0cnVlfSwiY25pVmVyc2lvbiI6IiIsIm5hbWUiOiIiLCJydW50aW1lQ29uZmlnIjp7InBvcnRNYXBwaW5ncyI6W3siaG9zdFBvcnQiOjMwMDAxLCJjb250YWluZXJQb3J0Ijo4MCwicHJvdG9jb2wiOiJ0Y3AiLCJob3N0SVAiOiIifV19fQ=="
}`
	req := fmt.Sprintf(rawReq, method, containerId, nsPath, pluginDir, containerId)
	return req, nil
}

const (
	FlannelSubnetFile = "/run/flannel/subnet.env"
	FlannelSubnetDir  = "/run/flannel"
)

var (
	g          *galaxy.Galaxy
	createFile bool
	jsonFile   string
)

var _ = BeforeSuite(func() {
	createFile = false

	if _, err := os.Stat(FlannelSubnetFile); err == nil {
		Expect(err).NotTo(HaveOccurred())
	} else if os.IsNotExist(err) {
		if _, err := os.Stat(FlannelSubnetDir); os.IsNotExist(err) {
			err = os.MkdirAll(FlannelSubnetDir, 0755)
			Expect(err).NotTo(HaveOccurred())
		}
		createFile = true
		content := `FLANNEL_NETWORK=172.16.0.0/13
FLANNEL_SUBNET=172.16.59.1/24
FLANNEL_MTU=1480
FLANNEL_IPMASQ=true`
		if err := ioutil.WriteFile(FlannelSubnetFile, []byte(content), 0644); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
	jsonConfigFile, err := ioutil.TempFile("", "")
	Expect(err).NotTo(HaveOccurred())
	jsonFile = jsonConfigFile.Name()
	Expect(jsonConfigFile.Close()).NotTo(HaveOccurred())
	err = ioutil.WriteFile(jsonFile, []byte(`{"NetworkConf":[{"type":"galaxy-flannel", "delegate":{"type":"galaxy-bridge","isDefaultGateway":true,"forceAddress":true},"subnetFile":"/run/flannel/subnet.env"}], "DefaultNetworks": ["galaxy-flannel"]}`), 0644)
	Expect(err).NotTo(HaveOccurred())

	g := galaxy.NewGalaxy()
	g.JsonConfigPath = jsonFile
	err = g.Init()
	Expect(err).NotTo(HaveOccurred())
	fakeCli := e2e.CreateFakeClient()
	g.SetClient(fakeCli)
	go g.StartServer()
})

var _ = AfterSuite(func() {
	if g != nil {
		g.Stop()
	}
	if createFile {
		if err := os.Remove(FlannelSubnetFile); err != nil {
			glog.Errorf("fail to remove %s", FlannelSubnetFile)
		}
	}
	if jsonFile != "" {
		if err := os.Remove(jsonFile); err != nil {
			glog.Errorf("fail to remove %s", jsonFile)
		}
	}
})

var _ = Describe("cni add request", func() {
	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupDummy()
	})
	It("test cni add request", func() {
		req, err := NewFakeRequest(cniutil.COMMAND_ADD)
		Expect(err).NotTo(HaveOccurred())

		client := &http.Client{
			Transport: &http.Transport{
				Dial: func(proto, addr string) (net.Conn, error) {
					return net.Dial("unix", private.GalaxySocketPath)
				},
			},
		}
		resp, err := client.Post("http://dummy/cni", "application/json", bytes.NewReader([]byte(req)))
		Expect(err).NotTo(HaveOccurred())
		content, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("response is %s", string(content))
	})
	It("cni delete request", func() {
		req, err := NewFakeRequest(cniutil.COMMAND_DEL)
		Expect(err).NotTo(HaveOccurred())

		client := &http.Client{
			Transport: &http.Transport{
				Dial: func(proto, addr string) (net.Conn, error) {
					return net.Dial("unix", private.GalaxySocketPath)
				},
			},
		}
		resp, err := client.Post("http://dummy/cni", "application/json", bytes.NewReader([]byte(req)))
		Expect(err).NotTo(HaveOccurred())
		content, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		glog.V(4).Infof("response is %s", string(content))
	})
})
