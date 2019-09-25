package helper

import (
	"fmt"
	"net"
	"reflect"

	"git.code.oa.com/tkestack/galaxy/pkg/network/netns"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type LinkDevice struct {
	Parent *LinkDevice
	Master *LinkDevice
	Addr   *net.IPNet
	Type   string
	Name   string
}

func NewLinkDevice(addr *net.IPNet, name, typeStr string) *LinkDevice {
	return &LinkDevice{
		Addr: addr,
		Type: typeStr,
		Name: name,
	}
}

func (n *LinkDevice) SetParent(parent *LinkDevice) *LinkDevice {
	n.Parent = parent
	return n
}

func (n *LinkDevice) SetMaster(master *LinkDevice) *LinkDevice {
	n.Master = master
	return n
}

type Route struct {
	LinkName string
	netlink.Route
}

type NetworkTopology struct {
	LeaveDevices []*LinkDevice
	Routes       []Route
	neighs       []netlink.Neigh
	Netns        string // all these objects are in netns
}

func (t *NetworkTopology) Verify() error {
	if t.Netns != "" {
		var err error
		netns.InvokeIn(t.Netns, func() {
			err = t.verify()
		})
		return err
	}
	return t.verify()
}

func (t *NetworkTopology) verify() error {
	var errs []error
	for _, device := range t.LeaveDevices {
		child, err := getLinkDevice(device)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := verifyDevice(device, child); err != nil {
			errs = append(errs, err)
			continue
		}
	}
	errs = append(errs, t.VerifyRoutes()...)
	errs = append(errs, t.VerifyNeighs()...)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("errs %v", errs)
}

func verifyDevice(device *LinkDevice, link netlink.Link) error {
	if device.Parent != nil {
		parent, err := getLinkDevice(device.Parent)
		if err != nil {
			return err
		}
		if link.Attrs().ParentIndex != parent.Attrs().Index {
			return fmt.Errorf("device %s's parent device is not %s (index %d), real parent index is %d", link.Attrs().Name, parent.Attrs().Name, parent.Attrs().Index, link.Attrs().ParentIndex)
		}
		return verifyDevice(device.Parent, parent)
	} else if link.Attrs().ParentIndex != 0 {
		// A veth device's parent is its peer device which is possibly in another namespace
		if link.Type() != "veth" {
			parentLink, err := netlink.LinkByIndex(link.Attrs().ParentIndex)
			if err != nil {
				return fmt.Errorf("device %s has a parent index %d", device.Name, link.Attrs().ParentIndex)
			}
			return fmt.Errorf("device %s has a parent index %d (%s)", device.Name, link.Attrs().ParentIndex, parentLink.Attrs().Name)
		}
	}
	if device.Master != nil {
		master, err := getLinkDevice(device.Master)
		if err != nil {
			return err
		}
		if link.Attrs().MasterIndex != master.Attrs().Index {
			return fmt.Errorf("device %s's master device is not %s (index %d), real master index is %d", link.Attrs().Name, master.Attrs().Name, master.Attrs().Index, link.Attrs().MasterIndex)
		}
		return verifyDevice(device.Master, master)
	} else if link.Attrs().MasterIndex != 0 {
		masterLink, err := netlink.LinkByIndex(link.Attrs().MasterIndex)
		if err != nil {
			return fmt.Errorf("device %s has a master index %d", device.Name, link.Attrs().MasterIndex)
		}
		return fmt.Errorf("device %s has a master index %d (%s)", device.Name, link.Attrs().MasterIndex, masterLink.Attrs().Name)
	}
	return nil
}

func (t *NetworkTopology) VerifyRoutes() []error {
	var errs []error
	rs, err := netlink.RouteList(nil, nl.FAMILY_V4)
	if err != nil {
		return []error{err}
	}
	for _, expect := range t.Routes {
		if expect.LinkName != "" {
			link, err := netlink.LinkByName(expect.LinkName)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to find route device %s", expect.LinkName))
				continue
			}
			expect.LinkIndex = link.Attrs().Index
		}
		expect.Table = 254 // default route table
		var find bool
		for _, r := range rs {
			if expect.Route.String() == r.String() {
				find = true
				break
			}
		}
		if !find {
			errs = append(errs, fmt.Errorf("missing route %+v", expect))
		}
	}
	return errs
}

func (t *NetworkTopology) VerifyNeighs() []error {
	var errs []error
	ns, err := netlink.NeighList(0, nl.FAMILY_V4)
	if err != nil {
		return []error{err}
	}
	for _, expect := range t.neighs {
		var find bool
		for _, n := range ns {
			if reflect.DeepEqual(expect, n) {
				find = true
				break
			}
		}
		if !find {
			errs = append(errs, fmt.Errorf("missing neigh %+v", expect))
		}
	}
	return errs
}

func getLinkDevice(device *LinkDevice) (netlink.Link, error) {
	if device.Name == "" {
		return nil, fmt.Errorf("device name is empty")
	}
	link, err := netlink.LinkByName(device.Name)
	if err != nil {
		return nil, fmt.Errorf("can't find device name %s", device.Name)
	}
	if link.Type() != device.Type {
		return nil, fmt.Errorf("device type %s is not %s", link.Type(), device.Type)
	}
	addrs, err := netlink.AddrList(link, nl.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("failed to list ip address of %s: %v", device.Name, err)
	}
	if device.Addr == nil {
		if len(addrs) != 0 {
			return nil, fmt.Errorf("expect device %s has no ip address, find %v", device.Name, addrs)
		}
	} else {
		if len(addrs) != 1 {
			return nil, fmt.Errorf("expect device %s has an ip address %s, find %v", device.Name, device.Addr.String(), addrs)
		}
		if addrs[0].IPNet.String() != device.Addr.String() {
			return nil, fmt.Errorf("expect device %s has an ip address %s, find %v", device.Name, device.Addr.String(), addrs)
		}
	}
	return link, nil
}
