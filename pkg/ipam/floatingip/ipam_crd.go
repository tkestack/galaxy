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
package floatingip

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	crd_clientset "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions/galaxy/v1alpha1"
	"tkestack.io/galaxy/pkg/utils/nets"
)

// Type is struct of IP type.
type Type uint16

const (
	// InternalIp is enum of pod's internal IP.
	InternalIp Type = iota
	// ExternalIp is enum of pod's external IP.
	ExternalIp
)

// String used to transform IP Type to string.
func (t *Type) String() (string, error) {
	if *t == InternalIp {
		return "internalIP", nil
	} else if *t == ExternalIp {
		return "externalIP", nil
	}
	return "", fmt.Errorf("unknown ip type %v", *t)
}

type crdIpam struct {
	FloatingIPs []*FloatingIPPool `json:"floatingips,omitempty"`
	client      crd_clientset.Interface
	ipType      Type
	//caches for FloatingIP crd, both stores allocated FloatingIPs and unallocated FloatingIPs
	cacheLock *sync.RWMutex
	// key is ip string
	allocatedFIPs   map[string]*FloatingIP
	unallocatedFIPs map[string]*FloatingIP

	ipCounterDesc *prometheus.Desc
}

// NewCrdIPAM init IPAM struct.
func NewCrdIPAM(fipClient crd_clientset.Interface, ipType Type, informer crdInformer.FloatingIPInformer) IPAM {
	ipam := &crdIpam{
		client:          fipClient,
		ipType:          ipType,
		cacheLock:       new(sync.RWMutex),
		allocatedFIPs:   make(map[string]*FloatingIP),
		unallocatedFIPs: make(map[string]*FloatingIP),
		ipCounterDesc: prometheus.NewDesc("galaxy_ip_counter", "Galaxy floating ip counter",
			[]string{"type", "subnet", "first_ip"}, nil),
	}
	// manually creating and fip to reserve it
	if informer != nil {
		informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if err := ipam.handleFIPAssign(obj); err != nil {
					glog.Warningf("handle add fip event: %v", err)
				}
			},
			DeleteFunc: func(obj interface{}) {
				if err := ipam.handleFIPUnassign(obj); err != nil {
					glog.Warningf("handle del fip event: %v", err)
				}
			},
		})
	}
	return ipam
}

// AllocateSpecificIP allocate pod a specific IP.
func (ci *crdIpam) AllocateSpecificIP(key string, ip net.IP, attr Attr) error {
	ipStr := ip.String()
	ci.cacheLock.RLock()
	spec, find := ci.unallocatedFIPs[ipStr]
	ci.cacheLock.RUnlock()
	if !find {
		return fmt.Errorf("failed to find floating ip by %s in cache", ipStr)
	}
	allocated := New(spec.pool, ip, key, &attr, time.Now())
	if err := ci.createFloatingIP(allocated); err != nil {
		glog.Errorf("failed to create floatingIP %s: %v", ipStr, err)
		return err
	}
	ci.cacheLock.Lock()
	ci.syncCacheAfterCreate(allocated)
	ci.cacheLock.Unlock()
	return nil
}

// AllocateInSubnet allocate subnet of IPs.
func (ci *crdIpam) AllocateInSubnet(key string, nodeSubnet *net.IPNet, attr Attr) (net.IP, error) {
	if nodeSubnet == nil {
		// this should never happen
		return nil, fmt.Errorf("nil nodeSubnet")
	}
	var ipStr string
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	nodeSubnetStr := nodeSubnet.String()
	for k, v := range ci.unallocatedFIPs {
		//find an unallocated fip, then use it
		if v.pool.nodeSubnets.Has(nodeSubnetStr) {
			ipStr = k
			// we never updates ip or subnet object, it's ok to share these objs.
			allocated := New(v.pool, v.IP, key, &attr, time.Now())
			if err := ci.createFloatingIP(allocated); err != nil {
				glog.Errorf("failed to create floatingIP %s: %v", ipStr, err)
				return nil, err
			}
			//sync cache when crd create success
			ci.syncCacheAfterCreate(allocated)
			break
		}
	}
	if ipStr == "" {
		return nil, ErrNoEnoughIP
	}
	return net.ParseIP(ipStr), nil
}

// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
func (ci *crdIpam) AllocateInSubnetWithKey(oldK, newK, subnet string, attr Attr) error {
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	var (
		recordTs int64
		latest   *FloatingIP
	)
	//find latest floatingIP by updateTime.
	for _, v := range ci.allocatedFIPs {
		if v.Key == oldK && v.pool.nodeSubnets.Has(subnet) {
			if v.UpdatedAt.UnixNano() > recordTs {
				latest = v
				recordTs = v.UpdatedAt.UnixNano()
			}
		}
	}
	if latest == nil {
		return fmt.Errorf("failed to find floatIP by key %s", oldK)
	}
	date := time.Now()
	cloned := latest.CloneWith(newK, &attr, date)
	if err := ci.updateFloatingIP(cloned); err != nil {
		glog.Errorf("failed to update floatingIP %s: %v", cloned.IP.String(), err)
		return err
	}
	latest.Assign(newK, &attr, date)
	return nil
}

// ReserveIP can reserve a IP entitled by a terminated pod.
func (ci *crdIpam) ReserveIP(oldK, newK string, attr Attr) (bool, error) {
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	date := time.Now()
	var reserved bool
	for k, v := range ci.allocatedFIPs {
		if v.Key == oldK {
			if oldK == newK && v.PodUid == attr.Uid && v.NodeName == attr.NodeName {
				// nothing changed
				continue
			}
			attr.Policy = constant.ReleasePolicy(v.Policy)
			if err := ci.updateFloatingIP(v.CloneWith(newK, &attr, date)); err != nil {
				glog.Errorf("failed to update floatingIP %s: %v", k, err)
				return false, err
			}
			v.Assign(newK, &attr, date)
			reserved = true
		}
	}
	return reserved, nil
}

// UpdateAttr update floatingIP's release policy and attr according to ip and key
func (ci *crdIpam) UpdateAttr(key string, ip net.IP, attr Attr) error {
	ipStr := ip.String()
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	v, find := ci.allocatedFIPs[ipStr]
	if !find {
		return fmt.Errorf("failed to find floatIP in cache by IP %s", ipStr)
	}
	if v.Key != key {
		return fmt.Errorf("key for %s is %s, not %s", ipStr, v.Key, key)
	}
	date := time.Now()
	if err := ci.updateFloatingIP(v.CloneWith(v.Key, &attr, date)); err != nil {
		glog.Errorf("failed to update floatingIP %s: %v", ipStr, err)
		return err
	}
	v.Assign(v.Key, &attr, date)
	return nil
}

// Release release a given IP.
func (ci *crdIpam) Release(key string, ip net.IP) error {
	ipStr := ip.String()
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	v, find := ci.allocatedFIPs[ipStr]
	if !find {
		return fmt.Errorf("failed to find floatIP in cache by IP %s", ipStr)
	}
	if v.Key != key {
		return fmt.Errorf("key for %s is %s, not %s", ipStr, v.Key, key)
	}
	if err := ci.deleteFloatingIP(ipStr); err != nil {
		return err
	}
	ci.syncCacheAfterDel(v)
	return nil
}

// First returns the first matched IP by key.
func (ci *crdIpam) First(key string) (*FloatingIPInfo, error) {
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for _, spec := range ci.allocatedFIPs {
		if spec.Key == key {
			return ci.toFloatingIPInfo(spec), nil
		}
	}
	return nil, nil
}

// ByIP transform a given IP to FloatingIP struct.
func (ci *crdIpam) ByIP(ip net.IP) (FloatingIP, error) {
	ipStr := ip.String()
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	v, find := ci.allocatedFIPs[ipStr]
	if !find {
		v, find = ci.unallocatedFIPs[ipStr]
		if !find {
			return FloatingIP{}, nil
		}
	}
	return *v, nil
}

