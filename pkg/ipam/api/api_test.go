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
package api

import (
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
)

func TestReserveFIP(t *testing.T) {
	fip := &v1alpha1.FloatingIP{
		TypeMeta:   metav1.TypeMeta{Kind: constant.ResourceKind, APIVersion: constant.ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: "10.49.27.216", Labels: map[string]string{constant.ReserveFIPLabel: ""}},
		Spec:       v1alpha1.FloatingIPSpec{Key: "hello-xx"},
	}
	ipam, _ := floatingip.CreateTestIPAM(t, fip)
	fips, err := listIPs("xx", ipam, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fips) != 1 {
		t.Fatal(fips)
	}

	if fips[0].labels == nil {
		t.Fatal()
	}
	if _, ok := fips[0].labels[constant.ReserveFIPLabel]; !ok {
		t.Fatal()
	}
	c := NewController(ipam, nil, nil)
	releasable, _ := c.checkReleasableAndStatus(&fips[0])
	if releasable {
		t.Fatal()
	}
}

func TestCheckReleasableAndStatus(t *testing.T) {
	client := fake.NewSimpleClientset(&v1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "xx-1", Namespace: "demo"},
		Spec:       v1.PodSpec{},
		Status:     v1.PodStatus{Phase: v1.PodRunning},
	})
	stop := make(chan struct{})
	factory := informers.NewSharedInformerFactoryWithOptions(client, time.Minute)
	lister := factory.Core().V1().Pods().Lister()
	factory.Start(stop)
	factory.WaitForCacheSync(stop)
	c := NewController(nil, lister, nil)
	for i, testCase := range []struct {
		fip              *FloatingIP
		expectReleasable bool
		expectStatus     string
	}{
		{fip: &FloatingIP{PoolName: "pool1"}, expectReleasable: true, expectStatus: "Deleted"},
		{fip: &FloatingIP{AppName: "dep1", AppType: "deployment"}, expectReleasable: true, expectStatus: "Deleted"},
		{fip: &FloatingIP{}, expectReleasable: false},
		{fip: &FloatingIP{AppName: "dep1", AppType: "statefulset", PodName: "xx-1", Namespace: "demo"},
			expectReleasable: false, expectStatus: string(v1.PodRunning)},
		{fip: &FloatingIP{AppName: "dep1", AppType: "statefulset", PodName: "xx-2", Namespace: "demo"},
			expectReleasable: true, expectStatus: "Deleted"},
	} {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			releasable, status := c.checkReleasableAndStatus(testCase.fip)
			if releasable != testCase.expectReleasable {
				t.Fatalf("expect %v, got %v", testCase.expectReleasable, releasable)
			}
			if status != testCase.expectStatus {
				t.Fatalf("expect %v, got %v", testCase.expectStatus, status)
			}
		})
	}
}
