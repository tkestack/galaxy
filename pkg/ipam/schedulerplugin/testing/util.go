package testing

import (
	"strings"

	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateStatefulSetPodWithLabels creates a statefulset pod with labels as `labels` for testing
func CreateStatefulSetPodWithLabels(name, namespace string, labels map[string]string) *corev1.Pod {
	pod := CreateStatefulSetPod(name, namespace, nil)
	pod.Labels = labels
	return pod
}

// CreateStatefulSetPod creates a statefulset pod for testing, input name should be a valid statefulset pod name like 'a-1'
func CreateStatefulSetPod(name, namespace string, annotations map[string]string) *corev1.Pod {
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

// CreateDeploymentPod creates a deployment pod for testing
func CreateDeploymentPod(name, namespace string, annotation map[string]string) *corev1.Pod {
	parts := strings.Split(name, "-")
	pod := CreateStatefulSetPod(name, namespace, annotation)
	pod.OwnerReferences = []v1.OwnerReference{{
		Kind: "ReplicaSet",
		Name: strings.Join(parts[:len(parts)-1], "-"),
	}}
	return pod
}

// CreateTAppPod creates a tapp pod for testing
func CreateTAppPod(name, namespace string, annotations map[string]string) *corev1.Pod {
	pod := CreateStatefulSetPod(name, namespace, annotations)
	pod.OwnerReferences[0].Kind = "TApp"
	return pod
}
