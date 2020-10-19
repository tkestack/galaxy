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
	"strings"
	"testing"
	"time"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreInformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	fakeV1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	fakeGalaxyCli "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned/fake"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	schedulerplugin_util "tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/ipam/utils"
	//fakeTAppCli "tkestack.io/tapp/pkg/client/clientset/versioned/fake"
	//tappInformer "tkestack.io/tapp/pkg/client/informers/externalversions"
	//"tkestack.io/tapp/pkg/tapp"
	"tkestack.io/galaxy/pkg/api/k8s/eventhandler"
)

const (
	drainedNode, nodeHasNoIP, node3, node4 = "node1", "node2", "node3", "node4"
)

var (
	immutableAnnotation = map[string]string{constant.ReleasePolicyAnnotation: constant.Immutable}
	neverAnnotation     = map[string]string{constant.ReleasePolicyAnnotation: constant.Never}

	pod         = CreateStatefulSetPod("pod1-0", "ns1", immutableAnnotation)
	podKey, _   = schedulerplugin_util.FormatKey(pod)
	node3Subnet = &net.IPNet{IP: net.ParseIP("10.49.27.0"), Mask: net.IPv4Mask(255, 255, 255, 0)}

	toFailedPod = func(pod *corev1.Pod) {
		pod.Status.Phase = corev1.PodFailed
	}
	toSuccessPod = func(pod *corev1.Pod) {
		pod.Status.Phase = corev1.PodSucceeded
	}
)

func createPluginTestNodes(t *testing.T, objs ...runtime.Object) (*FloatingIPPlugin, chan struct{}, []corev1.Node) {
	nodes := []corev1.Node{
		createNode(drainedNode, nil, "10.180.1.3"), // no floating ip left on this node
		createNode(nodeHasNoIP, nil, "10.48.28.2"), // no floating ip configured for this node
		createNode(node3, nil, "10.49.27.3"),       // good node
		createNode(node4, nil, "10.173.13.4"),      // good node
	}
	allObjs := append([]runtime.Object{&nodes[0], &nodes[1], &nodes[2], &nodes[3]}, objs...)
	fipPlugin, stopChan := createPlugin(t, allObjs...)
	// drain drainedNode 10.180.1.3/32
	subnet := &net.IPNet{IP: net.ParseIP("10.180.1.3"), Mask: net.CIDRMask(32, 32)}
	if err := drainNode(fipPlugin, subnet, nil); err != nil {
		t.Fatal(err)
	}
	return fipPlugin, stopChan, nodes
}

func createPlugin(t *testing.T, objs ...runtime.Object) (*FloatingIPPlugin, chan struct{}) {
	var conf Conf
	if err := json.Unmarshal([]byte(utils.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	fipPlugin, stopChan := newPlugin(t, conf, objs...)
	return fipPlugin, stopChan
}

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
		t.Fatalf("expect an error for non sts/deployment/tapp pod with policy immutable")
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

func TestUpdatePod(t *testing.T) {
	fipPlugin, stopChan, _ := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()

	pod.Status.Phase = corev1.PodRunning
	var ipInfo constant.IPInfo
	if err := json.Unmarshal([]byte(`{"ip":"10.173.13.2/24","vlan":2,"gateway":"10.173.13.1","routable_subnet":"10.173.13.0/24"}`), &ipInfo); err != nil {
		t.Fatal()
	}
	str, err := constant.MarshalCniArgs([]constant.IPInfo{ipInfo})
	if err != nil {
		t.Fatal(err)
	}
	pod.Annotations[constant.ExtendedCNIArgsAnnotation] = str
	if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", ""); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.UpdatePod(pod, pod); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", podKey.KeyInDB); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseIP(t *testing.T) {
	fipPlugin, stopChan, _ := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	if err := fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.173.13.2"),
		floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", podKey.KeyInDB); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.releaseIP(podKey.KeyInDB, ""); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.173.13.2", ""); err != nil {
		t.Fatal(err)
	}
}

// #lizard forgives
func TestFilterForDeployment(t *testing.T) {
	deadPod := CreateDeploymentPod("dp-aaa-bbb", "ns1", immutableAnnotation)
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", immutableAnnotation)
	dp := createDeployment(pod.ObjectMeta, 1)
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

func poolAnnotation(poolName string) map[string]string {
	return map[string]string{constant.IPPoolAnnotation: poolName}
}

func createDeployment(podMeta v1.ObjectMeta, replicas int32) *appv1.Deployment {
	parts := strings.Split(podMeta.OwnerReferences[0].Name, "-")
	return &appv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      strings.Join(parts[:len(parts)-1], "-"),
			Namespace: podMeta.Namespace,
			Labels:    podMeta.Labels,
		},
		Spec: appv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMeta,
			},
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: podMeta.GetLabels(),
			},
		},
	}
}

