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

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	. "tkestack.io/galaxy/pkg/utils/test"
)

func TestResyncAppNotExist(t *testing.T) {
	pod1 := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	pod2 := CreateDeploymentPod("dp2-aaa-bbb", "ns2", immutableAnnotation)
	fipPlugin, stopChan, _ := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	pod1Key, _ := util.FormatKey(pod1)
	pod2Key, _ := util.FormatKey(pod2)

	if err := fipPlugin.ipam.AllocateSpecificIP(pod1Key.KeyInDB, net.ParseIP("10.49.27.205"),
		floatingip.Attr{Policy: parseReleasePolicy(&pod1.ObjectMeta)}); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.ipam.AllocateSpecificIP(pod2Key.KeyInDB, net.ParseIP("10.49.27.216"),
		floatingip.Attr{Policy: parseReleasePolicy(&pod2.ObjectMeta)}); err != nil {
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

func getResyncCases(createPod func(annotations map[string]string) *corev1.Pod,
	createApp func(podMeta v1.ObjectMeta, replicas int32) runtime.Object) []bindCase {
	createSuccessPod := func(annotations map[string]string) *corev1.Pod {
		pod := createPod(annotations)
		pod.Status.Phase = corev1.PodSucceeded
		return pod
	}
	createFailedPod := func(annotations map[string]string) *corev1.Pod {
		pod := createPod(annotations)
		pod.Status.Phase = corev1.PodFailed
		return pod
	}
	cases := []bindCase{
		{annotations: nil, replicas: 1, expectKeyFunc: podNameFunc, createPod: true},                            // pod exist, ip won't be released
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc},                                           // pod and app not exist, ip will be released
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: emptyNameFunc},                           // app not exist, ip will be released from immutable pod
		{annotations: immutableAnnotation, replicas: 1, expectKeyFunc: podNameFunc, createAppFunc: createApp},   // app exist, ip won't be released from immutable pod
		{annotations: immutableAnnotation, replicas: 0, expectKeyFunc: emptyNameFunc, createAppFunc: createApp}, // app exist, ip will be released from scaled down immutable pod
		{annotations: neverAnnotation, replicas: 0, expectKeyFunc: podNameFunc, createAppFunc: createApp},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc, createAppFunc: createApp},
		{annotations: neverAnnotation, replicas: 1, expectKeyFunc: podNameFunc},
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc, createPod: true, createPodFunc: createFailedPod},  // pod failed, ip will be released
		{annotations: nil, replicas: 1, expectKeyFunc: emptyNameFunc, createPod: true, createPodFunc: createSuccessPod}, // pod completed, ip will be released
	}
	for i := range cases {
		if cases[i].createPodFunc == nil {
			cases[i].createPodFunc = createPod
		}
	}
	return cases
}

func resyncTest(t *testing.T, c *bindCase) error {
	var objs, crObjs []runtime.Object
	pod = c.createPodFunc(c.annotations)
	keyObj, _ := util.FormatKey(pod)
	if c.createPod {
		objs = append(objs, pod)
	}
	if c.createAppFunc != nil {
		app := c.createAppFunc(pod.ObjectMeta, c.replicas)
		if sts, ok := app.(*appv1.StatefulSet); ok {
			objs = append(objs, sts)
		} else {
			crObjs = append(crObjs, app)
		}
	}
	fipPlugin, stopChan, _ := createPluginTestNodesWithCrdObjs(t, objs, []runtime.Object{NotScalableCrd, FooCrd}, crObjs)
	defer func() { stopChan <- struct{}{} }()
	if err := fipPlugin.ipam.AllocateSpecificIP(keyObj.KeyInDB, net.ParseIP("10.49.27.205"),
		floatingip.Attr{Policy: parseReleasePolicy(&pod.ObjectMeta)}); err != nil {
		return err
	}
	if err := fipPlugin.resyncPod(); err != nil {
		return err
	}
	return checkIPKey(fipPlugin.ipam, "10.49.27.205", c.expectKeyFunc(keyObj))
}

func TestResyncStsPod(t *testing.T) {
	createPod := func(annotations map[string]string) *corev1.Pod {
		return CreateStatefulSetPod("sts-xxx-0", "ns1", annotations)
	}
	createApp := func(podMeta v1.ObjectMeta, replicas int32) runtime.Object {
		return CreateStatefulSet(podMeta, replicas)
	}
	for i, testCase := range getResyncCases(createPod, createApp) {
		if err := resyncTest(t, &testCase); err != nil {
			t.Errorf("Case %d, err: %v", i, err)
		}
	}
}

func TestResyncCRDPod(t *testing.T) {
	createPod := func(annotations map[string]string) *corev1.Pod {
		return CreateCRDPod("crd-xxx-0", "ns1", annotations, FooCrd)
	}
	createApp := func(podMeta v1.ObjectMeta, replicas int32) runtime.Object {
		return CreateCRDApp(podMeta, int64(replicas), FooCrd)
	}
	for i, testCase := range getResyncCases(createPod, createApp) {
		if err := resyncTest(t, &testCase); err != nil {
			t.Errorf("Case %d, err: %v", i, err)
		}
	}
}

func TestResyncPodUidChanged(t *testing.T) {
	oldUid, newUid := "uid-1", "uid-2"
	pod := CreateStatefulSetPod("dp-xxx-0", "ns1", immutableAnnotation)
	pod.SetUID(types.UID(newUid))
	sts := CreateStatefulSet(pod.ObjectMeta, 1)
	sts.Spec.Template.Spec = pod.Spec
	ip := net.ParseIP("10.49.27.205")
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod, sts)
	defer func() { stopChan <- struct{}{} }()
	podKey, _ := util.FormatKey(pod)
	attr := floatingip.Attr{
		Policy: parseReleasePolicy(&pod.ObjectMeta), NodeName: "node-1", Uid: oldUid}
	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, ip, attr); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.resyncPod(); err != nil {
		t.Fatal(err)
	}
	fip, err := fipPlugin.ipam.ByIP(ip)
	if err != nil {
		t.Fatal(err)
	}
	if fip.Key != podKey.KeyInDB {
		t.Fatalf("expect key: %s, got %s", podKey.KeyInDB, fip.Key)
	}
	// pod uid changed, ip should be reserved, i.e. key should keep, but nodeName and podUid should be empty
	if fip.PodUid != "" {
		t.Fatal(fip.PodUid)
	}
	if fip.NodeName != "" {
		t.Fatal(fip.NodeName)
	}
}
