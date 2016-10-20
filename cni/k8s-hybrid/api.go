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
