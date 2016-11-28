package host_local

import (
	"github.com/containernetworking/cni/plugins/ipam/host-local/backend/disk"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

func cmdAdd(args *skel.CmdArgs) error {
	ipamConf, err := LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}

	store, err := disk.New(ipamConf.Name)
	if err != nil {
		return err
	}
	defer store.Close()

	allocator, err := NewIPAllocator(ipamConf, store)
	if err != nil {
		return err
	}

	ipConf, err := allocator.Get(args.ContainerID)
	if err != nil {
		return err
	}

	r := &types.Result{
		IP4: ipConf,
	}
	return r.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	ipamConf, err := LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}

	store, err := disk.New(ipamConf.Name)
	if err != nil {
		return err
	}
	defer store.Close()

	allocator, err := NewIPAllocator(ipamConf, store)
	if err != nil {
		return err
	}

	return allocator.Release(args.ContainerID)
}

