package floatingip

import (
	"fmt"
	"net"
	"sort"
	"strings"

	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/utils/database"
	"tkestack.io/galaxy/pkg/utils/nets"
)

var (
	// ErrNoEnoughIP is error when there is no available floatingIPs
	ErrNoEnoughIP     = fmt.Errorf("no enough available ips left")
	ErrNoFIPForSubnet = fmt.Errorf("no fip configured for subnet")
)

// IPAM interface which implemented by database and kubernetes CRD
type IPAM interface {
	// ConfigurePool init floatingIP pool.
	ConfigurePool([]*FloatingIP) error
	// ReleaseIPs releases given ips as long as their keys match and returned released and unreleased map
	// released and unreleased map are guaranteed to be none nil even if err is not nil
	// unreleased map stores ip with its latest key if key changed
	ReleaseIPs(map[string]string) (map[string]string, map[string]string, error)
	// AllocateSpecificIP allocate pod a specific IP.
	AllocateSpecificIP(string, net.IP, constant.ReleasePolicy, string) error
	// AllocateInSubnet allocate subnet of IPs.
	AllocateInSubnet(string, *net.IPNet, constant.ReleasePolicy, string) (net.IP, error)
	// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
	AllocateInSubnetWithKey(oldK, newK, subnet string, policy constant.ReleasePolicy, attr string) error
	// ReserveIP can reserve a IP entitled by a terminated pod.
	ReserveIP(oldK, newK, attr string) error
	// UpdatePolicy update floatingIP's release policy.
	UpdatePolicy(string, net.IP, constant.ReleasePolicy, string) error
	// Release release a given IP.
	Release(string, net.IP) error
	// First returns the first matched IP by key.
	First(string) (*FloatingIPInfo, error) // returns nil,nil if key is not found
	// ByIP transform a given IP to database.FloatingIP struct.
	ByIP(net.IP) (database.FloatingIP, error)
	// ByPrefix filter floatingIPs by prefix key.
	ByPrefix(string) ([]database.FloatingIP, error)
	// ByKeyword returns floatingIP set by a given keyword.
	ByKeyword(string) ([]database.FloatingIP, error)
	// RoutableSubnet returns node's net subnet.
	RoutableSubnet(net.IP) *net.IPNet
	// RoutableSubnet returns node's net subnet.
	QueryRoutableSubnetByKey(key string) ([]string, error)
	// Shutdown shutdowns IPAM.
	Shutdown()
	// Name returns IPAM's name.
	Name() string
}

// FloatingIPInfo is floatingIP information
type FloatingIPInfo struct {
	IPInfo constant.IPInfo
	FIP    database.FloatingIP
}

// ipam manages floating ip allocation and release and does it atomically
// with the help of atomic operation of the store
type dbIpam struct {
	FloatingIPs []*FloatingIP `json:"floatingips,omitempty"`
	store       *database.DBRecorder
	TableName   string
}

// NewIPAM init database IPAM
func NewIPAM(store *database.DBRecorder) IPAM {
	return NewIPAMWithTableName(store, database.DefaultFloatingipTableName)
}

func NewIPAMWithTableName(store *database.DBRecorder, tableName string) IPAM {
	if err := store.CreateTableIfNotExist(&database.FloatingIP{Table: tableName}); err != nil {
		glog.Fatalf("failed to create table %s", tableName)
	}
	return &dbIpam{
		store:     store,
		TableName: tableName,
	}
}

// Name returns IPAM's name.
func (i *dbIpam) Name() string {
	return i.TableName
}

func (i *dbIpam) mergeWithDB(fipMap map[string]*FloatingIP) error {
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
				if fipConf.Contains(netIP) {
					found = true
					break
				}
			}
		}
		if !found {
			toBeDelete = append(toBeDelete, ip.IP)
		}
	}
	if len(toBeDelete) > 0 {
		deleted, err := i.deleteUnScoped(toBeDelete)
		if err != nil {
			return fmt.Errorf("failed to delete ip %v: %v", toBeDelete, err)
		}
		glog.Infof("expect to delete %d ips from ips from %v, deleted %d", len(toBeDelete), toBeDelete, deleted)
	}
	// insert new floating ips
	return i.insertNewFips(fipMap)
}

func (i *dbIpam) insertNewFips(fipMap map[string]*FloatingIP) error {
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
			}
		}
	}
	return nil
}

