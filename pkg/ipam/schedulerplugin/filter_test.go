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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	schedulerplugin_util "tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	. "tkestack.io/galaxy/pkg/utils/test"
)

// #lizard forgives
func TestFilter(t *testing.T) {
	fipPlugin, stopChan, nodes := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	// pod has no floating ip resource name, filter should return all nodes
	filtered, failed, err := fipPlugin.Filter(&corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "pod1", Namespace: "ns1"}}, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{drainedNode, nodeHasNoIP, node3, node4}, []string{}); err != nil {
		t.Fatal(err)
	}
	// a pod has floating ip resource name, filter should return nodes that has floating ips
	if filtered, failed, err = fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3, node4}, []string{drainedNode, nodeHasNoIP}); err != nil {
		t.Fatal(err)
	}
	// test filter for reserve situation
	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.173.13.2"),
		floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
		t.Fatal(err)
	}
	filtered, failed, err = fipPlugin.Filter(pod, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node4}, []string{drainedNode, nodeHasNoIP, node3}); err != nil {
		t.Fatal(err)
	}
	// filter again on a new pod2, all good nodes should be filteredNodes
	if filtered, failed, err = fipPlugin.Filter(CreateStatefulSetPod("pod2-1", "ns1", immutableAnnotation), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3, node4}, []string{drainedNode, nodeHasNoIP}); err != nil {
		t.Fatal(err)
	}
}

func TestFilterForPodWithoutRef(t *testing.T) {
	fipPlugin, stopChan, nodes := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	filtered, failed, err := fipPlugin.Filter(CreateSimplePod("pod1", "ns1", nil), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3, node4}, []string{drainedNode, nodeHasNoIP}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = fipPlugin.Filter(CreateSimplePod("pod1", "ns1", immutableAnnotation), nodes); err == nil {
		t.Fatalf("expect an error for pod not belong to a scalable app with policy immutable")
	}
}

// #lizard forgives
func TestFilterForDeployment(t *testing.T) {
	deadPod := CreateDeploymentPod("dp-aaa-bbb", "ns1", immutableAnnotation)
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", immutableAnnotation)
	dp := CreateDeployment(pod.ObjectMeta, 1)
	fipPlugin, stopChan, nodes := createPluginTestNodes(t, pod, deadPod, dp)
	defer func() { stopChan <- struct{}{} }()
	// pre-allocate ip in filter for deployment pod
	podKey, _ := schedulerplugin_util.FormatKey(pod)
	deadPodKey, _ := schedulerplugin_util.FormatKey(deadPod)
	cniArgs, err := fipPlugin.allocateIP(deadPodKey.KeyInDB, node3, deadPod)
	if err != nil || len(cniArgs.Common.IPInfos) != 1 {
		t.Fatal(err)
	}
	fip := cniArgs.Common.IPInfos[0]
	// because deployment ip is allocated to deadPod, check if pod gets none available subnets
	filtered, failed, err := fipPlugin.Filter(pod, nodes)
	if err == nil || !strings.Contains(err.Error(), "wait for releasing") {
		t.Fatal(err)
	}
	// because replicas = 1, ip will be reserved
	if err := fipPlugin.unbind(deadPod); err != nil {
		t.Fatal(err)
	}
	if filtered, failed, err = fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3}, []string{drainedNode, nodeHasNoIP, node4}); err != nil {
		t.Fatal(err)
	}
	fip2, err := fipPlugin.ipam.First(podKey.KeyInDB)
	if err != nil {
		t.Fatal(err)
	} else if fip.IP.String() != fip2.IPInfo.IP.String() {
		t.Fatalf("allocate another ip, expect reserved one")
	}

	pod.Annotations = neverAnnotation
	deadPod.Annotations = immutableAnnotation
	// when replicas = 0 and never release policy, ip will be reserved
	*dp.Spec.Replicas = 0
	if err := fipPlugin.unbind(pod); err != nil {
		t.Fatal(err)
	}
	*dp.Spec.Replicas = 1
	if filtered, failed, err = fipPlugin.Filter(deadPod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3}, []string{drainedNode, nodeHasNoIP, node4}); err != nil {
		t.Fatal(err)
	}
	fip3, err := fipPlugin.ipam.First(deadPodKey.KeyInDB)
	if err != nil {
		t.Fatal(err)
	} else if fip.IP.String() != fip3.IPInfo.IP.String() {
		t.Fatalf("allocate another ip, expect reserved one")
	}
}

