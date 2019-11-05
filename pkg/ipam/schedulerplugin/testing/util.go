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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
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
