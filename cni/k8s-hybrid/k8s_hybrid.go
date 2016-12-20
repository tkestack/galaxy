package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/remote"
)

type NetConf struct {
	types.NetConf
	NetworkType map[string]map[string]interface{} `json:"networkType,omitempty"`
	//ipam url, currently its the apiswitch
	URL        string `json:"url"`
	NetworkURI string `json:"network_uri"`
	NodeIP     string `json:"node_ip"`
	// get node ip from which network device
	Devices string `json:"devices"`
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
	return conf, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	kvMap, err := k8s.ParseK8SCNIArgs(args.Args)
	if err != nil {
		return err
	}
	networkInfo, err := getNetworkInfo(conf, kvMap)
	if err != nil {
		return err
	}
	if len(networkInfo) == 0 {
		return fmt.Errorf("No network info returned")
	}
	if err := remote.SaveNetworkInfo(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], networkInfo); err != nil {
		return fmt.Errorf("Error save network info %v for %s: %v", networkInfo, kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], err)
	}
	var result *types.Result
	for k, v := range networkInfo {
		if delegate, ok := conf.NetworkType[k]; ok {
			if len(v) != 0 {
				//TODO test this situation
				envs := os.Environ()
				//append additional args
				for i, env := range envs {
					if strings.HasPrefix(env, cniutil.CNI_ARGS) {
						envs[i] = fmt.Sprintf("%s;%s", env, cniutil.BuildCNIArgs(v))
						break
					}
				}
				result, err = remote.WithEnv(envs, func() (*types.Result, error) {
					return cniutil.DelegateCmd(delegate, true)
				})
			} else {
				result, err = cniutil.DelegateCmd(delegate, true)
			}
			// configure only one network
			break
		} else {
			return fmt.Errorf("No configures for network type %s", k)
		}
	}
	if err != nil {
		return err
	}
	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	kvMap, err := k8s.ParseK8SCNIArgs(args.Args)
	if err != nil {
		return err
	}
	networkInfo, err := remote.ConsumeNetworkInfo(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID])
	if err != nil {
		if os.IsNotExist(err) {
			// Duplicated cmdDel invoked by kubelet
			return nil
		}
		return fmt.Errorf("Error consume network info %v for %s: %v", networkInfo, kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], err)
	}
	if len(networkInfo) == 0 {
		return fmt.Errorf("No network info returned")
	}
	for k, v := range networkInfo {
		if delegate, ok := conf.NetworkType[k]; ok {
			if len(v) != 0 {
				//TODO test this situation
				envs := os.Environ()
				//append additional args
				for i, env := range envs {
					if strings.HasPrefix(env, cniutil.CNI_ARGS) {
						envs[i] = fmt.Sprintf("%s;%s", env, cniutil.BuildCNIArgs(v))
						break
					}
				}
				_, err = remote.WithEnv(envs, func() (*types.Result, error) {
					return cniutil.DelegateCmd(delegate, false)
				})
			} else {
				_, err = cniutil.DelegateCmd(delegate, false)
			}
			// configure only one network
			break
		} else {
			return fmt.Errorf("No configures for network type %s", k)
		}
	}
	return err
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}
