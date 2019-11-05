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
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	glog "k8s.io/klog"
)

var (
	flagAddSrc        = flag.Bool("add-src", true, "add src to route")
	flannelSubnetFile = flag.String("flannel-subnet-file", "/var/run/flannel/subnet.env", "flannel subnet file")
)

func main() {
	flag.Parse()
	ch := make(chan netlink.RouteUpdate)
	done := make(chan struct{})
	defer close(done)
	subnetIP := getRouteIP()
	if err := netlink.RouteSubscribe(ch, done); err != nil {
		glog.Errorf("can't subscribe route change event: %v", err)
	}
	for {
		select {
		case update, ok := <-ch:
			if !ok {
				glog.Errorf("route event closed for some unknown reason, re-subscribe")
				// some error happen and ch closed, recover
				ch = make(chan netlink.RouteUpdate)
				if err := netlink.RouteSubscribe(ch, done); err != nil {
					glog.Errorf("can't subscribe route change event: %v", err)
				}
			}
			if update.Type == syscall.RTM_DELROUTE {
				if update.Dst == nil || nl.GetIPFamily(update.Dst.IP) != nl.FAMILY_V4 {
					continue
				}
				glog.Infof("receive route delete event: %v", update.Route)
				index := update.Route.LinkIndex
				link, err := netlink.LinkByIndex(index)
				if err != nil {
					glog.Infof("unknow link index, route del event: %+v", update.Route)
					continue
				}
				if !strings.HasPrefix(link.Attrs().Name, "veth-h") {
					glog.Infof("not delete veth route, name %s: %+v", link.Attrs().Name, update.Route)
					continue
				}
				if *flagAddSrc && update.Route.Src == nil {
					if subnetIP == nil {
						subnetIP = getRouteIP()
					}
					if subnetIP != nil {
						update.Route.Src = subnetIP
					}
				}
				if err := netlink.RouteAdd(&update.Route); err != nil {
					glog.Warningf("failed to add back route for %s %+v: %v", link.Attrs().Name, update.Route, err)
					if *flagAddSrc && update.Route.Src == nil {
						// check if flannel subnet IP changed
						subnetIP = getRouteIP()
						if subnetIP == nil {
							glog.Warningf("subnet ip is nil")
							continue
						}
						update.Route.Src = subnetIP
						if err := netlink.RouteAdd(&update.Route); err != nil {
							glog.Warningf("failed to add back route for %s %+v: %v", link.Attrs().Name, update.Route, err)
						} else {
							glog.Infof("add back route for %s %+v", link.Attrs().Name, update.Route)
						}
					}
				} else {
					glog.Infof("add back route for %s %+v", link.Attrs().Name, update.Route)
				}
			}
		}
	}
}

func getRouteIP() net.IP {
	subnet, err := getFlannelSubnet(*flannelSubnetFile)
	if err != nil {
		glog.Warningf("failed to get flannel subnet %v", err)
		return nil
	}
	return subnet.IP
}

func getFlannelSubnet(fn string) (*net.IPNet, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close() // nolint: errcheck
	s := bufio.NewScanner(f)
	for s.Scan() {
		parts := strings.SplitN(s.Text(), "=", 2)
		switch parts[0] {
		case "FLANNEL_SUBNET":
			_, subnet, err := net.ParseCIDR(parts[1])
			if err != nil {
				return nil, err
			}
			return subnet, nil
		}
	}
	return nil, fmt.Errorf("can't find FLANNEL_SUBNET from %s", fn)
}
