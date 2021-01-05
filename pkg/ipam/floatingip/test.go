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
package floatingip

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	fakeGalaxyCli "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned/fake"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions"
	"tkestack.io/galaxy/pkg/ipam/utils"
)

// CreateTestIPAM creates an ipam for testing
func CreateTestIPAM(t *testing.T, objs ...runtime.Object) (*crdIpam, crdInformer.SharedInformerFactory) {
	galaxyCli := fakeGalaxyCli.NewSimpleClientset(objs...)
	crdInformerFactory := crdInformer.NewSharedInformerFactory(galaxyCli, 0)
	fipInformer := crdInformerFactory.Galaxy().V1alpha1().FloatingIPs()
	crdIPAM := NewCrdIPAM(galaxyCli, fipInformer).(*crdIpam)
	var conf struct {
		Floatingips []*FloatingIPPool `json:"floatingips"`
	}
	if err := json.Unmarshal([]byte(utils.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	if err := crdIPAM.ConfigurePool(conf.Floatingips); err != nil {
		t.Fatal(err)
	}
	return crdIPAM, crdInformerFactory
}
