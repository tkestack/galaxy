package floatingip

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
)

var (
	ErrNoEnoughIP     = fmt.Errorf("no enough available ips left")
	ErrNoFIPForSubnet = fmt.Errorf("no fip configured for subnet")
)

type IPAM interface {
	ConfigurePool([]*FloatingIP) error
	Config() []*FloatingIP
	Allocate([]string, ...database.ActionFunc) ([]net.IP, error)
	AllocateSpecificIP(string, net.IP) error
	AllocateInSubnet(string, *net.IPNet) (net.IP, error)
	Release([]string) error
	ReleaseByPrefix(string) error
	QueryFirst(string) (*IPInfo, error)
	QueryByIP(net.IP) (string, error)
	QueryByPrefix(string) (map[string]string, error) //ip to key
	RoutableSubnet(net.IP) *net.IPNet
	QueryRoutableSubnetByKey(key string) ([]string, error)
	Store() *database.DBRecorder //for test
	Shutdown()

	ApplyFloatingIPs([]FloatingIP) []*FloatingIP
}

type IPInfo struct {
	IP             *nets.IPNet `json:"ip"`
	Vlan           uint16      `json:"vlan"`
	Gateway        net.IP      `json:"gateway"`
	RoutableSubnet *nets.IPNet `json:"routable_subnet"` //the node subnet
}

// ipam manages floating ip allocation and release and does it atomically
// with the help of atomic operation of the store
type ipam struct {
	FloatingIPs []*FloatingIP `json:"floatingips,omitempty"`
	store       *database.DBRecorder
}

func NewIPAM(store *database.DBRecorder) IPAM {
	return &ipam{
		store: store,
	}
}

func (i *ipam) mergeWithDB(fipMap map[string]*FloatingIP) error {
	ips, err := i.findAll()
	if err != nil {
		return err
	}
	var toBeDelete []uint32
	// delete no longer available floating ips stored in the db first
	for _, ip := range ips {
		netIP := nets.IntToIP(ip.IP)
		found := false
		for _, fipConf := range fipMap {
			if fipConf.IPNet().Contains(netIP) {
				found = true
				if !fipConf.Contains(netIP) {
					toBeDelete = append(toBeDelete, ip.IP)
				}
				break
			}
		}
		if !found {
			toBeDelete = append(toBeDelete, ip.IP)
		}
	}
	deleted, err := i.deleteUnScoped(toBeDelete)
	if err != nil {
		return fmt.Errorf("failed to delete ip %v: %v", toBeDelete, err)
	}
	glog.Infof("expect to delete %d ips from ips from %v, deleted %d", len(toBeDelete), toBeDelete, deleted)
	// insert new floating ips
	for _, fipConf := range fipMap {
		subnet := fipConf.RoutableSubnet.String()
		for _, ipr := range fipConf.IPRanges {
			first := nets.IPToInt(ipr.First)
			last := nets.IPToInt(ipr.Last)
			for ; first <= last; first++ {
				fip := database.FloatingIP{IP: first, Key: "", Subnet: subnet}
				if err := i.create(&fip); err != nil {
					if !strings.Contains(err.Error(), fmt.Sprintf(`Duplicate entry '%d' for key 'PRIMARY'`, first)) {
						return fmt.Errorf("Error creating floating ip %d: %v", first, err)
					}
				}
				// update subnet so that the previous table is compatible
				if err := i.updateSubnet(&fip); err != nil {
					if err != ErrNotUpdated {
						return fmt.Errorf("Error update subnet of floating ip %v: %v", fip, err)
					}
				}
			}
		}
	}
	return nil
}

func (i *ipam) ConfigurePool(floatingIPs []*FloatingIP) error {
	sort.Sort(FloatingIPSlice(floatingIPs))
	glog.Infof("floating ip config %v", floatingIPs)
	i.FloatingIPs = floatingIPs
	floatingIPMap := make(map[string]*FloatingIP)
	for _, fip := range i.FloatingIPs {
		if _, exists := floatingIPMap[fip.Key()]; exists {
			glog.Warningf("Exists floating ip conf %v", fip)
			continue
		}
		floatingIPMap[fip.Key()] = fip
	}
	if err := i.mergeWithDB(floatingIPMap); err != nil {
		return err
	}
	return nil
}

// Allocate allocate len(keys) ips, it guarantees to allocate everything
// or nothing.
func (i *ipam) Allocate(keys []string, ops ...database.ActionFunc) (allocated []net.IP, err error) {
	var fips []database.FloatingIP
	for {
		if err = i.findAvailable(len(keys), &fips); err != nil {
			return
		}
		if len(fips) != len(keys) {
			err = ErrNoEnoughIP
			return
		}
		var updateOps []database.ActionFunc
		for j := 0; j < len(keys); j++ {
			fips[j].Key = keys[j]
			updateOps = append(updateOps, allocateOp(&fips[j]))
		}
		updateOps = append(updateOps, ops...)
		if err = i.store.Transaction(updateOps...); err != nil {
			if err == ErrNotUpdated {
				// Loop if a floating ip has been allocated by the others
				err = nil
			} else {
				return
			}
		} else {
			break
		}
	}
	for _, fip := range fips {
		allocated = append(allocated, nets.IntToIP(fip.IP))
	}
	return
}

func (i *ipam) Release(keys []string) error {
	return i.releaseIPs(keys)
}

