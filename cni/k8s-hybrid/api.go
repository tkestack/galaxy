package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/apiswitch"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputils"
)

func getNetworkInfo(conf *NetConf, kvMap map[string]string) (apiswitch.NetworkInfo, error) {
	var networkInfo apiswitch.NetworkInfo
	client := httputils.NewDefaultClient()
	resp, err := client.Post(fmt.Sprintf("%s/%s", conf.URL, fmt.Sprintf(conf.NetworkURI, kvMap[k8s.K8S_POD_NAME], conf.NodeIP)), "application/json", nil)
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
