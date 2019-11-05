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
	"reflect"
	"testing"

	"fmt"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
)

type fakeIPAM struct {
	floatingip.IPAM
	allocatedIPs   map[string]string
	unallocatedIPs map[string]string
	err            error
}

func (ipam fakeIPAM) ReleaseIPs(ipToKey map[string]string) (map[string]string, map[string]string, error) {
	if ipam.err != nil {
		return nil, ipToKey, ipam.err
	}
	released := map[string]string{}
	for ip, k := range ipToKey {
		allocatedK, ok := ipam.allocatedIPs[ip]
		if !ok {
			continue
		}
		if allocatedK == k {
			ipam.unallocatedIPs[ip] = ""
			delete(ipam.allocatedIPs, ip)
			released[ip] = k
			delete(ipToKey, ip)
		} else {
			ipToKey[ip] = allocatedK
		}
	}
	return released, ipToKey, nil
}

func TestBatchReleaseIPs(t *testing.T) {
	ipToKey := map[string]string{"10.0.0.1": "k1", "10.0.0.2": "k2", "10.0.0.3": "k3", "10.0.0.4": "k4"}
	ipam1 := fakeIPAM{allocatedIPs: map[string]string{"10.0.0.1": "k1", "10.0.0.2": "k2.1"}, unallocatedIPs: map[string]string{}}
	ipam2 := fakeIPAM{allocatedIPs: map[string]string{"10.0.0.3": "k3"}, unallocatedIPs: map[string]string{}}
	released, unreleased, err := batchReleaseIPs(ipToKey, ipam1, ipam2)
	if err != nil {
		t.Fatal()
	}
	if !reflect.DeepEqual(map[string]string{"10.0.0.1": "k1", "10.0.0.3": "k3"}, released) {
		t.Fatal(released)
	}
	if !reflect.DeepEqual(map[string]string{"10.0.0.2": "k2.1", "10.0.0.4": "k4"}, unreleased) {
		t.Fatal(unreleased)
	}

	ipToKey = map[string]string{"10.0.0.1": "k1", "10.0.0.2": "k2", "10.0.0.3": "k3", "10.0.0.4": "k4"}
	ipam1 = fakeIPAM{allocatedIPs: map[string]string{"10.0.0.1": "k1", "10.0.0.2": "k2.1"}, unallocatedIPs: map[string]string{}}
	ipam2 = fakeIPAM{err: fmt.Errorf("intentionally error")}
	released, unreleased, err = batchReleaseIPs(ipToKey, ipam1, ipam2)
	if err == nil {
		t.Fatal()
	}
	if !reflect.DeepEqual(map[string]string{"10.0.0.1": "k1"}, released) {
		t.Fatal(released)
	}
	if !reflect.DeepEqual(map[string]string{"10.0.0.2": "k2.1", "10.0.0.3": "k3", "10.0.0.4": "k4"}, unreleased) {
		t.Fatal(unreleased)
	}
}
