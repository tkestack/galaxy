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

package vlan_ipam

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/apiswitch"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputils"
)

//TODO: make this as CNI ipam plugin like host-local

type IPAMConf struct {
	//ipam url, currently its the apiswitch
	URL         string `json:"url"`
	AllocateURI string `json:"allocate_uri"`
	NodeIP      string `json:"node_ip"`
	// get node ip from which network device
	Devices string `json:"devices"`
}

// retrieve ipinfo either from args or remote api of apiswitch
func Allocate(conf *IPAMConf, args string, kvMap map[string]string) (*types.Result, uint16, error) {
	var ipInfo *apiswitch.IPInfo
	if ipInfo = tryGetIPInfo(args); ipInfo == nil {
		client := httputils.NewDefaultClient()
		podName := fmt.Sprintf("%s_%s", kvMap[k8s.K8S_POD_NAME], kvMap[k8s.K8S_POD_NAMESPACE])
		resp, err := client.Post(fmt.Sprintf("%s/%s", conf.URL, fmt.Sprintf(conf.AllocateURI, podName, conf.NodeIP)), "application/json", nil)
		if err == nil {
			defer resp.Body.Close()
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read body: %v", err)
			}
			if resp.StatusCode != 200 {
				return nil, 0, errors.New(string(data))
			}
			ipInfo = new(apiswitch.IPInfo)
			if err := json.Unmarshal(data, ipInfo); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal ipinfo %s: %v", string(data), err)
			}
			if len(ipInfo.Gateway) == 0 {
				return nil, 0, fmt.Errorf("no enough floating ips")
			}
		} else {
			return nil, 0, err
		}
	}
	return IPInfoToResult(ipInfo), ipInfo.Vlan, nil
}

func tryGetIPInfo(args string) *apiswitch.IPInfo {
	var ipInfoFromArgs = &struct {
		types.CommonArgs
		IP      cniutil.IPNet
		Vlan    cniutil.Uint16
		Gateway net.IP
	}{}
	if err := types.LoadArgs(args, ipInfoFromArgs); err != nil {
		return nil
	}
	if len(ipInfoFromArgs.IP.IP) == 0 {
		return nil
	}
	if len(ipInfoFromArgs.Gateway) == 0 {
		return nil
	}
	return &apiswitch.IPInfo{
		IP:      types.IPNet(net.IPNet(ipInfoFromArgs.IP)),
		Vlan:    uint16(ipInfoFromArgs.Vlan),
		Gateway: ipInfoFromArgs.Gateway,
	}
}

func IPInfoToResult(ipInfo *apiswitch.IPInfo) *types.Result {
	return &types.Result{
		IP4: &types.IPConfig{
			IP:      net.IPNet(ipInfo.IP),
			Gateway: ipInfo.Gateway,
			Routes: []types.Route{{
				Dst: net.IPNet{
					IP:   net.IPv4(0, 0, 0, 0),
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
			}},
		},
	}
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
	if conf.AllocateURI == "" {
		conf.AllocateURI = "v2/network/floatingip/%s/allocate/%s"
	}
	return conf, nil
}
