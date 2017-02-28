package vlan

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/vishvananda/netlink"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils"
	"git.code.oa.com/gaiastack/galaxy/pkg/network"
)

const (
	vethPrefix    = "veth"
	vethLen       = 7
	vlanPrefix    = "vlan"
	bridgePrefix  = "docker"
	defaultBridge = "docker"
)

type VlanDriver struct {
	//FIXME add a file lock cause we are running multiple processes?
	*NetConf
	// The device id of physical device which is to be the parent of all vlan devices, eg.eth1
	vlanParentIndex int
	sync.Mutex
}

type NetConf struct {
	types.NetConf
	// The device which has IDC ip address, eg. eth1 or eth1.12 (A vlan device)
	Device string `json:"device"`
}

func (d *VlanDriver) LoadConf(bytes []byte) (*NetConf, error) {
	conf := &NetConf{}
	if err := json.Unmarshal(bytes, conf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	d.NetConf = conf
	return conf, nil
}

func (d *VlanDriver) SetupBridge() error {
	device, err := netlink.LinkByName(d.Device)
	if err != nil {
		return fmt.Errorf("Error getting device %s: %v", d.Device, err)
	}
	d.vlanParentIndex = device.Attrs().Index
	//defer glog.Infof("root device %q, vlan parent index %d", d.Device, d.vlanParentIndex)
	if device.Type() == "vlan" {
		//A vlan device
		d.vlanParentIndex = device.Attrs().ParentIndex
		//glog.Infof("root device %s is a vlan device, parent index %d", d.Device, d.vlanParentIndex)
	}
	v4Addr, err := netlink.AddrList(device, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("Errror getting ipv4 address %v", err)
	}
	filteredAddr := network.FilterLoopbackAddr(v4Addr)
	if len(filteredAddr) == 0 {
		bri, err := netlink.LinkByName(defaultBridge)
		if err != nil {
			return fmt.Errorf("Error getting bri device %s: %v", defaultBridge, err)
		}
		if bri.Attrs().Index != device.Attrs().MasterIndex {
			return fmt.Errorf("No available address found on device %s", d.Device)
		}
	} else if len(filteredAddr) > 1 {
		return fmt.Errorf("Multiple v4 address on device %s", d.Device)
	} else {
		bri, err := getOrCreateBridge(defaultBridge, device.Attrs().HardwareAddr)
		if err != nil {
			return err
		}
		if err := netlink.LinkSetUp(bri); err != nil {
			return fmt.Errorf("Failed to set up bridge device %s: %v", defaultBridge, err)
		}
		if r, err := utils.GetDefaultRoute(); err != nil {
			return err
		} else {
			var err error
			if r.LinkIndex == device.Attrs().Index {
				if err := netlink.RouteDel(r); err != nil {
					return fmt.Errorf("Failed to remove default route %v", err)
				}
				defer func() {
					if err != nil {
						if err := netlink.RouteAdd(r); err != nil {
							//glog.Warningf("Failed to rollback default route %v: %v", r, err)
						}
					}
				}()
			}
			if err = netlink.AddrDel(device, &filteredAddr[0]); err != nil {
				return fmt.Errorf("Failed to remove v4address from device %s: %v", d.Device, err)
			}
			defer func() {
				if err != nil {
					if err = netlink.AddrAdd(device, &filteredAddr[0]); err != nil {
						//glog.Warningf("Failed to rollback v4address to device %s: %v, address %v", device, err, v4Addr[0])
					}
				}
			}()
			filteredAddr[0].Label = ""
			if err = netlink.AddrAdd(bri, &filteredAddr[0]); err != nil {
				return fmt.Errorf("Failed to add v4address to bridge device %s: %v, address %v", defaultBridge, err, filteredAddr[0])
			}
			if err = netlink.LinkSetMaster(device, &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: defaultBridge}}); err != nil {
				return fmt.Errorf("Failed to add device %s to bridge device %s: %v", d.Device, defaultBridge, err)
			}
			if r.LinkIndex == device.Attrs().Index {
				if err = netlink.RouteAdd(&netlink.Route{Gw: r.Gw, LinkIndex: bri.Attrs().Index}); err != nil {
					return fmt.Errorf("Failed to remove default route %v", err)
				}
			}
		}
	}
	return nil
}

func getOrCreateBridge(bridgeName string, mac net.HardwareAddr) (netlink.Link, error) {
	return getOrCreateDevice(bridgeName, func(name string) error {
		if err := utils.CreateBridgeDevice(bridgeName, mac); err != nil {
			return fmt.Errorf("Failed to add bridge device %s: %v", bridgeName, err)
		}
		return nil
	})
}

func getOrCreateDevice(name string, createDevice func(name string) error) (netlink.Link, error) {
	device, err := netlink.LinkByName(name)
	if err != nil {
		if err := createDevice(name); err != nil {
			return nil, fmt.Errorf("Failed to add %s: %v", name, err)
		}
		if device, err = netlink.LinkByName(name); err != nil {
			return nil, fmt.Errorf("Failed to get %s: %v", name, err)
		}
	}
	return device, nil
}

