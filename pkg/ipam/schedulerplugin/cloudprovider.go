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
package schedulerplugin

import (
	"encoding/json"
	"fmt"

	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
)

// cloudProviderAssignIP send assign ip req to cloud provider
func (p *FloatingIPPlugin) cloudProviderAssignIP(req *rpc.AssignIPRequest) error {
	if p.cloudProvider == nil {
		return nil
	}
	reply, err := p.cloudProvider.AssignIP(req)
	if err != nil {
		return fmt.Errorf("cloud provider AssignIP reply err %v", err)
	}
	if reply == nil {
		return fmt.Errorf("cloud provider AssignIP nil reply")
	}
	if !reply.Success {
		return fmt.Errorf("cloud provider AssignIP reply failed, message %s", reply.Msg)
	}
	glog.Infof("AssignIP %v success", req)
	return nil
}

// cloudProviderUnAssignIP send unassign ip req to cloud provider
func (p *FloatingIPPlugin) cloudProviderUnAssignIP(req *rpc.UnAssignIPRequest) error {
	if p.cloudProvider == nil {
		return nil
	}
	reply, err := p.cloudProvider.UnAssignIP(req)
	if err != nil {
		return fmt.Errorf("cloud provider UnAssignIP reply err %v", err)
	}
	if reply == nil {
		return fmt.Errorf("cloud provider UnAssignIP nil reply")
	}
	if !reply.Success {
		return fmt.Errorf("cloud provider UnAssignIP reply failed, message %s", reply.Msg)
	}
	glog.Infof("UnAssignIP %v success", req)
	return nil
}

// resyncCloudProviderIPs resyncs assigned ips with cloud provider
func (p *FloatingIPPlugin) resyncCloudProviderIPs(ipam floatingip.IPAM, meta *resyncMeta) {
	for key, obj := range meta.assignedPods {
		if _, ok := meta.existPods[key]; ok {
			continue
		}
		// check with apiserver to confirm it really not exist
		if p.podExist(obj.keyObj.PodName, obj.keyObj.Namespace) {
			continue
		}
		var attr Attr
		if err := json.Unmarshal([]byte(obj.fip.Attr), &attr); err != nil {
			glog.Errorf("failed to unmarshal attr %s for pod %s: %v", obj.fip.Attr, key, err)
			continue
		}
		if attr.NodeName == "" {
			glog.Errorf("empty nodeName for %s in db", key)
			continue
		}
		glog.Infof("UnAssignIP nodeName %s, ip %s, key %s during resync", attr.NodeName,
			obj.fip.IP.String(), key)
		if err := p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
			NodeName:  attr.NodeName,
			IPAddress: obj.fip.IP.String(),
		}); err != nil {
			// delete this record from allocatedIPs map to have a retry
			delete(meta.allocatedIPs, key)
			glog.Warningf("failed to unassign ip %s to %s: %v", obj.fip.IP.String(), key, err)
			continue
		}
		// for tapp and sts pod, we need to clean its node attr
		if err := ipam.ReserveIP(key, key, getAttr("")); err != nil {
			glog.Errorf("failed to reserve %s ip: %v", key, err)
		}
	}
}
