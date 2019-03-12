package util

import (
	"reflect"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

func TestResolveDpKey(t *testing.T) {
	tests := map[string][]string{"dp_default_dp1_dp1-rs1-pod1": {"default", "dp1", "dp1-rs1-pod1"}, "sts_default_fip_fip-0": {"", "", ""}}
	for k, v := range tests {
		appname, podName, namespace := ResolveDpKey(k)
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
		expect              KeyObj
		expectPoolPrefix    string
		expectPoolAppPrefix string
	}{
		{
			pod: createStatefulSetPod("sts-1", "ns1", nil),
			expect: KeyObj{
				KeyInDB:      "sts_ns1_sts_sts-1",
				IsDeployment: false,
				AppName:      "sts",
				PodName:      "sts-1",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix:    "sts_ns1_sts_",
			expectPoolAppPrefix: "sts_ns1_sts_",
		},
		{
			pod: createStatefulSetPod("sts-1", "ns1", map[string]string{constant.IPPoolAnnotation: "pl1"}),
			expect: KeyObj{
				KeyInDB:      "pool__pl1_sts_ns1_sts_sts-1",
				IsDeployment: false,
				AppName:      "sts",
				PodName:      "sts-1",
				Namespace:    "ns1",
				PoolName:     "pl1",
			},
			expectPoolPrefix:    "pool__pl1_",
			expectPoolAppPrefix: "pool__pl1_sts_ns1_sts",
		},
		{
			pod: createDeploymentPod("dp-xxx-yyy", "ns1", nil),
			expect: KeyObj{
				KeyInDB:      "dp_ns1_dp_dp-xxx-yyy",
				IsDeployment: true,
				AppName:      "dp",
				PodName:      "dp-xxx-yyy",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix:    "dp_ns1_dp_",
			expectPoolAppPrefix: "dp_ns1_dp_",
		},
		{
			pod: createDeploymentPod("dp-xxx-yyy", "ns1", map[string]string{constant.IPPoolAnnotation: "pl1"}),
			expect: KeyObj{
				KeyInDB:      "pool__pl1_dp_ns1_dp_dp-xxx-yyy",
				IsDeployment: true,
				AppName:      "dp",
				PodName:      "dp-xxx-yyy",
				Namespace:    "ns1",
				PoolName:     "pl1",
			},
			expectPoolPrefix:    "pool__pl1_",
			expectPoolAppPrefix: "pool__pl1_dp_ns1_dp_",
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := FormatKey(testCase.pod)
		if got == nil {
			t.Fatal()
		}
		if !reflect.DeepEqual(*got, testCase.expect) {
			t.Errorf("case %d, expect %+v, got %+v", i, testCase.expect, *got)
		}
		if testCase.expectPoolPrefix != got.PoolPrefix() {
			t.Errorf("case %d, expect %+v, got %+v", i, testCase.expectPoolPrefix, got.PoolPrefix())
		}
	}
}

func TestParseKey(t *testing.T) {
	testCases := []struct {
		expect           KeyObj
		expectPoolPrefix string
		keyInDB          string
	}{
		// statefulset pod key
		{
			keyInDB: "sts_ns1_demo_demo-1",
			expect: KeyObj{
				KeyInDB:      "sts_ns1_demo_demo-1",
				IsDeployment: false,
				AppName:      "demo",
				PodName:      "demo-1",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix: "sts_ns1_demo_",
		},
		// statefulset pod key
		{
			keyInDB: "sts_ns1_sts-demo_sts-demo-1",
			expect: KeyObj{
				KeyInDB:      "sts_ns1_sts-demo_sts-demo-1",
				IsDeployment: false,
				AppName:      "sts-demo",
				PodName:      "sts-demo-1",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix: "sts_ns1_sts-demo_",
		},
		// pool statefulset pod key
		{
			keyInDB: "pool__pl1_sts_ns1_demo_demo-1",
			expect: KeyObj{
				KeyInDB:      "pool__pl1_sts_ns1_demo_demo-1",
				IsDeployment: false,
				AppName:      "demo",
				PodName:      "demo-1",
				Namespace:    "ns1",
				PoolName:     "pl1",
			},
			expectPoolPrefix: "pool__pl1_",
		},
		// statefulset key
		{
			keyInDB: "sts_ns1_demo_",
			expect: KeyObj{
				KeyInDB:      "sts_ns1_demo_",
				IsDeployment: false,
				AppName:      "demo",
				PodName:      "",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix: "sts_ns1_demo_",
		},
		// pool key
		{
			keyInDB: "pool__pl1_",
			expect: KeyObj{
				KeyInDB:      "pool__pl1_",
				IsDeployment: false,
				AppName:      "",
				PodName:      "",
				Namespace:    "",
				PoolName:     "pl1",
			},
			expectPoolPrefix: "pool__pl1_",
		},
		// deployment pod key
		{
			keyInDB: "dp_ns1_dp_dp-xxx-yyy",
			expect: KeyObj{
				KeyInDB:      "dp_ns1_dp_dp-xxx-yyy",
				IsDeployment: true,
				AppName:      "dp",
				PodName:      "dp-xxx-yyy",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix: "dp_ns1_dp_",
		},
		// deployment key
		{
			keyInDB: "dp_ns1_dp_",
			expect: KeyObj{
				KeyInDB:      "dp_ns1_dp_",
				IsDeployment: true,
				AppName:      "dp",
				PodName:      "",
				Namespace:    "ns1",
				PoolName:     "",
			},
			expectPoolPrefix: "dp_ns1_dp_",
		},
		// pool deployment pod key
		{
			keyInDB: "pool__pl1_dp_ns1_dp_dp-xxx-yyy",
			expect: KeyObj{
				KeyInDB:      "pool__pl1_dp_ns1_dp_dp-xxx-yyy",
				IsDeployment: true,
				AppName:      "dp",
				PodName:      "dp-xxx-yyy",
				Namespace:    "ns1",
				PoolName:     "pl1",
			},
			expectPoolPrefix: "pool__pl1_",
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := ParseKey(testCase.keyInDB)
		if got == nil {
			t.Fatal()
		}
		if !reflect.DeepEqual(*got, testCase.expect) {
			t.Errorf("case %d, expect %+v, got %+v", i, testCase.expect, *got)
		}
		if testCase.expectPoolPrefix != got.PoolPrefix() {
			t.Errorf("case %d, PoolPrefix expect %+v, got %+v", i, testCase.expectPoolPrefix, got.PoolPrefix())
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

func createDeploymentPod(name, namespace string, annotation map[string]string) *corev1.Pod {
	parts := strings.Split(name, "-")
	pod := createStatefulSetPod(name, namespace, annotation)
	pod.OwnerReferences = []v1.OwnerReference{{
		Kind: "ReplicaSet",
		Name: strings.Join(parts[:len(parts)-1], "-"),
	}}
	return pod
}
