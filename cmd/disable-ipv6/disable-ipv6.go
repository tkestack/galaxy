package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"git.code.oa.com/gaiastack/galaxy/pkg/cmdline"
)

func main() {
	cmdline.NSInvoke(func() {
		if err := ioutil.WriteFile(fmt.Sprintf("/proc/sys/net/ipv6/conf/all/disable_ipv6"), []byte{'1', '\n'}, 0644); err != nil {
			fmt.Errorf("failed to disable IPv6 forwarding %v", err)
			os.Exit(4)
		}
	})
}
