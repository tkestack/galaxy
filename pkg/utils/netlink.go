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
