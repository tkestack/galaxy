package constant

import (
	"net"
	"reflect"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
)

func TestFormatParseIPInfo(t *testing.T) {
	testCase := []IPInfo{
		{
			IP:      nets.NetsIPNet(&net.IPNet{IP: net.ParseIP("192.168.0.2"), Mask: net.IPv4Mask(255, 255, 0, 0)}),
			Vlan:    2,
			Gateway: net.ParseIP("192.168.0.1"),
		},
		{
			IP:      nets.NetsIPNet(&net.IPNet{IP: net.ParseIP("192.168.0.3"), Mask: net.IPv4Mask(255, 255, 0, 0)}),
			Vlan:    3,
			Gateway: net.ParseIP("192.168.0.1"),
		},
	}
	str, err := FormatIPInfo(testCase)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(str)
	parsed, err := ParseIPInfo(str)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, testCase) {
		t.Fatalf("real: %v, expect: %v", parsed, testCase)
	}
}
