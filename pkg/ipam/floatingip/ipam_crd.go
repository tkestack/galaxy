package floatingip

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	crd_clientset "git.code.oa.com/gaiastack/galaxy/pkg/ipam/client/clientset/versioned"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
	"github.com/golang/glog"
)

type Type uint16

const (
	InternalIp Type = iota
	ExternalIp
)

func (t *Type) String() (string, error) {
	if *t == InternalIp {
		return "internalIP", nil
	} else if *t == ExternalIp {
		return "externalIP", nil
	}
	return "", fmt.Errorf("unknown ip type %v", *t)
}

type FloatingIPObj struct {
	key    string
	att    string
	policy constant.ReleasePolicy
	subnet string
}

//in FIP cache, key is FloatingIP name (ip typed as uint32)
//value(FloatingIPObj) stores FloatingIPSpec in FloatingIP CRD
type FIPCache struct {
	cacheLock       *sync.RWMutex
	allocatedFIPs   map[string]*FloatingIPObj
	unallocatedFIPs map[string]*FloatingIPObj
}

type crdIpam struct {
	FloatingIPs []*FloatingIP `json:"floatingips,omitempty"`
	client      crd_clientset.Interface
	ipType      Type
	//caches for FloatingIP crd, both stores allocated FloatingIPs and unallocated FloatingIPs
	caches FIPCache
}

func NewCrdIPAM(fipClient crd_clientset.Interface, ipType Type) IPAM {
	ipam := &crdIpam{
		client: fipClient,
		ipType: ipType,
	}
	ipam.caches.cacheLock = new(sync.RWMutex)
	return ipam
}

func (ci *crdIpam) ConfigurePool(floatIPs []*FloatingIP) error {
	sort.Sort(FloatingIPSlice(floatIPs))
	glog.V(3).Infof("floating ip config %v", floatIPs)
	ci.FloatingIPs = floatIPs
	floatingIPMap := make(map[string]*FloatingIP)
	for _, fip := range ci.FloatingIPs {
		if _, exist := floatingIPMap[fip.Key()]; exist {
			glog.Warningf("Exists floating ip conf %v", fip)
			continue
		}
		floatingIPMap[fip.Key()] = fip
	}
	if err := ci.freshCache(floatingIPMap); err != nil {
		return err
	}
	return nil
}

func (ci *crdIpam) AllocateSpecificIP(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	ipStr := strconv.FormatUint(uint64(nets.IPToInt(ip)), 10)
	ci.caches.cacheLock.RLock()
	spec, find := ci.caches.allocatedFIPs[ipStr]
	ci.caches.cacheLock.RUnlock()
	if !find {
		return fmt.Errorf("failed to find floating ip by %s in cache", ipStr)
	}
	if err := ci.createFloatingIP(ipStr, key, policy, attr, spec.subnet); err != nil {
		return err
	}
	ci.syncCacheAfterCreate(ipStr, key, attr, policy, spec.subnet)
	return nil
}

func (ci *crdIpam) AllocateInSubnet(key string, routableSubnet *net.IPNet, policy constant.ReleasePolicy, attr string) (allocated net.IP, err error) {
	if routableSubnet == nil {
		// this should never happen
		return nil, fmt.Errorf("nil routableSubnet")
	}
	ipNet := ci.toFIPSubnet(routableSubnet)
	if ipNet == nil {
		var allRoutableSubnet []string
		for j := range ci.FloatingIPs {
			allRoutableSubnet = append(allRoutableSubnet, ci.FloatingIPs[j].RoutableSubnet.String())
		}
		glog.V(3).Infof("can't find fit routableSubnet %s, all routableSubnets %v", routableSubnet.String(), allRoutableSubnet)
		err = ErrNoFIPForSubnet
		return
	}
	var ipStr string
	ci.caches.cacheLock.Lock()
	for k, v := range ci.caches.unallocatedFIPs {
		//find an unallocated fip, then use it
		ipStr = k
		if v.subnet == routableSubnet.String() {
			if err = ci.createFloatingIP(ipStr, key, policy, attr, v.subnet); err != nil {
				ci.caches.cacheLock.Unlock()
				return
			}
			//sync cache when crd create success
			ci.syncCacheAfterCreate(ipStr, key, attr, policy, v.subnet)
			break
		}
	}
	ci.caches.cacheLock.Unlock()

	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	if err = ci.getFloatingIP(ipStr); err != nil {
		return
	}
	ipInt, err := strconv.ParseUint(ipStr, 10, 32)
	if err != nil {
		return allocated, err
	}
	allocated = nets.IntToIP(uint32(ipInt))
	return
}

