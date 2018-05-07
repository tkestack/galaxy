package main

import (
	"fmt"
	"runtime"

	"git.code.oa.com/gaiastack/galaxy/cni/ipam"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/vlan"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
)

var (
	d *vlan.VlanDriver
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := d.LoadConf(args.StdinData)
	if err != nil {
		return err
	}
	if err := d.Init(); err != nil {
		return fmt.Errorf("failed to setup bridge %v", err)
	}
	vlanId, result, err := ipam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return err
	}
	result020, err := t020.GetResult(result)
	if err != nil {
		return err
	}
	if d.MacVlanMode() {
		if err := d.MaybeCreateVlanDevice(vlanId); err != nil {
			return err
		}
		if err := utils.MacVlanConnectsHostWithContainer(result020, args, d.DeviceIndex); err != nil {
			return err
		}
	} else if d.IPVlanMode() {
		if err := d.MaybeCreateVlanDevice(vlanId); err != nil {
			return err
		}
		if err := utils.IPVlanConnectsHostWithContainer(result020, args, d.DeviceIndex); err != nil {
			return err
		}
	} else {
		if err := d.CreateBridgeAndVlanDevice(vlanId); err != nil {
			return err
		}
		if err := utils.VethConnectsHostWithContainer(result020, args, d.BridgeNameForVlan(vlanId)); err != nil {
			return err
		}
	}
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	if d.PureMode() {
		utils.SendGratuitousARP(d.Device, result020.IP4.IP.IP.String(), "")
	} else {
		utils.SendGratuitousARP(args.IfName, result020.IP4.IP.IP.String(), args.Netns)
	}
	result020.DNS = conf.DNS
	return result020.Print()
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
