package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"github.com/jinzhu/gorm"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	fakeV1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
)

const (
	nodeUnlabeld, nodeHasNoIP, node3, node4 = "node1", "node2", "node3", "node4"
)

var (
	objectLabel    = map[string]string{private.LabelKeyNetworkType: private.LabelValueNetworkTypeFloatingIP}
	secondIPLabel  = map[string]string{private.LabelKeyNetworkType: private.LabelValueNetworkTypeFloatingIP, private.LabelKeyEnableSecondIP: private.LabelValueEnabled}
	immutableLabel = map[string]string{private.LabelKeyNetworkType: private.LabelValueNetworkTypeFloatingIP, private.LabelKeyFloatingIP: private.LabelValueImmutable}
	nodeLabel      = map[string]string{private.LabelKeyNetworkType: private.NodeLabelValueNetworkTypeFloatingIP}
)

func TestFilter(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	var conf Conf
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	nodes := []corev1.Node{
		createNode(nodeUnlabeld, nil, "10.49.27.2"),      // no floating ip label node
		createNode(nodeHasNoIP, nodeLabel, "10.49.28.2"), // no floating ip configured for this node
		createNode(node3, nodeLabel, "10.49.27.3"),       // good node
		createNode(node4, nodeLabel, "10.173.13.4"),      // good node
	}
	fipPlugin, stopChan := newPlugin(t, conf, &nodes[0], &nodes[1], &nodes[2], &nodes[3])
	defer func() { stopChan <- struct{}{} }()
	if err := fipPlugin.ipam.ReleaseByPrefix(""); err != nil {
		t.Fatal(err)
	}
	// pod doesn't have floating ip label, filter should return all nodes
	filtered, failed, err := fipPlugin.Filter(createPod("pod1", "ns1", nil), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, nodeUnlabeld, nodeHasNoIP, node3, node4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed); err != nil {
		t.Fatal(err)
	}
	// a pod has floating ip label, filter should return nodes that has floating ips
	if filtered, failed, err = fipPlugin.Filter(createPod("pod1", "ns1", objectLabel), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node3, node4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed, nodeUnlabeld, nodeHasNoIP); err != nil {
		t.Fatal(err)
	}

	// the following is to check release policy
	// allocate a ip of 10.173.13.0/24
	_, ipNet, _ := net.ParseCIDR("10.173.13.0/24")
	pod := createPod("pod1-0", "ns1", immutableLabel)
	if ipInfo, err := fipPlugin.ipam.AllocateInSubnet(keyInDB(pod), ipNet, database.PodDelete, ""); err != nil || ipInfo == nil || "10.173.13.2" != ipInfo.String() {
		t.Fatal(err, ipInfo)
	}
	if err := checkPolicyAndAttr(fipPlugin.ipam, keyInDB(pod), database.PodDelete, expectAttrEmpty()); err != nil {
		t.Fatal(err)
	}
	// check filter result is expected
	if filtered, failed, err = fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed, nodeUnlabeld, nodeHasNoIP, node3); err != nil {
		t.Fatal(err)
	}
	// check pod allocated the previous ip and policy should be updated to AppDeleteOrScaleDown
	pod.Spec.NodeName = node4
	ipInfo, err := fipPlugin.allocateIP(fipPlugin.ipam, keyInDB(pod), pod.Spec.NodeName, pod)
	if err != nil {
		t.Fatal(err)
	}
	if ipInfo == nil || ipInfo.IP.String() != "10.173.13.2/24" {
		t.Fatal(ipInfo)
	}
	if err := checkPolicyAndAttr(fipPlugin.ipam, keyInDB(pod), database.AppDeleteOrScaleDown, expectAttrNotEmpty()); err != nil {
		t.Fatal(err)
	}

	// filter again on a new pod2, all good nodes should be filteredNodes
	if filtered, failed, err = fipPlugin.Filter(createPod("pod2-1", "ns1", immutableLabel), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node3, node4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed, nodeUnlabeld, nodeHasNoIP); err != nil {
		t.Fatal(err)
	}
	// forget the pod1, the ip should be reserved, because pod1 has immutable label attached
	if err := fipPlugin.DeletePod(pod); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := fipPlugin.ipam.First(keyInDB(pod)); err != nil || ipInfo == nil {
		t.Fatal(err, ipInfo)
	} else {
		if ipInfo.IPInfo.IP.String() != "10.173.13.2/24" {
			t.Fatal(ipInfo)
		}
	}
	// allocates all ips to pods of a new  statefulset
	newPod := createPod("temp", "ns1", immutableLabel)
	newPod.Spec.NodeName = node4
	ipInfoSet := sets.NewString()
	for i := 0; ; i++ {
		newPod.Name = fmt.Sprintf("temp-%d", i)
		if ipInfo, err := fipPlugin.allocateIP(fipPlugin.ipam, keyInDB(newPod), newPod.Spec.NodeName, newPod); err != nil {
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
	if ipInfo, err = fipPlugin.allocateIP(fipPlugin.ipam, keyInDB(pod), pod.Spec.NodeName, pod); err != nil {
		t.Fatal(err)
	}
	if ipInfo == nil || ipInfo.IP.String() != "10.173.13.2/24" {
		t.Fatal(ipInfo)
	}
	if err := checkPolicyAndAttr(fipPlugin.ipam, keyInDB(pod), database.AppDeleteOrScaleDown, expectAttrNotEmpty()); err != nil {
		t.Fatal(err)
	}
	pod.Status.PodIP = "10.173.13.2"
	pod.Status.Phase = corev1.PodRunning
	if err := fipPlugin.releaseIP(keyInDB(pod), "", pod); err != nil {
		t.Fatal(err)
	}
	if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
		t.Fatal(err)
	} else if fip.Key != "" {
		t.Fatal("failed release ip 10.173.13.2")
	}
	fipPlugin.UpdatePod(pod, pod)
	if fip, err := fipPlugin.ipam.ByIP(net.ParseIP("10.173.13.2")); err != nil {
		t.Fatal(err)
	} else if fip.Key != keyInDB(pod) {
		t.Fatal("failed resync ip 10.173.13.2")
	}

	// pre-allocate ip in filter for deployment pod
	deadPod := createPod("dp-aaa-bbb", "ns1", immutableLabel)
	pod = createPod("dp-xxx-yyy", "ns1", immutableLabel)
	pod.OwnerReferences = append(pod.OwnerReferences, v1.OwnerReference{
		Kind: "ReplicaSet",
	})
	deadPod.OwnerReferences = append(deadPod.OwnerReferences, v1.OwnerReference{
		Kind: "ReplicaSet",
	})
	var replicas int32 = 1
	fipPlugin.getDeployment = func(name, namespace string) (*appv1.Deployment, error) {
		return &appv1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: pod.ObjectMeta,
				},
				Replicas: &replicas,
			},
		}, nil
	}
	if err := fipPlugin.ipam.ReleaseByPrefix(""); err != nil {
		t.Fatal(err)
	}
	fip, err := fipPlugin.allocateIP(fipPlugin.ipam, keyForDeploymentPod(deadPod, "dp"), node3, deadPod)
	if err != nil {
		t.Fatal(err)
	}
	if filteredNodes, _, err := fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	} else if len(filteredNodes) != 0 {
		t.Fatalf("shoult has no node for deployment, wait for release, but got %v", filteredNodes)
	}
	// because replicas = 1, ip will be reserved
	if err := fipPlugin.unbind(deadPod); err != nil {
		t.Fatal(err)
	}
	if _, failedNodes, err := fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	} else if err = checkFailed(failedNodes, nodeUnlabeld, nodeHasNoIP, node4); err != nil {
		t.Fatal(err)
	}
	fip2, err := fipPlugin.ipam.First(keyForDeploymentPod(pod, "dp"))
	if err != nil {
		t.Fatal(err)
	} else if fip.IP.String() != fip2.IPInfo.IP.String() {
		t.Fatalf("allocate another ip, expect reserved one")
	}

	neverLabel := make(map[string]string)
	neverLabel[private.LabelKeyFloatingIP] = private.LabelValueNeverRelease
	neverLabel[private.LabelKeyNetworkType] = private.LabelValueNetworkTypeFloatingIP
	pod.Labels[private.LabelKeyFloatingIP] = private.LabelValueNeverRelease
	deadPod.Labels[private.LabelKeyFloatingIP] = private.LabelValueNeverRelease
	// when replicas = 0 and never release policy, ip will be reserved
	replicas = 0
	if err := fipPlugin.unbind(pod); err != nil {
		t.Fatal(err)
	}
	replicas = 1
	if _, failedNodes, err := fipPlugin.Filter(deadPod, nodes); err != nil {
		t.Fatal(err)
	} else if err = checkFailed(failedNodes, nodeUnlabeld, nodeHasNoIP, node4); err != nil {
		t.Fatal(err)
	}
	fip3, err := fipPlugin.ipam.First(keyForDeploymentPod(deadPod, "dp"))
	if err != nil {
		t.Fatal(err)
	} else if fip.IP.String() != fip3.IPInfo.IP.String() {
		t.Fatalf("allocate another ip, expect reserved one")
	}
}

