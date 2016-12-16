package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/apiswitch"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"io/ioutil"
)

const (
	stateDir = "/var/lib/cni/galaxy"
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
	if err := saveNetworkInfo(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], networkInfo); err != nil {
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
				result, err = withEnv(envs, func() (*types.Result, error) {
					return delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, true)
				})
			} else {
				result, err = delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, true)
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
	networkInfo, err := consumeNetworkInfo(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID])
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
				_, err = withEnv(envs, func() (*types.Result, error) {
					return delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, false)
				})
			} else {
				_, err = delegateCmd(kvMap[k8s.K8S_POD_INFRA_CONTAINER_ID], delegate, false)
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

func saveNetworkInfo(containerID string, info apiswitch.NetworkInfo) error {
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

func consumeNetworkInfo(containerID string) (apiswitch.NetworkInfo, error) {
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
