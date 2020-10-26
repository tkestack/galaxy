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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	extensionClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	coreInformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	fakeGalaxyCli "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned/fake"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions"
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
		CreateNode(drainedNode, nil, "10.180.1.3"), // no floating ip left on this node
		CreateNode(nodeHasNoIP, nil, "10.48.28.2"), // no floating ip configured for this node
		CreateNode(node3, nil, "10.49.27.3"),       // good node
		CreateNode(node4, nil, "10.173.13.4"),      // good node
	}
	allObjs := append([]runtime.Object{&nodes[0], &nodes[1], &nodes[2], &nodes[3]}, objs...)
	fipPlugin, stopChan := createPlugin(t, allObjs...)
	// drain drainedNode 10.180.1.3/32
	subnet := &net.IPNet{IP: net.ParseIP("10.180.1.3"), Mask: net.CIDRMask(32, 32)}
	if err := drainNode(fipPlugin, subnet, nil); err != nil {
		t.Fatal(err)
	}
	for i := range objs {
		if node, ok := objs[i].(*corev1.Node); ok {
			nodes = append(nodes, *node)
		}
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

func poolAnnotation(poolName string) map[string]string {
	return map[string]string{constant.IPPoolAnnotation: poolName}
}

func createPluginFactoryArgs(objs ...runtime.Object) (*PluginFactoryArgs, coreInformer.PodInformer, chan struct{}) {
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
	pluginArgs, podInformer, stopChan := createPluginFactoryArgs(objs...)
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
