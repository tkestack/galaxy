package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/vlan"
)

var (
	d *vlan.VlanDriver
)

type IPAMConf struct {
	//ipam url, currently its the apiswitch
	URL         string `json:"url"`
	QueryURI    string `json:"query_uri"`
	AllocateURI string `json:"allocate_uri"`
	NodeIP      string `json:"node_ip"`
}

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
	ipamConf, err := loadIPAMConf(args.StdinData)
	if err != nil {
		return err
	}
	if err := d.SetupBridge(); err != nil {
		return fmt.Errorf("failed to setup bridge %v", err)
	}
	kvMap, err := k8s.ParseK8SCNIArgs(args.Args)
	if err != nil {
		return err
	}
	result, vlanId, err := allocate(ipamConf, args.Args, kvMap)
	if err != nil {
		return err
	}
	if err := d.CreateVlanDevice(vlanId); err != nil {
		return err
	}
	if err := d.CreateVeth(result, args, vlanId); err != nil {
		return err
	}
	result.DNS = conf.DNS
	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	if err := d.DeleteVeth(args); err != nil {
		return err
	}
	return nil
}

func main() {
	d = &vlan.VlanDriver{}
	skel.PluginMain(cmdAdd, cmdDel)
}

func loadIPAMConf(bytes []byte) (*IPAMConf, error) {
	conf := &IPAMConf{}
	if err := json.Unmarshal(bytes, conf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return conf, nil
}
