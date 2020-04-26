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
	"net"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
	"tkestack.io/galaxy/cni/ipam"
	"tkestack.io/galaxy/pkg/network/vlan"
	"tkestack.io/galaxy/pkg/utils"
)

var (
	d                   *vlan.VlanDriver
	pANet, pBNet, pCNet *net.IPNet
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
	_, pANet, _ = net.ParseCIDR("10.0.0.0/8")
	_, pBNet, _ = net.ParseCIDR("172.16.0.0/12")
	_, pCNet, _ = net.ParseCIDR("192.168.0.0/16")
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := d.LoadConf(args.StdinData)
	if err != nil {
		return err
	}
	vlanIds, results, err := ipam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return err
	}
	if d.DisableDefaultBridge == nil {
		defaultTrue := true
		d.DisableDefaultBridge = &defaultTrue
		for i := range vlanIds {
			if vlanIds[i] == 0 {
				*d.DisableDefaultBridge = false
			}
		}
	}
	if err := d.Init(); err != nil {
		return fmt.Errorf("failed to setup bridge %v", err)
	}
	result020s, err := resultConvert(results)
	if err != nil {
		return err
	}
	if err := setupNetwork(result020s, vlanIds, args); err != nil {
		return err
	}
	result020s[0].DNS = conf.DNS
	return result020s[0].Print()
}

func setupNetwork(result020s []*t020.Result, vlanIds []uint16, args *skel.CmdArgs) error {
	if d.MacVlanMode() {
		if err := setupMacvlan(result020s[0], vlanIds[0], args); err != nil {
			return err
		}
	} else if d.IPVlanMode() {
		if err := setupIPVlan(result020s[0], vlanIds[0], args); err != nil {
			return err
		}
	} else {
		ifName := args.IfName
		if err := setupVlanDevice(result020s, vlanIds, args); err != nil {
			return err
		}
		args.IfName = ifName
	}
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	if d.PureMode() {
		_ = utils.SendGratuitousARP(d.Device, result020s[0].IP4.IP.IP.String(), "", d.GratuitousArpRequest)
	}
	return nil
}

func setupMacvlan(result *t020.Result, vlanId uint16, args *skel.CmdArgs) error {
	if err := d.MaybeCreateVlanDevice(vlanId); err != nil {
		return err
	}
	if err := utils.MacVlanConnectsHostWithContainer(result, args, d.DeviceIndex); err != nil {
		return err
	}
	_ = utils.SendGratuitousARP(args.IfName, result.IP4.IP.IP.String(), args.Netns, d.GratuitousArpRequest)
	return nil
}

func setupIPVlan(result *t020.Result, vlanId uint16, args *skel.CmdArgs) error {
	if err := d.MaybeCreateVlanDevice(vlanId); err != nil {
		return err
	}
	if err := utils.IPVlanConnectsHostWithContainer(result, args, d.DeviceIndex); err != nil {
		return err
	}
	_ = utils.SendGratuitousARP(args.IfName, result.IP4.IP.IP.String(), args.Netns, d.GratuitousArpRequest)
	return nil
}

func setupVlanDevice(result020s []*t020.Result, vlanIds []uint16, args *skel.CmdArgs) error {
	ifName := args.IfName
	ifIndex := 0
	for i := 0; i < len(result020s); i++ {
		vlanId := vlanIds[i]
		result020 := result020s[i]
		bridgeName, err := d.CreateBridgeAndVlanDevice(vlanId)
		if err != nil {
			return err
		}
		suffix := ""
		if i != 0 {
			suffix = fmt.Sprintf("-%d", i+1)
			ifIndex++
			args.IfName = fmt.Sprintf("eth%d", ifIndex)
			if args.IfName == ifName {
				ifIndex++
				args.IfName = fmt.Sprintf("eth%d", ifIndex)
			}
		}
		if err := utils.VethConnectsHostWithContainer(result020, args, bridgeName, suffix, nil); err != nil {
			return err
		}
		_ = utils.SendGratuitousARP(args.IfName, result020s[0].IP4.IP.IP.String(), args.Netns, d.GratuitousArpRequest)
	}
	return nil
}

func resultConvert(results []types.Result) ([]*t020.Result, error) {
	var result020s []*t020.Result
	for i := 0; i < len(results); i++ {
		result020, err := t020.GetResult(results[i])
		if err != nil {
			return nil, err
		}
		result020s = append(result020s, result020)
	}
	if len(result020s) == 2 {
		routes := result020s[0].IP4.Routes
		for i := 0; i < len(routes); i++ {
			if routes[i].Dst.String() == "0.0.0.0/0" {
				routes = append(routes[:i], routes[i+1:]...)
				break
			}
		}
		routes = append(routes, types.Route{Dst: *pANet}, types.Route{Dst: *pBNet}, types.Route{Dst: *pCNet})
		result020s[0].IP4.Routes = routes
	}
	return result020s, nil
}

func cmdDel(args *skel.CmdArgs) error {
	if err := utils.DeleteAllVeth(args.Netns); err != nil {
		return err
	}
	conf, err := d.LoadConf(args.StdinData)
	if err != nil {
		return err
	}
	return ipam.Release(conf.IPAM.Type, args)
}

func main() {
	d = &vlan.VlanDriver{}
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}
