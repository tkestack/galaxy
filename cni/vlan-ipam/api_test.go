package vlan_ipam

import (
	"net"
	"testing"
)

func TestTryGetIPInfo(t *testing.T) {
	if ipInfo := tryGetIPInfo(""); ipInfo != nil {
		t.Fatal()
	}
	if ipInfo := tryGetIPInfo("ip=192.168.1.1;IgnoreUnknown=true"); ipInfo != nil {
		t.Fatal()
	}
	if ipInfo := tryGetIPInfo("IP=192.168.1.1/24;Gateway=192.168.1.1;Vlan=3;IgnoreUnknown=true"); ipInfo == nil {
		t.Fatal()
	} else {
		if (*net.IPNet)(&ipInfo.IP).String() != "192.168.1.1/24" {
			t.Fatal((*net.IPNet)(&ipInfo.IP).String())
		}
		if ipInfo.Gateway.String() != "192.168.1.1" {
			t.Fatal()
		}
		if ipInfo.Vlan != 3 {
			t.Fatal()
		}
	}
}
