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
	"os"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
)

var interval = 5 * time.Minute

func BridgeNFCallIptables(quit <-chan struct{}, set bool) {
	expect := "1"
	if !set {
		expect = "0"
	}
	setArg(expect, "/proc/sys/net/bridge/bridge-nf-call-iptables", quit)
}

func IPForward(quit <-chan struct{}, set bool) {
	expect := "1"
	if !set {
		expect = "0"
	}
	setArg(expect, "/proc/sys/net/ipv4/ip_forward", quit)
}

func DisableRPFilter(quit <-chan struct{}) {
	setArg("0", "/proc/sys/net/ipv4/conf/all/rp_filter", quit)
	setArg("0", "/proc/sys/net/ipv4/conf/eth0/rp_filter", quit)
	setArg("0", "/proc/sys/net/ipv4/conf/eth1/rp_filter", quit)
}

func setArg(expect string, file string, quit <-chan struct{}) {
	go wait.Until(func() {
		glog.V(4).Infof("starting to ensure kernel args %s", file)
		data, err := ioutil.ReadFile(file)
		if err != nil {
			glog.Warningf("Error open %s: %v", file, err)
		}
		if string(data) != expect+"\n" {
			glog.Warningf("%s unset, setting it", file)
			if err := ioutil.WriteFile(file, []byte(expect), 0644); err != nil && !os.IsNotExist(err) {
				glog.Warningf("Error set kernel args %s: %v", file, err)
			}
		}
	}, interval, quit)
}

// nolint: deadcode
func remountSysfs() error {
	if err := syscall.Mount("", "/", "none", syscall.MS_SLAVE|syscall.MS_REC, ""); err != nil {
		return err
	}
	if err := syscall.Unmount("/sys", syscall.MNT_DETACH); err != nil {
		return err
	}
	return syscall.Mount("sysfs", "/sys", "sysfs", 0, "")
}