func (d *VlanDriver) CreateVlanDevice(vlanId uint16) error {
	if vlanId == 0 {
		return nil
	}
	vlanIfName := fmt.Sprintf("%s%d", vlanPrefix, vlanId)
	bridgeIfName := fmt.Sprintf("%s%d", bridgePrefix, vlanId)

	d.Lock()
	defer d.Unlock()
	// Get vlan device
	vlan, err := getOrCreateDevice(vlanIfName, func(name string) error {
		vlanIf := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanIfName, ParentIndex: d.vlanParentIndex}, VlanId: (int)(vlanId)}
		if err := netlink.LinkAdd(vlanIf); err != nil {
			return fmt.Errorf("Failed to add vlan device %s: %v", vlanIfName, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	bridge, err := getOrCreateBridge(bridgeIfName, nil)
	if err != nil {
		return err
	}
	if vlan.Attrs().MasterIndex != bridge.Attrs().Index {
		if err := netlink.LinkSetMaster(vlan, &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeIfName}}); err != nil {
			return fmt.Errorf("Failed to add vlan device %s to bridge device %s: %v", vlanIfName, bridgeIfName, err)
		}
	}
	if err := netlink.LinkSetUp(vlan); err != nil {
		return fmt.Errorf("Failed to set up vlan device %s: %v", vlanIfName, err)
	}
	if err := netlink.LinkSetUp(bridge); err != nil {
		return fmt.Errorf("Failed to set up bridge device %s: %v", bridgeIfName, err)
	}
	return nil
}

func (d *VlanDriver) CreateVeth(result *types.Result, args *skel.CmdArgs, vlanId uint16) error {
	bridgeName := defaultBridge
	if vlanId != 0 {
		bridgeName = fmt.Sprintf("%s%d", bridgePrefix, vlanId)
	}
	hostIfName := fmt.Sprintf("%s-h%s", vethPrefix, args.ContainerID[0:9])
	containerIfName := fmt.Sprintf("%s-s%s", vethPrefix, args.ContainerID[0:9])
	// Generate and add the interface pipe host <-> sandbox
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostIfName, TxQLen: 0},
		PeerName:  containerIfName}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("failed to add the host %q <=> sandbox %q pair interfaces: %v", hostIfName, containerIfName, err)
	}

	// Get the host side pipe interface handler
	host, err := netlink.LinkByName(hostIfName)
	if err != nil {
		return fmt.Errorf("failed to find host side interface %q: %v", hostIfName, err)
	}
	defer func() {
		if err != nil {
			netlink.LinkDel(host)
		}
	}()

	// Get the sandbox side pipe interface handler
	sbox, err := netlink.LinkByName(containerIfName)
	if err != nil {
		return fmt.Errorf("failed to find sandbox side interface %q: %v", containerIfName, err)
	}
	defer func() {
		if err != nil {
			netlink.LinkDel(sbox)
		}
	}()

	// Attach host side pipe interface into the bridge
	if err = utils.AddToBridge(hostIfName, bridgeName); err != nil {
		return fmt.Errorf("adding interface %q to bridge %q failed: %v", hostIfName, bridgeName, err)
	}
	// Down the interface before configuring mac address.
	if err = netlink.LinkSetDown(sbox); err != nil {
		return fmt.Errorf("could not set link down for container interface %q: %v", containerIfName, err)
	}

	if err = netlink.LinkSetHardwareAddr(sbox, utils.GenerateMACFromIP(result.IP4.IP.IP)); err != nil {
		return fmt.Errorf("could not set mac address for container interface %q: %v", containerIfName, err)
	}

	// Up the host interface after finishing all netlink configuration
	if err = netlink.LinkSetUp(host); err != nil {
		return fmt.Errorf("could not set link up for host interface %q: %v", hostIfName, err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()
	// move sbox veth device to ns
	if err = netlink.LinkSetNsFd(sbox, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move sbox device %q to netns: %v", sbox.Attrs().Name, err)
	}
	return netns.Do(func(_ ns.NetNS) error {
		if err := netlink.LinkSetName(sbox, args.IfName); err != nil {
			return fmt.Errorf("failed to rename sbox device %q to %q: %v", sbox.Attrs().Name, args.IfName, err)
		}
		// Add IP and routes to sbox, including default route
		return ipam.ConfigureIface(args.IfName, result)
	})
}

func (d *VlanDriver) DeleteVeth(args *skel.CmdArgs) error {
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		if _, ok := err.(ns.NSPathNotExistErr); ok {
			return nil
		}
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	return netns.Do(func(_ ns.NetNS) error {
		// get sbox device
		sbox, err := netlink.LinkByName(args.IfName)
		if err != nil {
			return fmt.Errorf("failed to lookup sbox device %q: %v", args.IfName, err)
		}

		// shutdown sbox device
		if err = netlink.LinkSetDown(sbox); err != nil {
			return fmt.Errorf("failed to down sbox device %q: %v", sbox.Attrs().Name, err)
		}

		if err = netlink.LinkDel(sbox); err != nil {
			return fmt.Errorf("failed to delete sbox device %q: %v", sbox.Attrs().Name, err)
		}
		return nil
	})
}
