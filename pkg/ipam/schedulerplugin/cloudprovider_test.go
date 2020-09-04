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
	"net"
	"sync"
	"testing"

	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	schedulerplugin_util "tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func TestConcurrentBindUnbind(t *testing.T) {
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	podKey, _ := schedulerplugin_util.FormatKey(pod)
	dp1 := createDeployment(pod.ObjectMeta, 1)
	plugin, stopChan, _ := createPluginTestNodes(t, pod, dp1)
	defer func() { stopChan <- struct{}{} }()
	cloudProvider := &fakeCloudProvider1{m: make(map[string]string)}
	plugin.cloudProvider = cloudProvider
	if err := plugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.49.27.216"),
		floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plugin.unbind(pod); err != nil {
			t.Fatal(err)
		}
	}()
	if err := plugin.Bind(&schedulerapi.ExtenderBindingArgs{
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		Node:         node3,
	}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
