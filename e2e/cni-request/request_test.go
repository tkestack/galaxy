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
package cni_request_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"

	"tkestack.io/galaxy/e2e"
	"tkestack.io/galaxy/e2e/helper"
	"tkestack.io/galaxy/pkg/api/cniutil"
	"tkestack.io/galaxy/pkg/api/galaxy/private"
	"tkestack.io/galaxy/pkg/galaxy"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	glog "k8s.io/klog"
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
    }
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
		content := call(cniutil.COMMAND_ADD)
		// the result ip may change, check other things.
		Expect(strings.HasPrefix(content, `{"cniVersion":"0.2.0","ip4":{"ip":"172.16.59.`)).Should(BeTrue())
		Expect(strings.HasSuffix(content, `/24","gateway":"172.16.59.1","routes":[{"dst":"172.16.0.0/13","gw":"172.16.59.1"},{"dst":"0.0.0.0/0","gw":"172.16.59.1"}]},"dns":{}}`)).
			Should(BeTrue())
	})
	It("cni delete request", func() {
		content := call(cniutil.COMMAND_DEL)
		Expect(content).Should(Equal(``), "result: %s", content)
	})
})

func call(method string) string {
	req, err := NewFakeRequest(method)
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
	return string(content)
}
