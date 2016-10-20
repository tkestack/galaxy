package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/containernetworking/cni/pkg/types"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/apiswitch"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputils"
)

// retrieve ipinfo either from args or remote api of apiswitch
func allocate(conf *IPAMConf, args string, kvMap map[string]string) (*types.Result, uint16, error) {
	var ipInfo *apiswitch.IPInfo
	if ipInfo = tryGetIPInfo(args); ipInfo == nil {
		client := httputils.NewDefaultClient()
		resp, err := client.Post(fmt.Sprintf("%s/%s", conf.URL, fmt.Sprintf(conf.AllocateURI, kvMap[k8s.K8S_POD_NAME], conf.NodeIP)), "application/json", nil)
		if err == nil {
			defer resp.Body.Close()
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read body: %v", err)
			}
			if resp.StatusCode != 200 {
				return nil, 0, errors.New(string(data))
			}
			//TODO handle no enough ip return
			ipInfo = new(apiswitch.IPInfo)
			if err := json.Unmarshal(data, ipInfo); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal ipinfo %s: %v", string(data), err)
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
