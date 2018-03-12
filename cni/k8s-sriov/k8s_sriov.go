package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"

	galaxyIpam "git.code.oa.com/gaiastack/galaxy/cni/ipam"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
)

type NetConf struct {
	types.NetConf
	Device string `json:"device"`
	VFNum  int    `json:"vf_num"`
}

const kindTotalVfs = "sriov_totalvfs"
const kindNumVfs = "sriov_numvfs"

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func loadConf(bytes []byte) (*NetConf, error) {
	n := &NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	if n.Device == "" {
		return nil, fmt.Errorf(`"device" field is required. It specifies the host interface name to virtualize`)
	}
	if n.VFNum <= 0 {
		n.VFNum = 256
	}
	return n, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	vlanId, result, err := galaxyIpam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return err
	}
	result020, err := t020.GetResult(result)
	if err != nil {
		return err
	}
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	if err := setupVF(conf, result, args.IfName, int(vlanId), netns); err != nil {
		return err
	}
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	utils.SendGratuitousARP(result020, args)
	result020.DNS = conf.DNS
	return result020.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	if err = releaseVF(args.IfName, netns); err != nil {
		return err
	}
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	return galaxyIpam.Release(conf.IPAM.Type, args)
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}

// code from https://raw.githubusercontent.com/Intel-Corp/sriov-cni/master/sriov/sriov.go

func setupVF(conf *NetConf, result types.Result, podifName string, vlan int, netns ns.NetNS) error {
	resultCurrent, err := current.GetResult(result)
	if err != nil {
		return err
	}
	cpus := runtime.NumCPU()
	ifName := conf.Device
	var vfIdx int
	var infos []os.FileInfo

	m, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup master %q: %v", conf.Device, err)
	}

	// get the ifname sriov vf num
	vfTotal, err := getSriovNumVfs(ifName, kindTotalVfs)
	if err != nil {
		return err
	}

	if vfTotal <= 0 {
		return fmt.Errorf("no virtual function in the device %q: %v", ifName)
	}

	vfNums, err := getSriovNumVfs(ifName, kindNumVfs)
	if err != nil {
		return err
	}

	minVfNum := min(vfTotal, conf.VFNum)
	// only set vf when `sriov_numvfs` is 0
	if vfNums == 0 {
		if err := setSriovNumVfs(ifName, minVfNum); err != nil {
			return err
		}
	} else if vfNums < minVfNum {
		glog.Warning("sriov_numvfs is set but small")
	}

	for vf := 0; vf <= (minVfNum - 1); vf++ {
		vfDir := fmt.Sprintf("/sys/class/net/%s/device/virtfn%d/net", ifName, vf)
		if _, err := os.Lstat(vfDir); err != nil {
			if vf == (minVfNum - 1) {
				return fmt.Errorf("failed to open the virtfn%d dir of the device %q: %v", vf, ifName, err)
			}
			continue
		}

		infos, err = ioutil.ReadDir(vfDir)
		if err != nil {
			return fmt.Errorf("failed to read the virtfn%d dir of the device %q: %v", vf, ifName, err)
		}

		if (len(infos) == 0) && (vf == (minVfNum - 1)) {
			return fmt.Errorf("no Virtual function exist in directory %s, last vf is virtfn%d", vfDir, vf)
		}

		if (len(infos) == 0) && (vf != (minVfNum - 1)) {
			continue
		}
		vfIdx = vf
		break

	}

	// VF NIC name
	if len(infos) != 1 {
		return fmt.Errorf("no virutal network resources avaiable for the %q", conf.Device)
	}
	vfName := infos[0].Name()

	vfDev, err := netlink.LinkByName(vfName)
	if err != nil {
		return fmt.Errorf("failed to lookup vf device %q: %v", vfName, err)
	}

	if err = netlink.LinkSetVfVlan(m, vfIdx, vlan); err != nil {
		return fmt.Errorf("failed to set vf %d vlan: %v", vfIdx, err)
	}

	if err = netlink.LinkSetUp(vfDev); err != nil {
		return fmt.Errorf("failed to setup vf %d device: %v", vfIdx, err)
	}

	// move VF device to ns
	if err = netlink.LinkSetNsFd(vfDev, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move vf %d to netns: %v", vfIdx, err)
	}

	if err = netns.Do(func(_ ns.NetNS) error {
		err := renameLink(vfName, podifName)
		if err != nil {
			return fmt.Errorf("failed to rename %d vf of the device %q to %q: %v", vfIdx, vfName, ifName, err)
		}
		return ipam.ConfigureIface(podifName, resultCurrent)
	}); err != nil {
		return err
	}
	hiDir := fmt.Sprintf("/sys/class/net/%s/device/virtfn%d/msi_irqs", ifName, vfIdx)
	if infos, err = ioutil.ReadDir(hiDir); err == nil {
		for i := range infos {
			hiNum, err := strconv.Atoi(infos[i].Name())
			if err == nil {
				selectedCPU := fmt.Sprintf("%d", hiNum%cpus)
				irqFile := fmt.Sprintf("/proc/irq/%d/smp_affinity_list", hiNum)
				ioutil.WriteFile(irqFile, []byte(selectedCPU), 0644)
			}
		}
	}
	return nil
}

