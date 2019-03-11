package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/cloudprovider/rpc"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"github.com/jinzhu/gorm"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	fakeV1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
)

const (
	drainedNode, nodeHasNoIP, node3, node4 = "node1", "node2", "node3", "node4"
)

var (
	secondIPLabel       = map[string]string{private.LabelKeyEnableSecondIP: private.LabelValueEnabled}
	immutableAnnotation = map[string]string{constant.ReleasePolicyAnnotation: constant.Immutable}
	neverAnnotation     = map[string]string{constant.ReleasePolicyAnnotation: constant.Never}
)

func createPluginTestNodes(t *testing.T, objs ...runtime.Object) (*FloatingIPPlugin, chan struct{}, []corev1.Node) {
	var conf Conf
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	nodes := []corev1.Node{
		createNode(drainedNode, nil, "10.180.1.3"), // no floating ip left on this node
		createNode(nodeHasNoIP, nil, "10.49.28.2"), // no floating ip configured for this node
		createNode(node3, nil, "10.49.27.3"),       // good node
		createNode(node4, nil, "10.173.13.4"),      // good node
	}
	allObjs := append([]runtime.Object{&nodes[0], &nodes[1], &nodes[2], &nodes[3]}, objs...)
	fipPlugin, stopChan := newPlugin(t, conf, allObjs...)
	// drain drainedNode
	drainedNodeIPNet := &net.IPNet{IP: net.ParseIP("10.180.1.3"), Mask: net.IPv4Mask(255, 255, 255, 255)}
	if ipInfo, err := fipPlugin.ipam.AllocateInSubnet("ns_notexistpod", drainedNodeIPNet, constant.ReleasePolicyPodDelete, ""); err != nil || ipInfo == nil || "10.180.154.7" != ipInfo.String() {
		t.Fatal(err, ipInfo)
	}
	if ipInfo, err := fipPlugin.ipam.AllocateInSubnet("ns_notexistpod", drainedNodeIPNet, constant.ReleasePolicyPodDelete, ""); err != nil || ipInfo == nil || "10.180.154.8" != ipInfo.String() {
		t.Fatal(err, ipInfo)
	}
	return fipPlugin, stopChan, nodes
}

