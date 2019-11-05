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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	glog "k8s.io/klog"
	galaxyIpam "tkestack.io/galaxy/cni/ipam"
	"tkestack.io/galaxy/pkg/api/cniutil"
	"tkestack.io/galaxy/pkg/utils"
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
	vlanIds, results, err := galaxyIpam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return err
	}
	result020, err := t020.GetResult(results[0])
	if err != nil {
		return err
	}
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close() // nolint: errcheck

	if err := setupVF(conf, result020, args.IfName, int(vlanIds[0]), netns); err != nil {
		return err
	}
	//send Gratuitous ARP to let switch knows IP floats onto this node
	//ignore errors as we can't print logs and we do this as best as we can
	_ = utils.SendGratuitousARP(args.IfName, result020.IP4.IP.IP.String(), args.Netns)
	result020.DNS = conf.DNS
	return result020.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close() // nolint: errcheck

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
// #lizard forgives
func setupVF(conf *NetConf, result *t020.Result, podifName string, vlan int, netns ns.NetNS) error {
	cpus := runtime.NumCPU()
	ifName := conf.Device

	m, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup master %q: %v", conf.Device, err)
	}
	// get the ifname sriov vf num
	minVfNum, err := getSriovVfNum(conf)
	if err != nil {
		return err
	}
	infos, vfIdx, vfDev, vfName, err := findAvailableVf(minVfNum, conf)
	if err != nil {
		return err
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
		return cniutil.ConfigureIface(podifName, result)
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
				if err = ioutil.WriteFile(irqFile, []byte(selectedCPU), 0644); err != nil {
					return fmt.Errorf("failed set irq smp affinity: %v", err)
				}
			}
		}
	}
	return nil
}

// #lizard forgives
func findAvailableVf(minVfNum int, conf *NetConf) ([]os.FileInfo, int, netlink.Link, string, error) {
	ifName := conf.Device
	var infos []os.FileInfo
	var err error
	var vfIdx int
	for vf := 0; vf <= (minVfNum - 1); vf++ {
		vfDir := fmt.Sprintf("/sys/class/net/%s/device/virtfn%d/net", ifName, vf)
		if _, err := os.Lstat(vfDir); err != nil {
			if vf == (minVfNum - 1) {
				return nil, 0, nil, "", fmt.Errorf("failed to open the virtfn%d dir of the device %q: %v", vf, ifName, err)
			}
			continue
		}
		infos, err = ioutil.ReadDir(vfDir)
		if err != nil {
			return nil, 0, nil, "", fmt.Errorf("failed to read the virtfn%d dir of the device %q: %v", vf, ifName, err)
		}
		if (len(infos) == 0) && (vf == (minVfNum - 1)) {
			return nil, 0, nil, "", fmt.Errorf("no Virtual function exist in directory %s, last vf is virtfn%d", vfDir, vf)
		}
		if (len(infos) == 0) && (vf != (minVfNum - 1)) {
			continue
		}
		vfIdx = vf
		break
	}

	// VF NIC name
	if len(infos) != 1 {
		return nil, 0, nil, "", fmt.Errorf("no virtual network resources available for the %q", ifName)
	}
	vfName := infos[0].Name()
	vfDev, err := netlink.LinkByName(vfName)
	if err != nil {
		return nil, 0, nil, "", fmt.Errorf("failed to lookup vf device %q: %v", vfName, err)
	}
	return infos, vfIdx, vfDev, vfName, nil
}

func getSriovVfNum(conf *NetConf) (int, error) {
	ifName := conf.Device
	vfTotal, err := getSriovNumVfs(ifName, kindTotalVfs)
	if err != nil {
		return 0, err
	}
	if vfTotal <= 0 {
		return 0, fmt.Errorf("no virtual function in the device %q", ifName)
	}
	vfNums, err := getSriovNumVfs(ifName, kindNumVfs)
	if err != nil {
		return 0, err
	}
	minVfNum := min(vfTotal, conf.VFNum)
	// only set vf when `sriov_numvfs` is 0
	if vfNums == 0 {
		if err := setSriovNumVfs(ifName, minVfNum); err != nil {
			return 0, err
		}
	} else if vfNums < minVfNum {
		glog.Warning("sriov_numvfs is set but small")
	}
	return minVfNum, nil
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
		return fmt.Errorf("failed to move vf device %q to init netns: %v", ifName, err)
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
