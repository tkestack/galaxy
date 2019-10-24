package floatingip

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jinzhu/gorm"

	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/utils/database"
)

// Start will create dbIpam.
func Start(t *testing.T) *dbIpam {
	return CreateIPAMWithTableName(t, database.DefaultFloatingipTableName)
}

// #lizard forgives
// CreateIPAMWithTableName can create fake dbIpam.
func CreateIPAMWithTableName(t *testing.T, tableName string) *dbIpam {
	var err error
	db, err := database.NewTestDB()
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	var conf struct {
		Floatingips []*FloatingIP `json:"floatingips"`
	}
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	i := NewIPAMWithTableName(db, tableName)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf("TRUNCATE %s;", tableName)).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := i.ConfigurePool(conf.Floatingips); err != nil {
		t.Fatal(err)
	}
	// There should be 14 ips
	if m, err := i.ByPrefix(""); err != nil || len(m) != 14 {
		t.Fatalf("map %v, err %v", m, err)
	}
	return i.(*dbIpam)
}

// #lizard forgives
// TestApplyFloatingIPs test ipam applyFloatingIPs function.
func TestApplyFloatingIPs(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	fips := []*FloatingIP{}
	fipStr := `[{
      "routableSubnet": "10.49.27.0/24",
      "ips": ["10.49.27.206", "10.49.27.215~10.49.27.219"],
      "subnet": "10.49.27.0/24",
      "gateway": "10.49.27.1",
      "vlan": 2
    }, {
      "routableSubnet": "10.173.13.0/24",
      "ips": ["10.173.13.14"],
      "subnet": "10.173.13.0/24",
      "gateway": "10.173.13.1",
      "vlan": 2
    }, {
      "routableSubnet": "10.173.14.0/24",
      "ips": ["10.173.14.14"],
      "subnet": "10.173.14.0/24",
      "gateway": "10.173.14.1",
      "vlan": 2
    }, {
      "routableSubnet": "10.173.14.0/24",
      "ips": ["10.173.14.15"],
      "subnet": "10.173.14.0/24",
      "gateway": "10.173.14.1",
      "vlan": 2
    }]`
	if err := json.Unmarshal([]byte(fipStr), &fips); err != nil {
		t.Fatal(err)
	}
	conf := ipam.applyFloatingIPs(fips)
	if len(conf) != 5 {
		t.Fatal(conf)
	}
	if obj, err := json.Marshal(conf); err != nil {
		t.Fatal(err)
	} else {
		str := string(obj)
		if !strings.Contains(str, "10.49.27.205~10.49.27.206") ||
			!strings.Contains(str, "10.49.27.215~10.49.27.219") ||
			!strings.Contains(str, "10.173.13.10~10.173.13.15") ||
			!strings.Contains(str, "10.173.14.14~10.173.14.15") {
			t.Fatal(conf)
		}
	}
}

// #lizard forgives
// TestRaceCondition test floatingIP allocation race.
func TestRaceCondition(t *testing.T) {
	var ipams []*dbIpam
	for i := 0; i < 7; i++ {
		ipam := Start(t)
		defer ipam.Shutdown()
		ipams = append(ipams, ipam)
	}
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	var ips []net.IP
	for i, m := range ipams {
		wg.Add(1)
		j := i
		go func(m *dbIpam) {
			defer wg.Done()
			keys := []string{fmt.Sprintf("pod%d", j*2), fmt.Sprintf("pod%d", j*2+1)}
			allocated, err := m.allocate(keys)
			if err != nil {
				t.Error(err)
			}
			lock.Lock()
			defer lock.Unlock()
			ips = append(ips, allocated...)
		}(m)
	}
	wg.Wait()
	if len(ips) != 14 {
		t.Fatal()
	}
	m := make(map[string]interface{})
	for _, ip := range ips {
		if _, ok := m[ip.String()]; ok {
			t.Fatal("allocated the same ip")
		}
		m[ip.String()] = ip
	}
}

// TestEmptyFloatingIPConf test ConfigurePool function.
func TestEmptyFloatingIPConf(t *testing.T) {
	var err error
	db, err := database.NewTestDB()
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	i := NewIPAM(db)
	defer i.Shutdown()
	if err := i.ConfigurePool(nil); err != nil {
		t.Fatal(err)
	}
}

