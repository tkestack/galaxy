package ipam

import (
	"encoding/json"
	"fmt"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ipam"
)

// Allocate tries to find IPInfo from args firstly
// Otherwise invoke third party ipam binaries
func Allocate(ipamType string, args *skel.CmdArgs) (uint16, types.Result, error) {
	var (
		vlanId uint16
		err    error
	)
	kvMap, err := cniutil.ParseCNIArgs(args.Args)
	if err != nil {
		return vlanId, nil, err
	}
	if ipInfoStr := kvMap[cniutil.IPInfoInArgs]; ipInfoStr != "" {
		// get ipinfo from cni args
		var ipInfo cniutil.IPInfo
		if err := json.Unmarshal([]byte(ipInfoStr), &ipInfo); err != nil {
			return vlanId, nil, fmt.Errorf("failed to unmarshal ipInfo from args %q: %v", args.Args, err)
		}
		return ipInfo.Vlan, cniutil.IPInfoToResult(&ipInfo), nil
	}
	if ipamType == "" {
		return 0, nil, fmt.Errorf("neither ipInfo from cni args nor ipam type from netconf")
	}
	// run the IPAM plugin and get back the config to apply
	generalResult, err := ipam.ExecAdd(ipamType, args.StdinData)
	if err != nil {
		return vlanId, nil, err
	}
	result, err := t020.GetResult(generalResult)
	if err != nil {
		return vlanId, nil, err
	}
	if result.IP4 == nil {
		return vlanId, nil, fmt.Errorf("IPAM plugin returned missing IPv4 config")
	}
	return vlanId, generalResult, err
}

func Release(ipamType string, args *skel.CmdArgs) error {
	if ipamType == "" {
		return nil
	}
	// run the IPAM plugin and get back the config to apply
	return ipam.ExecDel(ipamType, args.StdinData)
}
