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
	"testing"

	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

var (
	poolPrefixFunc = func(obj *util.KeyObj) string {
		return obj.PoolPrefix()
	}
	podNameFunc = func(obj *util.KeyObj) string {
		return obj.KeyInDB
	}
	emptyNameFunc = func(obj *util.KeyObj) string {
		return ""
	}
)

func TestStsReleasePolicy(t *testing.T) {
	for i, testCase := range []struct {
		annotations   map[string]string
		replicas      int32
		expectKeyFunc func(obj *util.KeyObj) string
	}{
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc},
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: podNameFunc},
		{annotations: immutableAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc},
		{annotations: neverAnnotation, replicas: 0, expectKeyFunc: podNameFunc},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc},
	} {
		pod := CreateStatefulSetPod("dp-xxx-0", "ns1", testCase.annotations)
		keyObj, _ := util.FormatKey(pod)
		sts := CreateStatefulSet(pod.ObjectMeta, testCase.replicas)
		func() {
			fipPlugin, stopChan, _ := createPluginTestNodes(t, pod, sts)
			defer func() { stopChan <- struct{}{} }()
			fip, err := checkBind(fipPlugin, pod, node3, keyObj.KeyInDB, node3Subnet)
			if err != nil {
				t.Fatalf("case %d, err %v", i, err)
			}
			if err := fipPlugin.unbind(pod); err != nil {
				t.Fatalf("case %d, err %v", i, err)
			}
			if err := checkIPKey(fipPlugin.ipam, fip.FIP.IP.String(), testCase.expectKeyFunc(keyObj)); err != nil {
				t.Fatalf("case %d, err %v", i, err)
			}
		}()
	}
}