// #lizard forgives
// TestAllocateIPInSubnet test AllocateInSubnet function.
func TestAllocateIPInSubnet(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	_, routableSubnet, _ := net.ParseCIDR("10.173.13.0/24")
	if _, err := ipam.AllocateInSubnet("pod1-1", routableSubnet, constant.ReleasePolicyPodDelete, ""); err != nil {
		t.Fatal(err)
	}
	ipInfo, err := ipam.first("pod1-1")
	if err != nil {
		t.Fatal(err)
	}
	if ipInfo.IP.String() != "10.173.13.2/24" {
		t.Fatal(ipInfo.IP.String())
	}
	//test can't find available ip
	_, routableSubnet, _ = net.ParseCIDR("10.173.14.0/24")
	if _, err := ipam.AllocateInSubnet("pod1-1", routableSubnet, constant.ReleasePolicyPodDelete, ""); err == nil || err != ErrNoFIPForSubnet {
		t.Fatalf("should fail because of ErrNoFIPForSubnet: %v", err)
	}
}

// #lizard forgives
// TestRoutableSubnet test RoutableSubnet function.
func TestRoutableSubnet(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	//10.173.13.0/24
	if ipNet := ipam.RoutableSubnet(net.ParseIP("10.173.13.3")); ipNet == nil || ipNet.String() != "10.173.13.0/24" {
		t.Fatal()
	}
	if ipNet := ipam.RoutableSubnet(net.ParseIP("10.173.13.254")); ipNet == nil || ipNet.String() != "10.173.13.0/24" {
		t.Fatal()
	}
	if ipNet := ipam.RoutableSubnet(net.ParseIP("10.173.14.1")); ipNet != nil {
		t.Fatal()
	}
	//10.49.27.0/24
	if ipNet := ipam.RoutableSubnet(net.ParseIP("10.49.26.254")); ipNet != nil {
		t.Fatal()
	}
	if ipNet := ipam.RoutableSubnet(net.ParseIP("10.49.27.1")); ipNet == nil || ipNet.String() != "10.49.27.0/24" {
		t.Fatal()
	}
}

// #lizard forgives
// TestAllocateInSubnetAndQueryRoutableSubnetByKey test QueryRoutableSubnetByKey and AllocateInSubnet functions.
func TestAllocateInSubnetAndQueryRoutableSubnetByKey(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()

	subnets, err := ipam.QueryRoutableSubnetByKey("")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(subnets)
	if fmt.Sprintf("%v", subnets) != "[10.173.13.0/24 10.180.1.2/32 10.180.1.3/32 10.49.27.0/24]" {
		t.Fatal(subnets)
	}
	// drain ips of 10.49.27.0/24
	if ips, err := ipam.allocate([]string{"p1", "p2", "p3", "p4"}); err != nil || len(ips) != 4 {
		t.Fatalf("ips %v err %v", ips, err)
	}
	subnets, err = ipam.QueryRoutableSubnetByKey("p1")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v", subnets) != "[10.49.27.0/24]" {
		t.Fatal(subnets)
	}
	subnets, err = ipam.QueryRoutableSubnetByKey("")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v", subnets) != "[10.173.13.0/24 10.180.1.2/32 10.180.1.3/32]" {
		t.Fatal(subnets)
	}
	// drain ips of 10.180.1.3/32
	ipnet := &net.IPNet{IP: net.ParseIP("10.180.1.3"), Mask: net.IPv4Mask(255, 255, 255, 255)}
	if ip, err := ipam.AllocateInSubnet("p5", ipnet, constant.ReleasePolicyPodDelete, ""); err != nil || ip == nil {
		t.Fatalf("ip %v err %v", ip, err)
	}
	if ip, err := ipam.AllocateInSubnet("p6", ipnet, constant.ReleasePolicyPodDelete, ""); err != nil || ip == nil {
		t.Fatalf("ip %v err %v", ip, err)
	}
	subnets, err = ipam.QueryRoutableSubnetByKey("")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v", subnets) != "[10.173.13.0/24 10.180.1.2/32]" {
		t.Fatal(subnets)
	}
}

// TestAllocateSpecificIP test AllocateSpecificIP function.
func TestAllocateSpecificIP(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()

	ip := net.ParseIP("10.49.27.216")
	if err := ipam.AllocateSpecificIP("pod1", ip, constant.ReleasePolicyPodDelete, ""); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(ipam, "10.49.27.216", "pod1"); err != nil {
		t.Fatal(err)
	}
	// check if an allocated ip can be allocated again, should return an ErrNotUpdated error
	if err := ipam.AllocateSpecificIP("pod2", ip, constant.ReleasePolicyPodDelete, ""); err == nil || err != ErrNotUpdated {
		t.Fatal(err)
	}
}

