package ipam

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ipam"
	"tkestack.io/galaxy/pkg/api/cniutil"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

// Allocate tries to find IPInfo from args firstly
// Otherwise invoke third party ipam binaries
func Allocate(ipamType string, args *skel.CmdArgs) ([]uint16, []types.Result, error) {
	var (
		vlanId uint16
		err    error
	)
	kvMap, err := cniutil.ParseCNIArgs(args.Args)
	if err != nil {
		return nil, nil, err
	}
	var results []types.Result
	var vlanIDs []uint16
	if ipInfoStr := kvMap[constant.IPInfosKey]; ipInfoStr != "" {
		// get ipinfo from cni args
		var ipInfos []constant.IPInfo
		if err := json.Unmarshal([]byte(ipInfoStr), &ipInfos); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal ipInfo from args %q: %v", args.Args, err)
		}
		if len(ipInfos) == 0 {
			return nil, nil, fmt.Errorf("empty ipInfos")
		}
		for j := range ipInfos {
			results = append(results, cniutil.IPInfoToResult(&ipInfos[j]))
			vlanIDs = append(vlanIDs, ipInfos[j].Vlan)
		}
		return vlanIDs, results, nil
	}
	if ipamType == "" {
		return nil, nil, fmt.Errorf("neither ipInfo from cni args nor ipam type from netconf")
	}
	// run the IPAM plugin and get back the config to apply
	generalResult, err := ipam.ExecAdd(ipamType, args.StdinData)
	if err != nil {
		return nil, nil, err
	}
	result, err := t020.GetResult(generalResult)
	if err != nil {
		return nil, nil, err
	}
	if result.IP4 == nil {
		return nil, nil, fmt.Errorf("IPAM plugin returned missing IPv4 config")
	}
	return append(vlanIDs, vlanId), append(results, generalResult), err
}

func Release(ipamType string, args *skel.CmdArgs) error {
	if ipamType == "" {
		return nil
	}
	// run the IPAM plugin and get back the config to apply
	return ipam.ExecDel(ipamType, args.StdinData)
}
