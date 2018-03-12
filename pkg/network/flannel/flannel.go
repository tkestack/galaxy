package flannel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

const (
	defaultSubnetFile = "/run/flannel/subnet.env"
	stateDir          = "/var/lib/cni/flannel"
)

type NetConf struct {
	types.NetConf
	SubnetFile string                 `json:"subnetFile"`
	Delegate   map[string]interface{} `json:"delegate"`
}

type subnetEnv struct {
	nw     *net.IPNet
	sn     *net.IPNet
	mtu    *uint
	ipmasq *bool
}

func (se *subnetEnv) missing() string {
	m := []string{}

	if se.nw == nil {
		m = append(m, "FLANNEL_NETWORK")
	}
	if se.sn == nil {
		m = append(m, "FLANNEL_SUBNET")
	}
	if se.mtu == nil {
		m = append(m, "FLANNEL_MTU")
	}
	if se.ipmasq == nil {
		m = append(m, "FLANNEL_IPMASQ")
	}
	return strings.Join(m, ", ")
}

func loadFlannelNetConf(bytes []byte) (*NetConf, error) {
	n := &NetConf{
		SubnetFile: defaultSubnetFile,
	}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return n, nil
}

func loadFlannelSubnetEnv(fn string) (*subnetEnv, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	se := &subnetEnv{}

	s := bufio.NewScanner(f)
	for s.Scan() {
		parts := strings.SplitN(s.Text(), "=", 2)
		switch parts[0] {
		case "FLANNEL_NETWORK":
			_, se.nw, err = net.ParseCIDR(parts[1])
			if err != nil {
				return nil, err
			}

		case "FLANNEL_SUBNET":
			_, se.sn, err = net.ParseCIDR(parts[1])
			if err != nil {
				return nil, err
			}

		case "FLANNEL_MTU":
			mtu, err := strconv.ParseUint(parts[1], 10, 32)
			if err != nil {
				return nil, err
			}
			se.mtu = new(uint)
			*se.mtu = uint(mtu)

		case "FLANNEL_IPMASQ":
			ipmasq := parts[1] == "true"
			se.ipmasq = &ipmasq
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}

	if m := se.missing(); m != "" {
		return nil, fmt.Errorf("%v is missing %v", fn, m)
	}

	return se, nil
}

func saveScratchNetConf(containerID string, netconf []byte) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, containerID)
	return ioutil.WriteFile(path, netconf, 0600)
}

func consumeScratchNetConf(containerID string) ([]byte, error) {
	path := filepath.Join(stateDir, containerID)
	defer os.Remove(path)

	return ioutil.ReadFile(path)
}

func delegateAdd(args *skel.CmdArgs, netconf map[string]interface{}) (types.Result, error) {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return nil, fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	// save the rendered netconf for cmdDel
	if err = saveScratchNetConf(args.ContainerID, netconfBytes); err != nil {
		return nil, err
	}

	pluginPath, err := invoke.FindInPath(netconf["type"].(string), strings.Split(args.Path, ":"))
	if err != nil {
		return nil, err
	}

	// We'll need to configure veth device in container's netns during bridge
	// network setup. We can't embed bridge network setup and must do it in a
	// new process, because switching netns in golang is very dangerous.
	// See https://github.com/containernetworking/cni/issues/262
	return invoke.ExecPluginWithResult(pluginPath, netconfBytes, &invoke.Args{
		Command:     "ADD",
		ContainerID: args.ContainerID,
		NetNS:       args.Netns,
		IfName:      args.IfName,
		Path:        args.Path,
	})
}

func hasKey(m map[string]interface{}, k string) bool {
	_, ok := m[k]
	return ok
}

func isString(i interface{}) bool {
	_, ok := i.(string)
	return ok
}

func CmdAdd(args *skel.CmdArgs) (types.Result, error) {
	n, err := loadFlannelNetConf(args.StdinData)
	if err != nil {
		return nil, err
	}

	fenv, err := loadFlannelSubnetEnv(n.SubnetFile)
	if err != nil {
		return nil, err
	}

	if n.Delegate == nil {
		n.Delegate = make(map[string]interface{})
	} else {
		if hasKey(n.Delegate, "type") && !isString(n.Delegate["type"]) {
			return nil, fmt.Errorf("'delegate' dictionary, if present, must have (string) 'type' field")
		}
		if hasKey(n.Delegate, "name") {
			return nil, fmt.Errorf("'delegate' dictionary must not have 'name' field, it'll be set by flannel")
		}
		if hasKey(n.Delegate, "ipam") {
			return nil, fmt.Errorf("'delegate' dictionary must not have 'ipam' field, it'll be set by flannel")
		}
	}

	n.Delegate["name"] = n.Name

	if !hasKey(n.Delegate, "type") {
		n.Delegate["type"] = "bridge"
	}

	if !hasKey(n.Delegate, "ipMasq") {
		// if flannel is not doing ipmasq, we should
		ipmasq := !*fenv.ipmasq
		n.Delegate["ipMasq"] = ipmasq
	}

	if !hasKey(n.Delegate, "mtu") {
		mtu := fenv.mtu
		n.Delegate["mtu"] = mtu
	}

	if n.Delegate["type"].(string) == "bridge" {
		if !hasKey(n.Delegate, "isGateway") {
			n.Delegate["isGateway"] = true
		}
	}

	n.Delegate["ipam"] = map[string]interface{}{
		"type":   "host-local",
		"subnet": fenv.sn.String(),
		"routes": []types.Route{
			{
				Dst: *fenv.nw,
			},
		},
	}

	return delegateAdd(args, n.Delegate)
}

func CmdDel(args *skel.CmdArgs) error {
	netconfBytes, err := consumeScratchNetConf(args.ContainerID)
	if err != nil {
		// kubelet invokes twice cmdDel, if file not exist, be silence
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	n := &types.NetConf{}
	if err = json.Unmarshal(netconfBytes, n); err != nil {
		return fmt.Errorf("failed to parse netconf: %v", err)
	}

	pluginPath, err := invoke.FindInPath(n.Type, strings.Split(args.Path, ":"))
	if err != nil {
		return err
	}

	return invoke.ExecPluginWithoutResult(pluginPath, netconfBytes, &invoke.Args{
		Command:     "DEL",
		ContainerID: args.ContainerID,
		NetNS:       args.Netns,
		IfName:      args.IfName,
		Path:        args.Path,
	})
}
