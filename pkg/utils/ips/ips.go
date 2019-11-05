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
package ips

import "net"

func ParseIPv4Mask(mask string) net.IPMask {
	ip := net.ParseIP(mask)
	if ip == nil {
		return nil
	}
	return net.IPv4Mask(ip[12], ip[13], ip[14], ip[15])
}

// ParseCIDR returns cidr notation IP address
// This func differs with net.ParseCIDR which returns the masked cidr
func ParseCIDR(cidr string) (*net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ipNet.IP = ip
	return ipNet, nil
}