func (ci *crdIpam) AllocateInSubnetWithKey(oldK, newK, subnet string, policy constant.ReleasePolicy, attr string) error {
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	for k, v := range ci.caches.allocatedFIPs {
		if v.key == oldK && v.subnet == subnet {
			if err := ci.updateFloatingIP(k, newK, subnet, policy, attr); err != nil {
				return err
			}
			v.key = newK
			return nil
		}
	}
	return fmt.Errorf("failed to find floatIP by key %s", oldK)
}

func (ci *crdIpam) UpdateKey(oldK, newK string) error {
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	for k, v := range ci.caches.allocatedFIPs {
		if v.key == oldK {
			if err := ci.updateFloatingIP(k, newK, v.subnet, v.policy, v.att); err != nil {
				return err
			}
			v.key = newK
			return nil
		}
	}
	return fmt.Errorf("failed to find floatIP by key %s", oldK)
}

func (ci *crdIpam) UpdatePolicy(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	ipStr := strconv.FormatUint(uint64(nets.IPToInt(ip)), 10)
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	v, find := ci.caches.allocatedFIPs[ipStr]
	if !find {
		return fmt.Errorf("failed to find floatIP in cache by IP %s", ipStr)
	}
	if err := ci.updateFloatingIP(ipStr, key, v.subnet, policy, attr); err != nil {
		return err
	}
	v.policy = policy
	return nil
}

func (ci *crdIpam) Release(key string, ip net.IP) error {
	ipStr := strconv.FormatUint(uint64(nets.IPToInt(ip)), 10)
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	v, find := ci.caches.allocatedFIPs[ipStr]
	if !find {
		return fmt.Errorf("failed to find floatIP in cache by IP %s", ipStr)
	}
	if v.key != key {
		return fmt.Errorf("key in %s is %s, not %s", ipStr, v.key, key)
	}
	if err := ci.deleteFloatingIP(ipStr); err != nil {
		return err
	}
	ci.syncCacheAfterDel(ipStr)
	return nil
}

