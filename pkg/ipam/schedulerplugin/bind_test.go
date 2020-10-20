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
	"net"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	fakeV1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	. "tkestack.io/galaxy/pkg/ipam/cloudprovider/testing"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	schedulerplugin_util "tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func TestBind(t *testing.T) {
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod)
	defer func() { stopChan <- struct{}{} }()
	fipInfo, err := checkBind(fipPlugin, pod, node3, podKey.KeyInDB, node3Subnet)
	if err != nil {
		t.Fatalf("checkBind error %v", err)
	}
	fakePods := fipPlugin.PluginFactoryArgs.Client.CoreV1().Pods(pod.Namespace).(*fakeV1.FakePods)

	actualBinding, err := fakePods.GetBinding(pod.GetName())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
		return
	}
	str, err := constant.MarshalCniArgs([]constant.IPInfo{fipInfo.IPInfo})
	if err != nil {
		t.Fatal(err)
	}
	expect := &corev1.Binding{
		ObjectMeta: v1.ObjectMeta{
			Namespace: pod.Namespace, Name: pod.Name,
			Annotations: map[string]string{
				constant.ExtendedCNIArgsAnnotation: str}},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: node3,
		},
	}
	if !reflect.DeepEqual(expect, actualBinding) {
		t.Fatalf("Binding did not match expectation, expect %v, actual %v", expect, actualBinding)
	}
}

func TestAllocateIP(t *testing.T) {
	fipPlugin, stopChan, _ := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()

	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.173.13.2"),
		floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
		t.Fatal(err)
	}
	// check update from ReleasePolicyPodDelete to ReleasePolicyImmutable
	pod.Spec.NodeName = node4
	pod.SetUID("pod-xx-1")
	cniArgs, err := fipPlugin.allocateIP(podKey.KeyInDB, pod.Spec.NodeName, pod)
	if err != nil || len(cniArgs.Common.IPInfos) != 1 {
		t.Fatal(err)
	}
	if cniArgs.Common.IPInfos[0].IP.String() != "10.173.13.2/24" {
		t.Fatal(cniArgs.Common.IPInfos[0])
	}
	fip, err := fipPlugin.ipam.First(podKey.KeyInDB)
	if err != nil {
		t.Fatal(err)
	}
	if fip.FIP.Policy != uint16(constant.ReleasePolicyImmutable) {
		t.Fatal(fip.FIP.Policy)
	}
	if fip.FIP.NodeName != node4 {
		t.Fatal(fip.FIP.NodeName)
	}
	if fip.FIP.PodUid != string(pod.UID) {
		t.Fatal(fip.FIP.PodUid)
	}
}

// #lizard forgives
func TestAllocateRecentIPs(t *testing.T) {
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	dp := CreateDeployment(pod.ObjectMeta, 1)
	fipPlugin, stopChan, nodes := createPluginTestNodes(t, pod, dp)
	defer func() { stopChan <- struct{}{} }()
	podKey, _ := schedulerplugin_util.FormatKey(pod)
	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.PoolPrefix(), net.ParseIP("10.49.27.205"),
		floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
		t.Fatal(err)
	}
	// update time of 10.49.27.216 is more recently than 10.49.27.205
	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.PoolPrefix(), net.ParseIP("10.49.27.216"),
		floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
		t.Fatal(err)
	}
	// check filter allocates recent ips for deployment pod from ip pool
	if err := checkFilterCase(fipPlugin, filterCase{
		testPod: pod, expectFiltererd: []string{node3}, expectFailed: []string{drainedNode, nodeHasNoIP, node4},
	}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.49.27.205", podKey.PoolPrefix()); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.49.27.216", podKey.KeyInDB); err != nil {
		t.Fatal(err)
	}
}