// Attr has a time field which makes it hard to check, so creating this struct to do part check
type expectAttr struct {
	empty    bool
	contains []string
}

func expectAttrContains(substr ...string) expectAttr {
	return expectAttr{contains: substr}
}

func expectAttrEmpty() expectAttr {
	return expectAttr{empty: true}
}

func expectAttrNotEmpty() expectAttr {
	return expectAttr{empty: false}
}

func checkPolicyAndAttr(ipam floatingip.IPAM, key string, expectPolicy database.ReleasePolicy, expectAttr expectAttr) error {
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

func createPod(name, namespace string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}}
}

func createNode(name string, labels map[string]string, address string) corev1.Node {
	return corev1.Node{
		ObjectMeta: v1.ObjectMeta{Name: name, Labels: labels},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: address}}},
	}
}

func TestResolveAppPodName(t *testing.T) {
	tests := map[string][]string{"default_fip-0": {"default", "fip", "0"}, "kube-system_fip-bj-111": {"kube-system", "fip-bj", "111"}, "_deployment_default_dp1_dp1-rs1-pod1": {"", "", ""}}
	for k, v := range tests {
		appname, podId, namespace := resolveAppPodName(k)
		if namespace != v[0] {
			t.Fatal(namespace)
		}
		if appname != v[1] {
			t.Fatal(appname)
		}
		if podId != v[2] {
			t.Fatal(podId)
		}
	}
	tests = map[string][]string{"_deployment_default_dp1_dp1-rs1-pod1": {"default", "dp1", "dp1-rs1-pod1"}}
	for k, v := range tests {
		appname, podId, namespace := resolveDpAppPodName(k)
		if namespace != v[0] {
			t.Fatal(namespace)
		}
		if appname != v[1] {
			t.Fatal(appname)
		}
		if podId != v[2] {
			t.Fatal(podId)
		}
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
	pod1 := createPod("pod1", "demo", objectLabel)
	pod2 := createPod("pod1", "demo", secondIPLabel) // want second ips
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
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	node := createNode("node1", nil, "10.49.27.2")
	pod1 := createPod("pod1", "demo", objectLabel)
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
				private.AnnotationKeyIPInfo: `{"ip":"10.49.27.205/24","vlan":2,"gateway":"10.49.27.1","routable_subnet":"10.49.27.0/24"}`}},
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