func CreateStatefulSet(podMeta v1.ObjectMeta, replicas int32) *appv1.StatefulSet {
	return &appv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      podMeta.OwnerReferences[0].Name,
			Namespace: podMeta.GetNamespace(),
			Labels:    podMeta.GetLabels(),
		},
		Spec: appv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMeta,
			},
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: podMeta.GetLabels(),
			},
		},
	}
}

type filterCase struct {
	testPod                       *corev1.Pod
	expectErr                     error
	expectFiltererd, expectFailed []string
	preHook                       func() error
	postHook                      func() error
}

// #lizard forgives
func checkFilterCase(fipPlugin *FloatingIPPlugin, testCase filterCase, nodes []corev1.Node) error {
	if testCase.preHook != nil {
		if err := testCase.preHook(); err != nil {
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
	dp1, dp2 := createDeployment(pod.ObjectMeta, 1), createDeployment(pod2.ObjectMeta, 1)
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
			preHook: func() error {
				return fipPlugin.ipam.AllocateSpecificIP(podKey.KeyInDB, net.ParseIP("10.173.13.2"),
					floatingip.Attr{Policy: constant.ReleasePolicyNever})
			},
		},
		{
			// test unbind gets the right key, i.e. pool__pool1_, and filter on pod2 gets reserved node and key is updating to pod2, i.e. dp_ns1_dp2_dp2-abc-def
			testPod: pod2, expectFiltererd: []string{node4}, expectFailed: []string{drainedNode, nodeHasNoIP, node3},
			preHook: func() error {
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

func createNode(name string, labels map[string]string, address string) corev1.Node {
	return corev1.Node{
		ObjectMeta: v1.ObjectMeta{Name: name, Labels: labels},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: address}}},
	}
}

func createPluginFactoryArgs(t *testing.T, objs ...runtime.Object) (*PluginFactoryArgs, coreInformer.PodInformer, chan struct{}) {
	galaxyCli := fakeGalaxyCli.NewSimpleClientset()
	crdInformerFactory := crdInformer.NewSharedInformerFactory(galaxyCli, 0)
	poolInformer := crdInformerFactory.Galaxy().V1alpha1().Pools()
	FIPInformer := crdInformerFactory.Galaxy().V1alpha1().FloatingIPs()
	client := fake.NewSimpleClientset(objs...)
	informerFactory := informers.NewFilteredSharedInformerFactory(client, time.Minute, v1.NamespaceAll, nil)
	podInformer := informerFactory.Core().V1().Pods()
	statefulsetInformer := informerFactory.Apps().V1().StatefulSets()
	deploymentInformer := informerFactory.Apps().V1().Deployments()
	//tappCli := fakeTAppCli.NewSimpleClientset()
	//tappInformerFactory := tappInformer.NewSharedInformerFactory(tappCli, 0)
	//tappInformer := tappInformerFactory.Tappcontroller().V1().TApps()
	stopChan := make(chan struct{})
	pluginArgs := &PluginFactoryArgs{
		PodLister:         podInformer.Lister(),
		StatefulSetLister: statefulsetInformer.Lister(),
		DeploymentLister:  deploymentInformer.Lister(),
		Client:            client,
		PoolLister:        poolInformer.Lister(),
		//TAppClient:        tappCli,
		//TAppHasSynced:     tappInformer.Informer().HasSynced,
		//TAppLister:        tappInformer.Lister(),
		ExtClient:   extensionClient.NewSimpleClientset(),
		CrdClient:   galaxyCli,
		FIPInformer: FIPInformer,
	}
	//tapp.EnsureCRDCreated(pluginArgs.ExtClient)
	informerFactory.Start(stopChan)
	crdInformerFactory.Start(stopChan)
	informerFactory.WaitForCacheSync(stopChan)
	crdInformerFactory.WaitForCacheSync(stopChan)
	//go tappInformerFactory.Start(stopChan)
	return pluginArgs, podInformer, stopChan
}

func newPlugin(t *testing.T, conf Conf, objs ...runtime.Object) (*FloatingIPPlugin, chan struct{}) {
	pluginArgs, podInformer, stopChan := createPluginFactoryArgs(t, objs...)
	fipPlugin, err := NewFloatingIPPlugin(conf, pluginArgs)
	if err != nil {
		t.Fatal(err)
	}
	podInformer.Informer().AddEventHandler(eventhandler.NewPodEventHandler(fipPlugin))
	if err = fipPlugin.Init(); err != nil {
		t.Fatal(err)
	}
	return fipPlugin, stopChan
}

func TestLoadConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: "testConf", Namespace: "demo"},
		Data: map[string]string{
			"key": `[{"routableSubnet":"10.49.27.0/24","ips":["10.49.27.216~10.49.27.218"],"subnet":"10.49.27.0/24","gateway":"10.49.27.1","vlan":2}]`,
		},
	}
	var conf Conf
	if err := json.Unmarshal([]byte(utils.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	conf.FloatingIPs = nil
	conf.ConfigMapName = cm.Name
	conf.ConfigMapNamespace = cm.Namespace
	conf.FloatingIPKey = "key"
	fipPlugin, stopChan := newPlugin(t, conf, cm)
	defer func() { stopChan <- struct{}{} }()
	if fipPlugin.lastIPConf != cm.Data["key"] {
		t.Errorf(fipPlugin.lastIPConf)
	}
}

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

func TestParseReleasePolicy(t *testing.T) {
	testCases := []struct {
		meta   *v1.ObjectMeta
		expect constant.ReleasePolicy
	}{
		{
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}},
			expect: constant.ReleasePolicyPodDelete,
		},
		{
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: immutableAnnotation},
			expect: constant.ReleasePolicyImmutable,
		},
		{
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: neverAnnotation},
			expect: constant.ReleasePolicyNever,
		},
		{
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{constant.IPPoolAnnotation: "11"}},
			expect: constant.ReleasePolicyNever,
		},
		{
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{constant.IPPoolAnnotation: ""}},
			expect: constant.ReleasePolicyPodDelete,
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := parseReleasePolicy(testCase.meta)
		if got != testCase.expect {
			t.Errorf("case %d, expect %v, got %v", i, testCase.expect, got)
		}
	}
}

