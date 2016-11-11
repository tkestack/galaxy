package k8s

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
	K8S_PORTS                  = "K8S_PORTS"
)

func ParseK8SCNIArgs(args string) (map[string]string, error) {
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
		return kvMap, fmt.Errorf("invalid args, k8s_pod_name is unknown: %s", args)
	}
	return kvMap, nil
}

func ParsePorts(portStr string) ([]*Port, error) {
	if portStr == "" {
		return nil, nil
	}
	var ports []*Port
	if err := json.Unmarshal([]byte(portStr), &ports); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ports %s: %v", portStr, err)
	}
	return ports, nil
}

type Port struct {
	// This must be a valid port number, 0 < x < 65536.
	// If HostNetwork is specified, this must match ContainerPort.
	HostPort int32 `json:"hostPort"`
	// Required: This must be a valid port number, 0 < x < 65536.
	ContainerPort int32 `json:"containerPort"`
	// Required: Supports "TCP" and "UDP".
	Protocol string `json:"protocol"`

	PodName string `json:"podName"`

	PodIP string `json:"podIP"`
}
