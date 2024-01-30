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
	"strings"

	extensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomResourceDefinitionBuilder helps building CustomResourceDefinition for tests.
type CustomResourceDefinitionBuilder interface {
	WithName(name string) CustomResourceDefinitionBuilder
	WithGroupVersion(group, version string) CustomResourceDefinitionBuilder
	WithSubresources(crs *extensionv1.CustomResourceSubresources) CustomResourceDefinitionBuilder
	Get() *extensionv1.CustomResourceDefinition
}

// CustomResourceDefinition returns new CustomResourceDefinitionBuilder.
func CustomResourceDefinition() CustomResourceDefinitionBuilder {
	return &customResourceDefinitionBuilder{}
}

type customResourceDefinitionBuilder struct {
	name         string
	version      string
	group        string
	subresources *extensionv1.CustomResourceSubresources
}

func (c *customResourceDefinitionBuilder) WithName(name string) CustomResourceDefinitionBuilder {
	r := *c
	r.name = name
	return &r
}

func (c *customResourceDefinitionBuilder) WithGroupVersion(group, version string) CustomResourceDefinitionBuilder {
	r := *c
	r.group = group
	r.version = version
	return &r
}

func (c *customResourceDefinitionBuilder) WithSubresources(crs *extensionv1.CustomResourceSubresources) CustomResourceDefinitionBuilder {
	r := *c
	r.subresources = crs
	return &r
}

func (c *customResourceDefinitionBuilder) Get() *extensionv1.CustomResourceDefinition {
	name := c.name
	kind := strings.ToUpper(name[:1]) + name[1:]
	plural := name + "s"
	return &extensionv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: plural + "." + c.group,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		Spec: extensionv1.CustomResourceDefinitionSpec{
			Group: c.group,
			Versions: []extensionv1.CustomResourceDefinitionVersion{{
				Name:         c.version,
				Subresources: c.subresources,
			}},
			Scope: "Namespaced",
			Names: extensionv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: name,
				Kind:     kind,
				ListKind: kind + "List",
			},
		},
	}
}

// CrdApiVersionAndKind returns the apiVersion and kind of the given CustomResourceDefinition
func CrdApiVersionAndKind(crd *extensionv1.CustomResourceDefinition) (string, string) {
	return crd.Spec.Group + "/" + crd.Spec.Versions[0].Name, crd.Spec.Names.Kind
}
