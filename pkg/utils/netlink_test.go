package utils

import (
	"os"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestBridgeOps(t *testing.T) {
	env := os.Getenv("TEST_ENV")
	if env != "linux_root" {
		t.Skip("skip test")
	}
	mac := GenerateRandomMAC()
	briName, _ := GenerateIfaceName("bri", 5)
	dmyName, _ := GenerateIfaceName("dmy", 5)
	if err := CreateBridgeDevice(briName, mac); err != nil {
		t.Fatal(err)
	}
	if err := netlink.LinkAdd(&netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: dmyName,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := AddToBridge(dmyName, briName); err != nil {
		t.Fatal(err)
	}
	bri, err := netlink.LinkByName(briName)
	if err != nil {
		t.Fatal(err)
	}
	dmy0, err := netlink.LinkByName(dmyName)
	if err != nil {
		t.Fatal(err)
	}
	if dmy0.Attrs().MasterIndex != bri.Attrs().Index {
		t.Fatalf("expect %s(%d) has master %s with masterIndex %d but got %d", dmyName, dmy0.Attrs().Index, briName, bri.Attrs().Index, dmy0.Attrs().MasterIndex)
	}
}
