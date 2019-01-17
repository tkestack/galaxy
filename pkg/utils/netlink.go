package utils

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// CreateBridgeDevice create a new bridge interface/
func CreateBridgeDevice(bridgeName string, hwAddr net.HardwareAddr) error {
	// Set the bridgeInterface netlink.Bridge.
	bridgeIf := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: bridgeName,
		},
	}
	if err := netlink.LinkAdd(bridgeIf); err != nil {
		return fmt.Errorf("failed to create bridge device %s: %v", bridgeIf.Name, err)
	}
	if hwAddr == nil {
		hwAddr = GenerateRandomMAC()
	}
	if err := netlink.LinkSetHardwareAddr(bridgeIf, hwAddr); err != nil {
		return fmt.Errorf("failed to set bridge mac-address %s : %s", hwAddr, err.Error())
	}
	return nil
}

func AddToBridge(ifaceName, bridgeName string) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("could not find interface %s: %v", ifaceName, err)
	}
	if err = netlink.LinkSetMaster(link,
		&netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}); err != nil {
		_, err1 := net.InterfaceByName(ifaceName)
		if err1 != nil {
			return fmt.Errorf("could not find network interface %s: %v", ifaceName, err1)
		}
		_, err1 = net.InterfaceByName(bridgeName)
		if err != nil {
			return fmt.Errorf("could not find bridge %s: %v", bridgeName, err1)
		}
		return err
	}
	return nil
}
