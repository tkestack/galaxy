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
package gc

import (
	"testing"

	"strings"

	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/pkg/api/docker"
	"tkestack.io/galaxy/pkg/utils"
)

func TestCleanupVeth(t *testing.T) {
	dockerCli, err := docker.NewDockerInterface()
	if err != nil {
		t.Fatalf("init docker client failed: %v", err)
	}
	host, _, err := utils.CreateVeth("250d700f45ccb18925db0317cde6d9a48390c2ce49882d770115deeeeda55df4", 1500, "")
	if err != nil {
		t.Fatalf("can't setup veth pair: %v", err)
	}
	fgc := &flannelGC{dockerCli: dockerCli}
	if err := fgc.cleanupVeth(); err != nil {
		t.Fatal(err)
	} else {
		_, err := netlink.LinkByName(host.Attrs().Name)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expect no link exist, but found or got an error: %v", err)
		}
	}
}