type filterCase struct {
	testPod                       *corev1.Pod
	expectErr                     error
	expectFiltererd, expectFailed []string
	preHook                       func(*filterCase) error
	postHook                      func() error
}

// #lizard forgives
func checkFilterCase(fipPlugin *FloatingIPPlugin, testCase filterCase, nodes []corev1.Node) error {
	if testCase.preHook != nil {
		if err := testCase.preHook(&testCase); err != nil {
			return fmt.Errorf("preHook failed: %v", err)
		}
	}
	filtered, failed, err := fipPlugin.Filter(testCase.testPod, nodes)
	if !reflect.DeepEqual(err, testCase.expectErr) {
		return fmt.Errorf("filter failed, expect err: %v, got: %v", testCase.expectErr, err)
	}
	if testCase.expectErr == nil && err != nil {
		return fmt.Errorf("filter failed, expect nil err, got: %v", err)
	}
	if testCase.expectErr != nil && err == nil {
		return fmt.Errorf("filter failed, expect none nil err %v, got nil err", testCase.expectErr)
	}
	if err := checkFilterResult(filtered, failed, testCase.expectFiltererd, testCase.expectFailed); err != nil {
		return fmt.Errorf("checkFilterResult failed: %v", err)
	}
	if testCase.postHook != nil {
		if err := testCase.postHook(); err != nil {
			return fmt.Errorf("postHook failed: %v", err)
		}
	}
	return nil
}

// #lizard forgives
func TestFilterForDeploymentIPPool(t *testing.T) {
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	pod2 := CreateDeploymentPod("dp2-abc-def", "ns2", poolAnnotation("pool1"))
	podKey, _ := schedulerplugin_util.FormatKey(pod)
	pod2Key, _ := schedulerplugin_util.FormatKey(pod2)
	dp1, dp2 := CreateDeployment(pod.ObjectMeta, 1), CreateDeployment(pod2.ObjectMeta, 1)
	fipPlugin, stopChan, nodes := createPluginTestNodes(t, pod, pod2, dp1, dp2)
	defer func() { stopChan <- struct{}{} }()
	testCases := []filterCase{
		{
			// test normal filter gets all good nodes
			testPod: pod, expectFiltererd: []string{node3, node4}, expectFailed: []string{drainedNode, nodeHasNoIP},
		},
		{
			// test bind gets the right key, i.e. dp_ns1_dp_dp-xxx-yyy, and filter gets reserved node
			testPod: pod, expectFiltererd: []string{node4}, expectFailed: []string{drainedNode, nodeHasNoIP, node3},
			preHook: func(*filterCase) error {
				return fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.173.13.2"),
					floatingip.Attr{Policy: constant.ReleasePolicyNever})
			},
		},
		{
			// test unbind gets the right key, i.e. pool__pool1_, and filter on pod2 gets reserved node and key is updating to pod2, i.e. dp_ns1_dp2_dp2-abc-def
			testPod: pod2, expectFiltererd: []string{node4}, expectFailed: []string{drainedNode, nodeHasNoIP, node3},
			preHook: func(*filterCase) error {
				// because replicas = 1, ip will be reserved
				if err := fipPlugin.unbind(pod); err != nil {
					t.Fatal(err)
				}
				if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", podKey.PoolPrefix()); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			postHook: func() error {
				if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", pod2Key.KeyInDB); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			// test filter again on the same pool but different deployment pod and bind gets the right key, i.e. dp_ns1_dp_dp-xxx-yyy
			// two pool deployment, deployment 1 gets enough ips, grow the pool size for deployment 2
			testPod: pod, expectFiltererd: []string{node3, node4}, expectFailed: []string{drainedNode, nodeHasNoIP},
		},
	}
	for i := range testCases {
		if err := checkFilterCase(fipPlugin, testCases[i], nodes); err != nil {
			t.Fatalf("Case %d: %v", i, err)
		}
	}
}

