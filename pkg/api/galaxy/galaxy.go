/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package galaxy

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/cniutil"
	"tkestack.io/galaxy/pkg/api/k8s"
)

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
	Ports []k8s.Port
	// Channel for returning the operation result to the CNIServer
	Result chan *PodResult
	// Args
	*skel.CmdArgs
	// specific CNI plugin args, key: cni type, inner key: args name, value: args value
	ExtendedCNIArgs map[string]map[string]json.RawMessage
}

// Result of a PodRequest sent through the PodRequest's Result channel.
type PodResult struct {
	// Response to be returned to the OpenShift SDN CNI plugin on success
	Response []byte
	// Error to be returned to the OpenShift SDN CNI plugin on failure
	Err error
}

// #lizard forgives
func CniRequestToPodRequest(data []byte) (*PodRequest, error) {
	var cr CNIRequest
	if err := json.Unmarshal(data, &cr); err != nil {
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

	cniArgs, err := cniutil.ParseCNIArgs(req.Args)
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
	glog.V(4).Infof("req.Args %s req.StdinData %s", req.Args, cr.Config)

	return req, nil
}

func (req *PodRequest) String() string {
	return fmt.Sprintf("%s %s_%s, %s, %s, %v", req.Command, req.PodName, req.PodNamespace, req.ContainerID, req.Netns,
		req.Ports)
}