// ConfigurePool init floatingIP pool.
func (i *dbIpam) ConfigurePool(floatingIPs []*FloatingIP) error {
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

// Release release a given IP.
func (i *dbIpam) Release(key string, ip net.IP) error {
	return i.releaseIP(key, nets.IPToInt(ip))
}

func (i *dbIpam) ReleaseByPrefix(keyPrefix string) error {
	return i.releaseByPrefix(keyPrefix)
}

func (i *dbIpam) first(key string) (*constant.IPInfo, error) {
	fipInfo, err := i.First(key)
	if err != nil || fipInfo == nil {
		return nil, err
	}
	return &fipInfo.IPInfo, nil
}

// First returns the first matched IP by key.
func (i *dbIpam) First(key string) (*FloatingIPInfo, error) {
	var fip database.FloatingIP
	if err := i.findByKey(key, &fip); err != nil {
		return nil, err
	}
	if fip.IP == 0 {
		return nil, nil
	}
	netIP := nets.IntToIP(fip.IP)
	for _, fips := range i.FloatingIPs {
		if fips.Contains(netIP) {
			ip := nets.IPNet(net.IPNet{
				IP:   netIP,
				Mask: fips.Mask,
			})
			return &FloatingIPInfo{
				IPInfo: constant.IPInfo{
					IP:             &ip,
					Vlan:           fips.Vlan,
					Gateway:        fips.Gateway,
					RoutableSubnet: nets.NetsIPNet(fips.RoutableSubnet),
				},
				FIP: fip,
			}, nil
		}
	}
	return nil, nil
}

// Shutdown shutdowns IPAM.
func (i *dbIpam) Shutdown() {
	if i.store != nil {
		i.store.Shutdown()
	}
}

// AllocateInSubnet allocate subnet of IPs.
func (i *dbIpam) AllocateInSubnet(key string, routableSubnet *net.IPNet, policy constant.ReleasePolicy,
	attr string) (allocated net.IP, err error) {
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
		glog.V(3).Infof("can't find fit routableSubnet %s, all routableSubnets %v", routableSubnet.String(),
			allRoutableSubnet)
		err = ErrNoFIPForSubnet
		return
	}
	if err = i.updateOneInSubnet("", key, routableSubnet.String(), uint16(policy), attr); err != nil {
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

func (i *dbIpam) applyFloatingIPs(fips []*FloatingIP) []*FloatingIP {
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
			res[fip.RoutableSubnet.String()] = fips[j]
		}
	}

	var s []*FloatingIP
	for _, fip := range res {
		s = append(s, fip)
	}
	return s
}

func (i *dbIpam) toFIPSubnet(routableSubnet *net.IPNet) *net.IPNet {
	for _, fip := range i.FloatingIPs {
		if fip.RoutableSubnet.String() == routableSubnet.String() {
			return fip.IPNet()
		}
	}
	return nil
}

// ByPrefix filter floatingIPs by prefix key.
func (i *dbIpam) ByPrefix(prefix string) ([]database.FloatingIP, error) {
	var fips []database.FloatingIP
	if err := i.findByPrefix(prefix, &fips); err != nil {
		return nil, fmt.Errorf("failed to find by prefix %s: %v", prefix, err)
	}
	return fips, nil
}

// RoutableSubnet returns node's net subnet.
func (i *dbIpam) RoutableSubnet(nodeIP net.IP) *net.IPNet {
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

// RoutableSubnet returns node's net subnet.
func (i *dbIpam) QueryRoutableSubnetByKey(key string) ([]string, error) {
	return i.queryByKeyGroupBySubnet(key)
}

// ByIP transform a given IP to database.FloatingIP struct.
func (i *dbIpam) ByIP(ip net.IP) (database.FloatingIP, error) {
	return i.findByIP(nets.IPToInt(ip))
}

// AllocateSpecificIP allocate pod a specific IP.
func (i *dbIpam) AllocateSpecificIP(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	return i.allocateSpecificIP(nets.IPToInt(ip), key, uint16(policy), attr)
}

// UpdatePolicy update floatingIP's release policy.
func (i *dbIpam) UpdatePolicy(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	return i.updatePolicy(nets.IPToInt(ip), key, uint16(policy), attr)
}

// ReserveIP can reserve a IP entitled by a terminated pod.
func (i *dbIpam) ReserveIP(oldK, newK, attr string) error {
	return i.updateKey(oldK, newK, attr)
}

// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
func (i *dbIpam) AllocateInSubnetWithKey(oldK, newK, subnet string, policy constant.ReleasePolicy, attr string) error {
	return i.updateOneInSubnet(oldK, newK, subnet, uint16(policy), attr)
}

// ByKeyword returns floatingIP set by a given keyword.
func (i *dbIpam) ByKeyword(keyword string) ([]database.FloatingIP, error) {
	var fips []database.FloatingIP
	fips, err := i.getIPsByKeyword(i.TableName, keyword)
	if err != nil {
		return fips, err
	}
	return fips, nil
}

// ReleaseIPs releases given ips
func (i *dbIpam) ReleaseIPs(ipToKey map[string]string) (map[string]string, map[string]string, error) {
	return i.deleteIPs(i.TableName, ipToKey)
}
