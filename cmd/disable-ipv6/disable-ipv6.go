package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"

	"github.com/vishvananda/netns"
)

func main() {
	NSInvoke(func() {
		if err := ioutil.WriteFile(fmt.Sprintf("/proc/sys/net/ipv6/conf/all/disable_ipv6"), []byte{'1', '\n'}, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to disable IPv6 forwarding %v", err)
			os.Exit(4)
		}
	})
}

func NSInvoke(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "invalid number of arguments for %s", os.Args[0])
		os.Exit(1)
	}

	ns, err := netns.GetFromPath(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed get network namespace %q: %v", os.Args[1], err)
		os.Exit(2)
	}
	defer ns.Close()

	if err = netns.Set(ns); err != nil {
		fmt.Fprintf(os.Stderr, "setting into container netns %q failed: %v", os.Args[1], err)
		os.Exit(3)
	}

	f()
}
