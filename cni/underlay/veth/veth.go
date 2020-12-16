package main

import (
	"encoding/json"
	"fmt"
	"net"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
	"tkestack.io/galaxy/cni/ipam"
	"tkestack.io/galaxy/pkg/network"
	"tkestack.io/galaxy/pkg/network/vlan"
	"tkestack.io/galaxy/pkg/utils"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}

func cmdDel(args *skel.CmdArgs) error {
	if err := utils.DeleteAllVeth(args.Netns); err != nil {
		return err
	}
	return nil
}

func cmdAdd(args *skel.CmdArgs) error {
	conf := vlan.NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("conf error: %v", err)
	}
	vlanIds, results, err := ipam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return fmt.Errorf("allocate failed: %v", err)
	}
	if err := utils.UnSetArpIgnore("all"); err != nil {
		return err
	}
	if err := utils.EnableNonlocalBind(); err != nil {
		return err
	}
	ifName := args.IfName
	ifIndex := 0
	for i := range vlanIds {
		vlanId := vlanIds[i]
		result, err := t020.GetResult(results[i])
		if err != nil {
			return fmt.Errorf("result convert failed: %v", err)
		}
		device := conf.Device
		// fixme: make route configurable
		if i != 0 {
			result.IP4.Routes = []types.Route{{
				Dst: net.IPNet{
					IP:   result.IP4.IP.IP.Mask(result.IP4.IP.Mask),
					Mask: result.IP4.IP.Mask,
				},
			}}
		}
		var masterDevice netlink.Link
		if masterDevice, err = vlan.SetupVlanInPureMode(device, vlanId); err != nil {
			return fmt.Errorf("failed setup vlan: %v", err)
		}
		suffix := fmt.Sprintf("-%s%d", utils.UnderlayVethDeviceSuffix, i+1)
		if i != 0 {
			ifIndex++
			args.IfName = fmt.Sprintf("eth%d", ifIndex)
			if args.IfName == ifName {
				ifIndex++
				args.IfName = fmt.Sprintf("eth%d", ifIndex)
			}
		}
		v4Addr, err := netlink.AddrList(masterDevice, netlink.FAMILY_V4)
		if err != nil {
			return fmt.Errorf("error getting ipv4 address %v", err)
		}
		filteredAddr := network.FilterLoopbackAddr(v4Addr)
		var src net.IP
		if len(filteredAddr) > 0 {
			src = filteredAddr[0].IP
		}
		if err := utils.VethConnectsHostWithContainer(result, args, "", suffix, src); err != nil {
			return fmt.Errorf("veth connect failed: %v", err)
		}
		utils.SendGratuitousARP(masterDevice.Attrs().Name, result.IP4.IP.IP.String(), "", conf.GratuitousArpRequest)
	}
	args.IfName = ifName
	result, _ := t020.GetResult(results[0])
	return result.Print()
}
