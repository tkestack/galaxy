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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func TestResyncAppNotExist(t *testing.T) {
	pod1 := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	pod2 := CreateDeploymentPod("dp2-aaa-bbb", "ns2", immutableAnnotation)
	fipPlugin, stopChan, _ := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	pod1Key, _ := util.FormatKey(pod1)
	pod2Key, _ := util.FormatKey(pod2)

	if err := fipPlugin.ipam.AllocateSpecificIP(pod1Key.KeyInDB, net.ParseIP("10.49.27.205"), parseReleasePolicy(&pod1.ObjectMeta), ""); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.ipam.AllocateSpecificIP(pod2Key.KeyInDB, net.ParseIP("10.49.27.216"), parseReleasePolicy(&pod2.ObjectMeta), ""); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.resyncPod(); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.49.27.205", pod1Key.PoolPrefix()); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.49.27.216", ""); err != nil {
		t.Fatal(err)
	}
}

func TestResyncStsPod(t *testing.T) {
	for i, testCase := range []struct {
		annotations   map[string]string
		replicas      int32
		createPod     bool
		updateStatus  func(*corev1.Pod)
		createApp     bool
		expectKeyFunc func(obj *util.KeyObj) string
	}{
		{annotations: nil, replicas: 1, expectKeyFunc: podNameFunc, createPod: true},                    // pod exist, ip won't be released
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc},                                   // pod and app not exist, ip will be released
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: emptyNameFunc, createApp: false}, // app not exist, ip will be released from immutable pod
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: podNameFunc, createApp: true},    // app exist, ip won't be released from immutable pod
		{annotations: immutableAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc, createApp: true},  // app exist, ip will be released from scaled down immutable pod
		{annotations: neverAnnotation, replicas: 0, expectKeyFunc: podNameFunc, createApp: true},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc, createApp: true},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc, createApp: false},
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc, createPod: true, updateStatus: toFailedPod},  // pod failed, ip will be released
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc, createPod: true, updateStatus: toSuccessPod}, // pod completed, ip will be released
	} {
		var objs []runtime.Object
		pod := CreateStatefulSetPod("sts-xxx-0", "ns1", testCase.annotations)
		if testCase.updateStatus != nil {
			testCase.updateStatus(pod)
		}
		keyObj, _ := util.FormatKey(pod)
		if testCase.createPod {
			objs = append(objs, pod)
		}
		if testCase.createApp {
			sts := CreateStatefulSet(pod.ObjectMeta, testCase.replicas)
			sts.Spec.Template.Spec = pod.Spec
			objs = append(objs, sts)
		}
		func() {
			fipPlugin, stopChan, _ := createPluginTestNodes(t, objs...)
			defer func() { stopChan <- struct{}{} }()
			if err := fipPlugin.ipam.AllocateSpecificIP(keyObj.KeyInDB, net.ParseIP("10.49.27.205"), parseReleasePolicy(&pod.ObjectMeta), ""); err != nil {
				t.Fatalf("case %d, err %v", i, err)
			}
			if err := fipPlugin.resyncPod(); err != nil {
				t.Fatalf("case %d, err %v", i, err)
			}
			if err := checkIPKey(fipPlugin.ipam, "10.49.27.205", testCase.expectKeyFunc(keyObj)); err != nil {
				t.Fatalf("case %d, err %v", i, err)
			}
		}()
	}
}
