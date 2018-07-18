package main

import (
	"fmt"
	"net"
	"runtime"

	"git.code.oa.com/gaiastack/galaxy/cni/ipam"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/vlan"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
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
	if err := d.Init(); err != nil {
		return fmt.Errorf("failed to setup bridge %v", err)
	}
	vlanIds, results, err := ipam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return err
	}
	var result020s []*t020.Result
	for i := 0; i < len(results); i++ {
		result020, err := t020.GetResult(results[i])
		if err != nil {
			return err
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

	if d.MacVlanMode() {
		if err := d.MaybeCreateVlanDevice(vlanIds[0]); err != nil {
			return err
		}
		if err := utils.MacVlanConnectsHostWithContainer(result020s[0], args, d.DeviceIndex); err != nil {
			return err
		}
		utils.SendGratuitousARP(args.IfName, result020s[0].IP4.IP.IP.String(), args.Netns)
	} else if d.IPVlanMode() {
		if err := d.MaybeCreateVlanDevice(vlanIds[0]); err != nil {
			return err
		}
		if err := utils.IPVlanConnectsHostWithContainer(result020s[0], args, d.DeviceIndex); err != nil {
			return err
		}
		utils.SendGratuitousARP(args.IfName, result020s[0].IP4.IP.IP.String(), args.Netns)
	} else {
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
			if err := utils.VethConnectsHostWithContainer(result020, args, bridgeName, suffix); err != nil {
				return err
			}
			utils.SendGratuitousARP(args.IfName, result020s[0].IP4.IP.IP.String(), args.Netns)
		}
		args.IfName = ifName
	}
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	if d.PureMode() {
		utils.SendGratuitousARP(d.Device, result020s[0].IP4.IP.IP.String(), "")
	}
	result020s[0].DNS = conf.DNS
	return result020s[0].Print()
}

func cmdDel(args *skel.CmdArgs) error {
	if err := utils.DeleteVeth(args.Netns, args.IfName); err != nil {
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
