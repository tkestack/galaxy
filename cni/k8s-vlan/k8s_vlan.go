package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/vlan"
)

var (
	d *vlan.VlanDriver
)

type IPAMConf struct {
	//ipam url, currently its the apiswitch
	URL         string `json:"url"`
	AllocateURI string `json:"allocate_uri"`
	NodeIP      string `json:"node_ip"`
	// get node ip from which network device
	Devices string `json:"devices"`
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
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	sendGratuitousARP(result, args)
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
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}

func loadIPAMConf(bytes []byte) (*IPAMConf, error) {
	conf := &IPAMConf{}
	if err := json.Unmarshal(bytes, conf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	if conf.NodeIP == "" {
		if conf.Devices == "" {
			return nil, fmt.Errorf("no node ip configured")
		}
		devices := strings.Split(conf.Devices, ",")
		if len(devices) == 0 {
			return nil, fmt.Errorf("no node ip configured")
		}
		for _, dev := range devices {
			nic, err := netlink.LinkByName(dev)
			if err != nil {
				continue
			}
			addr, err := netlink.AddrList(nic, netlink.FAMILY_V4)
			if err != nil {
				continue
			}
			if len(addr) == 1 {
				conf.NodeIP = addr[0].IPNet.String()
				break
			}
		}
		if conf.NodeIP == "" {
			return nil, fmt.Errorf("no node ip configured")
		}
	}
	if conf.AllocateURI == "" {
		conf.AllocateURI = "v2/network/floatingip/%s/allocate/%s"
	}
	return conf, nil
}

func sendGratuitousARP(result *types.Result, args *skel.CmdArgs) error {
	arping, err := exec.LookPath("arping")
	if err != nil {
		return fmt.Errorf("unable to locate arping")
	}
	command := exec.Command(arping, "-c", "2", "-A", "-I", args.IfName, result.IP4.IP.IP.String())
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()
	return netns.Do(func(_ ns.NetNS) error {
		_, err = command.CombinedOutput()
		return err
	})
}
