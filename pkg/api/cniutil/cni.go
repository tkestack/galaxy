package cniutil

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/network/flannel"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/golang/glog"
)

const (
	// CNI_ARGS="IP=192.168.33.3"
	// CNI_COMMAND="ADD"
	// CNI_CONTAINERID=ctn1
	// CNI_NETNS=/var/run/netns/ctn
	// CNI_IFNAME=eth0
	// CNI_PATH=$CNI_PATH
	CNI_ARGS        = "CNI_ARGS"
	CNI_COMMAND     = "CNI_COMMAND"
	CNI_CONTAINERID = "CNI_CONTAINERID"
	CNI_NETNS       = "CNI_NETNS"
	CNI_IFNAME      = "CNI_IFNAME"
	CNI_PATH        = "CNI_PATH"

	COMMAND_ADD = "ADD"
	COMMAND_DEL = "DEL"
)

// like net.IPNet but adds JSON marshalling and unmarshalling
type IPNet net.IPNet

// ParseCIDR takes a string like "10.2.3.1/24" and
// return IPNet with "10.2.3.1" and /24 mask
func ParseCIDR(s string) (*net.IPNet, error) {
	ip, ipn, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}

	ipn.IP = ip
	return ipn, nil
}

func (n IPNet) MarshalJSON() ([]byte, error) {
	return json.Marshal((*net.IPNet)(&n).String())
}

func (n *IPNet) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tmp, err := ParseCIDR(s)
	if err != nil {
		return err
	}

	*n = IPNet(*tmp)
	return nil
}

func (n *IPNet) UnmarshalText(data []byte) error {
	ipNet, err := ParseCIDR(string(data))
	if err != nil {
		return fmt.Errorf("failed to parse cidr %s", string(data))
	}
	*n = IPNet(*ipNet)
	return nil
}

type Uint16 uint16

func (n *Uint16) UnmarshalText(data []byte) error {
	u, err := strconv.ParseUint(string(data), 10, 16)
	if err != nil {
		return fmt.Errorf("failed to parse uint16 %s", string(data))
	}
	*n = Uint16(uint16(u))
	return nil
}

func BuildCNIArgs(args map[string]string) string {
	var entries []string
	for k, v := range args {
		entries = append(entries, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(entries, ";")
}

func DelegateCmd(netconf map[string]interface{}, add bool) (*types.Result, error) {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return nil, fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	if add {
		result, err := invoke.DelegateAdd(netconf["type"].(string), netconfBytes)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, invoke.DelegateDel(netconf["type"].(string), netconfBytes)
}

func DelegateAdd(netconf map[string]interface{}, args *skel.CmdArgs) (*types.Result, error) {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return nil, fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	if netconf["type"] == "galaxy-flannel" {
		args.StdinData = netconfBytes
		return flannel.CmdAdd(args)
	} else {
		pluginPath, err := invoke.FindInPath(netconf["type"].(string), strings.Split(args.Path, ":"))
		if err != nil {
			return nil, err
		}
		glog.Infof("delegate add %s args %s conf %s", args.ContainerID, args.Args, string(netconfBytes))
		return invoke.ExecPluginWithResult(pluginPath, netconfBytes, &invoke.Args{
			Command:       "ADD",
			ContainerID:   args.ContainerID,
			NetNS:         args.Netns,
			PluginArgsStr: args.Args,
			IfName:        args.IfName,
			Path:          args.Path,
		})
	}
}

func DelegateDel(netconf map[string]interface{}, args *skel.CmdArgs) error {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	if netconf["type"] == "galaxy-flannel" {
		args.StdinData = netconfBytes
		return flannel.CmdDel(args)
	} else {
		pluginPath, err := invoke.FindInPath(netconf["type"].(string), strings.Split(args.Path, ":"))
		if err != nil {
			return err
		}
		glog.Infof("delegate del %s args %s conf %s", args.ContainerID, args.Args, string(netconfBytes))
		return invoke.ExecPluginWithoutResult(pluginPath, netconfBytes, &invoke.Args{
			Command:       "DEL",
			ContainerID:   args.ContainerID,
			NetNS:         args.Netns,
			PluginArgsStr: args.Args,
			IfName:        args.IfName,
			Path:          args.Path,
		})
	}
}
