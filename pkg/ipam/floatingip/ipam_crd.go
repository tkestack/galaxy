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
	// key is FloatingIP name (ip typed as uint32)
	allocatedFIPs   map[string]*FloatingIP
	unallocatedFIPs map[string]*FloatingIP
}

// NewCrdIPAM init IPAM struct.
func NewCrdIPAM(fipClient crd_clientset.Interface, ipType Type, informer crdInformer.FloatingIPInformer) IPAM {
	ipam := &crdIpam{
		client:          fipClient,
		ipType:          ipType,
		cacheLock:       new(sync.RWMutex),
		allocatedFIPs:   make(map[string]*FloatingIP),
		unallocatedFIPs: make(map[string]*FloatingIP),
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
func (ci *crdIpam) AllocateSpecificIP(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	ipStr := ip.String()
	ci.cacheLock.RLock()
	spec, find := ci.unallocatedFIPs[ipStr]
	ci.cacheLock.RUnlock()
	if !find {
		return fmt.Errorf("failed to find floating ip by %s in cache", ipStr)
	}
	date := time.Now()
	allocated := &FloatingIP{
		IP:        ip,
		Key:       key,
		Subnets:   spec.Subnets,
		Attr:      attr,
		Policy:    uint16(policy),
		UpdatedAt: date,
	}
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
func (ci *crdIpam) AllocateInSubnet(key string, nodeSubnet *net.IPNet, policy constant.ReleasePolicy,
	attr string) (allocated net.IP, err error) {
	if nodeSubnet == nil {
		// this should never happen
		return nil, fmt.Errorf("nil nodeSubnet")
	}
	var ipStr string
	ci.cacheLock.Lock()
	nodeSubnetStr := nodeSubnet.String()
	for k, v := range ci.unallocatedFIPs {
		//find an unallocated fip, then use it
		if v.Subnets.Has(nodeSubnetStr) {
			ipStr = k
			date := time.Now()
			// we never updates ip or subnet object, it's ok to share these objs.
			allocatedFIP := &FloatingIP{
				IP:        v.IP,
				Key:       key,
				Subnets:   v.Subnets,
				Attr:      attr,
				Policy:    uint16(policy),
				UpdatedAt: date,
			}
			if err = ci.createFloatingIP(allocatedFIP); err != nil {
				glog.Errorf("failed to create floatingIP %s: %v", ipStr, err)
				ci.cacheLock.Unlock()
				return
			}
			//sync cache when crd create success
			ci.syncCacheAfterCreate(allocatedFIP)
			break
		}
	}
	ci.cacheLock.Unlock()
	if ipStr == "" {
		return nil, ErrNoEnoughIP
	}
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	if err = ci.getFloatingIP(ipStr); err != nil {
		return
	}
	allocated = net.ParseIP(ipStr)
	return
}

// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
func (ci *crdIpam) AllocateInSubnetWithKey(oldK, newK, subnet string, policy constant.ReleasePolicy,
	attr string) error {
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	var (
		recordTs int64
		latest   *FloatingIP
	)
	//find latest floatingIP by updateTime.
	for _, v := range ci.allocatedFIPs {
		if v.Key == oldK && v.Subnets.Has(subnet) {
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
	cloned := latest.CloneWith(newK, attr, uint16(policy), date)
	if err := ci.updateFloatingIP(cloned); err != nil {
		glog.Errorf("failed to update floatingIP %s: %v", cloned.IP.String(), err)
		return err
	}
	latest.Assign(newK, attr, uint16(policy), date)
	return nil
}

// ReserveIP can reserve a IP entitled by a terminated pod.
func (ci *crdIpam) ReserveIP(oldK, newK, attr string) error {
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	for k, v := range ci.allocatedFIPs {
		if v.Key == oldK {
			date := time.Now()
			if err := ci.updateFloatingIP(v.CloneWith(newK, attr, v.Policy, date)); err != nil {
				glog.Errorf("failed to update floatingIP %s: %v", k, err)
				return err
			}
			v.Assign(newK, attr, v.Policy, date)
			return nil
		}
	}
	return fmt.Errorf("failed to find floatIP by key %s", oldK)
}

// UpdatePolicy update floatingIP's release policy and attr according to ip and key
func (ci *crdIpam) UpdatePolicy(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
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
	if err := ci.updateFloatingIP(v.CloneWith(v.Key, attr, uint16(policy), date)); err != nil {
		glog.Errorf("failed to update floatingIP %s: %v", ipStr, err)
		return err
	}
	v.Assign(v.Key, attr, uint16(policy), date)
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
	fip, err := ci.findFloatingIPByKey(key)
	if err != nil {
		return nil, err
	}
	if fip == nil {
		return nil, nil
	}
	for _, fips := range ci.FloatingIPs {
		if fips.Contains(fip.IP) {
			ip := nets.IPNet(net.IPNet{
				IP:   fip.IP,
				Mask: fips.Mask,
			})
			return &FloatingIPInfo{
				IPInfo: constant.IPInfo{
					IP:      &ip,
					Vlan:    fips.Vlan,
					Gateway: fips.Gateway,
				},
				FIP: *fip,
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find match floating ip config for ip %s", fip.IP.String())
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
func (ci *crdIpam) ByPrefix(prefix string) ([]FloatingIP, error) {
	var fips []FloatingIP
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for _, spec := range ci.allocatedFIPs {
		if strings.HasPrefix(spec.Key, prefix) {
			fips = append(fips, *spec)
		}
	}
	if prefix == "" {
		for _, spec := range ci.unallocatedFIPs {
			fips = append(fips, *spec)
		}
	}
	return fips, nil
}

func (ci *crdIpam) NodeSubnet(nodeIP net.IP) *net.IPNet {
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

func (ci *crdIpam) NodeSubnetsByKey(key string) (sets.String, error) {
	if key == "" {
		return ci.filterUnallocatedSubnet(), nil
	}
	return ci.filterAllocatedSubnet(key), nil
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
	nodeSubnets := make([]sets.String, len(floatIPs))
	for i, fipConf := range floatIPs {
		subnetSet := sets.NewString()
		for i := range fipConf.NodeSubnets {
			subnetSet.Insert(fipConf.NodeSubnets[i].String())
		}
		nodeSubnets[i] = subnetSet
	}
	var deletingIPs []string
	tmpCacheAllocated := make(map[string]*FloatingIP)
	//delete no longer available floating ips stored in etcd first
	for _, ip := range ips.Items {
		netIP := net.ParseIP(ip.Name)
		found := false
		for i, fipConf := range floatIPs {
			if fipConf.IPNet().Contains(netIP) && fipConf.Contains(netIP) {
				found = true
				//ip in config, insert it into cache
				tmpFip := &FloatingIP{
					IP:        netIP,
					Key:       ip.Spec.Key,
					Attr:      ip.Spec.Attribute,
					Policy:    uint16(ip.Spec.Policy),
					Subnets:   nodeSubnets[i],
					UpdatedAt: ip.Spec.UpdateTime.Time,
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
	for i, fipConf := range floatIPs {
		subnetSet := nodeSubnets[i]
		for _, ipr := range fipConf.IPRanges {
			first := nets.IPToInt(ipr.First)
			last := nets.IPToInt(ipr.Last)
			for ; first <= last; first++ {
				ip := nets.IntToIP(first)
				ipStr := ip.String()
				if _, contain := ci.allocatedFIPs[ipStr]; !contain {
					tmpFip := &FloatingIP{
						IP:        ip,
						Key:       "",
						Attr:      "",
						Policy:    uint16(constant.ReleasePolicyPodDelete),
						Subnets:   subnetSet,
						UpdatedAt: now,
					}
					tmpCacheUnallocated[ipStr] = tmpFip
				}
			}
		}
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
	released.Assign("", "", uint16(constant.ReleasePolicyPodDelete), time.Now())
	delete(ci.allocatedFIPs, ipStr)
	ci.unallocatedFIPs[ipStr] = released
	return
}

func (ci *crdIpam) findFloatingIPByKey(key string) (*FloatingIP, error) {
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for _, spec := range ci.allocatedFIPs {
		if spec.Key == key {
			return spec, nil
		}
	}
	return nil, nil
}

func (ci *crdIpam) filterAllocatedSubnet(key string) sets.String {
	//key would not be empty
	subnetSet := sets.NewString()
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	for _, spec := range ci.allocatedFIPs {
		if spec.Key == key {
			subnetSet.Insert(spec.Subnets.UnsortedList()...)
		}
	}
	return subnetSet
}

// Sometimes unallocated subnet(key equals "") is needed,
// it will filter all subnet in unallocated floatingIP in cache
func (ci *crdIpam) filterUnallocatedSubnet() sets.String {
	subnetSet := sets.NewString()
	ci.cacheLock.RLock()
	for _, val := range ci.unallocatedFIPs {
		subnetSet.Insert(val.Subnets.UnsortedList()...)
	}
	ci.cacheLock.RUnlock()
	return subnetSet
}

// ByKeyword returns floatingIP set by a given keyword.
func (ci *crdIpam) ByKeyword(keyword string) ([]FloatingIP, error) {
	//not implement
	var fips []FloatingIP
	ci.cacheLock.RLock()
	defer ci.cacheLock.RUnlock()
	if ci.allocatedFIPs == nil {
		return fips, nil
	}
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
	if ci.allocatedFIPs == nil {
		//for second ipam, caches may be nil
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
