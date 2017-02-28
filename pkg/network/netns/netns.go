package netns

import (
	"runtime"

	"github.com/vishvananda/netns"
)

func NsInvoke(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save the current network namespace
	origns, _ := netns.Get()
	defer origns.Close()

	// Create a new network namespace
	newns, _ := netns.New()
	netns.Set(newns)
	defer newns.Close()
	f()
	netns.Set(origns)
}
