package main

import (
	"fmt"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"

	"git.code.oa.com/gaiastack/galaxy/cni/vlan-ipam"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/vlan"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
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
	ipamConf, err := vlan_ipam.LoadIPAMConf(args.StdinData)
	if err != nil {
		return err
	}
	if err := d.Init(); err != nil {
		return fmt.Errorf("failed to setup bridge %v", err)
	}
	kvMap, err := k8s.ParseK8SCNIArgs(args.Args)
	if err != nil {
		return err
	}
	result, vlanId, err := vlan_ipam.Allocate(ipamConf, args.Args, kvMap)
	if err != nil {
		return err
	}
	if d.MacVlanMode() {
		if err := d.CreateVlanDevice(vlanId); err != nil {
			return err
		}
		if err := utils.MacVlanConnectsHostWithContainer(result, args, d.DeviceIndex); err != nil {
			return err
		}
	} else {
		if err := d.CreateBridgeAndVlanDevice(vlanId); err != nil {
			return err
		}
		if err := utils.VethConnectsHostWithContainer(result, args, d.BridgeNameForVlan(vlanId)); err != nil {
			return err
		}
	}
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	utils.SendGratuitousARP(result, args)
	result.DNS = conf.DNS
	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	if err := utils.DeleteVeth(args.Netns, args.IfName); err != nil {
		return err
	}
	return nil
}

func main() {
	d = &vlan.VlanDriver{}
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}
