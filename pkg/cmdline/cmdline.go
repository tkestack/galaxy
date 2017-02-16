package cmdline

import (
	"fmt"
	"os"
	"runtime"

	"github.com/vishvananda/netns"
)

func NSInvoke(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if len(os.Args) < 2 {
		fmt.Errorf("invalid number of arguments for %s", os.Args[0])
		os.Exit(1)
	}

	ns, err := netns.GetFromPath(os.Args[1])
	if err != nil {
		fmt.Errorf("failed get network namespace %q: %v", os.Args[1], err)
		os.Exit(2)
	}
	defer ns.Close()

	if err = netns.Set(ns); err != nil {
		fmt.Errorf("setting into container netns %q failed: %v", os.Args[1], err)
		os.Exit(3)
	}

	f()
}
