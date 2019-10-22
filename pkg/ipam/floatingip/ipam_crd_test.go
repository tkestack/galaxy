package floatingip

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
	fakeGalaxyCli "git.code.oa.com/tkestack/galaxy/pkg/ipam/client/clientset/versioned/fake"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/database"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	fip, err := ipam.client.GalaxyV1alpha1().FloatingIPs().Get("10.49.27.205", v1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !fip.Spec.UpdateTime.After(now) {
		t.Fatal(fip.Spec.UpdateTime)
	}
	fip.Spec.UpdateTime = v1.Time{time.Time{}}
	data, err := json.Marshal(fip)
	if err != nil {
		t.Fatal(err)
	}
	if `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.205","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"pod1","attribute":"212","policy":2,"subnet":"10.49.27.0/24","updateTime":null}}` != string(data) {
		t.Fatal(string(data))
	}
}

func TestCRDReleaseIPs(t *testing.T) {
	ipam := createTestCrdIPAM(t)
	testReleaseIPs(t, ipam)
	fips, err := ipam.client.GalaxyV1alpha1().FloatingIPs().List(v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fips.Items) != 1 {
		t.Fatal(fips)
	}
	fip := fips.Items[0]
	fip.Spec.UpdateTime = v1.Time{time.Time{}}
	data, err := json.Marshal(fip)
	if err != nil {
		t.Fatal(err)
	}
	if `{"kind":"FloatingIP","apiVersion":"galaxy.k8s.io/v1alpha1","metadata":{"name":"10.49.27.216","creationTimestamp":null,"labels":{"ipType":"internalIP"}},"spec":{"key":"pod2","attribute":"333","policy":1,"subnet":"10.49.27.0/24","updateTime":null}}` != string(data) {
		t.Fatal(string(data))
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

func testReleaseIPs(t *testing.T, ipam IPAM) {
	if err := ipam.AllocateSpecificIP("pod1", net.ParseIP("10.49.27.205"), constant.ReleasePolicyNever, "212"); err != nil {
		t.Fatal(err)
	}
	if err := ipam.AllocateSpecificIP("pod2", net.ParseIP("10.49.27.216"), constant.ReleasePolicyImmutable, "333"); err != nil {
		t.Fatal(err)
	}
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
	now := time.Now().Add(-time.Second) // sub one second because db stores unix timestamp without of Nano time
	allocateSomeIPs(t, ipam)
	fips, err := ipam.ByPrefix("od")
	if err != nil {
		t.Fatal(err)
	}
	if len(fips) != 0 {
		t.Fatal(len(fips))
	}

	fips, err = ipam.ByPrefix("pod")
	if err != nil {
		t.Fatal(err)
	}
	if len(fips) != 2 {
		t.Fatal(len(fips))
	}

	fips, err = ipam.ByPrefix("pod2")
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
		t.Fatal(fips[0].UpdatedAt)
	}
}
