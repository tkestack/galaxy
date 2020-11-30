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
	"testing"
	"time"

	extensionclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	extensioninformer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	dynamic "k8s.io/client-go/dynamic/fake"
	. "tkestack.io/galaxy/pkg/utils/test"
)

var (
	objHasReplicas = Unstructured().
			WithName("obj-replicas").WithNamespace("ns1").
			WithApiVersionKind(CrdApiVersionAndKind(FooCrd)).
			AddNestedField(int64(3), ReplicasFields...).Get()
	objNotScalable = Unstructured().
			WithName("obj-not-scalable").WithNamespace("ns1").
			WithApiVersionKind(CrdApiVersionAndKind(NotScalableCrd)).Get()
)

func TestGetReplicas(t *testing.T) {
	crdCache, stop := newFakeCrdCache()
	defer func() { stop <- struct{}{} }()
	replicas, err := crdCache.GetReplicas(GetGroupVersionResource(FooCrd), objHasReplicas.GetNamespace(),
		objHasReplicas.GetName())
	if err != nil {
		t.Fatal(err)
	}
	if replicas != 3 {
		t.Fatalf("expect replicas 3, got %d", replicas)
	}
	replicas, err = crdCache.GetReplicas(GetGroupVersionResource(NotScalableCrd), objNotScalable.GetNamespace(),
		objHasReplicas.GetName())
	if err != nil {
		t.Fatal(err)
	}
	if replicas != 0 {
		t.Fatal(replicas)
	}
}

func newFakeCrdCache() (CrdCache, chan struct{}) {
	stop := make(chan struct{})
	dynamicClient := dynamic.NewSimpleDynamicClient(runtime.NewScheme(), objHasReplicas, objNotScalable)
	extensionClient := extensionclient.NewSimpleClientset(FooCrd, NotScalableCrd)
	extensionFactory := extensioninformer.NewSharedInformerFactory(extensionClient, 0)
	extensionInformer := extensionFactory.Apiextensions().V1beta1().CustomResourceDefinitions()
	extensionInformer.Informer() // call Informer to actually create an informer
	extensionFactory.Start(stop)
	extensionFactory.WaitForCacheSync(stop)
	return NewCrdCache(dynamicClient, extensionInformer.Lister(), time.Minute*10), stop
}
