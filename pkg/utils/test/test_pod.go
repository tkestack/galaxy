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

package test

import (
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodBuilder helps building pods for tests.
type PodBuilder interface {
	WithName(name string) PodBuilder
	WithNamespace(namespace string) PodBuilder
	AddContainer(container apiv1.Container) PodBuilder
	AddOwnerReferences(ownerReference metav1.OwnerReference) PodBuilder
	WithLabels(labels map[string]string) PodBuilder
	WithAnnotations(annotations map[string]string) PodBuilder
	WithPhase(phase apiv1.PodPhase) PodBuilder
	Get() *apiv1.Pod
}

// Pod returns new PodBuilder.
func Pod() PodBuilder {
	return &podBuilderImpl{}
}

type podBuilderImpl struct {
	name            string
	namespace       string
	containers      []apiv1.Container
	ownerReferences []metav1.OwnerReference
	labels          map[string]string
	annotations     map[string]string
	phase           apiv1.PodPhase
}

func (pb *podBuilderImpl) WithLabels(labels map[string]string) PodBuilder {
	r := *pb
	r.labels = labels
	return &r
}

func (pb *podBuilderImpl) WithAnnotations(annotations map[string]string) PodBuilder {
	r := *pb
	r.annotations = annotations
	return &r
}

func (pb *podBuilderImpl) WithName(name string) PodBuilder {
	r := *pb
	r.name = name
	return &r
}

func (pb *podBuilderImpl) WithNamespace(namespace string) PodBuilder {
	r := *pb
	r.namespace = namespace
	return &r
}

func (pb *podBuilderImpl) AddContainer(container apiv1.Container) PodBuilder {
	r := *pb
	r.containers = append(r.containers, container)
	return &r
}

func (pb *podBuilderImpl) AddOwnerReferences(ownerReference metav1.OwnerReference) PodBuilder {
	r := *pb
	r.ownerReferences = append(r.ownerReferences, ownerReference)
	return &r
}

func (pb *podBuilderImpl) WithPhase(phase apiv1.PodPhase) PodBuilder {
	r := *pb
	r.phase = phase
	return &r
}

func (pb *podBuilderImpl) Get() *apiv1.Pod {
	return &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       pb.namespace,
			Name:            pb.name,
			Labels:          pb.labels,
			Annotations:     pb.annotations,
			OwnerReferences: pb.ownerReferences,
		},
		Spec: apiv1.PodSpec{
			Containers: pb.containers,
		},
		Status: apiv1.PodStatus{
			Phase: pb.phase,
		},
	}
}
