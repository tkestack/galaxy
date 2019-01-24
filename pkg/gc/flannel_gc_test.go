package gc

import (
	"testing"

	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"github.com/vishvananda/netlink"
)

func TestCleanupVeth(t *testing.T) {
	dockerCli, err := docker.NewDockerInterface()
	if err != nil {
		t.Fatalf("init docker client failed: %v", err)
	}
	host, _, err := utils.CreateVeth("250d700f45ccb18925db0317cde6d9a48390c2ce49882d770115deeeeda55df4", 1500, "")
	if err != nil {
		t.Fatalf("can't setup veth pair: %v", err)
	}
	fgc := &flannelGC{dockerCli: dockerCli}
	if err := fgc.cleanupVeth(); err != nil {
		t.Fatal(err)
	} else {
		_, err := netlink.LinkByName(host.Attrs().Name)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expect no link exist, but found or got an error: %v", err)
		}
	}
}
