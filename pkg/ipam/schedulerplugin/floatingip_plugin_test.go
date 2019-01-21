package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	nodeUnlabeld, nodeHasNoIP, node3, node4 = "node1", "node2", "node3", "node4"
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
	fipPlugin, err := NewFloatingIPPlugin(conf, &PluginFactoryArgs{
		PodHasSynced: func() bool { return false },
	})
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	if err = fipPlugin.Init(); err != nil {
		t.Fatal(err)
	}
	nodeLabel := make(map[string]string)
	nodeLabel[private.LabelKeyNetworkType] = private.NodeLabelValueNetworkTypeFloatingIP
	objectLabel := make(map[string]string)
	objectLabel[private.LabelKeyNetworkType] = private.LabelValueNetworkTypeFloatingIP
	immutableLabel := make(map[string]string)
	immutableLabel[private.LabelKeyFloatingIP] = private.LabelValueImmutable
	immutableLabel[private.LabelKeyNetworkType] = private.LabelValueNetworkTypeFloatingIP
	nodes := []corev1.Node{
		// no floating ip label node
		{
			ObjectMeta: v1.ObjectMeta{Name: nodeUnlabeld},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.49.27.2"}}},
		},
		// no floating ip configured for this node
		{
			ObjectMeta: v1.ObjectMeta{Name: nodeHasNoIP, Labels: nodeLabel},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.48.27.2"}}},
		},
		// good node
		{
			ObjectMeta: v1.ObjectMeta{Name: node3, Labels: nodeLabel},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.49.27.3"}}},
		},
		// good node
		{
			ObjectMeta: v1.ObjectMeta{Name: node4, Labels: nodeLabel},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.173.13.4"}}},
		},
	}
	_, subnet1, _ := net.ParseCIDR("10.49.27.0/24")
	_, subnet2, _ := net.ParseCIDR("10.48.27.0/24")
	_, subnet3, _ := net.ParseCIDR("10.173.13.0/24")
	fipPlugin.nodeSubnet[nodeUnlabeld] = subnet1
	fipPlugin.nodeSubnet[nodeHasNoIP] = subnet2
	fipPlugin.nodeSubnet[node3] = subnet1
	fipPlugin.nodeSubnet[node4] = subnet3
	// cleans all allocates first
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
	if ipInfo, err := fipPlugin.ipam.QueryFirst(keyInDB(pod)); err != nil || ipInfo == nil {
		t.Fatal(err, ipInfo)
	} else {
		if ipInfo.IP.String() != "10.173.13.2/24" {
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
	t.Log(ipInfoSet)
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
	deployLabel := immutableLabel
	fipPlugin.getDeployment = func(name, namespace string) (*appv1.Deployment, error) {
		return &appv1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    deployLabel,
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
	//if err = reserveDeploymentIP(fipPlugin.ipam, keyForDeploymentPod(deadPod, "dp"), deploymentPrefix("dp", "ns1")); err != nil {
	//	t.Fatal(err)
	//}
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
	deployLabel = neverLabel
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
