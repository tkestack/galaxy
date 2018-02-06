/*
Copyright 2016 The Gaia Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiswitch_ipam

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/apiswitch"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
)

//TODO: make this as CNI ipam plugin like host-local once if vlan id is supported by types.Result

type IPAMConf struct {
	//ipam url, currently its the apiswitch
	URL         string `json:"url"`
	AllocateURI string `json:"allocate_uri"`
	NodeIP      string `json:"node_ip"`
	// get node ip from which network device
	Devices string `json:"devices"`
}

// retrieve ipinfo either from args or remote api of apiswitch
func Allocate(conf *IPAMConf, kvMap map[string]string) (*cniutil.IPInfo, error) {
	return apiswitch.Allocate(conf.URL, conf.NodeIP, kvMap[k8s.K8S_POD_NAME], kvMap[k8s.K8S_POD_NAMESPACE], conf.AllocateURI)
}

func LoadIPAMConf(bytes []byte) (*IPAMConf, error) {
	conf := &IPAMConf{}
	if err := json.Unmarshal(bytes, conf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	if conf.NodeIP == "" {
		if conf.Devices == "" {
			return nil, fmt.Errorf("no node ip configured")
		}
		devices := strings.Split(conf.Devices, ",")
		if len(devices) == 0 {
			return nil, fmt.Errorf("no node ip configured")
		}
		for _, dev := range devices {
			nic, err := netlink.LinkByName(dev)
			if err != nil {
				continue
			}
			addr, err := netlink.AddrList(nic, netlink.FAMILY_V4)
			if err != nil {
				continue
			}
			if len(addr) == 1 {
				conf.NodeIP = addr[0].IPNet.String()
				break
			}
		}
		if conf.NodeIP == "" {
			return nil, fmt.Errorf("no node ip configured")
		}
	}
	return conf, nil
}