// ByPrefix filter floatingIPs by prefix key.
func (ci *crdIpam) ByPrefix(prefix string) ([]*FloatingIPInfo, error) {
	var fips []*FloatingIPInfo
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for _, spec := range ci.allocatedFIPs {
		if strings.HasPrefix(spec.Key, prefix) {
			fips = append(fips, ci.toFloatingIPInfo(spec))
		}
	}
	if prefix == "" {
		for _, spec := range ci.unallocatedFIPs {
			fips = append(fips, ci.toFloatingIPInfo(spec))
		}
	}
	return fips, nil
}

func (ci *crdIpam) NodeSubnet(nodeIP net.IP) *net.IPNet {
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for j := range ci.FloatingIPs {
		nodeSubnets := ci.FloatingIPs[j].NodeSubnets
		for k := range nodeSubnets {
			if nodeSubnets[k].Contains(nodeIP) {
				return nodeSubnets[k]
			}
		}
	}
	return nil
}

func (ci *crdIpam) NodeSubnetsByIPRanges(ipranges [][]nets.IPRange) (sets.String, error) {
	subnetSet := sets.NewString()
	insertSubnet := func(poolIndexSet sets.Int, subnetSet sets.String) {
		for _, index := range poolIndexSet.UnsortedList() {
			if index >= 0 && index <= len(ci.FloatingIPs)-1 {
				subnetSet.Insert(ci.FloatingIPs[index].nodeSubnets.UnsortedList()...)
			}
		}
	}
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	if len(ipranges) == 0 {
		poolIndexSet := sets.NewInt()
		for _, val := range ci.unallocatedFIPs {
			poolIndexSet.Insert(val.pool.index)
		}
		insertSubnet(poolIndexSet, subnetSet)
		return subnetSet, nil
	}
	for _, ranges := range ipranges {
		poolIndexSet := sets.NewInt()
		walkIPRanges(ranges, func(ip net.IP) bool {
			ipStr := ip.String()
			if fip, ok := ci.unallocatedFIPs[ipStr]; !ok {
				return false
			} else {
				poolIndexSet.Insert(fip.pool.index)
			}
			return false
		})
		// no ip left in this []nets.IPRange
		if poolIndexSet.Len() == 0 {
			glog.V(3).Infof("no enough ips for ip range %v", ranges)
			return sets.NewString(), nil
		}
		if subnetSet.Len() == 0 {
			insertSubnet(poolIndexSet, subnetSet)
		} else {
			partset := sets.NewString()
			insertSubnet(poolIndexSet, partset)
			subnetSet = subnetSet.Intersection(partset)
		}
	}
	// TODO try to allocate to check if each subnet has enough ips when [][]nets.IPRange has overlap ranges
	// e.g. if [][]nets.IPRange = [["10.0.0.1","10.0.1.1~10.0.1.3"]["10.0.0.1","10.0.1.1~10.0.1.3"]], we should
	// return 10.0.1.0/24 and exclude 10.0.0.0/24
	return subnetSet, nil
}

// Shutdown shutdowns IPAM.
func (ci *crdIpam) Shutdown() {
}

// Name returns IPAM's name.
func (ci *crdIpam) Name() string {
	name, err := ci.ipType.String()
	if err != nil {
		return "unknown type"
	}
	return name
}

