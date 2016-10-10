package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/vishvananda/netlink"
)

var (
	ErrNoDefaultRoute = errors.New("no default route was found")
)

// GetDefaultRouteGw returns the GW for the default route's interface.
func GetDefaultRouteGw() (net.IP, error) {
	if r, err := GetDefaultRoute(); err != nil {
		return nil, err
	} else {
		return r.Gw, nil
	}
}

// GetDefaultRoute returns the default route.
func GetDefaultRoute() (*netlink.Route, error) {
	routes, err := netlink.RouteList(nil, 0)
	if err != nil {
		return nil, err
	}
	for _, r := range routes {
		// a nil Dst means that this is the default route.
		if r.Dst == nil {
			return &r, nil
		}
	}
	return nil, ErrNoDefaultRoute
}

func ChildLinkSize(masterIndex int) (int, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return 0, err
	}

	var count int
	for _, link := range links {
		if link.Attrs().MasterIndex == masterIndex {
			count += 1
		}
	}
	return count, nil
}

func genMAC(ip net.IP) net.HardwareAddr {
	hw := make(net.HardwareAddr, 6)
	// The first byte of the MAC address has to comply with these rules:
	// 1. Unicast: Set the least-significant bit to 0.
	// 2. Address is locally administered: Set the second-least-significant bit (U/L) to 1.
	hw[0] = 0x02
	// The first 24 bits of the MAC represent the Organizationally Unique Identifier (OUI).
	// Since this address is locally administered, we can do whatever we want as long as
	// it doesn't conflict with other addresses.
	hw[1] = 0x42
	// Fill the remaining 4 bytes based on the input
	if ip == nil {
		rand.Read(hw[2:])
	} else {
		copy(hw[2:], ip.To4())
	}
	return hw
}

// GenerateRandomMAC returns a new 6-byte(48-bit) hardware address (MAC)
func GenerateRandomMAC() net.HardwareAddr {
	return genMAC(nil)
}

// GenerateMACFromIP returns a locally administered MAC address where the 4 least
// significant bytes are derived from the IPv4 address.
func GenerateMACFromIP(ip net.IP) net.HardwareAddr {
	return genMAC(ip)
}

// GenerateRandomName returns a new name joined with a prefix.  This size
// specified is used to truncate the randomly generated value
func GenerateRandomName(prefix string, size int) (string, error) {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(id)[:size], nil
}

// GenerateIfaceName returns an interface name using the passed in
// prefix and the length of random bytes. The api ensures that the
// there are is no interface which exists with that name.
func GenerateIfaceName(prefix string, len int) (string, error) {
	for i := 0; i < 5; i++ {
		name, err := GenerateRandomName(prefix, len)
		if err != nil {
			continue
		}
		if _, err := netlink.LinkByName(name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return name, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("could not generate interface name")
}
