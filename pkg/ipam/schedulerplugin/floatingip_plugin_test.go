package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/util/sets"
)

var (
	node_unlabeld, node_hasNoIP, node_10_49_27_3, node_10_173_13_4 = "node1", "node2", "node3", "node4"
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
		PodHasSynced:  func() bool { return false },
		TAppHasSynced: func() bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeLabel := make(map[string]string)
	nodeLabel["network"] = "floatingip"
	objectLabel := make(map[string]string)
	objectLabel["network"] = "FLOATINGIP"
	fipInvariantLabel := make(map[string]string)
	fipInvariantLabel["floatingip"] = "invariant"
	fipInvariantLabel["network"] = "FLOATINGIP"
	nodes := []v1.Node{
		// no floating ip label node
		{
			ObjectMeta: v1.ObjectMeta{Name: node_unlabeld},
			Status:     v1.NodeStatus{Addresses: []v1.NodeAddress{{Type: v1.NodeInternalIP, Address: "10.49.27.2"}}},
		},
		// no floating ip configured node
		{
			ObjectMeta: v1.ObjectMeta{Name: node_hasNoIP, Labels: nodeLabel},
			Status:     v1.NodeStatus{Addresses: []v1.NodeAddress{{Type: v1.NodeInternalIP, Address: "10.48.27.2"}}},
		},
		// good node
		{
			ObjectMeta: v1.ObjectMeta{Name: node_10_49_27_3, Labels: nodeLabel},
			Status:     v1.NodeStatus{Addresses: []v1.NodeAddress{{Type: v1.NodeInternalIP, Address: "10.49.27.3"}}},
		},
		// good node
		{
			ObjectMeta: v1.ObjectMeta{Name: node_10_173_13_4, Labels: nodeLabel},
			Status:     v1.NodeStatus{Addresses: []v1.NodeAddress{{Type: v1.NodeInternalIP, Address: "10.173.13.4"}}},
		},
	}
	// cleans all allocates first
	if err := fipPlugin.ipam.ReleaseByPrefix(""); err != nil {
		t.Fatal(err)
	}
	// pod doesn't has floating ip label, filter should return all nodes
	filtered, failed, err := fipPlugin.Filter(createPod("pod1", "ns1", nil), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node_unlabeld, node_hasNoIP, node_10_49_27_3, node_10_173_13_4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed); err != nil {
		t.Fatal(err)
	}
	// a pod has floating ip label, filter should return nodes that has floating ips
	if filtered, failed, err = fipPlugin.Filter(createPod("pod1", "ns1", objectLabel), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node_10_49_27_3, node_10_173_13_4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed, node_unlabeld, node_hasNoIP); err != nil {
		t.Fatal(err)
	}
	// allocate a ip of 10.173.13.0/24
	_, ipNet, _ := net.ParseCIDR("10.173.13.0/24")
	if ipInfo, err := fipPlugin.ipam.AllocateInSubnet("ns1_pod1-0", ipNet); err != nil || ipInfo == nil || "10.173.13.2" != ipInfo.String() {
		t.Fatal(err, ipInfo)
	}
	// check filter result is expected
	pod := createPod("pod1-0", "ns1", fipInvariantLabel)
	if filtered, failed, err = fipPlugin.Filter(pod, nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node_10_173_13_4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed, node_unlabeld, node_hasNoIP, node_10_49_27_3); err != nil {
		t.Fatal(err)
	}
	// check pod allocated the previous ip
	pod.Spec.NodeName = node_10_173_13_4
	ipInfo, err := fipPlugin.allocateIP(keyInDB(pod), pod.Spec.NodeName)
	if err != nil {
		t.Fatal(err)
	}
	if len(ipInfo) == 0 || !strings.Contains(ipInfo["floatingip"], "10.173.13.2") {
		t.Fatal(ipInfo)
	}
	// filter again on a new pod2, all good nodes should be filteredNodes
	if filtered, failed, err = fipPlugin.Filter(createPod("pod2-1", "ns1", fipInvariantLabel), nodes); err != nil {
		t.Fatal(err)
	}
	if err := checkFiltered(filtered, node_10_49_27_3, node_10_173_13_4); err != nil {
		t.Fatal(err)
	}
	if err := checkFailed(failed, node_unlabeld, node_hasNoIP); err != nil {
		t.Fatal(err)
	}
	// forget the pod1, the ip should be reserved
	if err := fipPlugin.RemovePod(pod); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := fipPlugin.ipam.QueryFirst("ns1_pod1-0"); err != nil || ipInfo == nil {
		t.Fatal(err, ipInfo)
	} else {
		if ipInfo.IP.String() != "10.173.13.2/24" {
			t.Fatal(ipInfo)
		}
	}
	// allocates all ips to pods of a new  tapp
	newPod := createPod("temp", "ns1", fipInvariantLabel)
	newPod.Spec.NodeName = node_10_173_13_4
	ipInfoSet := sets.NewString()
	for i := 0; ; i++ {
		newPod.Name = fmt.Sprintf("temp-%d", i)
		if ipInfo, err := fipPlugin.allocateIP(keyInDB(newPod), newPod.Spec.NodeName); err != nil {
			if !strings.Contains(err.Error(), floatingip.ErrNoEnoughIP.Error()) {
				t.Fatal(err)
			}
			break
		} else {
			if ipInfoSet.Has(ipInfo["floatingip"]) {
				t.Fatal("allocates an previous allocated ip")
			}
			ipInfoSet.Insert(ipInfo["floatingip"])
		}
		if i == 10 {
			t.Fatal("should not have so many ips")
		}
	}
	t.Log(ipInfoSet)
	// see if we can allocate the reserved ip
	if ipInfo, err = fipPlugin.allocateIP(keyInDB(pod), pod.Spec.NodeName); err != nil {
		t.Fatal(err)
	}
	if len(ipInfo) == 0 || !strings.Contains(ipInfo["floatingip"], "10.173.13.2") {
		t.Fatal(ipInfo)
	}
}

func checkFiltered(realFilterd []v1.Node, filtererd ...string) error {
	expect := sets.NewString(filtererd...)
	if expect.Len() != len(realFilterd) {
		return fmt.Errorf("filtered nodes missmatch, expect %v, real %v", expect, realFilterd)
	}
	for i := range realFilterd {
		if !expect.Has(realFilterd[i].Name) {
			return fmt.Errorf("filtered nodes missmatch, expect %v, real %v", expect, realFilterd)
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

func createPod(name, namespace string, labels map[string]string) *v1.Pod {
	return &v1.Pod{ObjectMeta: v1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}}
}

func TestResolveTAppPodName(t *testing.T) {
	tests := map[string][]string{"default_fip-0": {"default", "fip", "0"}, "kube-system_fip-bj-111": {"kube-system", "fip-bj", "1111"}}
	for k, v := range tests {
		tappname, podId, namespace := resolveTAppPodName(k)
		if namespace != v[0] {
			t.Fatal(namespace)
		}
		if tappname != v[1] {
			t.Fatal(tappname)
		}
		if podId != v[2] {
			t.Fatal(podId)
		}
	}
}
