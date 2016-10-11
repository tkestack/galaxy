package main

import (
	"fmt"
	"runtime"

	"git.code.oa.com/gaiastack/galaxy/pkg/network/vlan"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/skel"
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
	if err := d.SetupBridge(); err != nil {
		return fmt.Errorf("failed to setup bridge %v", err)
	}
	// run the IPAM plugin and get back the config to apply
	result, err := ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}
	if result.IP4 == nil {
		return fmt.Errorf("IPAM plugin returned missing IPv4 config")
	}
	if err := d.CreateVlanDevice(0); err != nil {
		return err
	}
	if err := d.CreateVeth(result, args, 0); err != nil {
		return err
	}

	result.DNS = conf.DNS
	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf, err := d.LoadConf(args.StdinData)
	if err != nil {
		return err
	}

	if err := d.DeleteVeth(args); err != nil {
		return err
	}

	if err := ipam.ExecDel(conf.IPAM.Type, args.StdinData); err != nil {
		return err
	}
	return nil
}

func main() {
	d = &vlan.VlanDriver{}
	skel.PluginMain(cmdAdd, cmdDel)
}
