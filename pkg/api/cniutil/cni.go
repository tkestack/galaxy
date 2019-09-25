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

	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
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

func ParseCNIArgs(args string) (map[string]string, error) {
	kvMap := make(map[string]string)
	kvs := strings.Split(args, ";")
	if len(kvs) == 0 {
		return kvMap, fmt.Errorf("invalid args %s", args)
	}
	for _, kv := range kvs {
		part := strings.SplitN(kv, "=", 2)
		if len(part) != 2 {
			continue
		}
		kvMap[strings.TrimSpace(part[0])] = strings.TrimSpace(part[1])
	}
	return kvMap, nil
}

func DelegateAdd(netconf map[string]interface{}, args *skel.CmdArgs, ifName string) (types.Result, error) {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return nil, fmt.Errorf("error serializing delegate netconf: %v", err)
	}
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
		IfName:        ifName,
		Path:          args.Path,
	})
}

func DelegateDel(netconf map[string]interface{}, args *skel.CmdArgs, ifName string) error {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return fmt.Errorf("error serializing delegate netconf: %v", err)
	}
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
		IfName:        ifName,
		Path:          args.Path,
	})
}

func CmdAdd(cmdArgs *skel.CmdArgs, networkInfos []*NetworkInfo) (types.Result, error) {
	if len(networkInfos) == 0 {
		return nil, fmt.Errorf("No network info returned")
	}
	if err := saveNetworkInfo(cmdArgs.ContainerID, networkInfos); err != nil {
		return nil, fmt.Errorf("Error save network info %v for %s: %v", networkInfos, cmdArgs.ContainerID, err)
	}
	var (
		err    error
		result types.Result
	)
	for idx, networkInfo := range networkInfos {
		//append additional args from network info
		cmdArgs.Args = strings.TrimRight(fmt.Sprintf("%s;%s", cmdArgs.Args, BuildCNIArgs(networkInfo.Args)), ";")
		result, err = DelegateAdd(networkInfo.Conf, cmdArgs, networkInfo.IfName)
		if err != nil {
			//fail to add cni, then delete all established CNIs recursively
			glog.Errorf("fail to add network %s: %v, begin to rollback and delete it", networkInfo.Args, err)
			delErr := CmdDel(cmdArgs, idx)
			glog.Warningf("fail to delete cni in rollback %v", delErr)
			return nil, fmt.Errorf("fail to establish network %s:%v", networkInfo.Args, err)
		}
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

type NetworkInfo struct {
	NetworkType string
	Args        map[string]string
	Conf        map[string]interface{}
	IfName      string
}

func NewNetworkInfo(networkType string, args map[string]string, conf map[string]interface{}, ifName string) *NetworkInfo {
	return &NetworkInfo{NetworkType: networkType, Args: args, Conf: conf, IfName: ifName}
}

func reverse(infos []*NetworkInfo) {
	for i, j := 0, len(infos)-1; i < j; i, j = i+1, j-1 {
		infos[i], infos[j] = infos[j], infos[i]
	}
}

func CmdDel(cmdArgs *skel.CmdArgs, lastIdx int) error {
	networkInfos, err := consumeNetworkInfo(cmdArgs.ContainerID)
	if err != nil {
		if os.IsNotExist(err) {
			// Duplicated cmdDel invoked by kubelet
			return nil
		}
		return fmt.Errorf("Error consume network info %v for %s: %v", networkInfos, cmdArgs.ContainerID, err)
	}
	if lastIdx == -1 {
		lastIdx = len(networkInfos) - 1
	}
	var errorSet []string
	var fails []*NetworkInfo
	for idx := lastIdx; idx >= 0; idx-- {
		networkInfo := networkInfos[idx]
		//append additional args from network info
		cmdArgs.Args = strings.TrimRight(fmt.Sprintf("%s;%s", cmdArgs.Args, BuildCNIArgs(networkInfo.Args)), ";")
		err := DelegateDel(networkInfo.Conf, cmdArgs, networkInfo.IfName)
		if err != nil {
			errorSet = append(errorSet, err.Error())
			fails = append(fails, networkInfo)
			glog.Errorf("failed to delete network %v: %v", networkInfo.Args, err)
		}
	}
	if len(errorSet) > 0 {
		reverse(fails)
		if err := saveNetworkInfo(cmdArgs.ContainerID, fails); err != nil {
			glog.Warningf("Error save network info %v for %s: %v", fails, cmdArgs.ContainerID, err)
		}
		return fmt.Errorf(strings.Join(errorSet, " / "))
	}
	return nil
}

func IPInfoToResult(ipInfo *constant.IPInfo) *t020.Result {
	return &t020.Result{
		IP4: &t020.IPConfig{
			IP:      net.IPNet(*ipInfo.IP),
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

const (
	stateDir = "/var/lib/cni/galaxy"
)

func saveNetworkInfo(containerID string, infos []*NetworkInfo) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, containerID)
	data, err := json.Marshal(infos)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0600)
}

func consumeNetworkInfo(containerID string) ([]*NetworkInfo, error) {
	var infos []*NetworkInfo
	path := filepath.Join(stateDir, containerID)
	defer os.Remove(path) // nolint: errcheck

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return infos, err
	}
	if err := json.Unmarshal(data, &infos); err != nil {
		return infos, err
	}
	return infos, nil
}
