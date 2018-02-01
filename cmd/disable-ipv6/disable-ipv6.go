package main

import (
	"fmt"
	"os"

	"git.code.oa.com/gaiastack/galaxy/pkg/cmdline"
	"git.code.oa.com/gaiastack/galaxy/pkg/network"
)

func main() {
	cmdline.NSInvoke(func() {
		if err := network.DisableIPv6("all"); err != nil {
			fmt.Errorf("failed to disable IPv6 forwarding %v", err)
			os.Exit(4)
		}
	})
}
