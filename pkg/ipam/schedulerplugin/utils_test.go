package schedulerplugin

import (
	"reflect"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveDpKey(t *testing.T) {
	tests := map[string][]string{"dp_default_dp1_dp1-rs1-pod1": {"default", "dp1", "dp1-rs1-pod1"}, "sts_default_fip_fip-0": {"", "", ""}}
	for k, v := range tests {
		appname, podName, namespace := resolveDpKey(k)
		if namespace != v[0] {
			t.Fatal(namespace)
		}
		if appname != v[1] {
			t.Fatal(appname)
		}
		if podName != v[2] {
			t.Fatal(podName)
		}
	}
}

func TestResolveStsKey(t *testing.T) {
	tests := map[string][]string{"sts_default_fip_fip-0": {"default", "fip", "fip-0"}, "sts_kube-system_fip-bj_fip-bj-111": {"kube-system", "fip-bj", "fip-bj-111"}, "dp_default_dp1_dp1-rs1-pod1": {"", "", ""}}
	for k, v := range tests {
		appName, podName, namespace := resolveStsKey(k)
		if namespace != v[0] {
			t.Fatal(namespace)
		}
		if appName != v[1] {
			t.Fatal(appName)
		}
		if podName != v[2] {
			t.Fatal(podName)
		}
	}
}

func TestFormatKey(t *testing.T) {
	testCases := []struct {
		pod                 *corev1.Pod
		expect              keyObj
		expectPoolPrefix    string
		expectPoolAppPrefix string
	}{
		{
			pod: createStatefulSetPod("sts-1", "ns1", nil),
			expect: keyObj{
				keyInDB:      "sts_ns1_sts_sts-1",
				isDeployment: false,
				appName:      "sts",
				podName:      "sts-1",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix:    "sts_ns1_sts_",
			expectPoolAppPrefix: "sts_ns1_sts_",
		},
		{
			pod: createStatefulSetPod("sts-1", "ns1", map[string]string{constant.IPPoolAnnotation: "pl1"}),
			expect: keyObj{
				keyInDB:      "pool__pl1_sts_ns1_sts_sts-1",
				isDeployment: false,
				appName:      "sts",
				podName:      "sts-1",
				namespace:    "ns1",
				poolName:     "pl1",
			},
			expectPoolPrefix:    "pool__pl1_",
			expectPoolAppPrefix: "pool__pl1_sts_ns1_sts",
		},
		{
			pod: createDeploymentPod("dp-xxx-yyy", "ns1", nil),
			expect: keyObj{
				keyInDB:      "dp_ns1_dp_dp-xxx-yyy",
				isDeployment: true,
				appName:      "dp",
				podName:      "dp-xxx-yyy",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix:    "dp_ns1_dp_",
			expectPoolAppPrefix: "dp_ns1_dp_",
		},
		{
			pod: createDeploymentPod("dp-xxx-yyy", "ns1", map[string]string{constant.IPPoolAnnotation: "pl1"}),
			expect: keyObj{
				keyInDB:      "pool__pl1_dp_ns1_dp_dp-xxx-yyy",
				isDeployment: true,
				appName:      "dp",
				podName:      "dp-xxx-yyy",
				namespace:    "ns1",
				poolName:     "pl1",
			},
			expectPoolPrefix:    "pool__pl1_",
			expectPoolAppPrefix: "pool__pl1_dp_ns1_dp_",
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := formatKey(testCase.pod)
		if got == nil {
			t.Fatal()
		}
		if !reflect.DeepEqual(*got, testCase.expect) {
			t.Errorf("case %d, expect %+v, got %+v", i, testCase.expect, *got)
		}
		if testCase.expectPoolPrefix != got.poolPrefix() {
			t.Errorf("case %d, expect %+v, got %+v", i, testCase.expectPoolPrefix, got.poolPrefix())
		}
	}
}

func TestParseKey(t *testing.T) {
	testCases := []struct {
		expect           keyObj
		expectPoolPrefix string
		keyInDB          string
	}{
		// statefulset pod key
		{
			keyInDB: "sts_ns1_demo_demo-1",
			expect: keyObj{
				keyInDB:      "sts_ns1_demo_demo-1",
				isDeployment: false,
				appName:      "demo",
				podName:      "demo-1",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix: "sts_ns1_demo_",
		},
		// statefulset pod key
		{
			keyInDB: "sts_ns1_sts-demo_sts-demo-1",
			expect: keyObj{
				keyInDB:      "sts_ns1_sts-demo_sts-demo-1",
				isDeployment: false,
				appName:      "sts-demo",
				podName:      "sts-demo-1",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix: "sts_ns1_sts-demo_",
		},
		// pool statefulset pod key
		{
			keyInDB: "pool__pl1_sts_ns1_demo_demo-1",
			expect: keyObj{
				keyInDB:      "pool__pl1_sts_ns1_demo_demo-1",
				isDeployment: false,
				appName:      "demo",
				podName:      "demo-1",
				namespace:    "ns1",
				poolName:     "pl1",
			},
			expectPoolPrefix: "pool__pl1_",
		},
		// statefulset key
		{
			keyInDB: "sts_ns1_demo_",
			expect: keyObj{
				keyInDB:      "sts_ns1_demo_",
				isDeployment: false,
				appName:      "demo",
				podName:      "",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix: "sts_ns1_demo_",
		},
		// pool key
		{
			keyInDB: "pool__pl1_",
			expect: keyObj{
				keyInDB:      "pool__pl1_",
				isDeployment: false,
				appName:      "",
				podName:      "",
				namespace:    "",
				poolName:     "pl1",
			},
			expectPoolPrefix: "pool__pl1_",
		},
		// deployment pod key
		{
			keyInDB: "dp_ns1_dp_dp-xxx-yyy",
			expect: keyObj{
				keyInDB:      "dp_ns1_dp_dp-xxx-yyy",
				isDeployment: true,
				appName:      "dp",
				podName:      "dp-xxx-yyy",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix: "dp_ns1_dp_",
		},
		// deployment key
		{
			keyInDB: "dp_ns1_dp_",
			expect: keyObj{
				keyInDB:      "dp_ns1_dp_",
				isDeployment: true,
				appName:      "dp",
				podName:      "",
				namespace:    "ns1",
				poolName:     "",
			},
			expectPoolPrefix: "dp_ns1_dp_",
		},
		// pool deployment pod key
		{
			keyInDB: "pool__pl1_dp_ns1_dp_dp-xxx-yyy",
			expect: keyObj{
				keyInDB:      "pool__pl1_dp_ns1_dp_dp-xxx-yyy",
				isDeployment: true,
				appName:      "dp",
				podName:      "dp-xxx-yyy",
				namespace:    "ns1",
				poolName:     "pl1",
			},
			expectPoolPrefix: "pool__pl1_",
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := parseKey(testCase.keyInDB)
		if got == nil {
			t.Fatal()
		}
		if !reflect.DeepEqual(*got, testCase.expect) {
			t.Errorf("case %d, expect %+v, got %+v", i, testCase.expect, *got)
		}
		if testCase.expectPoolPrefix != got.poolPrefix() {
			t.Errorf("case %d, PoolPrefix expect %+v, got %+v", i, testCase.expectPoolPrefix, got.poolPrefix())
		}
	}
}

func TestResolveDeploymentName(t *testing.T) {
	longNamePod := createDeploymentPod("dp1234567890dp1234567890dp1234567890dp1234567890dp1234567848p74", "ns1", nil)
	longNamePod.OwnerReferences = []v1.OwnerReference{{
		Kind: "ReplicaSet",
		Name: "dp1234567890dp1234567890dp1234567890dp1234567890dp1234567890dp1-69fd8dbc5c",
	}}
	testCases := []struct {
		pod    *corev1.Pod
		expect string
	}{
		{pod: createDeploymentPod("dp1-1-2", "ns1", nil), expect: "dp1"},
		{pod: createDeploymentPod("dp2-1-1-2", "ns1", nil), expect: "dp2-1"},
		{pod: createDeploymentPod("baddp-2", "ns1", nil), expect: ""},
		{pod: longNamePod, expect: "dp1234567890dp1234567890dp1234567890dp1234567890dp1234567890dp1"},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := resolveDeploymentName(testCase.pod)
		if got != testCase.expect {
			t.Errorf("case %d, expect %v, got %v", i, testCase.expect, got)
		}
	}
}