func releaseVF(podifName string, netns ns.NetNS) error {
	initns, err := ns.GetCurrentNS()
	if err != nil {
		return fmt.Errorf("failed to get init netns: %v", err)
	}

	if err = netns.Set(); err != nil {
		return fmt.Errorf("failed to enter netns %q: %v", netns, err)
	}

	ifName := podifName
	// get VF device
	vfDev, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup vf device %q: %v", ifName, err)
	}

	// device name in init netns
	index := vfDev.Attrs().Index
	devName := fmt.Sprintf("dev%d", index)

	// shutdown VF device
	if err = netlink.LinkSetDown(vfDev); err != nil {
		return fmt.Errorf("failed to down vf device %q: %v", ifName, err)
	}

	// rename VF device
	err = renameLink(ifName, devName)
	if err != nil {
		return fmt.Errorf("failed to rename vf device %q to %q: %v", ifName, devName, err)
	}

	// move VF device to init netns
	if err = netlink.LinkSetNsFd(vfDev, int(initns.Fd())); err != nil {
		return fmt.Errorf("failed to move vf device to init netns: %v", ifName, err)
	}

	return nil
}

func renameLink(curName, newName string) error {
	link, err := netlink.LinkByName(curName)
	if err != nil {
		return fmt.Errorf("failed to lookup device %q: %v", curName, err)
	}

	return netlink.LinkSetName(link, newName)
}

func getSriovNumVfs(ifName string, kind string) (int, error) {
	var vfTotal int

	sriovFile := fmt.Sprintf("/sys/class/net/%s/device/%s", ifName, kind)
	if _, err := os.Lstat(sriovFile); err != nil {
		return vfTotal, fmt.Errorf("failed to open the %s of device %q: %v", kind, ifName, err)
	}

	data, err := ioutil.ReadFile(sriovFile)
	if err != nil {
		return vfTotal, fmt.Errorf("failed to read the sriov_numfs of device %q: %v", ifName, err)
	}

	if len(data) == 0 {
		return vfTotal, fmt.Errorf("no data in the file %q", sriovFile)
	}

	sriovNumfs := strings.TrimSpace(string(data))
	vfTotal, err = strconv.Atoi(sriovNumfs)
	if err != nil {
		return vfTotal, fmt.Errorf("failed to convert sriov_numfs(byte value) to int of device %q: %v", ifName, err)
	}

	return vfTotal, nil
}

func setSriovNumVfs(ifName string, nums int) error {
	sriovFile := fmt.Sprintf("/sys/class/net/%s/device/%s", ifName, kindNumVfs)
	if _, err := os.Lstat(sriovFile); err != nil {
		return fmt.Errorf("failed to open the %s of device %q: %v", kindNumVfs, ifName, err)
	}
	numStr := fmt.Sprintf("%d", nums)

	err := ioutil.WriteFile(sriovFile, []byte(numStr), 0644)
	if err != nil {
		return fmt.Errorf("failed to write the %s of device %q: %v", kindNumVfs, ifName, err)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
