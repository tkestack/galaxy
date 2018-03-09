package floatingip

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
)

var (
	ipNet = &net.IPNet{net.IPv4(10, 173, 14, 0), net.IPv4Mask(255, 255, 255, 0)}
	ip    = net.IPv4(10, 173, 14, 1)
)

func TestMarshalFloatingIPConf(t *testing.T) {
	subnet := nets.NetsIPNet(ipNet)
	conf := FloatingIPConf{
		RoutableSubnet: subnet,
		IPs: []string{
			"10.173.14.205",
			"10.173.14.206~10.173.14.208",
		},
		Subnet:  subnet,
		Gateway: ip,
		Vlan:    2,
	}
	if _, err := json.Marshal(conf); err != nil {
		t.Fatal(err)
	}
}

func TestMarshalFloatingIP(t *testing.T) {
	ipr := nets.ParseIPRange("10.173.14.206~10.173.14.208")
	fip := FloatingIP{
		RoutableSubnet: ipNet,
		SparseSubnet: nets.SparseSubnet{
			IPRanges: []nets.IPRange{nets.IPtoIPRange(net.ParseIP("10.173.14.205")), *ipr},
			Gateway:  net.ParseIP("10.173.14.1"),
			Mask:     net.CIDRMask(24, 8*net.IPv4len),
			Vlan:     2,
		},
	}
	if _, err := json.Marshal(&fip); err != nil {
		t.Fatal(err)
	}
}

func TestUnmarshalFloatingIP(t *testing.T) {
	var (
		confStr  = `{"routableSubnet":"10.173.14.1/24","ips":["10.173.14.203","10.173.14.206~10.173.14.208"],"subnet":"10.173.14.0/24","gateway":"10.173.14.1","vlan":2}`
		wrongStr = `{"routableSubnet":"10.173.14.0/24","ips":["10.173.14.205","10.173.14.206~10.173.14.208"],"subnet":"10.173.14.0/24","gateway":"10.173.14.1","vlan":2}`
		fip      FloatingIP
	)
	if err := json.Unmarshal([]byte(confStr), &fip); err != nil {
		t.Fatal(err)
	}
	if fip.RoutableSubnet.String() != ipNet.String() {
		t.Fatal()
	}
	if fip.IPNet().String() != ipNet.String() {
		t.Fatal()
	}
	if !fip.Gateway.Equal(ip) {
		t.Fatal()
	}
	if fip.Vlan != 2 {
		t.Fatal()
	}
	if len(fip.IPRanges) != 2 {
		t.Fatal()
	}
	if fip.IPRanges[0].First.String() != "10.173.14.203" {
		t.Fatal()
	}
	if fip.IPRanges[0].Last.String() != "10.173.14.203" {
		t.Fatal()
	}
	if fip.IPRanges[1].First.String() != "10.173.14.206" {
		t.Fatal()
	}
	if fip.IPRanges[1].Last.String() != "10.173.14.208" {
		t.Fatal()
	}
	if err := json.Unmarshal([]byte(confStr), &fip); err == nil {
		t.Fatal(wrongStr)
	}
}

func TestInsertRemoveIP(t *testing.T) {
	fip := &FloatingIP{
		SparseSubnet: nets.SparseSubnet{
			Gateway: net.ParseIP("10.166.141.65"),
			Mask:    net.CIDRMask(26, 32),
		},
	}
	fip.InsertIP(net.ParseIP("10.166.141.115"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.123"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.122"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.117"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.125"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123 10.166.141.125]" {
		t.Fatal(fip.IPRanges)
	}
	fip.InsertIP(net.ParseIP("10.166.141.116"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115~10.166.141.117 10.166.141.122~10.166.141.123 10.166.141.125]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.116"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123 10.166.141.125]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.125"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.117 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.117"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.122~10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.122"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115 10.166.141.123]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.123"))
	if fmt.Sprintf("%v", fip.IPRanges) != "[10.166.141.115]" {
		t.Fatal(fip.IPRanges)
	}
	fip.RemoveIP(net.ParseIP("10.166.141.115"))
	if len(fip.IPRanges) != 0 {
		t.Fatal(fip.IPRanges)
	}
}
