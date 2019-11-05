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
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestDeleteHostVeth(t *testing.T) {
	containerId := "TestDeleteHostVeth"
	// check delete a not exist veth
	if err := DeleteHostVeth(containerId); err != nil {
		t.Fatal(err)
	}

	if err := netlink.LinkAdd(&netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: HostVethName(containerId, "")}, PeerName: ContainerVethName(containerId, "")}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteHostVeth(containerId); err != nil {
		t.Fatal(err)
	}

	if _, err := netlink.LinkByName(HostVethName(containerId, "")); err == nil || !strings.Contains(err.Error(), "Link not found") {
		t.Fatal(err)
	}
}
