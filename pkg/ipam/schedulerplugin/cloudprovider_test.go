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
	"errors"
	"net"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	schedulerplugin_util "tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

type fakeCloudProvider1 struct {
	proceedBind chan struct{}
}

func (f *fakeCloudProvider1) AssignIP(in *rpc.AssignIPRequest) (*rpc.AssignIPReply, error) {
	f.proceedBind <- struct{}{} // notify we are waiting in AssignIP now
	<-f.proceedBind             // sleep before we receive signal to continue again
	return &rpc.AssignIPReply{Success: true}, nil
}

func (f *fakeCloudProvider1) UnAssignIP(in *rpc.UnAssignIPRequest) (*rpc.UnAssignIPReply, error) {
	return &rpc.UnAssignIPReply{Success: true}, nil
}

func TestBindingAfterReceivingDeleteEvent(t *testing.T) {
	node := createNode("node1", nil, "10.49.27.2")
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	podKey := schedulerplugin_util.FormatKey(pod)
	dp1 := createDeployment("dp", "ns1", pod.ObjectMeta, 1)
	expectIP := "10.49.27.205"
	plugin, stopChan := createPlugin(t, pod, dp1, &node)
	defer func() { stopChan <- struct{}{} }()
	cloudProvider := &fakeCloudProvider1{proceedBind: make(chan struct{})}
	plugin.cloudProvider = cloudProvider
	go func() {
		// drain ips other than expectIP of this subnet
		if err := drainNode(plugin, node3Subnet, net.ParseIP(expectIP)); err != nil {
			t.Fatal(err)
		}
		// bind will hang on waiting event
		_, err := checkBind(plugin, pod, node.Name, podKey.KeyInDB, node3Subnet)
		if err == nil || !isPodNotFoundError(errors.Unwrap(err)) {
			t.Fatal(err)
		}
	}()
	<-cloudProvider.proceedBind
	// Before cloudProvider.AssignIP invoked allocating ip has already done, check ip allocated to pod
	if err := checkIPKey(plugin.ipam, expectIP, podKey.KeyInDB); err != nil {
		t.Fatal(err)
	}
	// before bind is done, we delete this pod
	if err := plugin.PluginFactoryArgs.Client.CoreV1().Pods(pod.Namespace).Delete(pod.Name, &v1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := waitForUnbind(plugin); err != nil {
		t.Fatal(err)
	}
	cloudProvider.proceedBind <- struct{}{}
	if err := waitForUnbind(plugin); err != nil {
		t.Fatal(err)
	}
	// key should be updated to pool prefix
	if err := checkIPKey(plugin.ipam, expectIP, podKey.PoolPrefix()); err != nil {
		t.Fatal(err)
	}
}

func waitForUnbind(plugin *FloatingIPPlugin) error {
	deleteEvent := <-plugin.unreleased
	return plugin.unbind(deleteEvent.pod)
}