func TestFilter(t *testing.T) {
	fipPlugin, stopChan, nodes := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	// pod doesn't has no floating ip resource name, filter should return all nodes
	filtered, failed, err := fipPlugin.Filter(&corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "pod1", Namespace: "ns1"}}, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{drainedNode, nodeHasNoIP, node3, node4}, []string{}); err != nil {
		t.Fatal(err)
	}
	// a pod has floating ip resource name, filter should return nodes that has floating ips
	if filtered, failed, err = fipPlugin.Filter(createStatefulSetPod("pod1", "ns1", nil), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3, node4}, []string{drainedNode, nodeHasNoIP}); err != nil {
		t.Fatal(err)
	}
	// the following is to check release policy
	// allocate a ip of 10.173.13.0/24
	_, ipNet, _ := net.ParseCIDR("10.173.13.0/24")
	pod := createStatefulSetPod("pod1-0", "ns1", immutableAnnotation)
	podKey := formatKey(pod)
	if ipInfo, err := fipPlugin.ipam.AllocateInSubnet(podKey.keyInDB, ipNet, constant.ReleasePolicyPodDelete, ""); err != nil || ipInfo == nil || "10.173.13.2" != ipInfo.String() {
		t.Fatal(err, ipInfo)
	}
	if err := checkPolicyAndAttr(fipPlugin.ipam, podKey.keyInDB, constant.ReleasePolicyPodDelete, expectAttrEmpty()); err != nil {
		t.Fatal(err)
	}
	// check filter result is expected
	if filtered, failed, err = fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node4}, []string{drainedNode, nodeHasNoIP, node3}); err != nil {
		t.Fatal(err)
	}
	// check pod allocated the previous ip and policy should be updated to AppDeleteOrScaleDown
	pod.Spec.NodeName = node4
	ipInfo, err := fipPlugin.allocateIP(fipPlugin.ipam, podKey.keyInDB, pod.Spec.NodeName, pod)
	if err != nil {
		t.Fatal(err)
	}
	if ipInfo == nil || ipInfo.IP.String() != "10.173.13.2/24" {
		t.Fatal(ipInfo)
	}
	if err := checkPolicyAndAttr(fipPlugin.ipam, podKey.keyInDB, constant.ReleasePolicyImmutable, expectAttrNotEmpty()); err != nil {
		t.Fatal(err)
	}

	// filter again on a new pod2, all good nodes should be filteredNodes
	if filtered, failed, err = fipPlugin.Filter(createStatefulSetPod("pod2-1", "ns1", immutableAnnotation), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3, node4}, []string{drainedNode, nodeHasNoIP}); err != nil {
		t.Fatal(err)
	}
	// forget the pod1, the ip should be reserved, because pod1 has immutable label attached
	if err := fipPlugin.DeletePod(pod); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := fipPlugin.ipam.First(podKey.keyInDB); err != nil || ipInfo == nil {
		t.Fatal(err, ipInfo)
	} else {
		if ipInfo.IPInfo.IP.String() != "10.173.13.2/24" {
			t.Fatal(ipInfo)
		}
	}
	// allocates all ips to pods of a new  statefulset
	ipInfoSet := sets.NewString()
	for i := 0; ; i++ {
		newPod := createStatefulSetPod(fmt.Sprintf("temp-%d", i), "ns1", immutableAnnotation)
		newPod.Spec.NodeName = node4
		newPodKey := formatKey(newPod)
		if ipInfo, err := fipPlugin.allocateIP(fipPlugin.ipam, newPodKey.keyInDB, newPod.Spec.NodeName, newPod); err != nil {
			if !strings.Contains(err.Error(), floatingip.ErrNoEnoughIP.Error()) {
				t.Fatal(err)
			}
			break
		} else {
			if ipInfo == nil {
				t.Fatal()
			}
			if ipInfoSet.Has(ipInfo.IP.String()) {
				t.Fatal("allocates an previous allocated ip")
			}
			ipInfoSet.Insert(ipInfo.IP.String())
		}
		if i == 10 {
			t.Fatal("should not have so many ips")
		}
	}
	// see if we can allocate the reserved ip
	if ipInfo, err = fipPlugin.allocateIP(fipPlugin.ipam, podKey.keyInDB, pod.Spec.NodeName, pod); err != nil {
		t.Fatal(err)
	}
	if ipInfo == nil || ipInfo.IP.String() != "10.173.13.2/24" {
		t.Fatal(ipInfo)
	}
	if err := checkPolicyAndAttr(fipPlugin.ipam, podKey.keyInDB, constant.ReleasePolicyImmutable, expectAttrNotEmpty()); err != nil {
		t.Fatal(err)
	}
	// check sync back into db according to pods annotation TODO move this to a separate test
	pod.Status.Phase = corev1.PodRunning
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	str, err := constant.FormatIPInfo([]constant.IPInfo{*ipInfo})
	if err != nil {
		t.Fatal(err)
	}
	pod.Annotations[constant.ExtendedCNIArgsAnnotation] = str
	if err := fipPlugin.releaseIP(podKey.keyInDB, "", pod); err != nil {
		t.Fatal(err)
	}
	if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
		t.Fatal(err)
	} else if fip.Key != "" {
		t.Fatal("failed release ip 10.173.13.2")
	}
	if err := fipPlugin.UpdatePod(pod, pod); err != nil {
		t.Fatal(err)
	}
	if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
		t.Fatal(err)
	} else if fip.Key != podKey.keyInDB {
		t.Fatal("failed resync ip 10.173.13.2")
	}
}