// ConfigurePool init floatingIP pool.
// #lizard forgives
func (ci *crdIpam) ConfigurePool(floatIPs []*FloatingIPPool) error {
	defer func() {
		glog.Infof("Configure pool done, %d fip pool, %d unallocated, %d allocated", len(ci.FloatingIPs),
			len(ci.unallocatedFIPs), len(ci.allocatedFIPs))
	}()

	sort.Sort(FloatingIPSlice(floatIPs))
	ips, err := ci.listFloatingIPs()
	if err != nil {
		glog.Errorf("fail to list floatIP %v", err)
		return err
	}
	glog.V(3).Infof("floating ip config %v", floatIPs)
	for index, fipConf := range floatIPs {
		subnetSet := sets.NewString()
		for i := range fipConf.NodeSubnets {
			subnetSet.Insert(fipConf.NodeSubnets[i].String())
		}
		fipConf.nodeSubnets = subnetSet
		fipConf.index = index
	}
	var deletingIPs []string
	tmpCacheAllocated := make(map[string]*FloatingIP)
	//delete no longer available floating ips stored in etcd first
	for _, ip := range ips.Items {
		netIP := net.ParseIP(ip.Name)
		found := false
		for _, fipConf := range floatIPs {
			if fipConf.IPNet().Contains(netIP) && fipConf.Contains(netIP) {
				found = true
				//ip in config, insert it into cache
				tmpFip := New(fipConf, netIP, ip.Spec.Key, &Attr{Policy: ip.Spec.Policy}, ip.Spec.UpdateTime.Time)
				if err := tmpFip.unmarshalAttr(ip.Spec.Attribute); err != nil {
					glog.Error(err)
				}
				tmpCacheAllocated[ip.Name] = tmpFip
				break
			}
		}
		if !found {
			deletingIPs = append(deletingIPs, ip.Name)
		}
	}
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	ci.FloatingIPs = floatIPs
	ci.allocatedFIPs = tmpCacheAllocated
	if len(deletingIPs) > 0 {
		for _, ip := range deletingIPs {
			if err := ci.deleteFloatingIP(ip); err != nil {
				//if a FloatingIP crd in etcd can't be deleted, every freshCache will produce an error
				//it won't return error when error happens in deletion
				glog.Errorf("failed to delete ip %v: %v", ip, err)
			}
		}
		glog.Infof("expect to delete %d ips from %v", len(deletingIPs), deletingIPs)
	}
	now := time.Now()
	// fresh unallocated floatIP
	tmpCacheUnallocated := make(map[string]*FloatingIP)
	for _, fipConf := range floatIPs {
		walkIPRanges(fipConf.IPRanges, func(ip net.IP) bool {
			ipStr := ip.String()
			if _, contain := ci.allocatedFIPs[ipStr]; !contain {
				tmpFip := New(fipConf, ip, "", &Attr{Policy: constant.ReleasePolicyPodDelete}, now)
				tmpCacheUnallocated[ipStr] = tmpFip
			}
			return false
		})
	}
	ci.unallocatedFIPs = tmpCacheUnallocated
	return nil
}

// cacheLock is used when the function called,
// don't use lock inner function, otherwise deadlock will be caused
func (ci *crdIpam) syncCacheAfterCreate(fip *FloatingIP) {
	ipStr := fip.IP.String()
	ci.allocatedFIPs[ipStr] = fip
	delete(ci.unallocatedFIPs, ipStr)
	return
}

// CacheLock will be used when syncCacheAfterDel called,
// don't use lock inner function, otherwise deadlock will be caused
func (ci *crdIpam) syncCacheAfterDel(released *FloatingIP) {
	ipStr := released.IP.String()
	released.Assign("", &Attr{Policy: constant.ReleasePolicyPodDelete}, time.Now())
	released.Labels = nil
	delete(ci.allocatedFIPs, ipStr)
	ci.unallocatedFIPs[ipStr] = released
	return
}

// ByKeyword returns floatingIP set by a given keyword.
func (ci *crdIpam) ByKeyword(keyword string) ([]FloatingIP, error) {
	//not implement
	var fips []FloatingIP
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for _, spec := range ci.allocatedFIPs {
		if strings.Contains(spec.Key, keyword) {
			fips = append(fips, *spec)
		}
	}
	return fips, nil
}

