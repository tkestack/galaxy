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
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	fakeV1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	. "tkestack.io/galaxy/pkg/ipam/cloudprovider/testing"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	schedulerplugin_util "tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/nets"
)

func TestBind(t *testing.T) {
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod)
	defer func() { stopChan <- struct{}{} }()
	fipInfo, err := checkBind(fipPlugin, pod, node3, podKey.KeyInDB, node3Subnet)
	if err != nil {
		t.Fatalf("checkBind error %v", err)
	}
	if err := checkBinding(fipPlugin, pod, &constant.CniArgs{Common: constant.CommonCniArgs{
		IPInfos: []constant.IPInfo{fipInfo.IPInfo},
	}}, node3); err != nil {
		t.Fatal(err)
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
	if fip.Policy != uint16(constant.ReleasePolicyImmutable) {
		t.Fatal(fip.Policy)
	}
	if fip.NodeName != node4 {
		t.Fatal(fip.NodeName)
	}
	if fip.PodUid != string(pod.UID) {
		t.Fatal(fip.PodUid)
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
	fakeCP := NewFakeCloudProvider()
	fipPlugin.cloudProvider = fakeCP
	fipInfo, err := checkBind(fipPlugin, pod1, node3, keyObj.KeyInDB, node3Subnet)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFakeCloudProviderState(fakeCP, map[string]string{"10.49.27.205": node3},
		map[string]string{}); err != nil {
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
	if err := checkFakeCloudProviderState(fakeCP, map[string]string{"10.49.27.205": node3},
		map[string]string{"10.49.27.205": node3}); err != nil {
		t.Fatal(err)
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
				return checkIPKey(fipPlugin.ipam, fipInfo.IP.String(), "") == nil, nil
			}); err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
		}()
	}
}

func TestBindUnBindRequestIPRange(t *testing.T) {
	// create a pod request for two ips
	request := `{"request_ip_range":[["10.49.27.205"],["10.49.27.217"]]}`
	cniArgs, err := constant.UnmarshalCniArgs(request)
	if err != nil {
		t.Fatal(err)
	}
	pod := CreateStatefulSetPod("pod1-0", "ns1", cniArgsAnnotation(request))
	podKey, _ := schedulerplugin_util.FormatKey(pod)
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod)
	cp := NewFakeCloudProvider()
	fipPlugin.cloudProvider = cp
	defer func() { stopChan <- struct{}{} }()
	if err := checkBindForIPRanges(fipPlugin, pod, node3, cniArgs, "10.49.27.205", "10.49.27.217"); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.unbind(pod); err != nil {
		t.Fatal(err)
	}
	if _, err := checkByKeyAndIPRanges(fipPlugin, podKey.KeyInDB, cniArgs.RequestIPRange); err != nil {
		t.Fatal(err)
	}
	expectAssigned := map[string]string{"10.49.27.205": node3, "10.49.27.217": node3}
	// both assigned ips should be unassigned
	if err := checkFakeCloudProviderState(cp, expectAssigned, expectAssigned); err != nil {
		t.Fatal(err)
	}
}

func TestFilterBindRequestIPRange(t *testing.T) {
	// test for both allocated iprange and unallocated iprange
	request := `{"request_ip_range":[["10.49.27.205"]]}`
	cniArgs, err := constant.UnmarshalCniArgs(request)
	if err != nil {
		t.Fatal(err)
	}
	pod := CreateStatefulSetPod("pod1-0", "ns1", cniArgsAnnotation(request))
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod)
	cp := NewFakeCloudProvider()
	fipPlugin.cloudProvider = cp
	defer func() { stopChan <- struct{}{} }()
	if err := checkBindForIPRanges(fipPlugin, pod, node3, cniArgs, "10.49.27.205"); err != nil {
		t.Fatal(err)
	}
	request = `{"request_ip_range":[["10.49.27.205"],["10.49.27.218"]]}`
	cniArgs, err = constant.UnmarshalCniArgs(request)
	if err != nil {
		t.Fatal(err)
	}
	pod.Annotations = cniArgsAnnotation(request)
	if _, err := fipPlugin.Client.CoreV1().Pods(pod.Namespace).Update(pod); err != nil {
		t.Fatal(err)
	}
	// wait for lister updates
	if err := wait.Poll(time.Millisecond*100, time.Minute, func() (done bool, err error) {
		pod1, err := fipPlugin.PodLister.Pods(pod.Namespace).Get(pod.Name)
		if err != nil {
			return false, nil
		}
		return pod1.Annotations[constant.ExtendedCNIArgsAnnotation] ==
			pod.Annotations[constant.ExtendedCNIArgsAnnotation], nil
	}); err != nil {
		t.Fatal()
	}
	if err := checkBindForIPRanges(fipPlugin, pod, node3, cniArgs, "10.49.27.205", "10.49.27.218"); err != nil {
		t.Fatal(err)
	}
}

