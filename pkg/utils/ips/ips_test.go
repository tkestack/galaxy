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
}

func TestParseCIDR(t *testing.T) {
	ipNet, err := ParseCIDR("192.168.0.1/24")
	if err != nil {
		t.Fatal(err)
	}
	if ipNet.String() != "192.168.0.1/24" {
		t.Fatal(ipNet.String())
	}
}
