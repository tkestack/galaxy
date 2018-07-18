package floatingip_test

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/jinzhu/gorm"

	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
)

func Start(t *testing.T) floatingip.IPAM {
	return CreateIPAMWithTableName(t, database.DefaultFloatingipTableName)
}

func CreateIPAMWithTableName(t *testing.T, tableName string) floatingip.IPAM {
	var err error
	db, err := database.NewTestDB()
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	var conf struct {
		Floatingips []*floatingip.FloatingIP `json:"floatingips"`
	}
	if err := json.Unmarshal([]byte(database.TestConfig), &conf); err != nil {
		t.Fatal(err)
	}
	i := floatingip.NewIPAMWithTableName(db, tableName)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf("TRUNCATE %s;", tableName)).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := i.ConfigurePool(conf.Floatingips); err != nil {
		t.Fatal(err)
	}
	// There should be 14 ips
	if m, err := i.QueryByPrefix(""); err != nil || len(m) != 14 {
		t.Fatalf("map %v, err %v", m, err)
	}
	return i
}

func TestAllocateRelease(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	ipam := Start(t)
	defer ipam.Shutdown()
	ips, err := ipam.Allocate([]string{"pod1-1", "pod1-2", "pod1-3"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v", ips) != "[10.49.27.205 10.49.27.216 10.49.27.217]" {
		t.Fatal(ips)
	}
	ipInfo, err := ipam.QueryFirst("pod1-1")
	if err != nil {
		t.Fatal(err)
	}
	if ipInfo.IP.String() != "10.49.27.205/24" {
		t.Fatal(ipInfo.IP.String())
	}
	if ipInfo.Gateway.String() != "10.49.27.1" {
		t.Fatal(ipInfo.Gateway.String())
	}
	if ipInfo.Vlan != 2 {
		t.Fatal(ipInfo.Vlan)
	}
	ipInfo, err = ipam.QueryFirst("pod1-4")
	if err != nil {
		t.Fatal(err)
	}
	if ipInfo != nil {
		t.Fatal(ipInfo)
	}
	if err := ipam.Release([]string{"pod1-2"}); err != nil {
		t.Fatal(err)
	}
	ips, err = ipam.Allocate([]string{"pod1-2", "pod1-4", "pod2-1"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v", ips) != "[10.49.27.216 10.49.27.218 10.173.13.2]" {
		t.Fatal(ips)
	}
	var fips []database.FloatingIP
	if err := ipam.Store().Transaction(func(tx *gorm.DB) error {
		return tx.Limit(100).Where("`key` = \"\"").Find(&fips).Error
	}); err != nil {
		t.Fatal(err)
	}
	if len(fips) != 9 {
		t.Fatal(len(fips))
	}
	if err := ipam.ReleaseByPrefix("pod1-"); err != nil {
		t.Fatal(err)
	}
	if err := ipam.Store().Transaction(func(tx *gorm.DB) error {
		return tx.Where("`key` != \"\"").Find(&fips).Error
	}); err != nil {
		t.Fatal(err)
	}
	if len(fips) != 1 {
		t.Fatal(len(fips))
	}
}

func TestApplyFloatingIPs(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	ipam := Start(t)
	defer ipam.Shutdown()
	fips := []floatingip.FloatingIP{}
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
	conf := ipam.ApplyFloatingIPs(fips)
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

func TestRaceCondition(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	var ipams []floatingip.IPAM
	for i := 0; i < 7; i++ {
		ipam := Start(t)
		defer ipam.Shutdown()
		ipams = append(ipams, ipam)
	}
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	var ips []net.IP
	for i, ipam := range ipams {
		wg.Add(1)
		j := i
		go func() {
			defer wg.Done()
			keys := []string{fmt.Sprintf("pod%d", j*2), fmt.Sprintf("pod%d", j*2+1)}
			allocated, err := ipam.Allocate(keys)
			if err != nil {
				t.Fatal(err)
			}
			lock.Lock()
			defer lock.Unlock()
			ips = append(ips, allocated...)
		}()
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

func TestEmptyFloatingIPConf(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	var err error
	db, err := database.NewTestDB()
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	i := floatingip.NewIPAM(db)
	defer i.Shutdown()
	if err := i.ConfigurePool(nil); err != nil {
		t.Fatal(err)
	}
}

func TestAllocateIPInSubnet(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	ipam := Start(t)
	defer ipam.Shutdown()
	_, routableSubnet, _ := net.ParseCIDR("10.173.13.0/24")
	if _, err := ipam.AllocateInSubnet("pod1-1", routableSubnet); err != nil {
		t.Fatal(err)
	}
	ipInfo, err := ipam.QueryFirst("pod1-1")
	if err != nil {
		t.Fatal(err)
	}
	if ipInfo.IP.String() != "10.173.13.2/24" {
		t.Fatal(ipInfo.IP.String())
	}
	//test can't find available ip
	_, routableSubnet, _ = net.ParseCIDR("10.173.14.0/24")
	if _, err := ipam.AllocateInSubnet("pod1-1", routableSubnet); err == nil || err != floatingip.ErrNoFIPForSubnet {
		t.Fatalf("should fail because of ErrNoFIPForSubnet: %v", err)
	}
}

func TestRoutableSubnet(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
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

func TestAllocateInSubnetAndQueryRoutableSubnetByKey(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	ipam := Start(t)
	defer ipam.Shutdown()

	subnets, err := ipam.QueryRoutableSubnetByKey("")
	if err != nil {
		t.Fatal(err)
	}
	// result in string order
	if fmt.Sprintf("%v", subnets) != "[10.173.13.0/24 10.180.1.2/32 10.180.1.3/32 10.49.27.0/24]" {
		t.Fatal(subnets)
	}
	// drain ips of 10.49.27.0/24
	if ips, err := ipam.Allocate([]string{"p1", "p2", "p3", "p4"}); err != nil || len(ips) != 4 {
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
	if ip, err := ipam.AllocateInSubnet("p5", ipnet); err != nil || ip == nil {
		t.Fatalf("ip %v err %v", ip, err)
	}
	if ip, err := ipam.AllocateInSubnet("p6", ipnet); err != nil || ip == nil {
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

func TestAllocateSpecificIP(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	ipam := Start(t)
	defer ipam.Shutdown()

	ip := net.ParseIP("10.49.27.216")
	if err := ipam.AllocateSpecificIP("pod1", ip); err != nil {
		t.Fatal(err)
	}
	key, err := ipam.QueryByIP(ip)
	if err != nil || key != "pod1" {
		t.Fatalf("key %s, err %v", key, err)
	}
	// check if an allocated ip can be allocated again, should return an ErrNotUpdated error
	if err := ipam.AllocateSpecificIP("pod2", ip); err == nil || err != floatingip.ErrNotUpdated {
		t.Fatal(err)
	}
}

func TestMultipleIPAM(t *testing.T) {
	database.ForceSequential <- true
	defer func() {
		<-database.ForceSequential
	}()
	ipam := Start(t)
	defer ipam.Shutdown()
	secondIPAM := CreateIPAMWithTableName(t, "test_table")
	defer secondIPAM.Shutdown()
	var check = func(ip net.IP, expectKey string) {
		t.Logf("testing expectKey %s, ip %s", expectKey, ip.String())
		t.Logf("secondIPAM...")
		// check secondIPAM query result is not empty
		key, err := secondIPAM.QueryByIP(ip)
		if err != nil || key != expectKey {
			t.Fatalf("key %s, err %v", key, err)
		}
		ipInfo, err := secondIPAM.QueryFirst(expectKey)
		if err != nil || ipInfo.IP.IP.String() != ip.String() {
			t.Fatalf("ipInfo %v, err %v", ipInfo, err)
		}
		m, err := secondIPAM.QueryByPrefix(expectKey)
		if err != nil {
			t.Fatal(err)
		}
		if len(m) == 0 || m[ip.String()] != expectKey {
			t.Fatalf("%v", m)
		}
		subnets, err := secondIPAM.QueryRoutableSubnetByKey(expectKey)
		if err != nil || len(subnets) == 0 {
			t.Fatalf("%v err %v", subnets, err)
		}
		// check ipam query result is empty
		t.Logf("ipam...")
		key, err = ipam.QueryByIP(ip)
		if err != nil || key != "" {
			t.Fatalf("key %s, err %v", key, err)
		}
		ipInfo, err = ipam.QueryFirst(expectKey)
		if err != nil || ipInfo != nil {
			t.Fatalf("ipInfo %v, err %v", ipInfo, err)
		}
		m, err = ipam.QueryByPrefix(expectKey)
		if err != nil {
			t.Fatal(err)
		}
		if len(m) != 0 {
			t.Fatalf("%v", m)
		}
		subnets, err = ipam.QueryRoutableSubnetByKey(expectKey)
		if err != nil || len(subnets) != 0 {
			t.Fatalf("%v err %v", subnets, err)
		}
	}
	ip := net.ParseIP("10.49.27.216")
	if err := secondIPAM.AllocateSpecificIP("pod1", ip); err != nil {
		t.Fatal(err)
	}
	check(ip, "pod1")

	_, routableSubnet, _ := net.ParseCIDR("10.173.13.0/24")
	ip2, err := secondIPAM.AllocateInSubnet("pod2", routableSubnet)
	if err != nil || ip2 == nil {
		t.Fatalf("ip %v, err %v", ip2, err)
	}
	check(ip2, "pod2")

	// check release ips
	if err := secondIPAM.Release([]string{"pod1"}); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := secondIPAM.QueryFirst("pod1"); err != nil || ipInfo != nil {
		t.Fatalf("ipInfo %v, err %v", ipInfo, err)
	}
	if err := secondIPAM.ReleaseByPrefix("pod2"); err != nil {
		t.Fatal(err)
	}
	if ipInfo, err := secondIPAM.QueryFirst("pod2"); err != nil || ipInfo != nil {
		t.Fatalf("ipInfo %v, err %v", ipInfo, err)
	}
}
