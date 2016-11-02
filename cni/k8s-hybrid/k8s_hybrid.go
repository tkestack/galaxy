package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
)

type NetConf struct {
	types.NetConf
	NetworkType map[string]map[string]interface{} `json:"networkType,omitempty"`
	//ipam url, currently its the apiswitch
	URL        string `json:"url"`
	NetworkURI string `json:"network_uri"`
	NodeIP     string `json:"node_ip"`
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
	networkInfo, err := getNetworkInfo(conf, kvMap)
	if err != nil {
		return err
	}
	index := 0
	// prints the result returned by the last network plugin
	var result *types.Result
	//TODO handle default route
	for k, v := range networkInfo {
		if delegate, ok := conf.NetworkType[k]; ok {
			os.Setenv(cniutil.CNI_IFNAME, fmt.Sprintf("eth%d", index))
			index++
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
				result, err = withEnv(envs, func() (*types.Result, error) {
					return delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, add)
				})
				if err != nil {
					//TODO handle deleting previous succeed
					return fmt.Errorf("failed to delegate setup network %s: %v", k, err)
				}
			} else {
				result, err = delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, add)
				if err != nil {
					return fmt.Errorf("failed to delegate setup network %s: %v", k, err)
				}
			}
		} else {
			return fmt.Errorf("No configures for network type %s", k)
		}
	}
	if result == nil {
		return fmt.Errorf("Network not configured %v", networkInfo)
	}
	if add {
		return result.Print()
	}
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
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

func withEnv(envs []string, f func() (*types.Result, error)) (*types.Result, error) {
	origin := os.Environ()
	setEnv(envs)
	defer setEnv(origin)
	return f()
}

func setEnv(envs []string) {
	os.Clearenv()
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		os.Setenv(parts[0], parts[1])
	}
}
