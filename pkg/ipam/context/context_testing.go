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
package context

import (
	extensionClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/runtime"
	dynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	fakeGalaxyCli "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned/fake"
)

// CreateTestIPAMContext creates a IPAMContext for testing based on fake clients
func CreateTestIPAMContext(builtinObjs, crdObjs, crObjs []runtime.Object) (*IPAMContext, chan struct{}) {
	ctx := NewIPAMContext(fake.NewSimpleClientset(builtinObjs...), fakeGalaxyCli.NewSimpleClientset(),
		extensionClient.NewSimpleClientset(crdObjs...), dynamic.NewSimpleDynamicClient(runtime.NewScheme(), crObjs...))
	stopChan := make(chan struct{})
	ctx.StartInformers(stopChan)
	return ctx, stopChan
}
