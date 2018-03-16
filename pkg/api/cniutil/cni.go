package cniutil

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/flannel"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
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

const (
	IPInfoInArgs = "IPInfo"
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

func DelegateCmd(netconf map[string]interface{}, add bool) (types.Result, error) {
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

func DelegateAdd(netconf map[string]interface{}, args *skel.CmdArgs) (types.Result, error) {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return nil, fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	if netconf["type"] == private.NetworkTypeOverlay.CNIType {
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

	if netconf["type"] == private.NetworkTypeOverlay.CNIType {
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

func CmdAdd(containerID string, cmdArgs *skel.CmdArgs, netConf map[string]map[string]interface{}, networkInfo NetworkInfo) (types.Result, error) {
	if len(networkInfo) == 0 {
		return nil, fmt.Errorf("No network info returned")
	}
	if err := SaveNetworkInfo(containerID, networkInfo); err != nil {
		return nil, fmt.Errorf("Error save network info %v for %s: %v", networkInfo, containerID, err)
	}
	var (
		err    error
		result types.Result
	)
	for t, v := range networkInfo {
		conf, ok := netConf[t]
		if !ok {
			return nil, fmt.Errorf("network %s not configured", t)
		}
		//append additional args from network info
		cmdArgs.Args = fmt.Sprintf("%s;%s", cmdArgs.Args, BuildCNIArgs(v))
		result, err = DelegateAdd(conf, cmdArgs)
		// configure only one network
		break
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

type NetworkInfo map[string]map[string]string

func CmdDel(containerID string, cmdArgs *skel.CmdArgs, netConf map[string]map[string]interface{}) error {
	networkInfo, err := ConsumeNetworkInfo(containerID)
	if err != nil {
		if os.IsNotExist(err) {
			// Duplicated cmdDel invoked by kubelet
			return nil
		}
		return fmt.Errorf("Error consume network info %v for %s: %v", networkInfo, containerID, err)
	}
	for t, v := range networkInfo {
		conf, ok := netConf[t]
		if !ok {
			return fmt.Errorf("network %s not configured", t)
		}
		//append additional args from network info
		cmdArgs.Args = fmt.Sprintf("%s;%s", cmdArgs.Args, BuildCNIArgs(v))
		err = DelegateDel(conf, cmdArgs)
		return err
	}
	return fmt.Errorf("No network info returned")
}

const (
	stateDir = "/var/lib/cni/galaxy"
)

func SaveNetworkInfo(containerID string, info NetworkInfo) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, containerID)
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0600)
}

func ConsumeNetworkInfo(containerID string) (NetworkInfo, error) {
	m := make(map[string]map[string]string)
	path := filepath.Join(stateDir, containerID)
	defer os.Remove(path)

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

/*
	{"ip":"10.49.27.205/24","vlan":2,"gateway":"10.49.27.1"}
*/
type IPInfo struct {
	IP             types.IPNet `json:"ip"`
	Vlan           uint16      `json:"vlan"`
	Gateway        net.IP      `json:"gateway"`
	RoutableSubnet types.IPNet `json:"routable_subnet"`
}

func IPInfoToResult(ipInfo *IPInfo) *t020.Result {
	return &t020.Result{
		IP4: &t020.IPConfig{
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

// ConfigureIface takes the result of IPAM plugin and
// applies to the ifName interface
func ConfigureIface(ifName string, res *t020.Result) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup %q: %v", ifName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set %q UP: %v", ifName, err)
	}

	// TODO(eyakubovich): IPv6
	addr := &netlink.Addr{IPNet: &res.IP4.IP, Label: ""}
	if err = netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add IP addr to %q: %v", ifName, err)
	}

	for _, r := range res.IP4.Routes {
		gw := r.GW
		if gw == nil {
			gw = res.IP4.Gateway
		}
		if err = ip.AddRoute(&r.Dst, gw, link); err != nil {
			// we skip over duplicate routes as we assume the first one wins
			if !os.IsExist(err) {
				return fmt.Errorf("failed to add route '%v via %v dev %v': %v", r.Dst, gw, ifName, err)
			}
		}
	}

	return nil
}