func TestFilterForDeployment(t *testing.T) {
	fipPlugin, stopChan, nodes := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	// pre-allocate ip in filter for deployment pod
	deadPod := createDeploymentPod("dp-aaa-bbb", "ns1", immutableAnnotation)
	pod := createDeploymentPod("dp-xxx-yyy", "ns1", immutableAnnotation)
	podKey, deadPodKey := formatKey(pod), formatKey(deadPod)
	var replicas int32 = 1
	fipPlugin.getDeployment = getDeploymentFunc(pod.ObjectMeta, &replicas)
	fip, err := fipPlugin.allocateIP(fipPlugin.ipam, deadPodKey.keyInDB, node3, deadPod)
	if err != nil {
		t.Fatal(err)
	}
	// because deployment ip is allocated to deadPod, check if pod gets none available subnets
	filtered, failed, err := fipPlugin.Filter(pod, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{}, []string{drainedNode, nodeHasNoIP, node3, node4}); err != nil {
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
	fip2, err := fipPlugin.ipam.First(podKey.keyInDB)
	if err != nil {
		t.Fatal(err)
	} else if fip.IP.String() != fip2.IPInfo.IP.String() {
		t.Fatalf("allocate another ip, expect reserved one")
	}

	pod.Annotations = neverAnnotation
	deadPod.Annotations = immutableAnnotation
	// when replicas = 0 and never release policy, ip will be reserved
	replicas = 0
	if err := fipPlugin.unbind(pod); err != nil {
		t.Fatal(err)
	}
	replicas = 1
	if filtered, failed, err = fipPlugin.Filter(deadPod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFilterResult(filtered, failed, []string{node3}, []string{drainedNode, nodeHasNoIP, node4}); err != nil {
		t.Fatal(err)
	}
	fip3, err := fipPlugin.ipam.First(deadPodKey.keyInDB)
	if err != nil {
		t.Fatal(err)
	} else if fip.IP.String() != fip3.IPInfo.IP.String() {
		t.Fatalf("allocate another ip, expect reserved one")
	}
}

func createDeploymentPod(name, namespace string, annotation map[string]string) *corev1.Pod {
	parts := strings.Split(name, "-")
	pod := createStatefulSetPod(name, namespace, annotation)
	pod.OwnerReferences = []v1.OwnerReference{{
		Kind: "ReplicaSet",
		Name: strings.Join(parts[:len(parts)-1], "-"),
	}}
	return pod
}

func poolAnnotation(poolName string) map[string]string {
	return map[string]string{constant.IPPoolAnnotation: poolName}
}

func getDeploymentFunc(meta v1.ObjectMeta, replicas *int32) func(name, namespace string) (*appv1.Deployment, error) {
	return func(name, namespace string) (*appv1.Deployment, error) {
		return &appv1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: meta,
				},
				Replicas: replicas,
			},
		}, nil
	}
}

