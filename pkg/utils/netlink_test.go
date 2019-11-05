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
	"os"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestBridgeOps(t *testing.T) {
	env := os.Getenv("TEST_ENV")
	if env != "linux_root" {
		t.Skip("skip test")
	}
	mac := GenerateRandomMAC()
	briName, _ := GenerateIfaceName("bri", 5)
	dmyName, _ := GenerateIfaceName("dmy", 5)
	if err := CreateBridgeDevice(briName, mac); err != nil {
		t.Fatal(err)
	}
	if err := netlink.LinkAdd(&netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: dmyName,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := AddToBridge(dmyName, briName); err != nil {
		t.Fatal(err)
	}
	bri, err := netlink.LinkByName(briName)
	if err != nil {
		t.Fatal(err)
	}
	dmy0, err := netlink.LinkByName(dmyName)
	if err != nil {
		t.Fatal(err)
	}
	if dmy0.Attrs().MasterIndex != bri.Attrs().Index {
		t.Fatalf("expect %s(%d) has master %s with masterIndex %d but got %d", dmyName, dmy0.Attrs().Index, briName, bri.Attrs().Index, dmy0.Attrs().MasterIndex)
	}
}
