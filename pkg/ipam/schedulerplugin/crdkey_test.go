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
package schedulerplugin

import (
	"testing"

	extensionclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	extensioninformer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	. "tkestack.io/galaxy/pkg/utils/test"
)

func TestGetGroupVersionResource(t *testing.T) {
	stop := make(chan struct{})
	extensionClient := extensionclient.NewSimpleClientset(FooCrd)
	extensionFactory := extensioninformer.NewSharedInformerFactory(extensionClient, 0)
	extensionInformer := extensionFactory.Apiextensions().V1().CustomResourceDefinitions()
	extensionInformer.Informer() // call Informer to actually create an informer
	extensionFactory.Start(stop)
	extensionFactory.WaitForCacheSync(stop)
	crdKey := NewCrdKey(extensionInformer.Lister())
	gvr := crdKey.GetGroupVersionResource("foo_")
	if gvr == nil {
		t.Fatal()
	}
	if gvr.Group != FooCrd.Spec.Group || gvr.Version != FooCrd.Spec.Version ||
		gvr.Resource != FooCrd.Spec.Names.Plural {
		t.Fatal(gvr)
	}
}
