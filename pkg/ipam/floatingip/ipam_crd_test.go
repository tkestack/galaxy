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
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	fakeGalaxyCli "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned/fake"
	"tkestack.io/galaxy/pkg/utils/database"
)

const (
	pod1CRD = `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.205","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"pod1","attribute":"212","policy":2,"subnet":"10.49.27.0/24","updateTime":null}}`
	pod2CRD = `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.216","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"pod2","attribute":"333","policy":1,"subnet":"10.49.27.0/24","updateTime":null}}`
)

func createTestCrdIPAM(t *testing.T, objs ...runtime.Object) *crdIpam {
	galaxyCli := fakeGalaxyCli.NewSimpleClientset(objs...)
	crdIPAM := NewCrdIPAM(galaxyCli, InternalIp).(*crdIpam)
	var conf struct {
		Floatingips []*FloatingIP `json:"floatingips"`
	}
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	if err := crdIPAM.ConfigurePool(conf.Floatingips); err != nil {
		t.Fatal(err)
	}
	return crdIPAM
}

func TestConfigurePool(t *testing.T) {
	now := time.Now()
	ipam := createTestCrdIPAM(t)
	if len(ipam.FloatingIPs) != 4 {
		t.Fatal(len(ipam.FloatingIPs))
	}
	if len(ipam.caches.unallocatedFIPs) != 14 {
		t.Fatal(len(ipam.caches.unallocatedFIPs))
	}
	if len(ipam.caches.allocatedFIPs) != 0 {
		t.Fatal(len(ipam.caches.allocatedFIPs))
	}
	unallocatedFIP, ok := ipam.caches.unallocatedFIPs["10.49.27.205"]
	if !ok {
		t.Fatal()
	}
	if !unallocatedFIP.updateTime.After(now) {
		t.Fatal(unallocatedFIP)
	}
}

func TestCRDAllocateSpecificIP(t *testing.T) {
	now := time.Now()
	ipam := createTestCrdIPAM(t)
	if err := ipam.AllocateSpecificIP("pod1", net.ParseIP("10.49.27.205"), constant.ReleasePolicyNever, "212"); err != nil {
		t.Fatal(err)
	}
	if len(ipam.caches.allocatedFIPs) != 1 {
		t.Fatal(len(ipam.caches.allocatedFIPs))
	}
	allocated, ok := ipam.caches.allocatedFIPs["10.49.27.205"]
	if !ok {
		t.Fatal()
	}
	if !allocated.updateTime.After(now) {
		t.Fatal(allocated.updateTime)
	}
	allocated.updateTime = time.Time{}
	if `&{key:pod1 att:212 policy:2 subnet:10.49.27.0/24 updateTime:{wall:0 ext:0 loc:<nil>}}` != fmt.Sprintf("%+v", allocated) {
		t.Fatal(allocated)
	}
	if err := checkFIP(ipam, pod1CRD); err != nil {
		t.Fatal(err)
	}
}

func checkFIP(ipam *crdIpam, expect string) error {
	fips, err := ipam.client.GalaxyV1alpha1().FloatingIPs().List(v1.ListOptions{})
	if err != nil {
		return err
	}
	if len(fips.Items) != 1 {
		return fmt.Errorf("expect 1 fip, found %v", fips)
	}
	fip := fips.Items[0]
	fip.Spec.UpdateTime = v1.Time{time.Time{}}
	data, err := json.Marshal(fip)
	if err != nil {
		return err
	}
	if expect != string(data) {
		return fmt.Errorf("expect %s, found %s", expect, string(data))
	}
	return nil
}

func TestCRDReserveIP(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testReserveIP(t, ipam)
	if err := checkFIP(ipam, `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.205","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"p1","attribute":"this is p1","policy":2,"subnet":"10.49.27.0/24","updateTime":null}}`); err != nil {
		t.Fatal(err)
	}
}

func TestCRDRelease(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testRelease(t, ipam)
	if err := checkFIP(ipam, pod1CRD); err != nil {
		t.Fatal(err)
	}
}

func TestCRDReleaseIPs(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testReleaseIPs(t, ipam)
	if err := checkFIP(ipam, pod2CRD); err != nil {
		t.Fatal(err)
	}
}

func TestCRDByKeyword(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testByKeyword(t, ipam)
}

func TestCRDByPrefix(t *testing.T) {
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

func testReserveIP(t *testing.T, ipam IPAM) {
	if err := ipam.AllocateSpecificIP("pod1", net.ParseIP("10.49.27.205"), constant.ReleasePolicyNever, "212"); err != nil {
		t.Fatal(err)
	}
	if err := ipam.ReserveIP("pod1", "p1", "this is p1"); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKeyAttr(ipam, "10.49.27.205", "p1", "this is p1"); err != nil {
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
	now := time.Now().Add(-time.Second) // sub one second because db stores unix timestamp without of Nano time
	allocateSomeIPs(t, ipam)
	fips, err := ipam.ByKeyword("od")
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
	if !fips[0].UpdatedAt.After(now) {
		t.Fatalf("now %v, update time %v", now, fips[0].UpdatedAt)
	}
}

func allocateSomeIPs(t *testing.T, ipam IPAM) {
	if err := ipam.AllocateSpecificIP("pod1", net.ParseIP("10.49.27.205"), constant.ReleasePolicyNever, "212"); err != nil {
		t.Fatal(err)
	}
	if err := ipam.AllocateSpecificIP("pod2", net.ParseIP("10.49.27.216"), constant.ReleasePolicyImmutable, "333"); err != nil {
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

func checkIPKey(ipam IPAM, checkIP, expectKey string) error {
	return checkByIP(ipam, checkIP, expectKey, nil)
}

func checkIPKeyAttr(ipam IPAM, checkIP, expectKey, expectAttr string) error {
	return checkByIP(ipam, checkIP, expectKey, &expectAttr)
}

func checkByIP(ipam IPAM, checkIP, expectKey string, expectAttr *string) error {
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
		if fip.Attr != *expectAttr {
			return fmt.Errorf("expect attr: %s, got %s, ip %s", *expectAttr, fip.Attr, checkIP)
		}
	}
	return nil
}
