package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"

	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
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

// DeleteVeth deletes veth device inside the container
func DeleteVeth(netnsPath, ifName string) error {
	netns, err := ns.GetNS(netnsPath)
	if err != nil {
		if _, ok := err.(ns.NSPathNotExistErr); ok {
			return nil
		}
		return fmt.Errorf("failed to open netns %q: %v", netnsPath, err)
	}
	defer netns.Close()

	return netns.Do(func(_ ns.NetNS) error {
		// get sbox device
		sbox, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to lookup sbox device %q: %v", ifName, err)
		}

		// shutdown sbox device
		if err = netlink.LinkSetDown(sbox); err != nil {
			return fmt.Errorf("failed to down sbox device %q: %v", sbox.Attrs().Name, err)
		}

		if err = netlink.LinkDel(sbox); err != nil {
			return fmt.Errorf("failed to delete sbox device %q: %v", sbox.Attrs().Name, err)
		}
		return nil
	})
}

func HostVethName(containerId string) string {
	return fmt.Sprintf("veth-h%s", containerId[0:9])
}

func ContainerVethName(containerId string) string {
	return fmt.Sprintf("veth-s%s", containerId[0:9])
}

func CreateVeth(containerID string, mtu int) (netlink.Link, netlink.Link, error) {
	hostIfName := HostVethName(containerID)
	containerIfName := ContainerVethName(containerID)
	// Generate and add the interface pipe host <-> sandbox
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostIfName, TxQLen: 0, MTU: mtu},
		PeerName:  containerIfName}
	if err := netlink.LinkAdd(veth); err != nil {
		return nil, nil, fmt.Errorf("failed to add the host %q <=> sandbox %q pair interfaces: %v", hostIfName, containerIfName, err)
	}

	// Get the host side pipe interface handler
	host, err := netlink.LinkByName(hostIfName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find host side interface %q: %v", hostIfName, err)
	}

	// Get the sandbox side pipe interface handler
	sbox, err := netlink.LinkByName(containerIfName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find sandbox side interface %q: %v", containerIfName, err)
	}

	return host, sbox, nil
}

// ConnectsHostWithContainer creates veth device pairs and connects container with host
// If bridgeName specified, it attaches host side veth device to the bridge
func ConnectsHostWithContainer(result *types.Result, args *skel.CmdArgs, bridgeName string) error {
	host, sbox, err := CreateVeth(args.ContainerID, 1500)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if host != nil {
				netlink.LinkDel(host)
			}
			if sbox != nil {
				netlink.LinkDel(sbox)
			}
		}
	}()

	if bridgeName != "" {
		// Attach host side pipe interface into the bridge
		if err = AddToBridge(host.Attrs().Name, bridgeName); err != nil {
			return fmt.Errorf("adding interface %q to bridge %q failed: %v", host.Attrs().Name, bridgeName, err)
		}
	}
	// Down the interface before configuring mac address.
	if err = netlink.LinkSetDown(sbox); err != nil {
		return fmt.Errorf("could not set link down for container interface %q: %v", sbox.Attrs().Name, err)
	}

	if err = netlink.LinkSetHardwareAddr(sbox, GenerateMACFromIP(result.IP4.IP.IP)); err != nil {
		return fmt.Errorf("could not set mac address for container interface %q: %v", sbox.Attrs().Name, err)
	}

	// Up the host interface after finishing all netlink configuration
	if err = netlink.LinkSetUp(host); err != nil {
		return fmt.Errorf("could not set link up for host interface %q: %v", host.Attrs().Name, err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()
	// move sbox veth device to ns
	if err = netlink.LinkSetNsFd(sbox, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move sbox device %q to netns: %v", sbox.Attrs().Name, err)
	}
	return netns.Do(func(_ ns.NetNS) error {
		if err := netlink.LinkSetName(sbox, args.IfName); err != nil {
			return fmt.Errorf("failed to rename sbox device %q to %q: %v", sbox.Attrs().Name, args.IfName, err)
		}
		// Add IP and routes to sbox, including default route
		return ipam.ConfigureIface(args.IfName, result)
	})
}

func SendGratuitousARP(result *types.Result, args *skel.CmdArgs) error {
	arping, err := exec.LookPath("arping")
	if err != nil {
		return fmt.Errorf("unable to locate arping")
	}
	command := exec.Command(arping, "-c", "2", "-A", "-I", args.IfName, result.IP4.IP.IP.String())
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()
	return netns.Do(func(_ ns.NetNS) error {
		_, err = command.CombinedOutput()
		return err
	})
}
