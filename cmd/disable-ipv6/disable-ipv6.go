package main

import (
	"os"
	"fmt"

	"git.code.oa.com/gaiastack/galaxy/pkg/network"
	"git.code.oa.com/gaiastack/galaxy/pkg/cmdline"
)

func main() {
	cmdline.NSInvoke(func() {
		if err := network.DisableIPv6("all"); err != nil {
			fmt.Errorf("failed to disable IPv6 forwarding %v", err)
			os.Exit(4)
		}
	})
}