func (ci *crdIpam) First(key string) (*FloatingIPInfo, error) {
	fip, err := ci.findFloatingIPByKey(key)
	if err != nil {
		return nil, err
	}
	if fip.Key == "" {
		return nil, nil
	}
	netIP := nets.IntToIP(fip.IP)
	for _, fips := range ci.FloatingIPs {
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
	return nil, fmt.Errorf("could not find match floating ip config for ip %s", netIP.String())
}

func (ci *crdIpam) ByIP(ip net.IP) (database.FloatingIP, error) {
	fip := database.FloatingIP{}

	ipStr := strconv.FormatUint(uint64(nets.IPToInt(ip)), 10)
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	v, find := ci.caches.allocatedFIPs[ipStr]
	if !find {
		v, find := ci.caches.unallocatedFIPs[ipStr]
		if !find {
			return fip, nil
		}
		fip.Subnet = v.subnet
		fip.Policy = uint16(v.policy)
		fip.Key = v.key
		fip.Attr = v.att
		fip.IP = nets.IPToInt(ip)
		return fip, nil
	}
	fip.Subnet = v.subnet
	fip.Policy = uint16(v.policy)
	fip.Key = v.key
	fip.Attr = v.att
	fip.IP = nets.IPToInt(ip)
	return fip, nil
}

func (ci *crdIpam) ByPrefix(prefix string) ([]database.FloatingIP, error) {
	var fips []database.FloatingIP
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	for ip, spec := range ci.caches.allocatedFIPs {
		if strings.Contains(spec.key, prefix) {
			ipInt, err := strconv.ParseUint(ip, 10, 32)
			if err != nil {
				return fips, err
			}
			tmp := database.FloatingIP{
				Key:    spec.key,
				Subnet: spec.subnet,
				Attr:   spec.att,
				Policy: uint16(spec.policy),
				IP:     uint32(ipInt),
			}
			fips = append(fips, tmp)
		}
	}
	if prefix == "" {
		for ip, spec := range ci.caches.unallocatedFIPs {
			ipInt, err := strconv.ParseUint(ip, 10, 32)
			if err != nil {
				return fips, err
			}
			tmp := database.FloatingIP{
				Key:    spec.key,
				Subnet: spec.subnet,
				Attr:   spec.att,
				Policy: uint16(spec.policy),
				IP:     uint32(ipInt),
			}
			fips = append(fips, tmp)
		}
	}
	return fips, nil
}

func (ci *crdIpam) RoutableSubnet(nodeIP net.IP) *net.IPNet {
	intIP := nets.IPToInt(nodeIP)
	minIndex := sort.Search(len(ci.FloatingIPs), func(j int) bool {
		return nets.IPToInt(ci.FloatingIPs[j].RoutableSubnet.IP) > intIP
	})
	if minIndex == 0 {
		return nil
	}
	if ci.FloatingIPs[minIndex-1].RoutableSubnet.Contains(nodeIP) {
		return ci.FloatingIPs[minIndex-1].RoutableSubnet
	}
	return nil
}

func (ci *crdIpam) QueryRoutableSubnetByKey(key string) ([]string, error) {
	var result []string
	if key == "" {
		result = ci.filterUnallocatedSubnet()
		return result, nil
	}
	result = ci.filterAllocatedSubnet(key)
	return result, nil
}

func (ci *crdIpam) Shutdown() {
}

func (ci *crdIpam) Name() string {
	name, err := ci.ipType.String()
	if err != nil {
		return "unknown type"
	}
	return name
}

func (ci *crdIpam) freshCache(fipMap map[string]*FloatingIP) error {
	glog.V(3).Infof("begin to fresh cache")
	ips, err := ci.listFloatingIPs()
	if err != nil {
		glog.Errorf("fail to list floatIP %v", err)
		return err
	}
	var toBeDelete []uint32
	tmpCacheAllocated := make(map[string]*FloatingIPObj)
	//delete no longer available floating ips stored in etcd first
	for _, ip := range ips.Items {
		ipInt, err := strconv.ParseUint(ip.Name, 10, 32)
		if err != nil {
			glog.Errorf("invalid FloatingIP name %s", ip.Name)
			return err
		}
		netIP := nets.IntToIP(uint32(ipInt))
		found := false
		for _, fipConf := range fipMap {
			if fipConf.IPNet().Contains(netIP) {
				found = true
				if !fipConf.Contains(netIP) {
					toBeDelete = append(toBeDelete, uint32(ipInt))
				} else {
					//ip in config, insert it into cache
					tmpFip := &FloatingIPObj{
						key:    ip.Spec.Key,
						att:    ip.Spec.Attribute,
						policy: ip.Spec.Policy,
						subnet: ip.Spec.Subnet,
					}
					tmpCacheAllocated[ip.Name] = tmpFip
				}
				break
			}
		}
		if !found {
			toBeDelete = append(toBeDelete, uint32(ipInt))
		}
	}
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	ci.caches.allocatedFIPs = tmpCacheAllocated
	if len(toBeDelete) > 0 {
		for _, del := range toBeDelete {
			if err := ci.deleteFloatingIP(strconv.FormatUint(uint64(del), 10)); err != nil {
				//if a FloatingIP crd in etcd can't be deleted, every freshCache will produce an error
				//it won't return error when error happens in deletion
				glog.Errorf("failed to delete ip %v: %v", del, err)
			}
		}
		glog.Infof("expect to delete %d ips from %v", len(toBeDelete), toBeDelete)
	}
	// fresh unallocated floatIP
	tmpCacheUnallocated := make(map[string]*FloatingIPObj)
	for _, fipConf := range fipMap {
		subnet := fipConf.RoutableSubnet.String()
		for _, ipr := range fipConf.IPRanges {
			first := nets.IPToInt(ipr.First)
			last := nets.IPToInt(ipr.Last)
			for ; first <= last; first++ {
				if _, contain := ci.caches.allocatedFIPs[strconv.FormatUint(uint64(first), 10)]; !contain {
					tmpFip := &FloatingIPObj{
						key:    "",
						att:    "",
						policy: constant.ReleasePolicyPodDelete,
						subnet: subnet,
					}
					tmpCacheUnallocated[strconv.FormatUint(uint64(first), 10)] = tmpFip
				}
			}
		}
	}
	ci.caches.unallocatedFIPs = tmpCacheUnallocated
	return nil
}

func (ci *crdIpam) toFIPSubnet(routableSubnet *net.IPNet) *net.IPNet {
	for _, fip := range ci.FloatingIPs {
		if fip.RoutableSubnet.String() == routableSubnet.String() {
			return fip.IPNet()
		}
	}
	return nil
}

func (ci *crdIpam) syncCacheAfterCreate(ip string, key string, att string, policy constant.ReleasePolicy, subnet string) {
	tmp := &FloatingIPObj{
		key:    key,
		att:    att,
		policy: policy,
		subnet: subnet,
	}
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	ci.caches.allocatedFIPs[ip] = tmp
	delete(ci.caches.unallocatedFIPs, ip)
	return
}

func (ci *crdIpam) syncCacheAfterDel(ip string) {
	tmp := &FloatingIPObj{
		key:    "",
		att:    "",
		policy: constant.ReleasePolicyPodDelete,
		subnet: ci.caches.allocatedFIPs[ip].subnet,
	}
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	delete(ci.caches.allocatedFIPs, ip)
	ci.caches.unallocatedFIPs[ip] = tmp
	return
}

func (ci *crdIpam) findFloatingIPByKey(key string) (database.FloatingIP, error) {
	var fip database.FloatingIP
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	for ip, spec := range ci.caches.allocatedFIPs {
		if spec.key == key {
			ipInt, err := strconv.ParseUint(ip, 10, 32)
			if err != nil {
				return fip, err
			}
			fip.IP = uint32(ipInt)
			fip.Key = key
			fip.Attr = spec.att
			fip.Subnet = spec.subnet
			fip.Policy = uint16(spec.policy)
			return fip, nil
		}
	}
	return fip, nil
}

func (ci *crdIpam) filterAllocatedSubnet(key string) []string {
	//key would not be empty
	var result []string
	subnetSet := make(map[string]struct{})
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	for _, spec := range ci.caches.allocatedFIPs {
		if spec.key == key {
			subnetSet[spec.subnet] = struct{}{}
		}
	}
	for k := range subnetSet {
		result = append(result, k)
	}
	return result
}

//in filter request, sometimes unallocated subnet(key equals "") is needed
//it will filter all subnet in unallocated floatingIP in cache
func (ci *crdIpam) filterUnallocatedSubnet() (result []string) {
	subnetSet := make(map[string]struct{})
	ci.caches.cacheLock.RLock()
	for _, val := range ci.caches.unallocatedFIPs {
		subnetSet[val.subnet] = struct{}{}
	}
	ci.caches.cacheLock.RUnlock()
	for subnet := range subnetSet {
		result = append(result, subnet)
	}
	return result
}

func (ci *crdIpam) GetAllIPs(keyword string) ([]database.FloatingIP, error) {
	//not implement
	var fips []database.FloatingIP
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	if ci.caches.allocatedFIPs == nil {
		return fips, nil
	}
	for ip, spec := range ci.caches.allocatedFIPs {
		if strings.Contains(spec.key, keyword) {
			ipInt, err := strconv.ParseUint(ip, 10, 32)
			if err != nil {
				glog.Errorf("invalid FloatingIP name %s", ip)
				return fips, err
			}
			tmp := database.FloatingIP{
				IP:     uint32(ipInt),
				Key:    spec.key,
				Subnet: spec.subnet,
				Attr:   spec.att,
				Policy: uint16(spec.policy),
			}
			fips = append(fips, tmp)
		}
	}
	return fips, nil
}

func (ci *crdIpam) ReleaseIPs(ipToKey map[uint32]string) ([]string, error) {
	var (
		undeleted []uint32
		deleted   []string
	)
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	if ci.caches.allocatedFIPs == nil {
		//for second ipam, caches may be nil
		return deleted, nil
	}
	for ip, key := range ipToKey {
		if v, find := ci.caches.allocatedFIPs[strconv.FormatUint(uint64(ip), 10)]; find {
			if v.key == key {
				if err := ci.deleteFloatingIP(strconv.FormatUint(uint64(ip), 10)); err != nil {
					undeleted = append(undeleted, ip)
					glog.Errorf("failed to delete %v", ip)
					continue
				}
				ci.syncCacheAfterDel(strconv.FormatUint(uint64(ip), 10))
				glog.Infof("%v has been deleted", ip)
				deleted = append(deleted, nets.IntToIP(ip).String())
			} else {
				glog.Warningf("for %v, key is %s, not %s, without deleting it", ip, v.key, key)
			}
		} else {
			glog.Warningf("failed to find %v with key", ip, key)
		}
	}
	if len(undeleted) > 0 {
		return deleted, fmt.Errorf("failed to delete these IPs %v", undeleted)
	}
	return deleted, nil
}