func TestFilterForDeploymentIPPool(t *testing.T) {
	pod := createDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	pod2 := createDeploymentPod("dp2-abc-def", "ns2", poolAnnotation("pool1"))
	podKey, pod2Key := formatKey(pod), formatKey(pod2)
	fipPlugin, stopChan, nodes := createPluginTestNodes(t, pod, pod2)
	defer func() { stopChan <- struct{}{} }()
	var replicas int32 = 1
	fipPlugin.getDeployment = getDeploymentFunc(pod.ObjectMeta, &replicas)
	testCases := []struct {
		testPod                       *corev1.Pod
		expectErr                     error
		expectFiltererd, expectFailed []string
		preHook                       func() error
		postHook                      func() error
	}{
		{
			// test normal filter gets all good nodes
			testPod: pod, expectFiltererd: []string{node3, node4}, expectFailed: []string{drainedNode, nodeHasNoIP},
		},
		{
			// test bind gets the right key, i.e. dp_ns1_dp_dp-xxx-yyy, and filter gets reserved node
			testPod: pod, expectFiltererd: []string{node4}, expectFailed: []string{drainedNode, nodeHasNoIP, node3},
			preHook: func() error {
				if err := fipPlugin.Bind(&schedulerapi.ExtenderBindingArgs{
					PodName:      pod.Name,
					PodNamespace: pod.Namespace,
					Node:         node4,
				}); err != nil {
					return err
				}

				if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
					t.Fatal(err)
				} else if fip.Key != podKey.keyInDB {
					t.Fatalf("real key: %s, expect %s", fip.Key, podKey.keyInDB)
				}
				return nil
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
				if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
					t.Fatal(err)
				} else if fip.Key != podKey.poolPrefix() {
					t.Fatalf("real key: %s, expect %s", fip.Key, podKey.poolPrefix())
				}
				return nil
			},
			postHook: func() error {
				if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
					t.Fatal(err)
				} else if fip.Key != pod2Key.keyInDB {
					t.Fatalf("real key: %s, expect %s", fip.Key, pod2Key.keyInDB)
				}
				return nil
			},
		},
		{
			// test filter again on the same pool but different deployment pod and bind gets the right key, i.e. dp_ns1_dp_dp-xxx-yyy
			// two pool deployment, deployment 1 gets enough ips, grow the pool size for deployment 2
			testPod: pod, expectFiltererd: []string{node3, node4}, expectFailed: []string{drainedNode, nodeHasNoIP},
			postHook: func() error {
				if err := fipPlugin.Bind(&schedulerapi.ExtenderBindingArgs{
					PodName:      pod.Name,
					PodNamespace: pod.Namespace,
					Node:         node3,
				}); err != nil {
					return err
				}
				if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.49.27.205")); err != nil {
					t.Fatal(err)
				} else if fip.Key != podKey.keyInDB {
					t.Fatalf("real key: %s, expect %s", fip.Key, podKey.keyInDB)
				}
				return nil
			},
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		if testCase.preHook != nil {
			if err := testCase.preHook(); err != nil {
				t.Fatalf("case %d, preHook failed: %v", i, err)
			}
		}
		filtered, failed, err := fipPlugin.Filter(testCase.testPod, nodes)
		if !reflect.DeepEqual(err, testCase.expectErr) {
			t.Fatalf("case %d, Filter failed, expect err: %v, got: %v", i, testCase.expectErr, err)
		}
		if testCase.expectErr == nil && err != nil {
			t.Fatalf("case %d, Filter failed: %v", i, err)
		}
		if testCase.expectErr != nil && err == nil {
			t.Fatalf("case %d, Filter failed: %v", i, err)
		}
		if err := checkFilterResult(filtered, failed, testCase.expectFiltererd, testCase.expectFailed); err != nil {
			t.Fatalf("case %d, checkFilterResult failed: %v", i, err)
		}
		if testCase.postHook != nil {
			if err := testCase.postHook(); err != nil {
				t.Fatalf("case %d, postHook failed: %v", i, err)
			}
		}
	}
}

// Attr has a time field which makes it hard to check, so creating this struct to do part check
type expectAttr struct {
	empty    bool
	contains []string
}

func expectAttrEmpty() expectAttr {
	return expectAttr{empty: true}
}

func expectAttrNotEmpty() expectAttr {
	return expectAttr{empty: false}
}

func checkPolicyAndAttr(ipam floatingip.IPAM, key string, expectPolicy constant.ReleasePolicy, expectAttr expectAttr) error {
	fip, err := ipam.First(key)
	if err != nil {
		return err
	}
	// policy should be
	if fip.FIP.Policy != uint16(expectPolicy) {
		return fmt.Errorf("expect policy %d, real %d", expectPolicy, fip.FIP.Policy)
	}
	if expectAttr.empty && fip.FIP.Attr != "" {
		return fmt.Errorf("expect attr empty, real attr %q", fip.FIP.Attr)
	}
	if !expectAttr.empty && fip.FIP.Attr == "" {
		return fmt.Errorf("expect attr not empty, real attr empty")
	}
	for i := range expectAttr.contains {
		if !strings.Contains(fip.FIP.Attr, expectAttr.contains[i]) {
			return fmt.Errorf("expect attr contains %q, real attr %q", expectAttr.contains[i], fip.FIP.Attr)
		}
	}
	return nil
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

func createStatefulSetPodWithLabels(name, namespace string, labels map[string]string) *corev1.Pod {
	pod := createStatefulSetPod(name, namespace, nil)
	pod.Labels = labels
	return pod
}

// createStatefulSetPod creates a statefulset pod, input name should be a valid statefulset pod name like 'a-1'
func createStatefulSetPod(name, namespace string, annotations map[string]string) *corev1.Pod {
	parts := strings.Split(name, "-")
	quantity := resource.NewQuantity(1, resource.DecimalSI)
	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
			OwnerReferences: []v1.OwnerReference{{
				Kind: "StatefulSet",
				Name: strings.Join(parts[:len(parts)-1], "-"),
			}}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceName(constant.ResourceName): *quantity},
				},
			}},
		},
	}
}

