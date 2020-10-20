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
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

func TestAddFloatingIPEventByUser(t *testing.T) {
	ipam, informerFactory := createIPAM(t)
	stop := make(chan struct{})
	go informerFactory.Start(stop)
	defer func() { close(stop) }()
	fip := &FloatingIP{
		IP:        net.ParseIP("10.49.27.205"),
		Key:       "pod2",
		Policy:    0,
		UpdatedAt: time.Now(),
	}
	fipCrd := ipam.newFIPCrd(fip.IP.String())
	fipCrd.Labels[constant.ReserveFIPLabel] = ""
	if err := assign(fipCrd, fip); err != nil {
		t.Fatal(err)
	}
	if _, err := ipam.client.GalaxyV1alpha1().FloatingIPs().Create(fipCrd); err != nil {
		t.Fatal(err)
	}
	if err := waitFor(ipam, fip.IP, fip.Key, true, node1IPNet.String()); err != nil {
		t.Fatal(err)
	}

	// test if an ip is not in within test config range
	fipCrd = ipam.newFIPCrd("172.16.1.145")
	fipCrd.Labels[constant.ReserveFIPLabel] = ""
	if err := ipam.handleFIPAssign(fipCrd); err == nil || err.Error() !=
		fmt.Sprintf("there is no ip %s in unallocated map", fipCrd.Name) {
		t.Fatal(err)
	}
}

func waitFor(ipam *crdIpam, ip net.IP, key string, expectReserveLabel bool, expectSubnetStr string) error {
	return wait.PollImmediate(time.Millisecond*100, time.Minute, func() (done bool, err error) {
		searched, err := ipam.ByIP(ip)
		if err != nil {
			return false, err
		}
		if searched.Key != key {
			return false, nil
		}
		var hasReserveLabel bool
		if searched.Labels != nil {
			_, hasReserveLabel = searched.Labels[constant.ReserveFIPLabel]
		}
		if hasReserveLabel != expectReserveLabel {
			return false, fmt.Errorf("expect has reserve label %v, got %v", expectReserveLabel, hasReserveLabel)
		}
		subnetStr := strings.Join(searched.pool.nodeSubnets.List(), ",")
		if subnetStr != expectSubnetStr {
			return false, fmt.Errorf("expect subnet %v, got %v", expectSubnetStr, subnetStr)
		}
		return true, nil
	})
}

func TestAddFloatingIPEventByIPAM(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	fipCrd := ipam.newFIPCrd("10.49.27.205")
	fipCrd.Spec.Key = "pool__reserved-for-pod_"
	if err := ipam.handleFIPAssign(fipCrd); err != nil {
		t.Fatal(err)
	}
	// AddFloatingIPEvent should ignore fip crd which hasn't reserve label, i.e. created by ipam
	if err := checkIPKey(ipam, fipCrd.Name, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteFloatingIPEvent(t *testing.T) {
	ipam, informerFactory := createIPAM(t)
	stop := make(chan struct{})
	go informerFactory.Start(stop)
	defer func() { close(stop) }()

	fipCrd := ipam.newFIPCrd("10.49.27.205")
	ip := net.ParseIP(fipCrd.Name)
	fipCrd.Labels[constant.ReserveFIPLabel] = ""
	fipCrd.Spec.Key = "pool__reserved-for-node_"
	if _, err := ipam.client.GalaxyV1alpha1().FloatingIPs().Create(fipCrd); err != nil {
		t.Fatal(err)
	}
	if err := waitFor(ipam, ip, fipCrd.Spec.Key, true, node1IPNet.String()); err != nil {
		t.Fatal(err)
	}
	// test if an event is created by ipam, deleteFloatingIPEvent should ignore it
	delete(fipCrd.Labels, constant.ReserveFIPLabel)
	if err := ipam.handleFIPUnassign(fipCrd); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(ipam, fipCrd.Name, fipCrd.Spec.Key); err != nil {
		t.Fatal(err)
	}

	// test if an event is created by user, deleteFloatingIPEvent should handle it
	fipCrd.Labels[constant.ReserveFIPLabel] = ""
	if err := ipam.client.GalaxyV1alpha1().FloatingIPs().Delete(fipCrd.Name, &v1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := waitFor(ipam, ip, "", false, node1IPNet.String()); err != nil {
		t.Fatal(err)
	}
}
