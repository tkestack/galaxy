package k8s

import (
	"testing"
)

func TestParsePodNetworkAnnotation(t *testing.T) {
	case1 := "test-ns/galaxy-flannel@eth0, test-ns/galaxy-k8s-vlan@eth1"
	res1, err := ParsePodNetworkAnnotation(case1)
	if err != nil {
		t.Errorf("case1 fail: %v", err)
	}
	if len(res1) == 2 {
		if res1[0].Name == "galaxy-flannel" && res1[0].InterfaceRequest == "eth0" {
			t.Log("case1: network1 parse success")
		} else {
			t.Errorf("network1 %s@%s not like galaxy-flannel@eth0", res1[0].Name, res1[0].InterfaceRequest)
		}
		if res1[1].Name == "galaxy-k8s-vlan" && res1[1].InterfaceRequest == "eth1" {
			t.Log("case1: network2 parse success")
		} else {
			t.Errorf("network2 %s@%s not like galaxy-flannel@eth1", res1[1].Name, res1[1].InterfaceRequest)
		}
	} else {
		t.Errorf("case1 network num not 2")
	}

	case3 := "test-ns/galaxy-flannel"
	res3, err := ParsePodNetworkAnnotation(case3)
	if err == nil {
		if len(res3) == 1 {
			if res3[0].Name == "galaxy-flannel" && res3[0].InterfaceRequest == "" {
				t.Log("case3 pass")
			} else {
				t.Errorf("case3 network isn't galaxy-flannel@{empty}")
			}
		} else {
			t.Errorf("case3 parse failed: wrong network num")
		}
	} else {
		t.Errorf("case3 parse failed")
	}
}
