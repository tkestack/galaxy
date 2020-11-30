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
package testing

import (
	"strings"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

const (
	TAppKind = "TApp"
)

// CreateStatefulSetPodWithLabels creates a statefulset pod with labels as `labels` for testing
func CreateStatefulSetPodWithLabels(name, namespace string, labels, annotations map[string]string) *corev1.Pod {
	pod := CreateStatefulSetPod(name, namespace, annotations)
	pod.Labels = labels
	return pod
}

// CreateStatefulSetPod creates a statefulset pod for testing, input name should be a valid statefulset
// pod name like 'a-1'
func CreateStatefulSetPod(name, namespace string, annotations map[string]string) *corev1.Pod {
	parts := strings.Split(name, "-")
	quantity := resource.NewQuantity(1, resource.DecimalSI)
	appName := strings.Join(parts[:len(parts)-1], "-")
	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{"app": appName},
			Annotations: annotations,
			OwnerReferences: []v1.OwnerReference{{
				Kind: "StatefulSet",
				Name: appName,
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
	pod.OwnerReferences[0].Kind = TAppKind
	return pod
}

// CreateSimplePod creates a pod given name, namespace and annotations for testing
func CreateSimplePod(name, namespace string, annotations map[string]string) *corev1.Pod {
	pod := CreateStatefulSetPod(name+"-0", namespace, annotations)
	pod.Name = name
	pod.OwnerReferences = nil
	return pod
}

// CreatePodWithKind creates a pod given name, namespace, owner kind and annotations for testing
func CreatePodWithKind(name, namespace, kind string, annotations map[string]string) *corev1.Pod {
	pod := CreateStatefulSetPod(name, namespace, annotations)
	pod.OwnerReferences[0].Kind = kind
	return pod
}

// CreateDeployment creates a controller deployment for the given pod for testing
func CreateDeployment(podMeta v1.ObjectMeta, replicas int32) *appv1.Deployment {
	parts := strings.Split(podMeta.OwnerReferences[0].Name, "-")
	return &appv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      strings.Join(parts[:len(parts)-1], "-"),
			Namespace: podMeta.Namespace,
			Labels:    podMeta.Labels,
		},
		Spec: appv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMeta,
			},
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: podMeta.GetLabels(),
			},
		},
	}
}

// CreateStatefulSet creates a controller statefulset for the given pod for testing
func CreateStatefulSet(podMeta v1.ObjectMeta, replicas int32) *appv1.StatefulSet {
	return &appv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      podMeta.OwnerReferences[0].Name,
			Namespace: podMeta.GetNamespace(),
			Labels:    podMeta.GetLabels(),
		},
		Spec: appv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMeta,
			},
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: podMeta.GetLabels(),
			},
		},
	}
}

// CreateNode creates a node for testing
func CreateNode(name string, labels map[string]string, address string) corev1.Node {
	return corev1.Node{
		ObjectMeta: v1.ObjectMeta{Name: name, Labels: labels},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: address}}},
	}
}
