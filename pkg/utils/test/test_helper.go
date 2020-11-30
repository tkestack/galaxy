package test

import (
	corev1 "k8s.io/api/core/v1"
	extensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

var (
	ReplicasFields = []string{"spec", "replicas"}
	ReplicasPath   = "." + strings.Join(ReplicasFields, ".")

	FooCrd = CustomResourceDefinition().
		WithName("foo").
		WithSubresources(&extensionv1.CustomResourceSubresources{
			Scale: &extensionv1.CustomResourceSubresourceScale{
				SpecReplicasPath: ReplicasPath,
			},
		}).
		WithGroupVersion("test.org", "v2").Get()

	NotScalableCrd = CustomResourceDefinition().
			WithName("not-scalable").
			WithSubresources(&extensionv1.CustomResourceSubresources{}).
			WithGroupVersion("test.org", "v1").Get()
)

// CreateCRDApp creates an app based on its child pod meta and custom resource definition
func CreateCRDApp(podMeta v1.ObjectMeta, replicas int64,
	crd *extensionv1.CustomResourceDefinition) *unstructured.Unstructured {
	return Unstructured().
		WithName(podMeta.OwnerReferences[0].Name).WithNamespace(podMeta.Namespace).
		WithLabels(podMeta.Labels).
		WithApiVersionKind(CrdApiVersionAndKind(crd)).
		AddNestedField(replicas, ReplicasFields...).Get()
}

// CreateCRDPod creates a pod based on its controller custom resource definition
func CreateCRDPod(name, namespace string, annotations map[string]string,
	crd *extensionv1.CustomResourceDefinition) *corev1.Pod {
	parts := strings.Split(name, "-")
	quantity := resource.NewQuantity(1, resource.DecimalSI)
	appName := strings.Join(parts[:len(parts)-1], "-")

	return Pod().WithName(name).WithNamespace(namespace).
		WithAnnotations(annotations).WithLabels(map[string]string{"app": appName}).
		AddOwnerReferences(v1.OwnerReference{
			APIVersion: crd.Spec.Version,
			Kind:       crd.Spec.Names.Kind,
			Name:       appName,
		}).
		AddContainer(corev1.Container{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{constant.ResourceName: *quantity},
			},
		}).Get()
}
