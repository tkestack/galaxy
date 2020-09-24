/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/pkg/api/cniutil"
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
		rand.Read(hw[2:]) // nolint: errcheck
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
	defer netns.Close() // nolint: errcheck

	return netns.Do(func(_ ns.NetNS) error {
		// get sbox device
		sbox, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to lookup sbox device %q: %v", ifName, err)
		}
		if sbox.Type() != "veth" {
			return nil
		}
		// shutdown sbox device
		if err = netlink.LinkSetDown(sbox); err != nil {
			return fmt.Errorf("failed to down sbox device %q %v: %v", sbox.Attrs().Name,
				sbox.Attrs().HardwareAddr, err)
		}

		if err = netlink.LinkDel(sbox); err != nil {
			return fmt.Errorf("failed to delete sbox device %q %v: %v", sbox.Attrs().Name,
				sbox.Attrs().HardwareAddr, err)
		}
		return nil
	})
}

// DeleteHostVeth deletes veth device in the host network namespace
func DeleteHostVeth(containerId string) error {
	hostIfName := HostVethName(containerId, "")
	link, err := netlink.LinkByName(hostIfName)
	if err != nil {
		// return nil if we can't find host veth device
		return nil
	}
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete host device %q: %v", hostIfName, err)
	}
	return nil
}

// DeleteAllVeth deletes all veth device inside the container
func DeleteAllVeth(netnsPath string) error {
	netns, err := ns.GetNS(netnsPath)
	if err != nil {
		if _, ok := err.(ns.NSPathNotExistErr); ok {
			return nil
		}
		return fmt.Errorf("failed to open netns %q: %v", netnsPath, err)
	}
	defer netns.Close() // nolint: errcheck

	return netns.Do(func(_ ns.NetNS) error {
		links, err := netlink.LinkList()
		if err != nil {
			return fmt.Errorf("failed to list links in netns %s", netnsPath)
		}
		for _, link := range links {
			if link.Type() != "veth" {
				continue
			}
			// shutdown sbox device
			if err = netlink.LinkSetDown(link); err != nil {
				err = fmt.Errorf("failed to down sbox device %q %v: %v", link.Attrs().Name,
					link.Attrs().HardwareAddr, err)
			} else if err = netlink.LinkDel(link); err != nil {
				err = fmt.Errorf("failed to delete sbox device %q %v: %v", link.Attrs().Name,
					link.Attrs().HardwareAddr, err)
			}
		}
		return err
	})
}

func HostVethName(containerId string, suffix string) string {
	return fmt.Sprintf("v-h%s%s", containerId[0:9], suffix)
}

func ContainerVethName(containerId string, suffix string) string {
	return fmt.Sprintf("v-s%s%s", containerId[0:9], suffix)
}

func HostMacVlanName(containerId string) string {
	return fmt.Sprintf("mv-%s", containerId[0:9])
}

func HostIPVlanName(containerId string) string {
	return fmt.Sprintf("iv-%s", containerId[0:9])
}

func CreateVeth(containerID string, mtu int, suffix string) (netlink.Link, netlink.Link, error) {
	hostIfName := HostVethName(containerID, suffix)
	containerIfName := ContainerVethName(containerID, suffix)
	// Generate and add the interface pipe host <-> sandbox
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostIfName, TxQLen: 0, MTU: mtu},
		PeerName:  containerIfName}
	if err := netlink.LinkAdd(veth); err != nil {
		return nil, nil, fmt.Errorf("failed to add the host %q <=> sandbox %q pair interfaces: %v",
			hostIfName, containerIfName, err)
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

func AddHostRoute(containerIP *net.IPNet, host netlink.Link, src net.IP) error {
	if err := netlink.RouteAdd(&netlink.Route{
		LinkIndex: host.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       containerIP,
		Gw:        nil,
		Src:       src,
	}); err != nil {
		if src != nil {
			// compatible change for old kernel which does not support src option such as tlinux 0041 0042
			if err1 := netlink.RouteAdd(&netlink.Route{
				LinkIndex: host.Attrs().Index,
				Scope:     netlink.SCOPE_LINK,
				Dst:       containerIP,
				Gw:        nil,
			}); err1 != nil {
				return fmt.Errorf("failed to add route '%v dev %v for old linux kernel': %v. With src option err: %v", containerIP, host.Attrs().Name, err1, err)
			} else {
				return nil
			}
		}
		return fmt.Errorf("failed to add route '%v dev %v src %v': %v", containerIP, host.Attrs().Name, src.String(), err)
	}
	return nil
}

// #lizard forgives
// VethConnectsHostWithContainer creates veth device pairs and connects container with host
// If bridgeName specified, it attaches host side veth device to the bridge
func VethConnectsHostWithContainer(result *t020.Result, args *skel.CmdArgs, bridgeName string, suffix string, src net.IP) error {
	host, sbox, err := CreateVeth(args.ContainerID, 1500, suffix)
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
	} else {
		// when vlanid=0 and in pure vlan mode, no bridge create, set proxy_arp instead
		if err = SetProxyArp(host.Attrs().Name); err != nil {
			return fmt.Errorf("error set proxyarp: %v", err)
		}
	}
	// Up the host interface after finishing all netlink configuration
	if err = netlink.LinkSetUp(host); err != nil {
		return fmt.Errorf("could not set link up for host interface %q: %v", host.Attrs().Name, err)
	}
	if bridgeName == "" {
		desIP := result.IP4.IP.IP
		ipn := net.IPNet{IP: desIP, Mask: net.CIDRMask(32, 32)}
		if err = AddHostRoute(&ipn, host, src); err != nil {
			return err
		}
	}
	if err = configSboxDevice(result, args, sbox); err != nil {
		return err
	}
	return nil
}

