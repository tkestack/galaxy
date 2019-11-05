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
package helper

import (
	"net"
	"testing"
)

func TestGateway(t *testing.T) {
	gateway := Gateway(net.IPNet{IP: net.ParseIP("192.168.0.2"), Mask: net.CIDRMask(24, 32)})
	if gateway == nil || gateway.String() != "192.168.0.1" {
		t.Error(gateway)
	}
	gateway = Gateway(net.IPNet{IP: net.ParseIP("192.168.0.68"), Mask: net.CIDRMask(26, 32)})
	if gateway == nil || gateway.String() != "192.168.0.65" {
		t.Error(gateway)
	}
}

func TestIPInfo(t *testing.T) {
	ipInfo, err := IPInfo("192.168.0.2/24", 0)
	if err != nil {
		t.Error()
	}
	if ipInfo != `{"ip":"192.168.0.2/24","vlan":0,"gateway":"192.168.0.1","routable_subnet":"192.168.0.0/24"}` {
		t.Error(ipInfo)
	}
	ipInfo, err = IPInfo("192.168.0.68/26", 3)
	if err != nil {
		t.Error()
	}
	if ipInfo != `{"ip":"192.168.0.68/26","vlan":3,"gateway":"192.168.0.65","routable_subnet":"192.168.0.64/26"}` {
		t.Error(ipInfo)
	}
}