func checkFilterResult(realFilterd []corev1.Node, realFailed schedulerapi.FailedNodesMap, expectFiltererd, expectFailed []string) error {
	if err := checkFiltered(realFilterd, expectFiltererd...); err != nil {
		return err
	}
	if err := checkFailed(realFailed, expectFailed...); err != nil {
		return err
	}
	return nil
}

func checkFiltered(realFilterd []corev1.Node, filtererd ...string) error {
	realNodeName := make([]string, len(realFilterd))
	for i := range realFilterd {
		realNodeName[i] = realFilterd[i].Name
	}
	expect := sets.NewString(filtererd...)
	if expect.Len() != len(realFilterd) {
		return fmt.Errorf("filtered nodes missmatch, expect %v, real %v", expect, realNodeName)
	}
	for i := range realFilterd {
		if !expect.Has(realFilterd[i].Name) {
			return fmt.Errorf("filtered nodes missmatch, expect %v, real %v", expect, realNodeName)
		}
	}
	return nil
}

func checkFailed(realFailed schedulerapi.FailedNodesMap, failed ...string) error {
	expect := sets.NewString(failed...)
	if expect.Len() != len(realFailed) {
		return fmt.Errorf("failed nodes missmatch, expect %v, real %v", expect, realFailed)
	}
	for nodeName := range realFailed {
		if !expect.Has(nodeName) {
			return fmt.Errorf("failed nodes missmatch, expect %v, real %v", expect, realFailed)
		}
	}
	return nil
}

