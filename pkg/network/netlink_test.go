package network

import (
	"testing"

	"github.com/vishvananda/netlink"
)

func TestIsNeighResolving(t *testing.T) {
	t.Log(IsNeighResolving(32))
	t.Log(32 & (netlink.NUD_INCOMPLETE | netlink.NUD_STALE | netlink.NUD_DELAY | netlink.NUD_PROBE))
}