func SendGratuitousARP(dev, ip, nns string, useArpRequest bool) error {
	arping, err := exec.LookPath("arping")
	if err != nil {
		return fmt.Errorf("unable to locate arping")
	}

	var command *exec.Cmd
	if useArpRequest {
		command = exec.Command(arping, "-c", "2", "-U", "-I", dev, ip)
	} else {
		command = exec.Command(arping, "-c", "2", "-A", "-I", dev, ip)
	}

	if nns == "" {
		_, err = command.CombinedOutput()
		return err
	}
	netns, err := ns.GetNS(nns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", nns, err)
	}
	defer netns.Close() // nolint: errcheck
	return netns.Do(func(_ ns.NetNS) error {
		_, err = command.CombinedOutput()
		return err
	})
}

// MacVlanConnectsHostWithContainer creates macvlan device onto parent and connects container with host
func MacVlanConnectsHostWithContainer(result *t020.Result, args *skel.CmdArgs, parent int) error {
	var err error
	macVlan := &netlink.Macvlan{
		Mode: netlink.MACVLAN_MODE_BRIDGE,
		LinkAttrs: netlink.LinkAttrs{
			Name:        HostMacVlanName(args.ContainerID),
			MTU:         1500,
			ParentIndex: parent,
		}}
	if err := netlink.LinkAdd(macVlan); err != nil {
		return err
	}
	// nolint: errcheck
	defer func() {
		if err != nil {
			netlink.LinkDel(macVlan)
		}
	}()
	if err = configSboxDevice(result, args, macVlan); err != nil {
		return err
	}
	return nil
}

// IPVlanConnectsHostWithContainer creates ipvlan device onto parent device and connects container with host
func IPVlanConnectsHostWithContainer(result *t020.Result, args *skel.CmdArgs, parent int, mode netlink.IPVlanMode) error {
	var err error
	ipVlan := &netlink.IPVlan{
		Mode: mode,
		LinkAttrs: netlink.LinkAttrs{
			Name:        HostMacVlanName(args.ContainerID),
			MTU:         1500,
			ParentIndex: parent,
		}}
	if err := netlink.LinkAdd(ipVlan); err != nil {
		return err
	}
	// nolint: errcheck
	defer func() {
		if err != nil {
			netlink.LinkDel(ipVlan)
		}
	}()
	if err = configSboxDevice(result, args, ipVlan); err != nil {
		return err
	}
	return nil
}

func configSboxDevice(result *t020.Result, args *skel.CmdArgs, sbox netlink.Link) error {
	// Down the interface before configuring mac address.
	if err := netlink.LinkSetDown(sbox); err != nil {
		return fmt.Errorf("could not set link down for container interface %q: %v", sbox.Attrs().Name, err)
	}
	if sbox.Type() != "ipvlan" {
		if err := netlink.LinkSetHardwareAddr(sbox, GenerateMACFromIP(result.IP4.IP.IP)); err != nil {
			return fmt.Errorf("could not set mac address for container interface %q: %v", sbox.Attrs().Name, err)
		}
	}
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close() // nolint: errcheck
	// move sbox device to ns
	if err = netlink.LinkSetNsFd(sbox, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move sbox device %q to netns: %v", sbox.Attrs().Name, err)
	}
	return netns.Do(func(_ ns.NetNS) error {
		if err := netlink.LinkSetName(sbox, args.IfName); err != nil {
			return fmt.Errorf("failed to rename sbox device %q to %q: %v", sbox.Attrs().Name, args.IfName, err)
		}
		// disable rp_filter is needed when there're multi network device in container and host,
		// the arp request from host will pick source ip un-determined,
		// so device in container should disable rp_filter to answer this request
		if err := DisableRpFilter(args.IfName); err != nil {
			return fmt.Errorf("failed disable rp_filter to dev %s: %v", args.IfName, err)
		}
		// Add IP and routes to sbox, including default route
		return cniutil.ConfigureIface(args.IfName, result)
	})
}

func SetProxyArp(dev string) error {
	file := fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/proxy_arp", dev)
	return ioutil.WriteFile(file, []byte("1\n"), 0644)
}

func DisableRpFilter(dev string) error {
	file := fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/rp_filter", dev)
	return ioutil.WriteFile(file, []byte("0\n"), 0644)
}

func UnSetArpIgnore(dev string) error {
	file := fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/arp_ignore", dev)
	return ioutil.WriteFile(file, []byte("0\n"), 0644)
}

func EnableNonlocalBind() error {
	return ioutil.WriteFile("/proc/sys/net/ipv4/ip_nonlocal_bind", []byte("1\n"), 0644)
}