func createNode(name string, labels map[string]string, address string) corev1.Node {
	return corev1.Node{
		ObjectMeta: v1.ObjectMeta{Name: name, Labels: labels},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: address}}},
	}
}

func createPluginFactoryArgs(t *testing.T, objs ...runtime.Object) (*PluginFactoryArgs, chan struct{}) {
	client := fake.NewSimpleClientset(objs...)
	informerFactory := informers.NewFilteredSharedInformerFactory(client, time.Minute, v1.NamespaceAll, nil)
	podInformer := informerFactory.Core().V1().Pods()
	statefulsetInformer := informerFactory.Apps().V1().StatefulSets()
	deploymentInformer := informerFactory.Apps().V1().Deployments()
	stopChan := make(chan struct{})
	go func() {
		informerFactory.Start(stopChan)
	}()
	pluginArgs := &PluginFactoryArgs{
		PodLister:         podInformer.Lister(),
		StatefulSetLister: statefulsetInformer.Lister(),
		DeploymentLister:  deploymentInformer.Lister(),
		Client:            client,
		PodHasSynced:      podInformer.Informer().HasSynced,
		StatefulSetSynced: statefulsetInformer.Informer().HasSynced,
		DeploymentSynced:  deploymentInformer.Informer().HasSynced,
	}
	if err := wait.PollImmediate(time.Millisecond*100, 20*time.Second, func() (done bool, err error) {
		return pluginArgs.PodHasSynced(), nil
	}); err != nil {
		t.Fatal(err)
	}
	return pluginArgs, stopChan
}

func newPlugin(t *testing.T, conf Conf, objs ...runtime.Object) (*FloatingIPPlugin, chan struct{}) {
	pluginArgs, stopChan := createPluginFactoryArgs(t, objs...)
	fipPlugin, err := NewFloatingIPPlugin(conf, pluginArgs)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	if err := fipPlugin.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf("TRUNCATE %s;", database.DefaultFloatingipTableName)).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err = fipPlugin.Init(); err != nil {
		t.Fatal(err)
	}
	return fipPlugin, stopChan
}

func TestLoadConfigMap(t *testing.T) {
	pod1 := createStatefulSetPodWithLabels("pod1", "demo", nil)
	pod2 := createStatefulSetPodWithLabels("pod1", "demo", secondIPLabel) // want second ips
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: "testConf", Namespace: "demo"},
		Data: map[string]string{
			"key": `[{"routableSubnet":"10.49.27.0/24","ips":["10.49.27.216~10.49.27.218"],"subnet":"10.49.27.0/24","gateway":"10.49.27.1","vlan":2}]`,
		},
	}
	var conf Conf
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
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
	if fipPlugin.enabledSecondIP(pod1) || fipPlugin.enabledSecondIP(pod2) {
		t.Error("plugin has no second ip configs")
	}

	// test secondips
	cm.Data["secondKey"] = `[{"routableSubnet":"10.173.13.0/24","ips":["10.173.13.15"],"subnet":"10.173.13.0/24","gateway":"10.173.13.1"}]`
	conf.SecondFloatingIPKey = "secondKey"
	fipPlugin, stopChan2 := newPlugin(t, conf, cm)
	defer func() { stopChan2 <- struct{}{} }()
	if fipPlugin.lastIPConf != cm.Data["key"] {
		t.Errorf(fipPlugin.lastIPConf)
	}
	if fipPlugin.lastSecondIPConf != cm.Data["secondKey"] {
		t.Errorf(fipPlugin.lastIPConf)
	}
	if fipPlugin.enabledSecondIP(pod1) || !fipPlugin.enabledSecondIP(pod2) {
		t.Error("pod1 doesn't want second ip, but pod2 does")
	}
}

