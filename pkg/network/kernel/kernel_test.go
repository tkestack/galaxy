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
package kernel

import (
	"io/ioutil"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"tkestack.io/galaxy/pkg/network/netns"
)

func TestIPForward(t *testing.T) {
	t.Skip("need to upgrade to go 1.10+")
	teardown := netns.NewContainerForTest()
	defer teardown()
	// remount sysfs in the new netns
	if err := remountSysfs(); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	quit := make(chan struct{})
	// make loop runs quickly to avoid race condition
	interval = time.Millisecond * 10
	IPForward(quit, true)
	if err := wait.Poll(time.Millisecond*50, time.Minute, func() (done bool, err error) {
		data, err := ioutil.ReadFile("/proc/sys/net/ipv4/ip_forward")
		if err != nil {
			return false, err
		}
		if string(data) == "1\n" {
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	close(quit)
}
