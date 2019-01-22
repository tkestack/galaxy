package utils

import (
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestDeleteHostVeth(t *testing.T) {
	containerId := "TestDeleteHostVeth"
	// check delete a not exist veth
	if err := DeleteHostVeth(containerId); err != nil {
		t.Fatal(err)
	}

	if err := netlink.LinkAdd(&netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: HostVethName(containerId, "")}, PeerName: ContainerVethName(containerId, "")}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteHostVeth(containerId); err != nil {
		t.Fatal(err)
	}

	if _, err := netlink.LinkByName(HostVethName(containerId, "")); err == nil || !strings.Contains(err.Error(), "Link not found") {
		t.Fatal(err)
	}
}
