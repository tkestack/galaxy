package nets

import (
	"encoding/json"
	"net"
	"testing"
)

func TestIPRange(t *testing.T) {
	ipr := IPRange{
		First: net.IPv4(192, 168, 0, 0),
		Last:  net.IPv4(192, 168, 1, 1),
	}
	if ipr.Size() != 258 {
		t.Fatal(ipr.Size())
	}
	if !ipr.Contains(net.IPv4(192, 168, 0, 0)) {
		t.Fatal()
	}
	if !ipr.Contains(net.IPv4(192, 168, 0, 3)) {
		t.Fatal()
	}
	if !ipr.Contains(net.IPv4(192, 168, 1, 0)) {
		t.Fatal()
	}
	if !ipr.Contains(net.IPv4(192, 168, 1, 1)) {
		t.Fatal()
	}
	ipr = IPRange{
		First: net.IPv4(192, 168, 1, 1),
		Last:  net.IPv4(192, 168, 1, 1),
	}
	if ipr.Size() != 1 {
		t.Fatal(ipr.Size())
	}
	ipr = IPRange{}
	if ipr.Size() != 0 {
		t.Fatal(ipr.Size())
	}
}

func TestSparseSubnet(t *testing.T) {
	subnet := SparseSubnet{
		IPRanges: []IPRange{
			IPtoIPRange(net.IPv4(192, 168, 0, 2)),
			{
				First: net.IPv4(192, 168, 0, 0),
				Last:  net.IPv4(192, 168, 1, 1),
			},
			{
				First: net.IPv4(172, 17, 0, 0),
				Last:  net.IPv4(172, 17, 0, 2),
			},
		},
		Gateway: net.IPv4(192, 168, 0, 1),
		Mask:    net.IPv4Mask(255, 255, 0, 0),
	}
	ipnet := subnet.IPNet()
	if ipnet.String() != "192.168.0.0/16" {
		t.Fatal(ipnet.String())
	}
	// Test IPNet is immutable
	ipnet.IP = net.IPv4(127, 127, 0, 1)
	if subnet.IPNet().String() != "192.168.0.0/16" {
		t.Fatal(subnet.IPNet().String())
	}
	if subnet.Size() != 262 {
		t.Fatal(subnet.Size())
	}
}

func TestParseIPRange(t *testing.T) {
	ipr := ParseIPRange("192.168.0.0~192.168.1.2")
	if ipr == nil {
		t.Fatal()
	}
	if ipr.First.String() != "192.168.0.0" {
		t.Fatal(ipr.First.String())
	}
	if ipr.Last.String() != "192.168.1.2" {
		t.Fatal(ipr.Last.String())
	}
	ipr = ParseIPRange("192.168.0.0")
	if ipr.First.String() != "192.168.0.0" {
		t.Fatal(ipr.First.String())
	}
	if ipr.Last.String() != "192.168.0.0" {
		t.Fatal(ipr.Last.String())
	}
	ipr = ParseIPRange("192.168.0.0~192.168.0.256")
	if ipr != nil {
		t.Fatal(ipr)
	}
}

func TestIPNet(t *testing.T) {
	ipNet := IPNet(net.IPNet{IP: net.IPv4(192, 168, 0, 1), Mask: net.IPv4Mask(255, 255, 0, 0)})
	data, err := json.Marshal(&ipNet)
	if err != nil {
		t.Fatalf("data %s %v", string(data), err)
	}
	if string(data) != `"192.168.0.1/16"` {
		t.Fatal(string(data))
	}
	var ipNet1 IPNet
	err = json.Unmarshal([]byte(`"192.168.0.1/16"`), &ipNet1)
	if err != nil {
		t.Fatal(err)
	}
	if ipNet1.String() != "192.168.0.1/16" {
		t.Fatalf("%s", ipNet1.String())
	}
}

func TestIPToInt(t *testing.T) {
	if IPToInt(net.IPv4(0, 0, 1, 1)) != 257 {
		t.Fatal()
	}
	if IPToInt(net.IPv4(0, 0, 0, 0)) != 0 {
		t.Fatal()
	}
	if IPToInt(net.IP{}) != 0 {
		t.Fatal()
	}
}

func TestIntToIP(t *testing.T) {
	if !IntToIP(257).Equal(net.IPv4(0, 0, 1, 1)) {
		t.Fatal()
	}
}

func TestLastIPV4(t *testing.T) {
	// test IPv4len ip
	_, ipNet, _ := net.ParseCIDR("10.149.27.112/26")
	if LastIPV4(ipNet).String() != "10.149.27.127" {
		t.Fatal()
	}
	// test IPv6len ip
	ipNet = &net.IPNet{
		IP:   net.IPv4(10, 149, 27, 112),
		Mask: net.CIDRMask(26, 32),
	}
	if LastIPV4(ipNet).String() != "10.149.27.127" {
		t.Fatal()
	}
}

func TestFirstAndLastIP(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("10.149.27.112/26")
	first, last := FirstAndLastIP(ipNet)
	if first != uint32(10*256*256*256+149*256*256+27*256+64) {
		t.Fatal(first)
	}
	if last != uint32(10*256*256*256+149*256*256+27*256+127) {
		t.Fatal(last)
	}
}
