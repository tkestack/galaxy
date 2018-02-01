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
