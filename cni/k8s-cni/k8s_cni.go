package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
)

type NetConf struct {
	types.NetConf
	NetworkType map[string]map[string]interface{} `json:"networkType,omitempty"`
	//ipam url, currently its the apiswitch
	URL        string `json:"url"`
	NetworkURI string `json:"network_uri"`
}

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func loadConf(bytes []byte) (*NetConf, error) {
	conf := &NetConf{}
	if err := json.Unmarshal(bytes, conf); err != nil {
		return nil, fmt.Errorf("failed to load hybrid netconf: %v", err)
	}
	return conf, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	return cmd(args, true)
}

func cmdDel(args *skel.CmdArgs) error {
	if args.Netns == "" {
		// avoid k8s double delete error
		// see https://github.com/kubernetes/kubernetes/issues/20379#issuecomment-255272531
		return nil
	}
	return cmd(args, false)
}

func cmd(args *skel.CmdArgs, add bool) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	kvMap, err := k8s.ParseK8SCNIArgs(args.Args)
	if err != nil {
		return err
	}
	for networkType, delegate := range conf.NetworkType {
		result, err := delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, add)
		if err != nil {
			return fmt.Errorf("failed to delegate setup network %s: %v", networkType, err)
		}
		ports, err := k8s.ParsePorts(kvMap[k8s.K8S_PORTS])
		if err != nil {
			return err
		}
		if add {
			// we have to fulfill ip field of the current pod
			if result.IP4 == nil {
				return fmt.Errorf("CNI plugin reported no IPv4 address")
			}
			ip4 := result.IP4.IP.IP.To4()
			if ip4 == nil {
				return fmt.Errorf("CNI plugin reported an invalid IPv4 address: %+v.", result.IP4)
			}
			for _, p := range ports {
				if p.PodName == kvMap[k8s.K8S_POD_NAME]+"_"+kvMap[k8s.K8S_POD_NAMESPACE] {
					p.PodIP = ip4.String()
				}
			}
			if err := setupPortMapping("cni0", ports); err != nil {
				cleanPortMapping("cni0", ports)
				return fmt.Errorf("failed to setup port mapping %v: %v", ports, err)
			}
		} else {
			if err := cleanPortMapping("cni0", ports); err != nil {
				setupPortMapping("cni0", ports)
				return fmt.Errorf("failed to delete port mapping %v: %v", ports, err)
			}
		}
		//TODO send http request for l5 setup
		if add {
			return result.Print()
		}
		return nil
	}
	return fmt.Errorf("no network configured")
}

func delegateCmd(cid string, netconf map[string]interface{}, add bool) (*types.Result, error) {
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

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}
