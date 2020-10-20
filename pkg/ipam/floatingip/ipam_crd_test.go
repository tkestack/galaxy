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
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	fakeGalaxyCli "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned/fake"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions"
	"tkestack.io/galaxy/pkg/ipam/utils"
	"tkestack.io/galaxy/pkg/utils/nets"
)

const (
	pod1CRD = `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.205","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"pod1","attribute":"{\"NodeName\":\"212\",\"Uid\":\"xx1\"}","policy":2,"subnet":"10.49.27.0/24","updateTime":null}}`
	pod2CRD = `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.216","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"pod2","attribute":"{\"NodeName\":\"333\",\"Uid\":\"xx2\"}","policy":1,"subnet":"10.49.27.0/24","updateTime":null}}`

	policy = constant.ReleasePolicyPodDelete
)

var (
	mask24         = net.IPv4Mask(255, 255, 255, 0)
	mask26         = net.IPv4Mask(255, 255, 255, 0xC0)
	mask32         = net.IPv4Mask(255, 255, 255, 255)
	node1IPNet     = &net.IPNet{IP: net.ParseIP("10.49.27.0"), Mask: mask24}
	node1FIPSubnet = node1IPNet
	node2IPNet     = &net.IPNet{IP: net.ParseIP("10.173.13.0"), Mask: mask24}
	node2FIPSubnet = node2IPNet
	node3IPNet     = &net.IPNet{IP: net.ParseIP("10.180.1.2"), Mask: mask32}
	node3FIPSubnet = &net.IPNet{IP: net.ParseIP("10.180.154.0"), Mask: mask24}
	node4IPNet     = &net.IPNet{IP: net.ParseIP("10.180.1.3"), Mask: mask32}
	node4FIPSubnet = node3FIPSubnet
	node5IPNet1    = &net.IPNet{IP: net.ParseIP("10.0.1.0"), Mask: mask24}
	node5IPNet2    = &net.IPNet{IP: net.ParseIP("10.0.2.0"), Mask: mask24}
	node5FIPSubnet = &net.IPNet{IP: net.ParseIP("10.0.70.0"), Mask: mask24}
	node6IPNet1    = &net.IPNet{IP: net.ParseIP("10.49.28.0"), Mask: mask26}
	node6IPNet2    = &net.IPNet{IP: net.ParseIP("10.49.29.0"), Mask: mask24}
	node6FIPSubnet = &net.IPNet{IP: net.ParseIP("10.0.80.0"), Mask: mask24}
	node7IPNet     = node6IPNet1
	node7FIPSubnet = &net.IPNet{IP: net.ParseIP("10.0.81.0"), Mask: mask24}
	allNodeSubnet  = []*net.IPNet{node1IPNet, node2IPNet, node3IPNet, node4IPNet, node5IPNet1, node5IPNet2, node6IPNet1, node6IPNet2, node7IPNet}
)

