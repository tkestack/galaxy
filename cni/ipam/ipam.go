package ipam

import (
	"encoding/json"
	"fmt"

	"git.code.oa.com/gaiastack/galaxy/cni/apiswitch-ipam"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

// Allocate tries to find IPInfo from args firstly. If it can't and ipamType is empty, try to allocate ip from apiswitch.
// Otherwise invoke third party ipam binaries
func Allocate(ipamType string, args *skel.CmdArgs) (uint16, *types.Result, error) {
	var (
		vlanId uint16
		result *types.Result
		err    error
	)
	kvMap, err := k8s.ParseK8SCNIArgs(args.Args)
	if err != nil {
		return vlanId, result, err
	}
	if ipInfoStr := kvMap[cniutil.IPInfoInArgs]; ipInfoStr != "" {
		// get ipinfo from cni args
		var ipInfo cniutil.IPInfo
		if err := json.Unmarshal([]byte(ipInfoStr), &ipInfo); err != nil {
			return vlanId, result, fmt.Errorf("failed to unmarshal ipInfo from args %q: %v", args.Args, err)
		}
		return ipInfo.Vlan, cniutil.IPInfoToResult(&ipInfo), nil
	}
	if ipamType == "" {
		// get ipinfo from apiswitch
		ipamConf, err := apiswitch_ipam.LoadIPAMConf(args.StdinData)
		if err != nil {
			return vlanId, result, err
		}
		ipInfo, err := apiswitch_ipam.Allocate(ipamConf, kvMap)
		if err != nil {
			return vlanId, result, err
		}
		return ipInfo.Vlan, cniutil.IPInfoToResult(ipInfo), nil
	}
	// run the IPAM plugin and get back the config to apply
	result, err = ipam.ExecAdd(ipamType, args.StdinData)
	if err != nil {
		return vlanId, result, err
	}
	if result.IP4 == nil {
		return vlanId, result, fmt.Errorf("IPAM plugin returned missing IPv4 config")
	}
	return vlanId, result, err
}

func Release(ipamType string, args *skel.CmdArgs) error {
	if ipamType == "" {
		return nil
	}
	// run the IPAM plugin and get back the config to apply
	return ipam.ExecDel(ipamType, args.StdinData)
}
