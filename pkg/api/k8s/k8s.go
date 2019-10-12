package k8s

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	glog "k8s.io/klog"
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
	defer os.Remove(path) // nolint: errcheck

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

var flagHostnameOverride = flag.String("hostname-override", "", "kubelet hostname override, if set, galaxy use this as node name to get node from apiserver")

// copied from kubelet
// GetHostname returns OS's hostname if 'hostnameOverride' is empty; otherwise, return 'hostnameOverride'.
func GetHostname() string {
	hostname := *flagHostnameOverride
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

//such as struct NetworkSelectionElement, function ParsePodNetworkAnnotation &  parsePodNetworkObjectName all written in compatible with multus-cni
//reference to https://github.com/intel/multus-cni/blob/master/k8sclient/k8sclient.go

// NetworkSelectionElement represents one element of the JSON format
// Network Attachment Selection Annotation as described in section 4.1.2
// of the CRD specification.
type NetworkSelectionElement struct {
	// Name contains the name of the Network object this element selects
	Name string `json:"name"`
	// Namespace contains the optional namespace that the network referenced
	// by Name exists in
	Namespace string `json:"namespace,omitempty"`
	// IPRequest contains an optional requested IP address for this network
	// attachment
	IPRequest string `json:"ips,omitempty"`
	// MacRequest contains an optional requested MAC address for this
	// network attachment
	MacRequest string `json:"mac,omitempty"`
	// InterfaceRequest contains an optional requested name for the
	// network interface this attachment will create in the container
	InterfaceRequest string `json:"interface,omitempty"`
}

func ParsePodNetworkAnnotation(podNetworks string) ([]*NetworkSelectionElement, error) {
	var networks []*NetworkSelectionElement
	if podNetworks == "" {
		return nil, fmt.Errorf("parsePodNetworkAnnotation: pod annotation should be written as <namespace>/<network name>@<ifname>")
	}

	//In multus-cni, network annotation written as <namespace>/<network name>@<ifname>
	//Actually, namespace in annotation will be ignored in parsing
	if strings.IndexAny(podNetworks, "[{\"") >= 0 {
		if err := json.Unmarshal([]byte(podNetworks), &networks); err != nil {
			return nil, fmt.Errorf("parsePodNetworkAnnotation: failed to parse pod Network Attachment Selection Annotation JSON format: %v", err)
		}
	} else {
		// Comma-delimited list of network attachment object names
		for _, item := range strings.Split(podNetworks, ",") {
			// Remove leading and trailing whitespace.
			item = strings.TrimSpace(item)

			// Parse network name (i.e. <namespace>/<network name>@<ifname>)
			_, networkName, netIfName, err := parsePodNetworkObjectName(item)
			if err != nil {
				return nil, fmt.Errorf("parsePodNetworkAnnotation: %v", err)
			}
			networks = append(networks, &NetworkSelectionElement{
				Name:             networkName,
				InterfaceRequest: netIfName,
			})
		}
	}

	return networks, nil
}

func parsePodNetworkObjectName(podNetwork string) (string, string, string, error) {
	var netNsName string
	var netIfName string
	var networkName string

	glog.V(5).Infof("parsePodNetworkObjectName: %s", podNetwork)
	slashItems := strings.Split(podNetwork, "/")
	if len(slashItems) == 2 {
		netNsName = strings.TrimSpace(slashItems[0])
		networkName = slashItems[1]
	} else if len(slashItems) == 1 {
		networkName = slashItems[0]
	} else {
		return "", "", "", fmt.Errorf("Invalid network object %s (failed at '/') ", podNetwork)
	}

	atItems := strings.Split(networkName, "@")
	networkName = strings.TrimSpace(atItems[0])
	if len(atItems) == 2 {
		netIfName = strings.TrimSpace(atItems[1])
	} else if len(atItems) != 1 {
		return "", "", "", fmt.Errorf("Invalid network object (failed at '@') ")
	}

	// Check and see if each item matches the specification for valid attachment name.
	// "Valid attachment names must be comprised of units of the DNS-1123 label format"
	// [a-z0-9]([-a-z0-9]*[a-z0-9])?
	// And we allow at (@), and forward slash (/) (units separated by commas)
	// It must start and end alphanumerically.
	allItems := []string{netNsName, networkName, netIfName}
	for i := range allItems {
		matched, _ := regexp.MatchString("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", allItems[i])
		if !matched && len([]rune(allItems[i])) > 0 {
			return "", "", "", fmt.Errorf("Failed to parse: one or more items did not match comma-delimited format (must consist of lower case alphanumeric characters). Must start and end with an alphanumeric character), mismatch @ '%v' ", allItems[i])
		}
	}

	glog.V(5).Infof("parsePodNetworkObjectName: parsed: %s, %s, %s", netNsName, networkName, netIfName)
	return netNsName, networkName, netIfName, nil
}