func TestBind(t *testing.T) {
	node := createNode("node1", nil, "10.49.27.2")
	pod1 := createStatefulSetPod("sts1-1", "demo", nil)
	var conf Conf
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	fipPlugin, stopChan := newPlugin(t, conf, pod1, &node)
	defer func() { stopChan <- struct{}{} }()
	if err := fipPlugin.Bind(&schedulerapi.ExtenderBindingArgs{
		PodName:      pod1.Name,
		PodNamespace: pod1.Namespace,
		Node:         node.Name,
	}); err != nil {
		t.Fatal(err)
	}
	fakePods := fipPlugin.PluginFactoryArgs.Client.CoreV1().Pods(pod1.Namespace).(*fakeV1.FakePods)

	actualBinding, err := fakePods.GetBinding(pod1.GetName())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
		return
	}
	expect := &corev1.Binding{
		ObjectMeta: v1.ObjectMeta{
			Namespace: pod1.Namespace, Name: pod1.Name,
			Annotations: map[string]string{
				constant.ExtendedCNIArgsAnnotation: `{"common":{"ipinfos":[{"ip":"10.49.27.205/24","vlan":2,"gateway":"10.49.27.1","routable_subnet":"10.49.27.0/24"}]}}`}},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: node.Name,
		},
	}
	if !reflect.DeepEqual(expect, actualBinding) {
		t.Errorf("Binding did not match expectation")
		t.Logf("Expected: %v", expect)
		t.Logf("Actual:   %v", actualBinding)
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
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{constant.ReleasePolicyAnnotation: constant.Immutable}},
			expect: constant.ReleasePolicyImmutable,
		},
		{
			meta:   &v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{constant.ReleasePolicyAnnotation: constant.Never}},
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

func TestUnBind(t *testing.T) {
	pod1 := createStatefulSetPod("pod1-1", "demo", map[string]string{})
	keyObj := formatKey(pod1)
	node := createNode("TestUnBindNode", nil, "10.173.13.4")
	var conf Conf
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	fipPlugin, stopChan := newPlugin(t, conf, pod1, &node)
	defer func() { stopChan <- struct{}{} }()
	fipPlugin.cloudProvider = &fakeCloudProvider{}
	// if a pod has not got cni args annotation, unbind should return nil
	if err := fipPlugin.unbind(pod1); err != nil {
		t.Fatal(err)
	}
	// if a pod has got bad cni args annotation, unbind should return error
	pod1.Annotations[constant.ExtendedCNIArgsAnnotation] = "fff"
	if err := fipPlugin.unbind(pod1); err == nil {
		t.Fatal(err)
	}

	// bind before testing normal unbind
	fakeCP := &fakeCloudProvider{expectIP: "10.173.13.2", expectNode: node.Name}
	fipPlugin.cloudProvider = fakeCP
	if err := fipPlugin.Bind(&schedulerapi.ExtenderBindingArgs{
		PodName:      pod1.Name,
		PodNamespace: pod1.Namespace,
		Node:         node.Name,
	}); err != nil {
		t.Fatal(err)
	}
	fipInfo, err := fipPlugin.ipam.First(keyObj.keyInDB)
	if err != nil {
		t.Fatal(err)
	}
	if fipInfo == nil {
		t.Fatal("expect 10.173.13.2")
	}
	if fipInfo.IPInfo.IP.IP.String() != "10.173.13.2" {
		t.Fatalf("real IP: %s, expect 10.173.13.2", fipInfo.IPInfo.IP.IP.String())
	}
	str, err := constant.FormatIPInfo([]constant.IPInfo{fipInfo.IPInfo})
	if err != nil {
		t.Fatal(err)
	}
	pod1.Annotations[constant.ExtendedCNIArgsAnnotation] = str
	pod1.Spec.NodeName = node.Name
	if err := fipPlugin.unbind(pod1); err != nil {
		t.Fatal(err)
	}
	if !fakeCP.invokedAssignIP || !fakeCP.invokedUnAssignIP {
		t.Fatal()
	}
}
