package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/apiswitch"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	galaxyapi "git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputils"
	"github.com/containernetworking/cni/pkg/types"
)

const (
	stateDir       = "/var/lib/cni/galaxy"
	networkTypeURI = "v2/network/type/%s/%s"
)

func CmdAdd(req *galaxyapi.PodRequest, url, nodeIP string, netConf map[string]map[string]interface{}) (*types.Result, error) {
	networkInfo, err := getNetworkInfo(url, nodeIP, req.PodName, req.PodNamespace)
	if err != nil {
		return nil, err
	}
	if len(networkInfo) == 0 {
		return nil, fmt.Errorf("No network info returned")
	}
	if err := SaveNetworkInfo(req.ContainerID, networkInfo); err != nil {
		return nil, fmt.Errorf("Error save network info %v for %s: %v", networkInfo, req.ContainerID, err)
	}
	var result *types.Result
	for t, v := range networkInfo {
		conf, ok := netConf[t]
		if !ok {
			return nil, fmt.Errorf("network %s not configured", t)
		}
		//append additional args from network info
		req.Args = fmt.Sprintf("%s;%s", req.Args, cniutil.BuildCNIArgs(v))
		result, err = cniutil.DelegateAdd(conf, req.CmdArgs)
		// configure only one network
		break
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func CmdDel(req *galaxyapi.PodRequest, netConf map[string]map[string]interface{}) error {
	networkInfo, err := ConsumeNetworkInfo(req.ContainerID)
	if err != nil {
		if os.IsNotExist(err) {
			// Duplicated cmdDel invoked by kubelet
			return nil
		}
		return fmt.Errorf("Error consume network info %v for %s: %v", networkInfo, req.ContainerID, err)
	}
	for t, v := range networkInfo {
		conf, ok := netConf[t]
		if !ok {
			return fmt.Errorf("network %s not configured", t)
		}
		//append additional args from network info
		req.Args = fmt.Sprintf("%s;%s", req.Args, cniutil.BuildCNIArgs(v))
		err = cniutil.DelegateDel(conf, req.CmdArgs)
		return err
	}
	return fmt.Errorf("No network info returned")
}

func getNetworkInfo(url, nodeIP, podName, podNamespace string) (apiswitch.NetworkInfo, error) {
	var networkInfo apiswitch.NetworkInfo
	client := httputils.NewDefaultClient()
	podFullName := fmt.Sprintf("%s_%s", podName, podNamespace)
	resp, err := client.Post(fmt.Sprintf("%s/%s", url, fmt.Sprintf(networkTypeURI, podFullName, nodeIP)), "application/json", nil)
	if err == nil {
		if resp.StatusCode != 200 {
			if resp.StatusCode == 400 {
				// Pod is not a gaiastack app
				infos := make(map[string]map[string]string)
				infos["galaxy-flannel"] = nil
				return infos, nil
			}
			defer resp.Body.Close()
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("apiswitch http response 500: %s", string(data))
		}
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &networkInfo); err != nil {
			return nil, fmt.Errorf("failed to unmarshal network info %s: %v", networkInfo, err)
		}
	} else {
		return nil, err
	}
	return networkInfo, nil
}

func SaveNetworkInfo(containerID string, info apiswitch.NetworkInfo) error {
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

func ConsumeNetworkInfo(containerID string) (apiswitch.NetworkInfo, error) {
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

func WithEnv(envs []string, f func() (*types.Result, error)) (*types.Result, error) {
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
