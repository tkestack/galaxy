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
	"fmt"

	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
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
