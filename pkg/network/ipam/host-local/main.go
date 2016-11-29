package host_local

import (
	"github.com/containernetworking/cni/plugins/ipam/host-local/backend/disk"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

func CmdAdd(args *skel.CmdArgs) (*types.Result, error) {
	ipamConf, err := LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return nil, err
	}

	store, err := disk.New(ipamConf.Name)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	allocator, err := NewIPAllocator(ipamConf, store)
	if err != nil {
		return nil, err
	}

	ipConf, err := allocator.Get(args.ContainerID)
	if err != nil {
		return nil, err
	}

	return &types.Result{
		IP4: ipConf,
	}, nil
}

func CmdDel(args *skel.CmdArgs) error {
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

