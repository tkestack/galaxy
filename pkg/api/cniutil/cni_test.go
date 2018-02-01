package cniutil

import (
	"encoding/json"
	"net"
	"testing"
)

func TestUnmarshal(t *testing.T) {
	var ipInfo IPInfo
	if err := json.Unmarshal([]byte(`{"ip":"10.173.13.2/24","vlan":2,"gateway":"10.173.13.1","routable_subnet":"10.173.13.0/24"}`), &ipInfo); err != nil {
		t.Fatal(err)
	}
	if (*net.IPNet)(&ipInfo.IP).String() != "10.173.13.2/24" {
		t.Fatal((*net.IPNet)(&ipInfo.IP).String())
	}
	if ipInfo.Gateway.String() != "10.173.13.1" {
		t.Fatal()
	}
	if ipInfo.Vlan != 2 {
		t.Fatal()
	}
}
