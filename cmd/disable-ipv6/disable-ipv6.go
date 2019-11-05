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
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"

	"github.com/vishvananda/netns"
)

// main is the main func of disable-ipv6
func main() {
	NSInvoke(func() {
		if err := ioutil.WriteFile(fmt.Sprintf("/proc/sys/net/ipv6/conf/all/disable_ipv6"),
			[]byte{'1', '\n'}, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to disable IPv6 forwarding %v", err) // nolint: errcheck
			os.Exit(4)
		}
	})
}

// NSInvoke invokes f inside container
func NSInvoke(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "invalid number of arguments for %s", os.Args[0]) // nolint: errcheck
		os.Exit(1)
	}

	ns, err := netns.GetFromPath(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed get network namespace %q: %v", os.Args[1], err) // nolint: errcheck
		os.Exit(2)
	}
	defer ns.Close() // nolint: errcheck

	if err = netns.Set(ns); err != nil {
		fmt.Fprintf(os.Stderr, "setting into container netns %q failed: %v", os.Args[1], err) // nolint: errcheck
		os.Exit(3)
	}

	f()
}
