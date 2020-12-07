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

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	. "tkestack.io/galaxy/pkg/utils/test"
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

type bindCase struct {
	annotations   map[string]string
	replicas      int32
	createPodFunc func(annotations map[string]string) *corev1.Pod
	createAppFunc func(podMeta v1.ObjectMeta, replicas int32) runtime.Object
	expectKeyFunc func(obj *util.KeyObj) string
	// for resync cases
	createPod bool
}

var testCases = []bindCase{
	{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc},
	{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: podNameFunc},
	{annotations: immutableAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc},
	{annotations: neverAnnotation, replicas: 0, expectKeyFunc: podNameFunc},
	{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc},
}

func bindTest(t *testing.T, c *bindCase) error {
	pod := c.createPodFunc(c.annotations)
	keyObj, _ := util.FormatKey(pod)
	app := c.createAppFunc(pod.ObjectMeta, c.replicas)
	var fipPlugin *FloatingIPPlugin
	var stopChan chan struct{}
	if sts, ok := app.(*appv1.StatefulSet); ok {
		fipPlugin, stopChan, _ = createPluginTestNodes(t, pod, sts)
	} else {
		fipPlugin, stopChan, _ = createPluginTestNodesWithCrdObjs(t, []runtime.Object{pod},
			[]runtime.Object{FooCrd, NotScalableCrd}, []runtime.Object{app})
	}
	defer func() { stopChan <- struct{}{} }()
	fip, err := checkBind(fipPlugin, pod, node3, keyObj.KeyInDB, node3Subnet)
	if err != nil {
		return err
	}
	if err := fipPlugin.unbind(pod); err != nil {
		return err
	}
	return checkIPKey(fipPlugin.ipam, fip.IP.String(), c.expectKeyFunc(keyObj))
}

func TestStsReleasePolicy(t *testing.T) {
	for i, testCase := range testCases {
		testCase.createPodFunc = func(annotations map[string]string) *corev1.Pod {
			return CreateStatefulSetPod("sts-xxx-0", "ns1", annotations)
		}
		testCase.createAppFunc = func(podMeta v1.ObjectMeta, replicas int32) runtime.Object {
			return CreateStatefulSet(podMeta, replicas)
		}
		if err := bindTest(t, &testCase); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestReleasePolicyForScalableCrd(t *testing.T) {
	for i, testCase := range testCases {
		testCase.createPodFunc = func(annotations map[string]string) *corev1.Pod {
			return CreateCRDPod("crd-xxx-0", "ns1", annotations, FooCrd)
		}
		testCase.createAppFunc = func(podMeta v1.ObjectMeta, replicas int32) runtime.Object {
			return CreateCRDApp(podMeta, int64(replicas), FooCrd)
		}
		if err := bindTest(t, &testCase); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestReleasePolicyForNotScalableCrd(t *testing.T) {
	// For not scalable crd app, immutable policy is not supported, we'll release ips
	// For not scalable crd app with pod name matching '.*-[0-9]*$', we'll reserver ips
	createPodFunc1 := func(annotations map[string]string) *corev1.Pod {
		// name matches '.*-[0-9]*$'
		return CreateCRDPod("crd-xxx-0", "ns1", annotations, NotScalableCrd)
	}
	createPodFunc2 := func(annotations map[string]string) *corev1.Pod {
		// name not matches '.*-[0-9]*$'
		return CreateCRDPod("crd-xxx-xx1", "ns1", annotations, NotScalableCrd)
	}
	createAppFunc := func(podMeta v1.ObjectMeta, replicas int32) runtime.Object {
		return CreateCRDApp(podMeta, int64(replicas), NotScalableCrd)
	}
	for i, testCase := range []bindCase{
		// pod name matches '.*-[0-9]*$'
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc1},
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc1},
		{annotations: immutableAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc1},
		{annotations: neverAnnotation, replicas: 0, expectKeyFunc: podNameFunc, createPodFunc: createPodFunc1},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc, createPodFunc: createPodFunc1},
		// pod name not matches '.*-[0-9]*$'
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc1},
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc2},
		{annotations: immutableAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc2},
		{annotations: neverAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc2},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: emptyNameFunc, createPodFunc: createPodFunc2},
	} {
		testCase.createAppFunc = createAppFunc
		if err := bindTest(t, &testCase); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}