type fakeCloudProvider struct {
	expectIP          string
	expectNode        string
	invokedAssignIP   bool
	invokedUnAssignIP bool
}

func (f *fakeCloudProvider) AssignIP(in *rpc.AssignIPRequest) (*rpc.AssignIPReply, error) {
	f.invokedAssignIP = true
	if in == nil {
		return nil, fmt.Errorf("nil request")
	}
	if in.IPAddress != f.expectIP {
		return nil, fmt.Errorf("expect ip %s, got %s", f.expectIP, in.IPAddress)
	}
	if in.NodeName != f.expectNode {
		return nil, fmt.Errorf("expect node name %s, got %s", f.expectNode, in.NodeName)
	}
	return &rpc.AssignIPReply{Success: true}, nil
}

func (f *fakeCloudProvider) UnAssignIP(in *rpc.UnAssignIPRequest) (*rpc.UnAssignIPReply, error) {
	f.invokedUnAssignIP = true
	if in == nil {
		return nil, fmt.Errorf("nil request")
	}
	if in.IPAddress != f.expectIP {
		return nil, fmt.Errorf("expect ip %s, got %s", f.expectIP, in.IPAddress)
	}
	if in.NodeName != f.expectNode {
		return nil, fmt.Errorf("expect node name %s, got %s", f.expectNode, in.NodeName)
	}
	return &rpc.UnAssignIPReply{Success: true}, nil
}

// #lizard forgives
func TestUnBind(t *testing.T) {
	pod1 := CreateStatefulSetPod("pod1-1", "demo", map[string]string{})
	keyObj, _ := schedulerplugin_util.FormatKey(pod1)
	fipPlugin, stopChan, _ := createPluginTestNodes(t, pod1)
	defer func() { stopChan <- struct{}{} }()
	fipPlugin.cloudProvider = &fakeCloudProvider{}
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
	fakeCP := &fakeCloudProvider{expectIP: expectIP.String(), expectNode: node3}
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
	if !fakeCP.invokedAssignIP || !fakeCP.invokedUnAssignIP {
		t.Fatal()
	}
}

func drainNode(fipPlugin *FloatingIPPlugin, subnet *net.IPNet, except net.IP) error {
	for {
		if _, err := fipPlugin.ipam.AllocateInSubnet("ns_notexistpod", subnet,
			floatingip.Attr{Policy: constant.ReleasePolicyPodDelete}); err != nil {
			if err == floatingip.ErrNoEnoughIP {
				break
			}
			return err
		}
	}
	if len(except) != 0 {
		return fipPlugin.ipam.Release("ns_notexistpod", except)
	}
	return nil
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

// #lizard forgives
func TestAllocateRecentIPs(t *testing.T) {
	pod := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	dp := createDeployment(pod.ObjectMeta, 1)
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

func checkIPKey(ipam floatingip.IPAM, checkIP, expectKey string) error {
	ip := net.ParseIP(checkIP)
	if ip == nil {
		return fmt.Errorf("bad check ip: %s", checkIP)
	}
	fip, err := ipam.ByIP(ip)
	if err != nil {
		return err
	}
	if fip.Key != expectKey {
		return fmt.Errorf("expect key: %s, got %s, ip %s", expectKey, fip.Key, checkIP)
	}
	return nil
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
