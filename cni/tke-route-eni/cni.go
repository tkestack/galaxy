package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/types/current"
	cniSpecVersion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	galaxyIpam "git.code.oa.com/gaiastack/galaxy/cni/ipam"
)

const (
	vethPrefix    = "eni"
	defaultIfName = "eth1"
)

var (
	defaultRouteTable = 1
)

type NetConf struct {
	types.NetConf
	Eni        string `json:"eni"`
	RouteTable *int   `json:"routeTable"`
}

// K8SArgs is the valid CNI_ARGS used for Kubernetes
type K8SArgs struct {
	types.CommonArgs

	// K8S_POD_NAME is pod's name
	K8S_POD_NAME types.UnmarshallableString

	// K8S_POD_NAMESPACE is pod's namespace
	K8S_POD_NAMESPACE types.UnmarshallableString

	// K8S_POD_INFRA_CONTAINER_ID is pod's container id
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

func init() {
	// This is to ensure that all the namespace operations are performed for
	// a single thread
	runtime.LockOSThread()
}

func setDefaultConf(conf *NetConf) {
	if conf.RouteTable == nil {
		conf.RouteTable = &defaultRouteTable
	}
	if conf.Eni == "" {
		conf.Eni = defaultIfName
	}
}

func loadConfAndK8SArgs(args *skel.CmdArgs) (*NetConf, *K8SArgs, error) {
	conf := NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return nil, nil, fmt.Errorf("failed to loading config from args: %v", err)
	}

	setDefaultConf(&conf)

	k8sArgs := K8SArgs{}
	if err := types.LoadArgs(args.Args, &k8sArgs); err != nil {
		return nil, nil, fmt.Errorf("failed to load k8s config from args: %v", err)
	}
	return &conf, &k8sArgs, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, k8sArgs, err := loadConfAndK8SArgs(args)
	if err != nil {
		return fmt.Errorf("failed to load config and k8s args: %v", err)
	}

	_, err = netlink.LinkByName(conf.Eni)
	if err != nil {
		return fmt.Errorf("failed to get link by name %s: %v", conf.Eni, err)
	}

	k8sPodName := string(k8sArgs.K8S_POD_NAME)
	k8sPodNamespace := string(k8sArgs.K8S_POD_NAMESPACE)

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	_, results, err := galaxyIpam.Allocate(conf.IPAM.Type, args)
	if err != nil {
		return err
	}
	result020, err := t020.GetResult(results[0])
	if err != nil {
		return err
	}

	savedIP := result020.IP4.IP

	addr := &net.IPNet{
		IP:   savedIP.IP,
		Mask: net.IPv4Mask(255, 255, 255, 255),
	}

	// build hostVethName
	// Note: the maximum length for linux interface name is 15
	hostVethName := generateHostVethName(vethPrefix, k8sPodNamespace, k8sPodName)

	driverClient := NewDriver()
	infList, err := driverClient.SetupNS(hostVethName, args.IfName, args.Netns, addr, *conf.RouteTable)
	if err != nil {
		return fmt.Errorf("failed to setup network: %v", err)
	}

	contIndex := 1
	ips := []*current.IPConfig{
		{
			Version:   "4",
			Address:   *addr,
			Interface: &contIndex,
		},
	}

	result := &current.Result{
		IPs:        ips,
		Interfaces: infList,
		DNS:        conf.DNS,
	}

	return types.PrintResult(result, conf.CNIVersion)
}

// generateHostVethName returns a name to be used on the host-side veth device.
func generateHostVethName(prefix, namespace, podname string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s.%s", namespace, podname)))
	return fmt.Sprintf("%s%s", prefix, hex.EncodeToString(h.Sum(nil))[:11])
}

func cmdDel(args *skel.CmdArgs) error {
	conf, _, err := loadConfAndK8SArgs(args)
	if err != nil {
		return fmt.Errorf("failed to load config and k8s args: %v", err)
	}

	// get ip
	_, results, err := galaxyIpam.Allocate("", args)
	if err != nil {
		return err
	}
	result020, err := t020.GetResult(results[0])
	if err != nil {
		return err
	}

	savedIP := result020.IP4.IP

	err = cleanHostRule(savedIP.String(), *conf.RouteTable)
	if err != nil {
		return err
	}

	// see https://github.com/kubernetes/kubernetes/issues/20379#issuecomment-255272531
	if args.Netns == "" {
		return nil
	}

	err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		subErr := ip.DelLinkByName(args.IfName)
		if subErr != nil && subErr == ip.ErrLinkNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete ns %s link %s: %v", args.Netns, args.IfName, subErr)
	})

	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, cniSpecVersion.All)
}