// ReleaseIPs function release a map of ip to key
func (ci *crdIpam) ReleaseIPs(ipToKey map[string]string) (map[string]string, map[string]string, error) {
	deleted, undeleted := map[string]string{}, map[string]string{}
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	for ipStr, key := range ipToKey {
		undeleted[ipStr] = key
	}
	if len(ci.allocatedFIPs) == 0 {
		return deleted, undeleted, nil
	}
	for ipStr, key := range ipToKey {
		if v, find := ci.allocatedFIPs[ipStr]; find {
			if v.Key == key {
				if err := ci.deleteFloatingIP(ipStr); err != nil {
					glog.Errorf("failed to delete %v", ipStr)
					return deleted, undeleted, fmt.Errorf("failed to delete %v", ipStr)
				}
				ci.syncCacheAfterDel(v)
				glog.Infof("%v has been deleted", ipStr)
				deleted[ipStr] = key
				delete(undeleted, ipStr)
			} else {
				// update key
				undeleted[ipStr] = v.Key
			}
		} else if _, find := ci.unallocatedFIPs[ipStr]; find {
			undeleted[ipStr] = ""
		}
	}
	return deleted, undeleted, nil
}

// Describe sends metrics description to ch
func (ci *crdIpam) Describe(ch chan<- *prometheus.Desc) {
	ch <- ci.ipCounterDesc
}

// Collect sends metrics to ch
func (ci *crdIpam) Collect(ch chan<- prometheus.Metric) {
	allocated, unallocated := map[string]*FloatingIP{}, map[string]*FloatingIP{}
	ci.cacheLock.RLock()
	pools := make([]*FloatingIPPool, len(ci.FloatingIPs))
	for ipStr, fip := range ci.allocatedFIPs {
		allocated[ipStr] = fip
	}
	for ipStr, fip := range ci.unallocatedFIPs {
		unallocated[ipStr] = fip
	}
	for i := range ci.FloatingIPs {
		pools[i] = ci.FloatingIPs[i]
	}
	ci.cacheLock.RUnlock()
	for _, pool := range pools {
		subnetStr := pool.IPNet().String()
		var firstIP string
		var allocatedNum float64
		for _, ipr := range pool.IPRanges {
			firstIP = ipr.First.String()
			break
		}
		for _, fip := range allocated {
			if !pool.Contains(fip.IP) {
				continue
			}
			allocatedNum += 1
		}
		// since subnetStr may be the same for different pools, add a first ip tag
		ch <- prometheus.MustNewConstMetric(ci.ipCounterDesc, prometheus.GaugeValue, allocatedNum,
			"allocated", subnetStr, firstIP)
		ch <- prometheus.MustNewConstMetric(ci.ipCounterDesc, prometheus.GaugeValue, float64(pool.Size()),
			"total", subnetStr, firstIP)
	}
}

