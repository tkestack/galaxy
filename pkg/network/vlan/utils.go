package vlan

import (
	"fmt"

	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/pkg/utils"
)

// SetupVlan is used in pure veth mode, in this mode, vlan will not attach to bridge device
func SetupVlanInPureMode(ethDevice string, vlanId uint16) (netlink.Link, error) {
	if vlanId == 0 {
		device, err := netlink.LinkByName(ethDevice)
		if err != nil {
			return nil, fmt.Errorf("device %v not found", err)
		}
		if err := utils.UnSetArpIgnore(ethDevice); err != nil {
			return nil, err
		}
		if err := utils.SetProxyArp(ethDevice); err != nil {
			return nil, err
		}
		return device, nil
	}
	vlanDevice, err := findVlanDevice(vlanId)
	if err != nil {
		return nil, fmt.Errorf("find vlan device failed: %v", err)
	}
	if err = netlink.LinkSetUp(vlanDevice); err != nil {
		return nil, fmt.Errorf("failed set vlan %v up: %v", vlanDevice.Attrs().Name, err)
	}
	if err := utils.UnSetArpIgnore(vlanDevice.Attrs().Name); err != nil {
		return nil, err
	}
	return vlanDevice, utils.SetProxyArp(vlanDevice.Attrs().Name)
}

func findVlanDevice(vlanID uint16) (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	var result netlink.Link
	for _, link := range links {
		if link.Type() == "vlan" {
			if vlan, ok := link.(*netlink.Vlan); !ok {
				return nil, fmt.Errorf("vlan device type case error: %T", link)
			} else {
				if vlan.VlanId == int(vlanID) {
					if result == nil {
						result = link
					} else {
						return nil, fmt.Errorf("found 2 device with same vlanID %d: %s, %s", vlanID, result.Attrs().Name, vlan.Name)
					}
				}
			}
		}
	}
	if result == nil {
		return nil, fmt.Errorf("no vlan device with vlanID %d", vlanID)
	}
	return result, nil
}
