package k8s

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
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

	stateDir                   = "/var/lib/cni/galaxy/port"
	PortMappingAnnotation      = "network.kubernetes.io/portmapping"
	PortMappingPortsAnnotation = "network.kubernetes.io/portmappingports"
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
		kvMap[strings.TrimSpace(part[0])] = strings.TrimSpace(part[1])
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
	// This must be a valid port number, 0 <= x < 65536.
	// If HostNetwork is specified, this must match ContainerPort.
	HostPort int32 `json:"hostPort"`
	// Required: This must be a valid port number, 0 < x < 65536.
	ContainerPort int32 `json:"containerPort"`
	// Required: Supports "TCP" and "UDP".
	Protocol string `json:"protocol"`

	HostIP string `json:"hostIP,omitempty"`

	PodName string `json:"podName"`

	PodIP string `json:"podIP"`
}

func SavePort(containerID string, data []byte) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, containerID)
	return ioutil.WriteFile(path, data, 0600)
}

func ConsumePort(containerID string) ([]Port, error) {
	path := filepath.Join(stateDir, containerID)
	defer os.Remove(path)

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var ports []Port
	if err := json.Unmarshal(data, &ports); err != nil {
		return nil, err
	}
	return ports, nil
}

// GetPodFullName returns a name that uniquely identifies a pod.
func GetPodFullName(podName, namespace string) string {
	return podName + "_" + namespace
}

// copied from kubelet
// GetHostname returns OS's hostname if 'hostnameOverride' is empty; otherwise, return 'hostnameOverride'.
func GetHostname(hostnameOverride string) string {
	hostname := hostnameOverride
	if hostname == "" {
		nodename, err := os.Hostname()
		if err != nil {
			glog.Fatalf("Couldn't determine hostname: %v", err)
		}
		hostname = nodename
	}
	return strings.ToLower(strings.TrimSpace(hostname))
}

type PortMapConf struct {
	RuntimeConfig struct {
		PortMaps []Port `json:"portMappings,omitempty"`
	} `json:"runtimeConfig,omitempty"`
}