// #lizard forgives
// TestMultipleIPAM test two dbIpam situation.
func TestMultipleIPAM(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	secondIPAM := CreateIPAMWithTableName(t, "test_table")
	defer secondIPAM.Shutdown()
	ip := net.ParseIP("10.49.27.216")
	if err := secondIPAM.AllocateSpecificIP("pod1", ip, constant.ReleasePolicyPodDelete, ""); err != nil {
		t.Fatal(err)
	}
	checkMultipleIPAM(t, ipam, secondIPAM, ip, "pod1")

	_, routableSubnet, _ := net.ParseCIDR("10.173.13.0/24")
	ip2, err := secondIPAM.AllocateInSubnet("pod2", routableSubnet, constant.ReleasePolicyPodDelete, "")
	if err != nil || ip2 == nil {
		t.Fatalf("ip %v, err %v", ip2, err)
	}
	checkMultipleIPAM(t, ipam, secondIPAM, ip2, "pod2")
	// check release ips
	if err := secondIPAM.Release("pod1", ip); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := secondIPAM.first("pod1"); err != nil || ipInfo != nil {
		t.Fatalf("ipInfo %v, err %v", ipInfo, err)
	}
	if err := secondIPAM.ReleaseByPrefix("pod2"); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := secondIPAM.first("pod2"); err != nil || ipInfo != nil {
		t.Fatalf("ipInfo %v, err %v", ipInfo, err)
	}
}

// #lizard forgives
func checkMultipleIPAM(t *testing.T, ipam, secondIPAM *dbIpam, ip net.IP, expectKey string) {
	t.Logf("testing expectKey %s, ip %s", expectKey, ip.String())
	t.Logf("secondIPAM...")
	// check secondIPAM query result is not empty
	secondFip, err := secondIPAM.ByIP(ip)
	if err != nil || secondFip.Key != expectKey {
		t.Fatalf("key %s, err %v", secondFip.Key, err)
	}
	ipInfo, err := secondIPAM.first(expectKey)
	if err != nil || ipInfo.IP.IP.String() != ip.String() {
		t.Fatalf("ipInfo %v, err %v", ipInfo, err)
	}
	if err := checkByPrefix(secondIPAM, expectKey, expectKey); err != nil {
		t.Fatal(err)
	}
	subnets, err := secondIPAM.QueryRoutableSubnetByKey(expectKey)
	if err != nil || len(subnets) == 0 {
		t.Fatalf("%v err %v", subnets, err)
	}
	// check ipam query result is empty
	t.Logf("ipam...")
	secondFip, err = ipam.ByIP(ip)
	if err != nil || secondFip.Key != "" {
		t.Fatalf("key %s, err %v", secondFip.Key, err)
	}
	ipInfo, err = ipam.first(expectKey)
	if err != nil || ipInfo != nil {
		t.Fatalf("ipInfo %v, err %v", ipInfo, err)
	}
	if err := checkByPrefix(ipam, expectKey); err != nil {
		t.Fatal(err)
	}
	subnets, err = ipam.QueryRoutableSubnetByKey(expectKey)
	if err != nil || len(subnets) != 0 {
		t.Fatalf("%v err %v", subnets, err)
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
	}
	return nil
}

// TestGetRoutableSubnet test RoutableSubnet function.
func TestGetRoutableSubnet(t *testing.T) {
	var fips []*FloatingIP
	if err := json.Unmarshal([]byte(`[{"routableSubnet":"10.239.228.0/22","ips":["10.239.238.3~10.239.238.6","10.239.238.11","10.239.238.26~10.239.238.61","10.239.238.115~10.239.238.116","10.239.238.164","10.239.238.166","10.239.238.207","10.239.238.226","10.239.238.236"],"subnet":"10.239.236.0/22","gateway":"10.239.236.1","vlan":13}]`), &fips); err != nil {
		t.Fatal(err)
	}
	ipam := &dbIpam{FloatingIPs: fips}
	ipnet := ipam.RoutableSubnet(net.ParseIP("10.239.229.142"))
	if ipnet == nil {
		t.Fatal()
	}
	ipnet = ipam.RoutableSubnet(net.ParseIP("10.239.230.32"))
	if ipnet == nil {
		t.Fatal()
	}
}

