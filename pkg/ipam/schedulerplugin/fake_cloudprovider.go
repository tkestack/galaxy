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
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
)

type fakeCloudProvider1 struct {
	m map[string]string // race test
}

func (f *fakeCloudProvider1) AssignIP(in *rpc.AssignIPRequest) (*rpc.AssignIPReply, error) {
	f.m["1"] = "a"
	glog.Infof(`f.m["1"] = "a"`)
	return &rpc.AssignIPReply{Success: true}, nil
}

func (f *fakeCloudProvider1) UnAssignIP(in *rpc.UnAssignIPRequest) (*rpc.UnAssignIPReply, error) {
	f.m["2"] = "b"
	glog.Infof(`f.m["2"] = "b"`)
	return &rpc.UnAssignIPReply{Success: true}, nil
}
