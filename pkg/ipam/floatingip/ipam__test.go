package floatingip

import (
	"encoding/json"
	"net"
	"testing"
)

var (
	testcase = `[{"routableSubnet":"10.239.228.0/22","ips":["10.239.238.3~10.239.238.6","10.239.238.11","10.239.238.26~10.239.238.61","10.239.238.115~10.239.238.116","10.239.238.164","10.239.238.166","10.239.238.207","10.239.238.226","10.239.238.236"],"subnet":"10.239.236.0/22","gateway":"10.239.236.1","vlan":13}]`
)

func TestGetRoutableSubnet(t *testing.T) {
	var fips []*FloatingIP
	if err := json.Unmarshal([]byte(testcase), &fips); err != nil {
		t.Fatal(err)
	}
	ipam := &ipam{FloatingIPs: fips}
	ipnet := ipam.RoutableSubnet(net.ParseIP("10.239.229.142"))
	if ipnet == nil {
		t.Fatal()
	}
	ipnet = ipam.RoutableSubnet(net.ParseIP("10.239.230.32"))
	if ipnet == nil {
		t.Fatal()
	}
}