// #lizard forgives
// TestAllocateInSubnet test AllocateInSubnet function.
func TestAllocateInSubnet(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	ipnet := &net.IPNet{IP: net.ParseIP("10.180.1.3"), Mask: net.IPv4Mask(255, 255, 255, 255)}
	allocatedIP, err := ipam.AllocateInSubnet("pod1", ipnet, constant.ReleasePolicyPodDelete, "")
	if err != nil {
		t.Fatal(err)
	}
	if allocatedIP.String() != "10.180.154.7" {
		t.Fatal(allocatedIP.String())
	}

	ipnet = &net.IPNet{IP: net.ParseIP("10.173.13.0"), Mask: net.IPv4Mask(255, 255, 255, 0)}
	allocatedIP, err = ipam.AllocateInSubnet("pod2", ipnet, constant.ReleasePolicyPodDelete, "")
	if err != nil {
		t.Fatal(err)
	}
	if allocatedIP.String() != "10.173.13.2" {
		t.Fatal(allocatedIP.String())
	}

	// test AllocateInSubnetWithKey
	if err = ipam.AllocateInSubnetWithKey("pod2", "pod3", ipnet.String(), constant.ReleasePolicyPodDelete, ""); err != nil {
		t.Fatal(err)
	}
	ipInfo, err := ipam.First("pod2")
	if err != nil || ipInfo != nil {
		t.Errorf("err %v ipInfo %v", err, ipInfo)
	}

	ipInfo, err = ipam.First("pod3")
	if err != nil || ipInfo.IPInfo.IP == nil || ipInfo.IPInfo.IP.String() != "10.173.13.2/24" {
		t.Errorf("err %v ipInfo %v", err, ipInfo)
	}
}

// #lizard forgives
// TestUpdateKeyUpdatePolicy test UpdatePolicy function.
func TestUpdateKeyUpdatePolicy(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()

	ipnet := &net.IPNet{IP: net.ParseIP("10.173.13.0"), Mask: net.IPv4Mask(255, 255, 255, 0)}
	allocatedIP, err := ipam.AllocateInSubnet("pod2", ipnet, constant.ReleasePolicyPodDelete, "")
	if err != nil {
		t.Fatal(err)
	}
	if allocatedIP.String() != "10.173.13.2" {
		t.Error(allocatedIP.String())
	}
	if err := ipam.ReserveIP("pod2", "pod3", ""); err != nil {
		t.Fatal(err)
	}

	ipInfo, err := ipam.First("pod2")
	if err != nil || ipInfo != nil {
		t.Errorf("err %v ipInfo %v", err, ipInfo)
	}

	ipInfo, err = ipam.First("pod3")
	if err != nil || ipInfo.IPInfo.IP == nil {
		t.Fatalf("err %v ipInfo %v", err, ipInfo)
	}
	ipInfo.FIP.UpdatedAt = time.Time{}
	if fmt.Sprintf("%+v", ipInfo) != "&{IPInfo:{IP:10.173.13.2/24 Vlan:2 Gateway:10.173.13.1 RoutableSubnet:10.173.13.0/24} FIP:{Table: Key:pod3 Subnet:10.173.13.0/24 Attr: IP:179113218 Policy:0 UpdatedAt:0001-01-01 00:00:00 +0000 UTC}}" {
		t.Error(fmt.Sprintf("%+v", ipInfo))
	}

	if err := ipam.UpdatePolicy("pod3", ipInfo.IPInfo.IP.IP, constant.ReleasePolicyNever, "111"); err != nil {
		t.Fatal(err)
	}
	ipInfo, err = ipam.First("pod3")
	if err != nil || ipInfo.IPInfo.IP == nil {
		t.Fatalf("err %v ipInfo %v", err, ipInfo)
	}
	ipInfo.FIP.UpdatedAt = time.Time{}
	if fmt.Sprintf("%+v", ipInfo) != "&{IPInfo:{IP:10.173.13.2/24 Vlan:2 Gateway:10.173.13.1 RoutableSubnet:10.173.13.0/24} FIP:{Table: Key:pod3 Subnet:10.173.13.0/24 Attr:111 IP:179113218 Policy:2 UpdatedAt:0001-01-01 00:00:00 +0000 UTC}}" {
		t.Error(fmt.Sprintf("%+v", ipInfo))
	}
}

// TestDBReserveIP test ReserveIP function.
func TestDBReserveIP(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	testReserveIP(t, ipam)
}

// TestDBRelease test Release function.
func TestDBRelease(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	testRelease(t, ipam)
}

// TestDBReleaseIPs test ReleaseIPs function.
func TestDBReleaseIPs(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	testReleaseIPs(t, ipam)
}

// TestDBByKeyword test ByKeyword function.
func TestDBByKeyword(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	testByKeyword(t, ipam)
}

// TestDBByPrefix test ByPrefix function.
func TestDBByPrefix(t *testing.T) {
	ipam := Start(t)
	defer ipam.Shutdown()
	testByPrefix(t, ipam)
}