func (i *ipam) ReleaseByPrefix(keyPrefix string) error {
	return i.releaseByPrefix(keyPrefix)
}

func (i *ipam) QueryFirst(key string) (*IPInfo, error) {
	var fip database.FloatingIP
	if err := i.findByKey(key, &fip); err != nil {
		return nil, err
	}
	if fip.IP == 0 {
		return nil, nil
	}
	netIP := nets.IntToIP(fip.IP)
	for _, fip := range i.FloatingIPs {
		if fip.Contains(netIP) {
			ip := nets.IPNet(net.IPNet{
				IP:   netIP,
				Mask: fip.Mask,
			})
			return &IPInfo{
				IP:             &ip,
				Vlan:           fip.Vlan,
				Gateway:        fip.Gateway,
				RoutableSubnet: nets.NetsIPNet(fip.RoutableSubnet),
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find match floating ip config for ip %s", netIP.String())
}

func (i *ipam) Shutdown() {
	if i.store != nil {
		i.store.Shutdown()
	}
}

func (i *ipam) AllocateInSubnet(key string, routableSubnet *net.IPNet) (allocated net.IP, err error) {
	if routableSubnet == nil {
		// this should never happen
		return nil, fmt.Errorf("nil routableSubnet")
	}
	ipNet := i.toFIPSubnet(routableSubnet)
	if ipNet == nil {
		var allRoutableSubnet []string
		for j := range i.FloatingIPs {
			allRoutableSubnet = append(allRoutableSubnet, i.FloatingIPs[j].RoutableSubnet.String())
		}
		glog.V(3).Infof("can't find fit routableSubnet %s, all routableSubnets %v", routableSubnet.String(), allRoutableSubnet)
		err = ErrNoFIPForSubnet
		return
	}
	if err = i.store.Transaction(allocateOneInSubnet(key, routableSubnet.String())); err != nil {
		if err == ErrNotUpdated {
			err = ErrNoEnoughIP
		}
		return
	}
	var fip database.FloatingIP
	if err = i.findByKey(key, &fip); err != nil {
		return
	}
	allocated = nets.IntToIP(fip.IP)
	return
}

func (i *ipam) Store() *database.DBRecorder {
	return i.store
}

func (i *ipam) ApplyFloatingIPs(fips []FloatingIP) []*FloatingIP {
	res := make(map[string]*FloatingIP, len(i.FloatingIPs))
	for j := range i.FloatingIPs {
		ofip := i.FloatingIPs[j]
		fip := FloatingIP{
			RoutableSubnet: ofip.RoutableSubnet,
			SparseSubnet: nets.SparseSubnet{
				Gateway: ofip.Gateway,
				Mask:    ofip.Mask,
				Vlan:    ofip.Vlan,
			},
		}
		for k := range ofip.IPRanges {
			fip.IPRanges = append(fip.IPRanges, ofip.IPRanges[k])
		}

		res[ofip.RoutableSubnet.String()] = &fip
	}
	for j, fip := range fips {
		if ofip, exist := res[fip.RoutableSubnet.String()]; exist {
			for _, ipRange := range fip.IPRanges {
				for ipn := nets.IPToInt(ipRange.First); ipn <= nets.IPToInt(ipRange.Last); ipn++ {
					ofip.InsertIP(nets.IntToIP(ipn))
				}
			}
		} else {
			res[fip.RoutableSubnet.String()] = &fips[j]
		}
	}

	var s []*FloatingIP
	for _, fip := range res {
		s = append(s, fip)
	}
	return s
}

func (i *ipam) toFIPSubnet(routableSubnet *net.IPNet) *net.IPNet {
	for _, fip := range i.FloatingIPs {
		if fip.RoutableSubnet.String() == routableSubnet.String() {
			return fip.IPNet()
		}
	}
	return nil
}

func (i *ipam) Config() []*FloatingIP {
	return i.FloatingIPs
}

func (i *ipam) QueryByPrefix(prefix string) (map[string]string, error) {
	var fips []database.FloatingIP
	if err := i.findByPrefix(prefix, &fips); err != nil {
		return nil, fmt.Errorf("failed to find by prefix %s: %v", prefix, err)
	}
	ips := make(map[string]string, len(fips))
	for i := range fips {
		ips[nets.IntToIP(fips[i].IP).String()] = fips[i].Key
	}
	return ips, nil
}

func (i *ipam) RoutableSubnet(nodeIP net.IP) *net.IPNet {
	intIP := nets.IPToInt(nodeIP)
	minIndex := sort.Search(len(i.FloatingIPs), func(j int) bool {
		return nets.IPToInt(i.FloatingIPs[j].RoutableSubnet.IP) > intIP
	})
	if minIndex == 0 {
		return nil
	}
	if i.FloatingIPs[minIndex-1].RoutableSubnet.Contains(nodeIP) {
		return i.FloatingIPs[minIndex-1].RoutableSubnet
	}
	return nil
}

func (i *ipam) QueryRoutableSubnetByKey(key string) ([]string, error) {
	return i.queryByKeyGroupBySubnet(key)
}

func (i *ipam) QueryByIP(ip net.IP) (string, error) {
	fip, err := i.findKeyOfIP(nets.IPToInt(ip))
	return fip.Key, err
}

func (i *ipam) AllocateSpecificIP(key string, ip net.IP) error {
	return i.updateKey(nets.IPToInt(ip), key)
}