// AllocateInSubnetsAndIPRange allocates an ip for each ip range array of the input node subnet.
// It guarantees allocating all ips or no ips.
// TODO Fix allocation for [][]nets.IPRange [["10.0.0.1~10.0.0.2"]["10.0.0.1"]]
func (ci *crdIpam) AllocateInSubnetsAndIPRange(key string, nodeSubnet *net.IPNet, ipranges [][]nets.IPRange,
	attr Attr) ([]net.IP, error) {
	if nodeSubnet == nil {
		// this should never happen
		return nil, fmt.Errorf("nil nodeSubnet")
	}
	if len(ipranges) == 0 {
		ip, err := ci.AllocateInSubnet(key, nodeSubnet, attr)
		if err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	// pick ips to allocate, one per []nets.IPRange
	var allocatedIPStrs []string
	// allocatedIPSet is the allocated ips in the previous loop, to avoid count in duplicate ips
	allocatedIPSet := sets.NewString()
	for _, ranges := range ipranges {
		var allocated bool
		walkIPRanges(ranges, func(ip net.IP) bool {
			ipStr := ip.String()
			if fip, ok := ci.unallocatedFIPs[ipStr]; !ok || !fip.pool.nodeSubnets.Has(nodeSubnet.String()) ||
				allocatedIPSet.Has(ipStr) {
				return false
			}
			allocatedIPStrs = append(allocatedIPStrs, ipStr)
			allocatedIPSet.Insert(ipStr)
			allocated = true
			return true
		})
		if !allocated {
			glog.V(3).Infof("no enough ips to allocate for %s node subnet %s, ip range %v", key,
				nodeSubnet.String(), ipranges)
			return nil, ErrNoEnoughIP
		}
	}
	var allocatedIPs []net.IP
	var allocatedFips []*FloatingIP
	// allocate all ips in crd before sync cache in memory
	for i, allocatedIPStr := range allocatedIPStrs {
		v := ci.unallocatedFIPs[allocatedIPStr]
		// we never updates ip or subnet object, it's ok to share these objs.
		allocated := New(v.pool, v.IP, key, &attr, time.Now())
		if err := ci.createFloatingIP(allocated); err != nil {
			glog.Errorf("failed to create floatingIP %s: %v", allocatedIPStr, err)
			// rollback all allocated ips
			for j := range allocatedIPStrs {
				if j == i {
					break
				}
				if err := ci.deleteFloatingIP(allocatedIPStrs[j]); err != nil {
					glog.Errorf("failed to delete floatingIP %s: %v", allocatedIPStrs[j], err)
				}
			}
			return nil, err
		}
		allocatedIPs = append(allocatedIPs, v.IP)
		allocatedFips = append(allocatedFips, allocated)
	}
	// sync cache when crds created
	for i := range allocatedFips {
		ci.syncCacheAfterCreate(allocatedFips[i])
	}
	return allocatedIPs, nil
}

// ByKeyAndIPRanges finds an allocated ip for each []iprange by key.
// If input [][]nets.IPRange is nil or empty, it finds all allocated ips by key.
// Otherwise, it always return the same size of []*FloatingIPInfo as [][]nets.IPRange, the element of
// []*FloatingIPInfo may be nil when it can't find an allocated ip for the same index of []iprange.
func (ci *crdIpam) ByKeyAndIPRanges(key string, ipranges [][]nets.IPRange) ([]*FloatingIPInfo, error) {
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	var ipinfos []*FloatingIPInfo
	if len(ipranges) != 0 {
		ipinfos = make([]*FloatingIPInfo, len(ipranges))
		for i, ranges := range ipranges {
			walkIPRanges(ranges, func(ip net.IP) bool {
				ipStr := ip.String()
				fip, ok := ci.allocatedFIPs[ipStr]
				if !ok || fip.Key != key {
					return false
				}
				ipinfos[i] = ci.toFloatingIPInfo(fip)
				return true
			})
		}
	} else {
		for _, fip := range ci.allocatedFIPs {
			if fip.Key == key {
				ipinfos = append(ipinfos, ci.toFloatingIPInfo(fip))
			}
		}
	}
	return ipinfos, nil
}

func (ci *crdIpam) toFloatingIPInfo(fip *FloatingIP) *FloatingIPInfo {
	fipPool := fip.pool
	ip := nets.IPNet(net.IPNet{
		IP:   fip.IP,
		Mask: fipPool.Mask,
	})
	return &FloatingIPInfo{
		IPInfo: constant.IPInfo{
			IP:      &ip,
			Vlan:    fipPool.Vlan,
			Gateway: fipPool.Gateway,
		},
		FloatingIP:  *fip,
		NodeSubnets: sets.NewString(fipPool.nodeSubnets.UnsortedList()...),
	}
}

// walkIPRanges walks all ips in the ranges, and calls f for each ip. If f returns true, walkIPRanges stops.
func walkIPRanges(ranges []nets.IPRange, f func(ip net.IP) bool) {
	for _, r := range ranges {
		first := nets.IPToInt(r.First)
		last := nets.IPToInt(r.Last)
		for ; first <= last; first++ {
			ip := nets.IntToIP(first)
			if f(ip) {
				return
			}
		}
	}
}