func checkBindForIPRanges(fipPlugin *FloatingIPPlugin, pod *corev1.Pod, node string, cniArgs *constant.CniArgs,
	expectIPs ...string) error {
	podKey, _ := schedulerplugin_util.FormatKey(pod)
	if err := fipPlugin.Bind(&schedulerapi.ExtenderBindingArgs{
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		Node:         node,
	}); err != nil {
		return err
	}
	fipInfos, err := checkByKeyAndIPRanges(fipPlugin, podKey.KeyInDB, cniArgs.RequestIPRange, expectIPs...)
	if err != nil {
		return err
	}
	var ipInfos []constant.IPInfo
	for i := range fipInfos {
		ipInfos = append(ipInfos, fipInfos[i].IPInfo)
	}
	cniArgs.Common.IPInfos = ipInfos
	if err := checkBinding(fipPlugin, pod, cniArgs, node); err != nil {
		return err
	}
	if fipPlugin.cloudProvider != nil {
		expectAssigned := map[string]string{}
		for _, ip := range expectIPs {
			expectAssigned[ip] = node
		}
		if err := checkFakeCloudProviderState(fipPlugin.cloudProvider.(*FakeCloudProvider), expectAssigned,
			map[string]string{}); err != nil {
			return err
		}
	}
	return nil
}

func checkFakeCloudProviderState(cp *FakeCloudProvider, expectAssigned, expectUnassigned map[string]string) error {
	if !reflect.DeepEqual(cp.Assigned, expectAssigned) {
		return fmt.Errorf("fake cloud provider assigned missmatch, expect %v, real %v", expectAssigned,
			cp.Assigned)
	}
	if !reflect.DeepEqual(cp.UnAssigned, expectUnassigned) {
		return fmt.Errorf("fake cloud provider unassigned missmatch, expect %v, real %v", expectAssigned,
			cp.Assigned)
	}
	return nil
}

func checkBinding(fipPlugin *FloatingIPPlugin, pod *corev1.Pod, expectCniArgs *constant.CniArgs,
	expectNode string) error {
	actualBinding, err := fipPlugin.Client.CoreV1().Pods(pod.Namespace).(*fakeV1.FakePods).
		GetBinding(pod.GetName())
	if err != nil {
		return err
	}
	data, err := json.Marshal(expectCniArgs)
	if err != nil {
		return err
	}
	expect := &corev1.Binding{
		ObjectMeta: v1.ObjectMeta{
			Namespace: pod.Namespace, Name: pod.Name,
			Annotations: cniArgsAnnotation(string(data))},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: expectNode,
		},
	}
	if !reflect.DeepEqual(expect, actualBinding) {
		return fmt.Errorf("binding did not match expectation, expect %v, actual %v", expect, actualBinding)
	}
	return nil
}

func checkByKeyAndIPRanges(fipPlugin *FloatingIPPlugin, key string, ipranges [][]nets.IPRange,
	expectIP ...string) ([]*floatingip.FloatingIPInfo, error) {
	fipInfos, err := fipPlugin.ipam.ByKeyAndIPRanges(key, ipranges)
	if err != nil {
		return nil, err
	}
	realIPs := sets.NewString()
	for i := range fipInfos {
		if fipInfos[i] != nil {
			realIPs.Insert(fipInfos[i].IPInfo.IP.IP.String())
		}
	}
	realIPList := realIPs.List()
	if len(realIPList) != len(expectIP) {
		return nil, fmt.Errorf("expect %v, real %v", expectIP, realIPList)
	}
	for i := range expectIP {
		if expectIP[i] != realIPList[i] {
			return nil, fmt.Errorf("expect %v, real %v", expectIP, realIPList)
		}
	}
	return fipInfos, nil
}
