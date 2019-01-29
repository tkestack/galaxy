package ips

import "testing"

func TestParseIPv4Mask(t *testing.T) {
	mask := ParseIPv4Mask("255.255.254.0")
	if mask == nil {
		t.Fatal()
	}
	if ones, _ := mask.Size(); ones != 23 {
		t.Fatal()
	}
	if e := ParseIPv4Mask("255.256.255.0"); e != nil {
		t.Fatal("expect parse error for mask 255.256.255.0")
	}

}

func TestParseCIDR(t *testing.T) {
	ipNet, err := ParseCIDR("192.168.0.1/24")
	if err != nil {
		t.Fatal(err)
	}
	if ipNet.String() != "192.168.0.1/24" {
		t.Fatal(ipNet.String())
	}
	if _, err := ParseCIDR("192.256.1.0/24"); err == nil {
		t.Fatal("expect parse error for CIDR 192.256.1.0/24")
	}
}
