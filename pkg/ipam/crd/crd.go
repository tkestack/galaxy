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
package crd

import (
	"context"

	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	glog "k8s.io/klog"

	"tkestack.io/galaxy/pkg/ipam/apis/galaxy"
)

// floatingipCrd is the crd format of floatingip
var floatingipCrd = &extensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: "floatingips.galaxy.k8s.io",
		Annotations: map[string]string{
			"api-approved.kubernetes.io": "https://github.com/kubernetes/kubernetes/pull/78458",
		},
	},
	TypeMeta: metav1.TypeMeta{
		Kind:       "CustomResourceDefinition",
		APIVersion: "apiextensions.k8s.io/v1",
	},
	Spec: extensionsv1.CustomResourceDefinitionSpec{
		Group: galaxy.GroupName,
		Scope: extensionsv1.ClusterScoped,
		Versions: []extensionsv1.CustomResourceDefinitionVersion{
			{
				Name:    "v1alpha1",
				Served:  true,
				Storage: true,
				Schema: &extensionsv1.CustomResourceValidation{
					&extensionsv1.JSONSchemaProps{
						Description: "FloatingIP provides configuration for floatingIP.",
						Properties: map[string]extensionsv1.JSONSchemaProps{
							"apiVersion": {
								Description: "APIVersion defines the versioned schema of this representation of an object.",
								Type:        "string",
							},
							"kind": {
								Description: "Kind is a string value representing the REST resource this object represents.",
								Type:        "string",
							},
							"metadata": {
								Type: "object",
							},
							"spec": {
								Description: "Spec defines the desired identities of FloatingIP.",
								Properties: map[string]extensionsv1.JSONSchemaProps{
									"attribute": {
										Description: "attribute used as node ip",
										Type:        "string",
									},
									"key": {
										Description: "key can be resolved as pool, namespace name, app name, app type and pod name",
										Type:        "string",
									},
									"policy": {
										Description: "policy used as",
										Type:        "integer",
									},
									"updateTime": {
										Description: "FloatingIP update(allocate, release or update) timestamp",
										Format:      "date-time",
										Type:        "string",
									},
								},
								Required: []string{"attribute", "key", "policy", "updateTime"},
								Type:     "object",
							},
						},
						Required: []string{"spec"},
						Type:     "object",
					},
				},
			},
		},
		Names: extensionsv1.CustomResourceDefinitionNames{
			Kind:       "FloatingIP",
			ListKind:   "FloatingIPList",
			Plural:     "floatingips",
			Singular:   "floatingip",
			ShortNames: []string{"fip"},
		},
	},
}

// poolCrd is the crd format of pool
var poolCrd = &extensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: "pools.galaxy.k8s.io",
		Annotations: map[string]string{
			"api-approved.kubernetes.io": "https://github.com/kubernetes/kubernetes/pull/78458",
		},
	},
	TypeMeta: metav1.TypeMeta{
		Kind:       "CustomResourceDefinition",
		APIVersion: "apiextensions.k8s.io/v1",
	},
	Spec: extensionsv1.CustomResourceDefinitionSpec{
		Group: galaxy.GroupName,
		Scope: extensionsv1.NamespaceScoped,
		Versions: []extensionsv1.CustomResourceDefinitionVersion{
			{
				Name:    "v1alpha1",
				Served:  true,
				Storage: true,
				Schema: &extensionsv1.CustomResourceValidation{
					&extensionsv1.JSONSchemaProps{
						Description: "Pool provides configuration for FloatingIP pool which used to store FloatingIP.",
						Properties: map[string]extensionsv1.JSONSchemaProps{
							"apiVersion": {
								Description: "APIVersion defines the versioned schema of this representation of an object.",
								Type:        "string",
							},
							"kind": {
								Description: "Kind is a string value representing the REST resource this object represents.",
								Type:        "string",
							},
							"metadata": {
								Type: "object",
							},
							"preAllocateIP": {
								Description: "Pre-allocate IP when creating pool",
								Type:        "boolean",
							},
							"size": {
								Description: "The pool size",
								Type:        "integer",
							},
						},
						Required: []string{"preAllocateIP", "size"},
						Type:     "object",
					},
				},
			},
		},
		Names: extensionsv1.CustomResourceDefinitionNames{
			Kind:     "Pool",
			ListKind: "PoolList",
			Plural:   "pools",
			Singular: "pool",
		},
	},
}

// EnsureCRDCreated ensures floatingip and pool are created in apiserver
func EnsureCRDCreated(client apiextensionsclient.Interface) error {
	crdClient := client.ApiextensionsV1().CustomResourceDefinitions()
	crds := []*extensionsv1.CustomResourceDefinition{floatingipCrd, poolCrd}
	for i := range crds {
		// try to create each crd and ignores already exist error
		if _, err := crdClient.Create(context.TODO(), crds[i], metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			glog.Errorf("Error creating CRD: %s", crds[i].Spec.Names.Kind)
			return err
		}
		glog.Infof("Create CRD %s successfully.", crds[i].Spec.Names.Kind)
	}
	return nil
}

// GetGroupVersionResource from crd
func GetGroupVersionResource(crd *extensionsv1.CustomResourceDefinition) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    crd.Spec.Group,
		Version:  "v1alpha1",
		Resource: crd.Spec.Names.Plural,
	}
}
