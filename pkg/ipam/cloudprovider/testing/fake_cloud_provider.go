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
package testing

import (
	"fmt"

	"tkestack.io/galaxy/pkg/ipam/cloudprovider"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
)

var _ cloudprovider.CloudProvider = &FakeCloudProvider{}

// FakeCloudProvider is a fake cloud provider for testing
type FakeCloudProvider struct {
	Assigned   map[string]string // Assigned ipaddress to nodeName
	UnAssigned map[string]string // UnAssigned ipaddress to nodeName
}

func NewFakeCloudProvider() *FakeCloudProvider {
	return &FakeCloudProvider{Assigned: map[string]string{}, UnAssigned: map[string]string{}}
}

func (f *FakeCloudProvider) AssignIP(in *rpc.AssignIPRequest) (*rpc.AssignIPReply, error) {
	if in == nil {
		return nil, fmt.Errorf("nil request")
	}
	f.Assigned[in.IPAddress] = in.NodeName
	return &rpc.AssignIPReply{Success: true}, nil
}

func (f *FakeCloudProvider) UnAssignIP(in *rpc.UnAssignIPRequest) (*rpc.UnAssignIPReply, error) {
	if in == nil {
		return nil, fmt.Errorf("nil request")
	}
	f.UnAssigned[in.IPAddress] = in.NodeName
	return &rpc.UnAssignIPReply{Success: true}, nil
}
