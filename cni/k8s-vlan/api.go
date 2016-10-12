package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/types"
)

var (
	client = &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{},
	}
)

/*
	{"ip":"10.49.27.205/24","vlan":2,"gateway":"10.49.27.1"}
 */
type IPInfo struct {
	IP      types.IPNet `json:"ip"`
	Vlan    uint16      `json:"vlan"`
	Gateway net.IP      `json:"gateway"`
}

/*
k8s cni args
Args: [][2]string{
	{"IgnoreUnknown", "1"},
	{"K8S_POD_NAMESPACE", podNs},
	{"K8S_POD_NAME", podName},
	{"K8S_POD_INFRA_CONTAINER_ID", podInfraContainerID.ID},
}
*/
const (
	K8S_POD_NAMESPACE          = "K8S_POD_NAMESPACE"
	K8S_POD_NAME               = "K8S_POD_NAME"
	K8S_POD_INFRA_CONTAINER_ID = "K8S_POD_INFRA_CONTAINER_ID"
)

func parseArgs(args string) (map[string]string, error) {
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
		kvMap[part[0]] = part[1]
	}
	if _, ok := kvMap[K8S_POD_NAME]; !ok {
		return kvMap, fmt.Errorf("invalid args %s", args)
	}
	return kvMap, nil
}

func retrieveResult(conf *IPAMConf, kvMap map[string]string) (*types.Result, uint16, error) {
	var ipInfo IPInfo
	maxRetry := 5
	for i := 0; i < maxRetry; i++ {
		resp, err := client.Post(fmt.Sprintf("%s/%s", conf.URL, strings.Replace(conf.QueryURI, "{podName}", kvMap[K8S_POD_NAME], 1)), "application/json", nil)
		if err == nil {
			defer resp.Body.Close()
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, 0, err
			}
			if err := json.Unmarshal(data, &ipInfo); err != nil {
				return nil, 0, err
			}
			break
		} else {
			if i == maxRetry-1 {
				return nil, 0, fmt.Errorf("retried %d times: %v", maxRetry, err)
			}
		}
	}

	return IPInfoToResult(&ipInfo), ipInfo.Vlan, nil
}

func IPInfoToResult(ipInfo *IPInfo) *types.Result {
	return &types.Result{
		IP4: &types.IPConfig{
			IP:      net.IPNet(ipInfo.IP),
			Gateway: ipInfo.Gateway,
			Routes:  []types.Route{types.Route{
				Dst: net.IPNet{
					IP:   net.IPv4(0, 0, 0, 0),
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
			}},
		},
	}
}
