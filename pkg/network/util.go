package network

import (
	"fmt"
	"io/ioutil"
)

func DisableIPv6(ifName string) error {
	path := fmt.Sprintf("/proc/sys/net/ipv6/conf/%s/disable_ipv6", ifName)
	return ioutil.WriteFile(path, []byte{'1', '\n'}, 0644)
}
