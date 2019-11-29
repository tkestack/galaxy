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
	"flag"
	"syscall"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	log "k8s.io/klog"
	"tkestack.io/galaxy/pkg/network"
)

var flagDevice = flag.String("device", "", "device name to listen")

func main() {
	flag.Parse()
	if *flagDevice == "" {
		log.Fatalf("please specify device name")
	}
	link, err := netlink.LinkByName(*flagDevice)
	if err != nil {
		log.Fatalf("failed to get device %s: %v", *flagDevice, err)
	}
	dev := device{l: link}
	dev.MonitorMisses()
}

type device struct {
	l netlink.Link
}

func (dev *device) MonitorMisses() {
	nlsock, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_NEIGH)
	if err != nil {
		log.Error("Failed to subscribe to netlink RTNLGRP_NEIGH messages")
		return
	}

	for {
		msgs, err := nlsock.Receive()
		if err != nil {
			log.Errorf("Failed to receive from netlink: %v ", err)

			time.Sleep(1 * time.Second)
			continue
		}

		for _, msg := range msgs {
			dev.processNeighMsg(msg)
		}
	}
}

func (dev *device) processNeighMsg(msg syscall.NetlinkMessage) {
	neigh, err := netlink.NeighDeserialize(msg.Data)
	if err != nil {
		log.Errorf("Failed to deserialize netlink ndmsg: %v", err)
		return
	}
	log.V(1).Infof("receiving neigh msg %#v, neigh %#v", msg, neigh)

	if neigh.LinkIndex != dev.l.Attrs().Index {
		log.Infof("ignore neigh msg from kernel %#v: not equal device id %d", neigh, dev.l.Attrs().Index)
		return
	}

	if msg.Header.Type != syscall.RTM_GETNEIGH && msg.Header.Type != syscall.RTM_NEWNEIGH {
		log.Infof("ignore neigh msg from kernel %#v: msg type is wrong %d", neigh, msg.Header.Type)
		return
	}

	if !isNeighResolving(neigh.State) {
		log.Infof("ignore neigh msg from kernel %#v: invalid state %d", neigh, neigh.State)
		return
	}

	log.Infof("receive good neigh msg from kernel %#v", neigh)
}

func isNeighResolving(state int) bool {
	return (state & (netlink.NUD_INCOMPLETE | netlink.NUD_STALE | netlink.NUD_DELAY | netlink.NUD_PROBE)) != 0
}