func TestFilterRequestIPRange(t *testing.T) {
	// node3 can allocate 10.49.27.0/24
	// node5 can allocate 10.49.27.0/24, 10.0.80.0/24, 10.0.81.0/24
	// node6 can allocate 10.0.80.0/24
	n5, n6 := CreateNode("node5", nil, "10.49.28.3"), CreateNode("node6", nil, "10.49.29.3")
	node5, node6 := n5.Name, n6.Name
	fipPlugin, stopChan, nodes := createPluginTestNodes(t, &n5, &n6)
	defer func() { stopChan <- struct{}{} }()
	for i, testCase := range []filterCase{
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.49.27.205"],["10.49.27.216~10.49.27.218"]]}`)),
			expectFiltererd: []string{node3},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node4, node5, node6},
		},
		// create a pod request for two ips, only nodes in 10.49.28.0/24() can meet the requests
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.0.80.2~10.0.80.4"],["10.0.81.2"]]}`)),
			expectFiltererd: []string{node5},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node3, node4, node6},
		},
		// create a pod request for 10.0.80.2~10.0.80.4, both node5 and node6 meet the requests
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.0.80.2~10.0.80.4"]]}`)),
			expectFiltererd: []string{node5, node6},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node3, node4},
		},
		// create a pod request for two ips, 10.49.27.205 and one of 10.0.80.2~10.0.80.4, no node meet the requests
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.49.27.205"],["10.0.80.2~10.0.80.4"]]}`)),
			expectFiltererd: []string{},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node3, node4, node5, node6},
		},
		// create a pod request for 10.49.27.205 or 10.0.80.2~10.0.80.4, node3, node5, node6 meet the requests
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.49.27.205","10.0.80.2~10.0.80.4"]]}`)),
			expectFiltererd: []string{node3, node5, node6},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node4},
		},
		// TODO allocates multiple ips in the overlapped ip range
		//{
		//	testPod: CreateStatefulSetPod("pod1-0", "ns1",
		//		cniArgsAnnotation(`{"request_ip_range":[["10.49.27.205","10.0.80.2~10.0.80.4"],["10.49.27.205","10.0.80.2~10.0.80.4"]]}`)),
		//	expectFiltererd: []string{node5, node6},
		//	expectFailed:    []string{drainedNode, nodeHasNoIP, node3, node4},
		//},
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.49.27.216~10.49.27.218","10.0.80.2~10.0.80.4"],["10.49.27.216~10.49.27.218","10.0.80.2~10.0.80.4"]]}`)),
			expectFiltererd: []string{node3, node5, node6},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node4},
		},
		{
			testPod: CreateStatefulSetPod("pod1-0", "ns1",
				cniArgsAnnotation(`{"request_ip_range":[["10.49.27.216~10.49.27.218","10.0.80.2~10.0.80.4"],["10.49.27.216~10.49.27.218","10.0.80.2~10.0.80.4"]]}`)),
			preHook: func(fc *filterCase) error {
				podKey, _ := schedulerplugin_util.FormatKey(fc.testPod)
				if _, err := fipPlugin.allocateIP(podKey.KeyInDB, node6, fc.testPod); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			expectFiltererd: []string{node5, node6},
			expectFailed:    []string{drainedNode, nodeHasNoIP, node3, node4},
		},
	} {
		if err := checkFilterCase(fipPlugin, testCase, nodes); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func cniArgsAnnotation(poolName string) map[string]string {
	return map[string]string{constant.ExtendedCNIArgsAnnotation: poolName}
}

func TestFilterForCRDPod(t *testing.T) {
	fipPlugin, stopChan, nodes := createPluginTestNodesWithCrdObjs(t, nil,
		[]runtime.Object{FooCrd, NotScalableCrd}, nil)
	defer func() { stopChan <- struct{}{} }()
	testCases := []filterCase{
		{
			testPod:         CreateCRDPod("crd-xxx-0", "ns1", nil, FooCrd),
			expectFiltererd: []string{node3, node4},
			expectFailed:    []string{drainedNode, nodeHasNoIP},
		},
		{
			testPod:         CreateCRDPod("crd-xxx-0", "ns1", immutableAnnotation, FooCrd),
			expectFiltererd: []string{node3, node4},
			expectFailed:    []string{drainedNode, nodeHasNoIP},
		},
		{
			testPod:         CreateCRDPod("crd-xxx-0", "ns1", neverAnnotation, FooCrd),
			expectFiltererd: []string{node3, node4},
			expectFailed:    []string{drainedNode, nodeHasNoIP},
		},
		{
			testPod: CreateCRDPod("crd-xxx-sbx", "ns1", neverAnnotation, FooCrd),
			expectErr: fmt.Errorf("release policy never is not supported for pod crd-xxx-sbx: %w",
				NotStatefulWorkload),
		},
		{
			testPod:         CreateCRDPod("crd-xxx-0", "ns1", nil, NotScalableCrd),
			expectFiltererd: []string{node3, node4},
			expectFailed:    []string{drainedNode, nodeHasNoIP},
		},
		{
			testPod:   CreateCRDPod("crd-xxx-0", "ns1", immutableAnnotation, NotScalableCrd),
			expectErr: fmt.Errorf("release policy immutable is not supported for pod crd-xxx-0: %w", NoReplicas),
		},
		{
			testPod:         CreateCRDPod("crd-xxx-0", "ns1", neverAnnotation, NotScalableCrd),
			expectFiltererd: []string{node3, node4},
			expectFailed:    []string{drainedNode, nodeHasNoIP},
		},
		{
			testPod: CreateCRDPod("crd-xxx-xb1", "ns1", neverAnnotation, NotScalableCrd),
			expectErr: fmt.Errorf("release policy never is not supported for pod crd-xxx-xb1: %w",
				NotStatefulWorkload),
		},
	}
	for i := range testCases {
		if err := checkFilterCase(fipPlugin, testCases[i], nodes); err != nil {
			t.Fatalf("Case %d: %v", i, err)
		}
	}
}
