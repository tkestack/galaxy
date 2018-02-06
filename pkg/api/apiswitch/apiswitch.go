package apiswitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputils"
)

const (
	default_allocate_URI = "v2/network/floatingip/%s/allocate/%s"
)

// retrieve ipinfo either from args or remote api of apiswitch
func Allocate(url, nodeIP, podName, podNamespace, allocateURI string) (*cniutil.IPInfo, error) {
	if allocateURI == "" {
		allocateURI = default_allocate_URI
	}
	client := httputils.NewDefaultClient()
	resp, err := client.Post(fmt.Sprintf("%s/%s", url, fmt.Sprintf(allocateURI, fmt.Sprintf("%s_%s", podName, podNamespace), nodeIP)), "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %v", err)
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(string(data))
	}
	ipInfo := new(cniutil.IPInfo)
	if err := json.Unmarshal(data, ipInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ipinfo %s: %v", string(data), err)
	}
	if len(ipInfo.Gateway) == 0 {
		return nil, fmt.Errorf("no enough floating ips")
	}
	return ipInfo, nil
}