// #lizard forgives
func TestUnBind(t *testing.T) {
	pod1 := CreateStatefulSetPod("pod1-1", "demo", map[string]string{})
	keyObj, _ := schedulerplugin_util.FormatKey(pod1)
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod1)
	defer func() { stopChan <- struct{}{} }()
	fipPlugin.cloudProvider = &FakeCloudProvider{}
	// if a pod has not got cni args annotation, unbind should return nil
	if err := fipPlugin.unbind(pod1); err != nil {
		t.Fatal(err)
	}
	// if a pod has got bad cni args annotation,
	// unbind should return nil because we got binded ip from store instead of annotation
	pod1.Annotations[constant.ExtendedCNIArgsAnnotation] = "fff"
	if err := fipPlugin.unbind(pod1); err != nil {
		t.Fatal(err)
	}
	// bind before testing normal unbind
	expectIP := net.ParseIP("10.49.27.205")
	if err := drainNode(fipPlugin, node3Subnet, expectIP); err != nil {
		t.Fatal(err)
	}
	fakeCP := &FakeCloudProvider{ExpectIP: expectIP.String(), ExpectNode: node3}
	fipPlugin.cloudProvider = fakeCP
	fipInfo, err := checkBind(fipPlugin, pod1, node3, keyObj.KeyInDB, node3Subnet)
	if err != nil {
		t.Fatal(err)
	}
	str, err := constant.MarshalCniArgs([]constant.IPInfo{fipInfo.IPInfo})
	if err != nil {
		t.Fatal(err)
	}
	pod1.Annotations[constant.ExtendedCNIArgsAnnotation] = str
	pod1.Spec.NodeName = node3
	if err := fipPlugin.unbind(pod1); err != nil {
		t.Fatal(err)
	}
	if !fakeCP.InvokedAssignIP || !fakeCP.InvokedUnAssignIP {
		t.Fatal()
	}
}

func TestUnBindImmutablePod(t *testing.T) {
	pod = CreateStatefulSetPodWithLabels("sts1-0", "ns1", map[string]string{"app": "sts1"}, immutableAnnotation)
	podKey, _ = schedulerplugin_util.FormatKey(pod)
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod, CreateStatefulSet(pod.ObjectMeta, 1))
	defer func() { stopChan <- struct{}{} }()
	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.173.13.2"),
		floatingip.Attr{Policy: constant.ReleasePolicyImmutable}); err != nil {
		t.Fatal(err)
	}
	// unbind the pod, check ip should be reserved, because pod has is immutable
	if err := fipPlugin.unbind(pod); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", podKey.KeyInDB); err != nil {
		t.Fatal(err)
	}
}

func checkBind(fipPlugin *FloatingIPPlugin, pod *corev1.Pod, nodeName, checkKey string,
	expectSubnet *net.IPNet) (*floatingip.FloatingIPInfo, error) {
	if err := fipPlugin.Bind(&schedulerapi.ExtenderBindingArgs{
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		Node:         nodeName,
	}); err != nil {
		return nil, err
	}
	fipInfo, err := fipPlugin.ipam.First(checkKey)
	if err != nil {
		return nil, err
	}
	if fipInfo == nil {
		return nil, fmt.Errorf("got nil ipInfo")
	}
	if !expectSubnet.Contains(fipInfo.IPInfo.IP.IP) {
		return nil, fmt.Errorf("allocated ip %s is not in expect subnet %s", fipInfo.IPInfo.IP.IP.String(),
			expectSubnet.String())
	}
	return fipInfo, nil
}

func TestReleaseIPOfFinishedPod(t *testing.T) {
	for i, testCase := range []struct {
		updatePodStatus func(pod *corev1.Pod)
	}{
		{updatePodStatus: toFailedPod},
		{updatePodStatus: toSuccessPod},
	} {
		pod := CreateStatefulSetPod("pod1-0", "ns1", nil)
		podKey, _ := schedulerplugin_util.FormatKey(pod)
		func() {
			fipPlugin, stopChan, _ := createPluginTestNodes(t, pod)
			fipPlugin.Run(stopChan)
			defer func() { stopChan <- struct{}{} }()
			fipInfo, err := checkBind(fipPlugin, pod, node3, podKey.KeyInDB, node3Subnet)
			if err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
			testCase.updatePodStatus(pod)
			if _, err := fipPlugin.Client.CoreV1().Pods(pod.Namespace).UpdateStatus(pod); err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
			if err := wait.Poll(time.Microsecond*10, time.Second*30, func() (done bool, err error) {
				return checkIPKey(fipPlugin.ipam, fipInfo.FIP.IP.String(), "") == nil, nil
			}); err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
		}()
	}
}