func createIPAM(t *testing.T, objs ...runtime.Object) (*crdIpam, crdInformer.SharedInformerFactory) {
	galaxyCli := fakeGalaxyCli.NewSimpleClientset(objs...)
	crdInformerFactory := crdInformer.NewSharedInformerFactory(galaxyCli, 0)
	fipInformer := crdInformerFactory.Galaxy().V1alpha1().FloatingIPs()
	crdIPAM := NewCrdIPAM(galaxyCli, InternalIp, fipInformer).(*crdIpam)
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

func createTestCrdIPAM(t *testing.T, objs ...runtime.Object) *crdIpam {
	crdIPAM, _ := createIPAM(t, objs...)
	return crdIPAM
}

func TestConfigurePool(t *testing.T) {
	now := time.Now()
	ipam := createTestCrdIPAM(t)
	if len(ipam.FloatingIPs) != 7 {
		t.Fatal(len(ipam.FloatingIPs))
	}
	if len(ipam.unallocatedFIPs) != 39 {
		t.Fatal(len(ipam.unallocatedFIPs))
	}
	if len(ipam.allocatedFIPs) != 0 {
		t.Fatal(len(ipam.allocatedFIPs))
	}
	unallocatedFIP, ok := ipam.unallocatedFIPs["10.49.27.205"]
	if !ok {
		t.Fatal()
	}
	if !unallocatedFIP.UpdatedAt.After(now) {
		t.Fatal(unallocatedFIP)
	}
}

func TestConfigurePoolWithAllocatedIP(t *testing.T) {
	expectFip := &FloatingIP{
		IP:        net.ParseIP("10.49.27.205"),
		Key:       "pod2",
		Policy:    0,
		UpdatedAt: time.Now(),
	}
	fipCrd := newFIPCrd(expectFip.IP.String())
	fipCrd.Labels[constant.ReserveFIPLabel] = ""
	internalIP := InternalIp
	ipType, _ := internalIP.String()
	fipCrd.Labels[constant.IpType] = ipType
	if err := assign(fipCrd, expectFip); err != nil {
		t.Fatal(err)
	}
	ipam := createTestCrdIPAM(t, fipCrd)
	if len(ipam.allocatedFIPs) != 1 {
		t.Fatal(len(ipam.allocatedFIPs))
	}
	fip, err := ipam.ByIP(expectFip.IP)
	if err != nil {
		t.Fatal(err)
	}
	if fip.Key != expectFip.Key {
		t.Fatal()
	}
}

func TestAllocateSpecificIP(t *testing.T) {
	now := time.Now()
	ipam := createTestCrdIPAM(t)
	if err := ipam.AllocateSpecificIP("pod1", net.ParseIP("10.49.27.205"),
		Attr{Policy: constant.ReleasePolicyNever, NodeName: "212", Uid: "xx1"}); err != nil {
		t.Fatal(err)
	}
	if len(ipam.allocatedFIPs) != 1 {
		t.Fatal(len(ipam.allocatedFIPs))
	}
	allocated, ok := ipam.allocatedFIPs["10.49.27.205"]
	if !ok {
		t.Fatal()
	}
	if !allocated.UpdatedAt.After(now) {
		t.Fatal(allocated.UpdatedAt)
	}
	if `FloatingIP{ip:10.49.27.205 key:pod1 policy:2 nodeName:212 podUid:xx1}` !=
		fmt.Sprintf("%+v", allocated) {
		t.Fatal(fmt.Sprintf("%+v", allocated))
	}
	if err := checkFIP(ipam, pod1CRD); err != nil {
		t.Fatal(err)
	}
}

func checkFIP(ipam *crdIpam, expect ...string) error {
	fips, err := ipam.client.GalaxyV1alpha1().FloatingIPs().List(v1.ListOptions{})
	if err != nil {
		return err
	}
	if len(fips.Items) != len(expect) {
		return fmt.Errorf("expect %d fip, found %v", len(expect), fips)
	}
	for i, fip := range fips.Items {
		fip.Spec.UpdateTime = v1.Time{time.Time{}}
		data, err := json.Marshal(fip)
		if err != nil {
			return err
		}
		if expect[i] != string(data) {
			return fmt.Errorf("case %d, expect crd %s, found %s", i, expect[i], string(data))
		}
	}
	return nil
}

func TestReserveIP(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	for _, ip := range []string{"10.49.27.205", "10.49.27.216"} {
		if err := ipam.AllocateSpecificIP("pod1", net.ParseIP(ip),
			Attr{Policy: constant.ReleasePolicyNever, NodeName: "node1", Uid: "xx1"}); err != nil {
			t.Fatal(err)
		}
	}
	newAttr := Attr{NodeName: "node2", Uid: "xx2", Policy: constant.ReleasePolicyNever}
	if reserved, err := ipam.ReserveIP("pod1", "p1", newAttr); err != nil {
		t.Fatal(err)
	} else if !reserved {
		t.Fatal()
	}
	for _, ip := range []string{"10.49.27.205", "10.49.27.216"} {
		if err := checkIPKeyAttr(ipam, ip, "p1", &newAttr); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkFIP(ipam,
		`{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.205","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"p1","attribute":"{\"NodeName\":\"node2\",\"Uid\":\"xx2\"}","policy":2,"subnet":"10.49.27.0/24","updateTime":null}}`,
		`{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.216","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"p1","attribute":"{\"NodeName\":\"node2\",\"Uid\":\"xx2\"}","policy":2,"subnet":"10.49.27.0/24","updateTime":null}}`); err != nil {
		t.Fatal(err)
	}
	// reserve again, should not succeed
	newAttr.Policy = constant.ReleasePolicyPodDelete // policy should not be updated by ReserveIP
	if reserved, err := ipam.ReserveIP("p1", "p1", newAttr); err != nil {
		t.Fatal(err)
	} else if reserved {
		t.Fatal()
	}
}

func TestRelease(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testRelease(t, ipam)
	if err := checkFIP(ipam, pod1CRD); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseIPs(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testReleaseIPs(t, ipam)
	if err := checkFIP(ipam, pod2CRD); err != nil {
		t.Fatal(err)
	}
}

func TestByKeyword(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testByKeyword(t, ipam)
}

func TestByPrefix(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testByPrefix(t, ipam)
}

func testRelease(t *testing.T, ipam IPAM) {
	allocateSomeIPs(t, ipam)
	// test key ip mismatch
	if err := ipam.Release("pod1", net.ParseIP("10.49.27.216")); err == nil {
		t.Fatal(err)
	}
	if err := checkIPKey(ipam, "10.49.27.216", "pod2"); err != nil {
		t.Fatal(err)
	}
	// test key ip match
	if err := ipam.Release("pod2", net.ParseIP("10.49.27.216")); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(ipam, "10.49.27.205", "pod1"); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(ipam, "10.49.27.216", ""); err != nil {
		t.Fatal(err)
	}
}

func testReleaseIPs(t *testing.T, ipam IPAM) {
	allocateSomeIPs(t, ipam)
	relesed, unreleased, err := ipam.ReleaseIPs(map[string]string{
		"10.49.27.205": "pod1",  // key match, expect to be released
		"10.49.27.216": "pod3",  // key mismatch, expect not to be released, and returned key should be updated
		"10.49.27.217": "pod4",  // unallocated ip, key mismatch, and returned key should be empty
		"10.0.0.1":     "pod5"}) // unknown ip
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(relesed, map[string]string{"10.49.27.205": "pod1"}) {
		t.Fatal(relesed)
	}
	if !reflect.DeepEqual(unreleased, map[string]string{"10.49.27.216": "pod2", "10.49.27.217": "", "10.0.0.1": "pod5"}) {
		t.Fatal(unreleased)
	}
}

func testByKeyword(t *testing.T, ipam IPAM) {
	now := time.Now()
	fips, err := ipam.ByKeyword("od")
	if err != nil {
		t.Fatal(err)
	}
	if len(fips) != 0 {
		t.Fatal(len(fips))
	}
	allocateSomeIPs(t, ipam)
	fips, err = ipam.ByKeyword("od")
	if err != nil {
		t.Fatal(err)
	}
	if len(fips) != 2 {
		t.Fatal(len(fips))
	}
	fips, err = ipam.ByKeyword("pod2")
	if err != nil {
		t.Fatal(err)
	}
	if len(fips) != 1 {
		t.Fatal(len(fips))
	}
	if fips[0].Key != "pod2" {
		t.Fatal(fips)
	}
	if fips[0].NodeName != "333" {
		t.Fatal(fips)
	}
	if fips[0].PodUid != "xx2" {
		t.Fatal(fips)
	}
	if !fips[0].UpdatedAt.After(now) {
		t.Fatalf("now %v, update time %v", now, fips[0].UpdatedAt)
	}
}

func allocateSomeIPs(t *testing.T, ipam IPAM) {
	if err := ipam.AllocateSpecificIP("pod1", net.ParseIP("10.49.27.205"),
		Attr{Policy: constant.ReleasePolicyNever, NodeName: "212", Uid: "xx1"}); err != nil {
		t.Fatal(err)
	}
	if err := ipam.AllocateSpecificIP("pod2", net.ParseIP("10.49.27.216"),
		Attr{Policy: constant.ReleasePolicyImmutable, NodeName: "333", Uid: "xx2"}); err != nil {
		t.Fatal(err)
	}
}

func testByPrefix(t *testing.T, ipam IPAM) {
	allocateSomeIPs(t, ipam)
	if err := checkByPrefix(ipam, "od"); err != nil {
		t.Fatal(err)
	}
	if err := checkByPrefix(ipam, "pod", "pod1", "pod2"); err != nil {
		t.Fatal(err)
	}
	if err := checkByPrefix(ipam, "pod2", "pod2"); err != nil {
		t.Fatal(err)
	}
}

func checkByPrefix(ipam IPAM, prefix string, expectKeys ...string) error {
	fips, err := ipam.ByPrefix(prefix)
	if err != nil {
		return err
	}
	if len(fips) != len(expectKeys) {
		return fmt.Errorf("%v", fips)
	}
	expectMap := make(map[string]string)
	for _, expect := range expectKeys {
		expectMap[expect] = ""
	}
	for _, fip := range fips {
		if _, ok := expectMap[fip.Key]; !ok {
			return fmt.Errorf("expect %v, got %v", expectKeys, fips)
		}
		if fip.NodeName == "" || fip.PodUid == "" {
			return fmt.Errorf("expect nodeName and podUid are not empty")
		}
	}
	return nil
}

func checkIPKey(ipam IPAM, checkIP, expectKey string) error {
	return checkByIP(ipam, checkIP, expectKey, nil)
}

func checkIPKeyAttr(ipam IPAM, checkIP, expectKey string, expectAttr *Attr) error {
	return checkByIP(ipam, checkIP, expectKey, expectAttr)
}

func checkByIP(ipam IPAM, checkIP, expectKey string, expectAttr *Attr) error {
	ip := net.ParseIP(checkIP)
	if ip == nil {
		return fmt.Errorf("bad check ip: %s", checkIP)
	}
	fip, err := ipam.ByIP(ip)
	if err != nil {
		return err
	}
	if fip.Key != expectKey {
		return fmt.Errorf("expect key: %s, got %s, ip %s", expectKey, fip.Key, checkIP)
	}
	if expectAttr != nil {
		if fip.PodUid != expectAttr.Uid {
			return fmt.Errorf("expect podUid: %s, got %s, ip %s", expectAttr.Uid, fip.PodUid, checkIP)
		}
		if fip.NodeName != expectAttr.NodeName {
			return fmt.Errorf("expect nodeName: %s, got %s, ip %s", expectAttr.NodeName, fip.NodeName, checkIP)
		}
		if fip.Policy != uint16(expectAttr.Policy) {
			return fmt.Errorf("expect policy: %v, got %d, ip %s", expectAttr.Policy, fip.Policy, checkIP)
		}
	}
	return nil
}

func TestAllocateInSubnet(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testCases := []struct {
		nodeIPNet       *net.IPNet
		expectFIPSubnet *net.IPNet
	}{
		{nodeIPNet: node2IPNet, expectFIPSubnet: node2FIPSubnet},
		{nodeIPNet: node3IPNet, expectFIPSubnet: node3FIPSubnet},
		{nodeIPNet: node4IPNet, expectFIPSubnet: node4FIPSubnet},
		{nodeIPNet: node5IPNet1, expectFIPSubnet: node5FIPSubnet},
		{nodeIPNet: node5IPNet2, expectFIPSubnet: node5FIPSubnet},
		{nodeIPNet: node6IPNet2, expectFIPSubnet: node6FIPSubnet},
	}
	for i := range testCases {
		testCase := testCases[i]
		allocatedIP, err := ipam.AllocateInSubnet("pod1", testCase.nodeIPNet, Attr{Policy: policy})
		if err != nil {
			t.Fatalf("test case %d: %v", i, err)
		}
		if !testCase.expectFIPSubnet.Contains(allocatedIP) {
			t.Fatalf("test case %d, expect %s contains allocatedIP %s", i, testCase.expectFIPSubnet, allocatedIP)
		}
	}
	// test can't find available ip
	_, noConfigNode, _ := net.ParseCIDR("10.173.14.0/24")
	if _, err := ipam.AllocateInSubnet("pod1-1", noConfigNode, Attr{Policy: policy}); err == nil || err != ErrNoEnoughIP {
		t.Fatalf("should fail because of ErrNoEnoughIP: %v", err)
	}
}

func TestAllocateInSubnetWithKey(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	allocatedIP, err := ipam.AllocateInSubnet("pod2", node2IPNet, Attr{Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	if err := ipam.AllocateInSubnetWithKey("pod2", "pod3", node2IPNet.String(), Attr{Policy: policy}); err != nil {
		t.Fatal(err)
	}
	ipInfo, err := ipam.First("pod2")
	if err != nil || ipInfo != nil {
		t.Errorf("err %v ipInfo %v", err, ipInfo)
	}

	ipInfo, err = ipam.First("pod3")
	if err != nil || ipInfo.IPInfo.IP == nil || ipInfo.IPInfo.IP.IP.String() != allocatedIP.String() {
		t.Errorf("err %v ipInfo %v", err, ipInfo)
	}
}

func TestNodeSubnet(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testCases := []struct {
		nodeIP string
		expect *net.IPNet
	}{
		{nodeIP: "10.49.27.0", expect: node1IPNet},
		{nodeIP: "10.173.13.1", expect: node2IPNet},
		{nodeIP: "10.180.1.2", expect: node3IPNet},
		{nodeIP: "10.180.1.3", expect: node4IPNet},
		{nodeIP: "10.180.1.4", expect: nil},
		{nodeIP: "10.0.1.0", expect: node5IPNet1},
		{nodeIP: "10.0.2.4", expect: node5IPNet2},
		{nodeIP: "10.49.28.63", expect: node6IPNet1},
		{nodeIP: "10.49.28.64", expect: nil},
		{nodeIP: "10.49.29.3", expect: node6IPNet2},
		{nodeIP: "", expect: nil},
	}
	for i := range testCases {
		testCase := testCases[i]
		subnet := ipam.NodeSubnet(net.ParseIP(testCase.nodeIP))
		var fail bool
		if subnet == nil {
			if testCase.expect != nil {
				fail = true
			}
		} else {
			if testCase.expect == nil || testCase.expect.String() != subnet.String() {
				fail = true
			}
		}
		if fail {
			t.Fatalf("test case %d, expect %v got %v", i, testCase.expect, subnet)
		}
	}
}

func TestAllocateInMultipleSubnet(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	nodeSubnets := sets.NewString()
	for {
		allocatedIP, err := ipam.AllocateInSubnet("pod1", node7IPNet, Attr{Policy: policy})
		if err != nil {
			if err == ErrNoEnoughIP {
				break
			}
			t.Fatal(err)
		}
		if !node7FIPSubnet.Contains(allocatedIP) && !node6FIPSubnet.Contains(allocatedIP) {
			t.Fatalf("expect %s or %s contains allocatedIP %s", node7FIPSubnet, node6FIPSubnet, allocatedIP)
		}
		if node7FIPSubnet.Contains(allocatedIP) {
			nodeSubnets.Insert(node7FIPSubnet.String())
		} else {
			nodeSubnets.Insert(node6FIPSubnet.String())
		}
	}
	if nodeSubnets.Len() != 2 {
		t.Fatalf("expect allocated ip both from %s and %s", node7FIPSubnet, node6FIPSubnet)
	}
}

func TestAllocateInSubnetsAndIPRange(t *testing.T) {
	for i, testCase := range []struct {
		nodeSubnet      *net.IPNet
		ipranges        string
		expectIPs       []string // skip check ips if expectIPs is empty
		expectFIPSubnet *net.IPNet
		expectError     error
	}{
		{nodeSubnet: node1IPNet, ipranges: "", expectFIPSubnet: node1FIPSubnet},
		{nodeSubnet: node1IPNet, ipranges: `[["10.49.27.216~10.49.27.218"]]`, expectFIPSubnet: node1FIPSubnet},
		{nodeSubnet: node1IPNet, ipranges: `[["10.49.27.217~10.49.27.218"],["10.49.27.217~10.49.27.218"]]`,
			expectFIPSubnet: node1FIPSubnet, expectIPs: []string{"10.49.27.217", "10.49.27.218"}},
		{nodeSubnet: node1IPNet, ipranges: `[["10.49.27.205", "10.49.27.218"]]`, expectFIPSubnet: node1FIPSubnet,
			expectIPs: []string{"10.49.27.205"}},
		{nodeSubnet: node1IPNet, ipranges: `[["10.49.27.205"],["10.49.27.218"]]`, expectFIPSubnet: node1FIPSubnet,
			expectIPs: []string{"10.49.27.205", "10.49.27.218"}},
		{nodeSubnet: node1IPNet, ipranges: `[["10.49.27.216"],["10.49.27.217"],["10.49.27.218"]]`,
			expectFIPSubnet: node1FIPSubnet,
			expectIPs:       []string{"10.49.27.216", "10.49.27.217", "10.49.27.218"}},
		// node1IPNet has not 10.50.0.1
		{nodeSubnet: node1IPNet, ipranges: `[["10.49.27.216"],["10.50.0.1"]]`, expectError: ErrNoEnoughIP},
		// node2IPNet has not 10.49.27.216
		{nodeSubnet: node2IPNet, ipranges: `[["10.49.27.216"]]`, expectError: ErrNoEnoughIP},
	} {
		ipam := createTestCrdIPAM(t)
		var ipranges [][]nets.IPRange
		if testCase.ipranges != "" {
			if err := json.Unmarshal([]byte(testCase.ipranges), &ipranges); err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
		}
		ips, err := ipam.AllocateInSubnetsAndIPRange("p1", testCase.nodeSubnet, ipranges, Attr{})
		if err != nil {
			if testCase.expectError != nil && testCase.expectError == err && len(ips) == 0 {
				continue
			}
			t.Fatalf("case %d: %v", i, err)
		}
		for i := range ips {
			if !testCase.expectFIPSubnet.Contains(ips[i]) {
				t.Fatalf("case %d, expect %s contains allocatedIP %s", i, testCase.expectFIPSubnet, ips[i])
			}
		}
		if len(testCase.expectIPs) == 0 {
			continue
		}
		if len(testCase.expectIPs) != len(ips) {
			t.Fatalf("case %d, expect %v, real %v", i, testCase.expectIPs, ips)
		}
		for i := range ips {
			if ips[i].String() != testCase.expectIPs[i] {
				t.Fatalf("case %d, expect %v, real %v", i, testCase.expectIPs, ips)
			}
		}
	}
}

func TestAllocateInSubnetsAndIPRange2(t *testing.T) {
	// check if AllocateInSubnetsAndIPRange allocates all ips or nothing
	ipam := createTestCrdIPAM(t)
	ipranges := [][]nets.IPRange{{*nets.ParseIPRange("10.49.27.216")}, {*nets.ParseIPRange("10.50.0.1")}}
	ips, err := ipam.AllocateInSubnetsAndIPRange("p1", node1IPNet, ipranges, Attr{})
	if err != ErrNoEnoughIP || len(ips) != 0 {
		t.Fatalf("%v, %v", ips, err)
	}
	if fip, err := ipam.ByIP(net.ParseIP("10.49.27.216")); err != nil {
		t.Fatal(err)
	} else if fip.Key != "" {
		t.Fatal()
	}
	ipranges = [][]nets.IPRange{{*nets.ParseIPRange("10.49.27.216")}, {*nets.ParseIPRange("10.49.27.218")}}
	// check if attr is correct
	ips, err = ipam.AllocateInSubnetsAndIPRange("p1", node1IPNet, ipranges,
		Attr{Policy: constant.ReleasePolicyImmutable, NodeName: "node2", Uid: "xx1"})
	if err != nil || len(ips) != 2 {
		t.Fatalf("%v, %v", ips, err)
	}
	for _, ipStr := range []string{"10.49.27.216", "10.49.27.218"} {
		fip, err := ipam.ByIP(net.ParseIP(ipStr))
		if err != nil {
			t.Fatal(err)
		}
		if fip.Policy != uint16(constant.ReleasePolicyImmutable) || fip.NodeName != "node2" || fip.PodUid != "xx1" {
			t.Fatal(fip)
		}
	}
}

func TestByKeyAndIPRanges(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	ipranges := [][]nets.IPRange{{*nets.ParseIPRange("10.49.27.216")}, {*nets.ParseIPRange("10.49.27.218")}}
	_, err := ipam.AllocateInSubnetsAndIPRange("p1", node1IPNet, ipranges, Attr{})
	if err != nil {
		t.Fatal()
	}
	ipInfos, err := ipam.ByKeyAndIPRanges("p1", ipranges)
	if err != nil || len(ipInfos) != 2 ||
		ipInfos[0].IP.String() != "10.49.27.216" || ipInfos[1].IP.String() != "10.49.27.218" ||
		ipInfos[0].IPInfo.Gateway.String() != "10.49.27.1" || ipInfos[1].IPInfo.Gateway.String() != "10.49.27.1" {
		t.Fatalf("%v, %v", ipInfos, err)
	}
	// test if ipranges is nil, result should be the same
	ipInfos1, err := ipam.ByKeyAndIPRanges("p1", nil)
	if err != nil || len(ipInfos1) != 2 {
		t.Fatalf("%v, %v", ipInfos1, err)
	}
	if ipInfos1[0].IP.String() == "10.49.27.218" {
		ipInfos1[0], ipInfos1[1] = ipInfos1[1], ipInfos1[0]
	}
	if ipInfos1[0].IP.String() != "10.49.27.216" || ipInfos1[1].IP.String() != "10.49.27.218" ||
		ipInfos1[0].IPInfo.Gateway.String() != "10.49.27.1" || ipInfos1[1].IPInfo.Gateway.String() != "10.49.27.1" {
		t.Fatalf("%v, %v", ipInfos1, err)
	}
	// test if there is unallocated ips in ipranges
	ipranges = append([][]nets.IPRange{{*nets.ParseIPRange("10.49.27.205~10.49.27.214")}}, ipranges...)
	ipInfos2, err := ipam.ByKeyAndIPRanges("p1", ipranges)
	if err != nil || len(ipInfos2) != 3 ||
		ipInfos2[0] != nil ||
		ipInfos2[1].IP.String() != "10.49.27.216" || ipInfos2[2].IP.String() != "10.49.27.218" ||
		ipInfos2[1].IPInfo.Gateway.String() != "10.49.27.1" || ipInfos2[2].IPInfo.Gateway.String() != "10.49.27.1" {
		t.Fatalf("%v, %v", ipInfos2, err)
	}
	// test if iprange is small
	ipInfos, err = ipam.ByKeyAndIPRanges("p1", [][]nets.IPRange{{*nets.ParseIPRange("10.49.27.218")}})
	if err != nil || len(ipInfos) != 1 ||
		ipInfos[0].IP.String() != "10.49.27.218" ||
		ipInfos[0].IPInfo.Gateway.String() != "10.49.27.1" {
		t.Fatalf("%v, %v", ipInfos, err)
	}
}

func TestNodeSubnetsByKeyAndIPRanges(t *testing.T) {
	allNodeSubnetsSet := sets.NewString()
	for i := range allNodeSubnet {
		allNodeSubnetsSet.Insert(allNodeSubnet[i].String())
	}
	ipam := createTestCrdIPAM(t)
	for i, testCase := range []struct {
		ipranges      string
		expectSubnets []*net.IPNet
	}{
		{expectSubnets: allNodeSubnet},
		{ipranges: `[["10.49.27.216~10.49.27.218"]]`, expectSubnets: []*net.IPNet{node1IPNet}},
		{ipranges: `[["10.49.27.216"],["10.49.27.218"]]`, expectSubnets: []*net.IPNet{node1IPNet}},
		{ipranges: `[["10.49.27.216", "10.173.13.10~10.173.13.13"]]`,
			expectSubnets: []*net.IPNet{node1IPNet, node2IPNet}},
		{ipranges: `[["10.49.27.216", "10.173.13.10~10.173.13.13"],["10.173.13.13"]]`,
			expectSubnets: []*net.IPNet{node2IPNet}},
		{ipranges: `[["10.49.27.216", "10.173.13.10~10.173.13.13", "10.180.154.2"]]`,
			expectSubnets: []*net.IPNet{node1IPNet, node2IPNet, node3IPNet}},
		{ipranges: `[["10.49.27.216"],["10.173.13.10~10.173.13.13"]]`, expectSubnets: []*net.IPNet{}},
		{ipranges: `[["10.0.70.3~10.0.70.20"]]`,
			expectSubnets: []*net.IPNet{node5IPNet1, node5IPNet2}},
		{ipranges: `[["10.0.70.3", "10.0.80.2"]]`,
			expectSubnets: []*net.IPNet{node5IPNet1, node5IPNet2, node6IPNet1, node6IPNet2}},
	} {
		var ipranges [][]nets.IPRange
		if testCase.ipranges != "" {
			if err := json.Unmarshal([]byte(testCase.ipranges), &ipranges); err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
		}
		subnets, err := ipam.NodeSubnetsByIPRanges(ipranges)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		expectSubnets := sets.NewString()
		for i := range testCase.expectSubnets {
			expectSubnets.Insert(testCase.expectSubnets[i].String())
		}
		if !reflect.DeepEqual(subnets.List(), expectSubnets.List()) {
			t.Fatalf("case %d, expect %v, real %v", i, testCase.expectSubnets, subnets)
		}
	}
}
