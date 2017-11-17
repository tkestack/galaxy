package galaxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"github.com/containernetworking/cni/pkg/skel"
)

const GalaxySocketPath string = "/var/run/galaxy.sock"

// Request sent to the Galaxy by the Galaxy SDN CNI plugin
type CNIRequest struct {
	// CNI environment variables, like CNI_COMMAND and CNI_NETNS
	Env map[string]string `json:"env,omitempty"`
	// CNI configuration passed via stdin to the CNI plugin
	Config []byte `json:"config,omitempty"`
}

// Request structure built from CNIRequest which is passed to the
// handler function given to the CNIServer at creation time
type PodRequest struct {
	// The CNI command of the operation
	Command string
	// kubernetes namespace name
	PodNamespace string
	// kubernetes pod name
	PodName string
	// kubernetes pod ports
	Ports string
	// Channel for returning the operation result to the CNIServer
	Result chan *PodResult
	// Args
	*skel.CmdArgs
}

// Result of a PodRequest sent through the PodRequest's Result channel.
type PodResult struct {
	// Response to be returned to the OpenShift SDN CNI plugin on success
	Response []byte
	// Error to be returned to the OpenShift SDN CNI plugin on failure
	Err error
}

func CniRequestToPodRequest(r *http.Request) (*PodRequest, error) {
	var cr CNIRequest
	b, _ := ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, fmt.Errorf("JSON unmarshal error: %v", err)
	}

	cmd, ok := cr.Env[cniutil.CNI_COMMAND]
	if !ok {
		return nil, fmt.Errorf("Unexpected or missing %s", cniutil.CNI_COMMAND)
	}

	req := &PodRequest{
		Command: cmd,
		Result:  make(chan *PodResult),
		CmdArgs: &skel.CmdArgs{
			StdinData: cr.Config,
		},
	}

	req.ContainerID, ok = cr.Env[cniutil.CNI_CONTAINERID]
	if !ok {
		return nil, fmt.Errorf("missing %s", cniutil.CNI_CONTAINERID)
	}
	req.Netns, ok = cr.Env[cniutil.CNI_NETNS]
	if !ok {
		return nil, fmt.Errorf("missing %s", cniutil.CNI_NETNS)
	}
	req.IfName, ok = cr.Env[cniutil.CNI_IFNAME]
	if !ok {
		return nil, fmt.Errorf("missing %s", cniutil.CNI_IFNAME)
	}
	req.Path, ok = cr.Env[cniutil.CNI_PATH]
	if !ok {
		return nil, fmt.Errorf("missing %s", cniutil.CNI_PATH)
	}
	req.Args, ok = cr.Env[cniutil.CNI_ARGS]
	if !ok {
		return nil, fmt.Errorf("missing %s", cniutil.CNI_ARGS)
	}

	cniArgs, err := k8s.ParseK8SCNIArgs(req.Args)
	if err != nil {
		return nil, err
	}

	req.PodNamespace, ok = cniArgs[k8s.K8S_POD_NAMESPACE]
	if !ok {
		return nil, fmt.Errorf("missing %s", k8s.K8S_POD_NAMESPACE)
	}

	req.PodName, ok = cniArgs[k8s.K8S_POD_NAME]
	if !ok {
		return nil, fmt.Errorf("missing %s", k8s.K8S_POD_NAME)
	}
	req.Ports = cniArgs[k8s.K8S_PORTS]

	return req, nil
}

func (req *PodRequest) String() string {
	return fmt.Sprintf("%s %s_%s, %s, %s, %s", req.Command, req.PodName, req.PodNamespace, req.ContainerID, req.Netns, req.Ports)
}
